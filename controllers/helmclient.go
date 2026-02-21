package controllers

import (
	"context"
	"fmt"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

// HelmClientInterface abstracts Helm operations so the reconciler can be tested
// with a mock without requiring a real Helm/Kubernetes cluster.
type HelmClientInterface interface {
	Install(ctx context.Context, releaseName, chartName, repoURL, version, namespace string, values map[string]interface{}) error
	Upgrade(ctx context.Context, releaseName, chartName, repoURL, version, namespace string, values map[string]interface{}) error
	Uninstall(ctx context.Context, releaseName, namespace string) error
	ReleaseExists(releaseName, namespace string) (bool, error)
}

var _ HelmClientInterface = (*HelmClient)(nil) // compile-time interface check

// HelmClient wraps helm.sh/helm/v3/pkg/action to provide install, upgrade,
// uninstall, and release-existence checks against a Kubernetes cluster.
type HelmClient struct {
	restConfig *rest.Config
}

// NewHelmClient creates a HelmClient from the given REST config.
func NewHelmClient(cfg *rest.Config) *HelmClient {
	return &HelmClient{restConfig: cfg}
}

// restClientGetter implements genericclioptions.RESTClientGetter so that the
// Helm action configuration can discover the cluster topology.
type restClientGetter struct {
	restConfig *rest.Config
	namespace  string
}

func (r *restClientGetter) ToRESTConfig() (*rest.Config, error) {
	return r.restConfig, nil
}

func (r *restClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	dc, err := discovery.NewDiscoveryClientForConfig(r.restConfig)
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(dc), nil
}

func (r *restClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	dc, err := r.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}
	gr, err := restmapper.GetAPIGroupResources(dc)
	if err != nil {
		return nil, err
	}
	return restmapper.NewDiscoveryRESTMapper(gr), nil
}

func (r *restClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{
		ClusterInfo: clientcmdapi.Cluster{Server: r.restConfig.Host},
		Context:     clientcmdapi.Context{Namespace: r.namespace},
	}
	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
}

// actionConfig builds a Helm action.Configuration scoped to the given namespace.
func (h *HelmClient) actionConfig(namespace string) (*action.Configuration, error) {
	getter := &restClientGetter{restConfig: h.restConfig, namespace: namespace}
	cfg := new(action.Configuration)
	if err := cfg.Init(getter, namespace, "secret", func(format string, v ...interface{}) {}); err != nil {
		return nil, fmt.Errorf("initialising helm action config: %w", err)
	}
	return cfg, nil
}

// Install performs a helm install for the given parameters.
func (h *HelmClient) Install(ctx context.Context, releaseName, chartName, repoURL, version, namespace string, values map[string]interface{}) error {
	cfg, err := h.actionConfig(namespace)
	if err != nil {
		return err
	}

	client := action.NewInstall(cfg)
	client.ReleaseName = releaseName
	client.Namespace = namespace
	client.Version = version
	client.ChartPathOptions.RepoURL = repoURL

	settings := cli.New()
	chartPath, err := client.ChartPathOptions.LocateChart(chartName, settings)
	if err != nil {
		return fmt.Errorf("locating chart: %w", err)
	}
	chart, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("loading chart: %w", err)
	}

	_, err = client.RunWithContext(ctx, chart, values)
	return err
}

// Upgrade performs a helm upgrade for the given parameters.
func (h *HelmClient) Upgrade(ctx context.Context, releaseName, chartName, repoURL, version, namespace string, values map[string]interface{}) error {
	cfg, err := h.actionConfig(namespace)
	if err != nil {
		return err
	}

	client := action.NewUpgrade(cfg)
	client.Namespace = namespace
	client.Version = version
	client.ChartPathOptions.RepoURL = repoURL

	settings := cli.New()
	chartPath, err := client.ChartPathOptions.LocateChart(chartName, settings)
	if err != nil {
		return fmt.Errorf("locating chart: %w", err)
	}
	chart, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("loading chart: %w", err)
	}

	_, err = client.RunWithContext(ctx, releaseName, chart, values)
	return err
}

// Uninstall removes the Helm release from the given namespace.
func (h *HelmClient) Uninstall(_ context.Context, releaseName, namespace string) error {
	cfg, err := h.actionConfig(namespace)
	if err != nil {
		return err
	}
	client := action.NewUninstall(cfg)
	_, err = client.Run(releaseName)
	return err
}

// ReleaseExists returns true if a Helm release with the given name exists in the namespace.
func (h *HelmClient) ReleaseExists(releaseName, namespace string) (bool, error) {
	cfg, err := h.actionConfig(namespace)
	if err != nil {
		return false, err
	}
	histClient := action.NewHistory(cfg)
	histClient.Max = 1
	_, err = histClient.Run(releaseName)
	if err == driver.ErrReleaseNotFound {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}
