package controllers_test

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	helmv1alpha1 "github.com/example/helm-operator/api/v1alpha1"
	"github.com/example/helm-operator/controllers"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var (
	cfg       *rest.Config
	k8sClient client.Client
	testEnv   *envtest.Environment
	scheme    = runtime.NewScheme()
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controllers Suite")
}

var _ = BeforeSuite(func() {
	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	testEnv = &envtest.Environment{
		CRDDirectoryPaths: []string{"../config/crd/bases"},
	}

	var err error
	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
	Expect(helmv1alpha1.AddToScheme(scheme)).To(Succeed())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	Expect(testEnv.Stop()).To(Succeed())
})

// startManager creates a manager with a fresh HelmReleaseReconciler backed by
// the given mock, starts it in a goroutine, and returns a cancel function that
// the caller must defer.
func startManager(mock *MockHelmClient) context.CancelFunc {
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: "0",
		},
		LeaderElection: false,
	})
	Expect(err).NotTo(HaveOccurred())

	err = (&controllers.HelmReleaseReconciler{
		Client:     mgr.GetClient(),
		Scheme:     mgr.GetScheme(),
		HelmClient: mock,
	}).SetupWithManager(mgr)
	Expect(err).NotTo(HaveOccurred())

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer GinkgoRecover()
		Expect(mgr.Start(ctx)).To(Succeed())
	}()

	return cancel
}
