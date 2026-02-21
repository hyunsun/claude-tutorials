package controllers_test

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	helmv1alpha1 "github.com/example/helm-operator/api/v1alpha1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	testNS  = "default"
	timeout = 10 * time.Second
	polling = 250 * time.Millisecond
)

func makeHR(name string) *helmv1alpha1.HelmRelease {
	return &helmv1alpha1.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNS,
		},
		Spec: helmv1alpha1.HelmReleaseSpec{
			Chart:           "nginx",
			RepoURL:         "https://charts.example.com",
			Version:         "1.0.0",
			TargetNamespace: testNS,
		},
	}
}

func getHR(ctx context.Context, name string) (*helmv1alpha1.HelmRelease, error) {
	hr := &helmv1alpha1.HelmRelease{}
	err := k8sClient.Get(ctx, types.NamespacedName{Name: name, Namespace: testNS}, hr)
	return hr, err
}

var _ = Describe("HelmReleaseReconciler", func() {
	ctx := context.Background()

	Describe("Finalizer", func() {
		It("adds the finalizer on the first reconcile", func() {
			mock := &MockHelmClient{}
			cancel := startManager(mock)
			defer cancel()

			hr := makeHR("test-finalizer")
			Expect(k8sClient.Create(ctx, hr)).To(Succeed())
			DeferCleanup(func() { k8sClient.Delete(ctx, hr) })

			Eventually(func(g Gomega) {
				fetched, err := getHR(ctx, hr.Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(fetched.Finalizers).To(ContainElement("helm.example.com/finalizer"))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())
		})
	})

	Describe("Install", func() {
		It("installs when the release is absent", func() {
			mock := &MockHelmClient{}
			cancel := startManager(mock)
			defer cancel()

			hr := makeHR("test-install")
			Expect(k8sClient.Create(ctx, hr)).To(Succeed())
			DeferCleanup(func() { k8sClient.Delete(ctx, hr) })

			Eventually(func(g Gomega) {
				mock.mu.Lock()
				called := mock.InstallCalled
				mock.mu.Unlock()
				g.Expect(called).To(BeTrue())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			Eventually(func(g Gomega) {
				fetched, err := getHR(ctx, hr.Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(fetched.Status.Phase).To(Equal(helmv1alpha1.PhaseReady))
				g.Expect(fetched.Status.DeployedVersion).To(Equal("1.0.0"))
				g.Expect(fetched.Status.ObservedGeneration).To(Equal(fetched.Generation))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())
		})

		It("uses Spec.ReleaseName override in Install", func() {
			mock := &MockHelmClient{}
			cancel := startManager(mock)
			defer cancel()

			hr := makeHR("test-releasename")
			hr.Spec.ReleaseName = "custom-name"
			Expect(k8sClient.Create(ctx, hr)).To(Succeed())
			DeferCleanup(func() { k8sClient.Delete(ctx, hr) })

			Eventually(func(g Gomega) {
				mock.mu.Lock()
				args := mock.InstallArgs
				mock.mu.Unlock()
				g.Expect(args.ReleaseName).To(Equal("custom-name"))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())
		})

		It("passes Spec.Values through to Install", func() {
			mock := &MockHelmClient{}
			cancel := startManager(mock)
			defer cancel()

			rawValues, _ := json.Marshal(map[string]interface{}{"replicaCount": 3})
			hr := makeHR("test-values")
			hr.Spec.Values = &apiextensionsv1.JSON{Raw: rawValues}
			Expect(k8sClient.Create(ctx, hr)).To(Succeed())
			DeferCleanup(func() { k8sClient.Delete(ctx, hr) })

			Eventually(func(g Gomega) {
				mock.mu.Lock()
				vals := mock.InstallArgs.Values
				mock.mu.Unlock()
				g.Expect(vals).To(HaveKeyWithValue("replicaCount", float64(3)))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())
		})

		It("sets Phase=Failed with Ready=False condition on install error", func() {
			mock := &MockHelmClient{InstallErr: errors.New("install failed")}
			cancel := startManager(mock)
			defer cancel()

			hr := makeHR("test-install-err")
			Expect(k8sClient.Create(ctx, hr)).To(Succeed())
			DeferCleanup(func() { k8sClient.Delete(ctx, hr) })

			Eventually(func(g Gomega) {
				fetched, err := getHR(ctx, hr.Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(fetched.Status.Phase).To(Equal(helmv1alpha1.PhaseFailed))
				var readyCond *metav1.Condition
				for i := range fetched.Status.Conditions {
					if fetched.Status.Conditions[i].Type == "Ready" {
						readyCond = &fetched.Status.Conditions[i]
					}
				}
				g.Expect(readyCond).NotTo(BeNil())
				g.Expect(readyCond.Status).To(Equal(metav1.ConditionFalse))
				g.Expect(readyCond.Message).To(ContainSubstring("install failed"))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())
		})
	})

	Describe("Upgrade", func() {
		It("upgrades when release exists and generation mismatches", func() {
			// ReleaseExists=true → first real reconcile sees gen(1) != observedGen(0) → Upgrade
			mock := &MockHelmClient{ReleaseExistsResult: true}
			cancel := startManager(mock)
			defer cancel()

			hr := makeHR("test-upgrade")
			Expect(k8sClient.Create(ctx, hr)).To(Succeed())
			DeferCleanup(func() { k8sClient.Delete(ctx, hr) })

			Eventually(func(g Gomega) {
				mock.mu.Lock()
				upgraded := mock.UpgradeCalled
				installed := mock.InstallCalled
				mock.mu.Unlock()
				g.Expect(upgraded).To(BeTrue())
				g.Expect(installed).To(BeFalse())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			Eventually(func(g Gomega) {
				fetched, err := getHR(ctx, hr.Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(fetched.Status.Phase).To(Equal(helmv1alpha1.PhaseReady))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())
		})

		It("does not upgrade when generation already matches", func() {
			mock := &MockHelmClient{ReleaseExistsResult: true}
			cancel := startManager(mock)
			defer cancel()

			hr := makeHR("test-noupgrade")
			Expect(k8sClient.Create(ctx, hr)).To(Succeed())
			DeferCleanup(func() { k8sClient.Delete(ctx, hr) })

			// Wait until reconciliation reaches Ready (the initial gen-mismatch upgrade is done)
			Eventually(func(g Gomega) {
				fetched, err := getHR(ctx, hr.Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(fetched.Status.Phase).To(Equal(helmv1alpha1.PhaseReady))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			// Reset call flag so we detect any future spurious upgrade
			mock.mu.Lock()
			mock.UpgradeCalled = false
			mock.mu.Unlock()

			// The controller does not requeue on success; verify it stays idle
			Consistently(func(g Gomega) {
				mock.mu.Lock()
				called := mock.UpgradeCalled
				mock.mu.Unlock()
				g.Expect(called).To(BeFalse())
			}).WithTimeout(2 * time.Second).WithPolling(polling).Should(Succeed())
		})

		It("sets Phase=Failed on upgrade error", func() {
			mock := &MockHelmClient{ReleaseExistsResult: true, UpgradeErr: errors.New("upgrade failed")}
			cancel := startManager(mock)
			defer cancel()

			hr := makeHR("test-upgrade-err")
			Expect(k8sClient.Create(ctx, hr)).To(Succeed())
			DeferCleanup(func() { k8sClient.Delete(ctx, hr) })

			Eventually(func(g Gomega) {
				fetched, err := getHR(ctx, hr.Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(fetched.Status.Phase).To(Equal(helmv1alpha1.PhaseFailed))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())
		})
	})

	Describe("Delete", func() {
		It("uninstalls and removes finalizer so the object disappears", func() {
			mock := &MockHelmClient{}
			cancel := startManager(mock)
			defer cancel()

			hr := makeHR("test-delete")
			Expect(k8sClient.Create(ctx, hr)).To(Succeed())

			// Wait for install to complete before deleting
			Eventually(func(g Gomega) {
				mock.mu.Lock()
				called := mock.InstallCalled
				mock.mu.Unlock()
				g.Expect(called).To(BeTrue())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			Expect(k8sClient.Delete(ctx, hr)).To(Succeed())

			Eventually(func(g Gomega) {
				mock.mu.Lock()
				called := mock.UninstallCalled
				mock.mu.Unlock()
				g.Expect(called).To(BeTrue())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			Eventually(func(g Gomega) {
				_, err := getHR(ctx, hr.Name)
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())
		})

		It("uses Spec.ReleaseName in Uninstall", func() {
			mock := &MockHelmClient{}
			cancel := startManager(mock)
			defer cancel()

			hr := makeHR("test-delete-relname")
			hr.Spec.ReleaseName = "override"
			Expect(k8sClient.Create(ctx, hr)).To(Succeed())

			Eventually(func(g Gomega) {
				mock.mu.Lock()
				called := mock.InstallCalled
				mock.mu.Unlock()
				g.Expect(called).To(BeTrue())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			Expect(k8sClient.Delete(ctx, hr)).To(Succeed())

			Eventually(func(g Gomega) {
				mock.mu.Lock()
				args := mock.UninstallArgs
				mock.mu.Unlock()
				g.Expect(args.ReleaseName).To(Equal("override"))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())
		})

		It("keeps finalizer and sets Phase=Failed when Uninstall errors", func() {
			mock := &MockHelmClient{UninstallErr: errors.New("uninstall failed")}
			cancel := startManager(mock)
			defer cancel()

			hr := makeHR("test-delete-err")
			Expect(k8sClient.Create(ctx, hr)).To(Succeed())

			Eventually(func(g Gomega) {
				mock.mu.Lock()
				called := mock.InstallCalled
				mock.mu.Unlock()
				g.Expect(called).To(BeTrue())
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			Expect(k8sClient.Delete(ctx, hr)).To(Succeed())

			Eventually(func(g Gomega) {
				fetched, err := getHR(ctx, hr.Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(fetched.Status.Phase).To(Equal(helmv1alpha1.PhaseFailed))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			Consistently(func(g Gomega) {
				fetched, err := getHR(ctx, hr.Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(fetched.Finalizers).To(ContainElement("helm.example.com/finalizer"))
			}).WithTimeout(2 * time.Second).WithPolling(polling).Should(Succeed())

			// Cleanup: remove the finalizer so the object can be deleted
			DeferCleanup(func() {
				fetched, err := getHR(ctx, hr.Name)
				if err != nil {
					return
				}
				patch := client.MergeFrom(fetched.DeepCopy())
				fetched.Finalizers = nil
				k8sClient.Patch(ctx, fetched, patch) //nolint:errcheck
			})
		})
	})

	Describe("ReleaseExists error", func() {
		It("sets Phase=Failed and skips Install/Upgrade when ReleaseExists errors", func() {
			mock := &MockHelmClient{ReleaseExistsErr: errors.New("exists check failed")}
			cancel := startManager(mock)
			defer cancel()

			hr := makeHR("test-exists-err")
			Expect(k8sClient.Create(ctx, hr)).To(Succeed())
			DeferCleanup(func() { k8sClient.Delete(ctx, hr) })

			Eventually(func(g Gomega) {
				fetched, err := getHR(ctx, hr.Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(fetched.Status.Phase).To(Equal(helmv1alpha1.PhaseFailed))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())

			Consistently(func(g Gomega) {
				mock.mu.Lock()
				installed := mock.InstallCalled
				upgraded := mock.UpgradeCalled
				mock.mu.Unlock()
				g.Expect(installed).To(BeFalse())
				g.Expect(upgraded).To(BeFalse())
			}).WithTimeout(2 * time.Second).WithPolling(polling).Should(Succeed())
		})
	})

	Describe("Conditions", func() {
		It("sets Ready=True and Progressing=False on success", func() {
			mock := &MockHelmClient{}
			cancel := startManager(mock)
			defer cancel()

			hr := makeHR("test-conditions")
			Expect(k8sClient.Create(ctx, hr)).To(Succeed())
			DeferCleanup(func() { k8sClient.Delete(ctx, hr) })

			Eventually(func(g Gomega) {
				fetched, err := getHR(ctx, hr.Name)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(fetched.Status.Phase).To(Equal(helmv1alpha1.PhaseReady))

				condMap := map[string]metav1.ConditionStatus{}
				for _, c := range fetched.Status.Conditions {
					condMap[c.Type] = c.Status
				}
				g.Expect(condMap["Ready"]).To(Equal(metav1.ConditionTrue))
				g.Expect(condMap["Progressing"]).To(Equal(metav1.ConditionFalse))
			}).WithTimeout(timeout).WithPolling(polling).Should(Succeed())
		})
	})
})
