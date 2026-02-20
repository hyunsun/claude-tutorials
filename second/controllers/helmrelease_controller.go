package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	helmv1alpha1 "github.com/example/helm-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	finalizerName    = "helm.example.com/finalizer"
	requeueOnFailure = 30 * time.Second
)

// HelmReleaseReconciler reconciles HelmRelease objects.
//
// +kubebuilder:rbac:groups=helm.example.com,resources=helmreleases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=helm.example.com,resources=helmreleases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=helm.example.com,resources=helmreleases/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods;services;configmaps;secrets;serviceaccounts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments;statefulsets;daemonsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=roles;rolebindings;clusterroles;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
type HelmReleaseReconciler struct {
	client.Client
	Scheme     *runtime.Scheme
	HelmClient HelmClientInterface
}

// Reconcile is the main reconciliation loop.
func (r *HelmReleaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	var release helmv1alpha1.HelmRelease
	if err := r.Get(ctx, req.NamespacedName, &release); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle deletion.
	if !release.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &release)
	}

	// Add finalizer if not present.
	if !controllerutil.ContainsFinalizer(&release, finalizerName) {
		controllerutil.AddFinalizer(&release, finalizerName)
		if err := r.Update(ctx, &release); err != nil {
			return ctrl.Result{}, fmt.Errorf("adding finalizer: %w", err)
		}
		log.Info("Added finalizer")
		return ctrl.Result{}, nil
	}

	return r.reconcileNormal(ctx, &release)
}

// reconcileNormal handles create and update operations.
func (r *HelmReleaseReconciler) reconcileNormal(ctx context.Context, release *helmv1alpha1.HelmRelease) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	releaseName := release.Name
	if release.Spec.ReleaseName != "" {
		releaseName = release.Spec.ReleaseName
	}

	// Parse optional values.
	values := map[string]interface{}{}
	if release.Spec.Values != nil {
		if err := json.Unmarshal(release.Spec.Values.Raw, &values); err != nil {
			return ctrl.Result{}, r.setFailedStatus(ctx, release, fmt.Errorf("parsing values: %w", err))
		}
	}

	exists, err := r.HelmClient.ReleaseExists(releaseName, release.Spec.TargetNamespace)
	if err != nil {
		return ctrl.Result{RequeueAfter: requeueOnFailure}, r.setFailedStatus(ctx, release, err)
	}

	if !exists {
		log.Info("Installing Helm release", "releaseName", releaseName)
		release.Status.Phase = helmv1alpha1.PhaseInstalling
		_ = r.Status().Update(ctx, release)

		if err := r.HelmClient.Install(ctx, releaseName, release.Spec.Chart, release.Spec.RepoURL,
			release.Spec.Version, release.Spec.TargetNamespace, values); err != nil {
			return ctrl.Result{RequeueAfter: requeueOnFailure}, r.setFailedStatus(ctx, release, err)
		}
	} else if release.Status.ObservedGeneration != release.Generation {
		log.Info("Upgrading Helm release", "releaseName", releaseName)
		release.Status.Phase = helmv1alpha1.PhaseUpgrading
		_ = r.Status().Update(ctx, release)

		if err := r.HelmClient.Upgrade(ctx, releaseName, release.Spec.Chart, release.Spec.RepoURL,
			release.Spec.Version, release.Spec.TargetNamespace, values); err != nil {
			return ctrl.Result{RequeueAfter: requeueOnFailure}, r.setFailedStatus(ctx, release, err)
		}
	}

	// Update status on success.
	now := metav1.Now()
	release.Status.Phase = helmv1alpha1.PhaseReady
	release.Status.DeployedVersion = release.Spec.Version
	release.Status.LastDeployedAt = &now
	release.Status.ObservedGeneration = release.Generation

	setCondition(release, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "ReconcileSuccess",
		Message:            "Helm release is ready",
		ObservedGeneration: release.Generation,
	})
	setCondition(release, metav1.Condition{
		Type:               "Progressing",
		Status:             metav1.ConditionFalse,
		Reason:             "ReconcileSuccess",
		Message:            "Helm release reconciliation complete",
		ObservedGeneration: release.Generation,
	})

	if err := r.Status().Update(ctx, release); err != nil {
		return ctrl.Result{}, fmt.Errorf("updating status: %w", err)
	}
	log.Info("Reconciliation complete", "phase", release.Status.Phase)
	return ctrl.Result{}, nil
}

// reconcileDelete handles CR deletion by uninstalling the Helm release.
func (r *HelmReleaseReconciler) reconcileDelete(ctx context.Context, release *helmv1alpha1.HelmRelease) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	if !controllerutil.ContainsFinalizer(release, finalizerName) {
		return ctrl.Result{}, nil
	}

	releaseName := release.Name
	if release.Spec.ReleaseName != "" {
		releaseName = release.Spec.ReleaseName
	}

	release.Status.Phase = helmv1alpha1.PhaseUninstalling
	_ = r.Status().Update(ctx, release)

	log.Info("Uninstalling Helm release", "releaseName", releaseName)
	if err := r.HelmClient.Uninstall(ctx, releaseName, release.Spec.TargetNamespace); err != nil {
		return ctrl.Result{RequeueAfter: requeueOnFailure}, r.setFailedStatus(ctx, release, err)
	}

	controllerutil.RemoveFinalizer(release, finalizerName)
	if err := r.Update(ctx, release); err != nil {
		return ctrl.Result{}, fmt.Errorf("removing finalizer: %w", err)
	}
	log.Info("Finalizer removed, deletion complete")
	return ctrl.Result{}, nil
}

// setFailedStatus records a failure condition and returns the original error.
func (r *HelmReleaseReconciler) setFailedStatus(ctx context.Context, release *helmv1alpha1.HelmRelease, err error) error {
	release.Status.Phase = helmv1alpha1.PhaseFailed
	setCondition(release, metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "ReconcileError",
		Message:            err.Error(),
		ObservedGeneration: release.Generation,
	})
	_ = r.Status().Update(ctx, release)
	return err
}

// setCondition upserts a condition on the HelmRelease status.
func setCondition(release *helmv1alpha1.HelmRelease, condition metav1.Condition) {
	condition.LastTransitionTime = metav1.Now()
	for i, c := range release.Status.Conditions {
		if c.Type == condition.Type {
			if c.Status == condition.Status {
				condition.LastTransitionTime = c.LastTransitionTime
			}
			release.Status.Conditions[i] = condition
			return
		}
	}
	release.Status.Conditions = append(release.Status.Conditions, condition)
}

// SetupWithManager registers the controller with the manager.
func (r *HelmReleaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&helmv1alpha1.HelmRelease{}).
		Complete(r)
}
