package controllers

import (
	"context"
	"fmt"

	helmv1alpha1 "github.com/example/helm-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HelmReleaseReconciler reconciles a HelmRelease object
// No RBAC markers — generated ClusterRole will be empty
type HelmReleaseReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

func (r *HelmReleaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var release helmv1alpha1.HelmRelease
	if err := r.Get(ctx, req.NamespacedName, &release); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if release.Status.Installed {
		fmt.Printf("HelmRelease %s already installed\n", release.Name)
		return ctrl.Result{}, nil
	}

	// TODO: actually install the Helm chart
	// This is where helm.sh/helm/v3 would be used, but it's not in go.mod
	fmt.Printf("Installing chart %s from %s\n", release.Spec.Chart, release.Spec.RepoURL)

	release.Status.Installed = true
	if err := r.Status().Update(ctx, &release); err != nil {
		return ctrl.Result{}, err
	}

	// No finalizer — deleting CR leaves orphaned Helm release
	// No error handling for Helm failures
	// No requeue on failure
	return ctrl.Result{}, nil
}

func (r *HelmReleaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&helmv1alpha1.HelmRelease{}).
		Complete(r)
}
