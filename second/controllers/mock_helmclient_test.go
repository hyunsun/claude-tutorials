package controllers_test

import (
	"context"
	"sync"
)

// InstallCallArgs captures arguments from the last Install call.
type InstallCallArgs struct {
	ReleaseName string
	ChartName   string
	RepoURL     string
	Version     string
	Namespace   string
	Values      map[string]interface{}
}

// UpgradeCallArgs captures arguments from the last Upgrade call.
type UpgradeCallArgs struct {
	ReleaseName string
	ChartName   string
	RepoURL     string
	Version     string
	Namespace   string
	Values      map[string]interface{}
}

// UninstallCallArgs captures arguments from the last Uninstall call.
type UninstallCallArgs struct {
	ReleaseName string
	Namespace   string
}

// MockHelmClient is a thread-safe mock implementation of HelmClientInterface.
// All exported fields may be set before use; reads during concurrent access
// are protected by the embedded mutex.
type MockHelmClient struct {
	mu sync.Mutex

	// Configurable return values.
	InstallErr          error
	UpgradeErr          error
	UninstallErr        error
	ReleaseExistsResult bool
	ReleaseExistsErr    error

	// Call-tracking booleans (guarded by mu).
	InstallCalled   bool
	UpgradeCalled   bool
	UninstallCalled bool

	// Last-call argument capture (guarded by mu).
	InstallArgs   InstallCallArgs
	UpgradeArgs   UpgradeCallArgs
	UninstallArgs UninstallCallArgs
}

func (m *MockHelmClient) Install(_ context.Context, releaseName, chartName, repoURL, version, namespace string, values map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.InstallCalled = true
	m.InstallArgs = InstallCallArgs{
		ReleaseName: releaseName,
		ChartName:   chartName,
		RepoURL:     repoURL,
		Version:     version,
		Namespace:   namespace,
		Values:      values,
	}
	return m.InstallErr
}

func (m *MockHelmClient) Upgrade(_ context.Context, releaseName, chartName, repoURL, version, namespace string, values map[string]interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.UpgradeCalled = true
	m.UpgradeArgs = UpgradeCallArgs{
		ReleaseName: releaseName,
		ChartName:   chartName,
		RepoURL:     repoURL,
		Version:     version,
		Namespace:   namespace,
		Values:      values,
	}
	return m.UpgradeErr
}

func (m *MockHelmClient) Uninstall(_ context.Context, releaseName, namespace string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.UninstallCalled = true
	m.UninstallArgs = UninstallCallArgs{
		ReleaseName: releaseName,
		Namespace:   namespace,
	}
	return m.UninstallErr
}

func (m *MockHelmClient) ReleaseExists(releaseName, namespace string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.ReleaseExistsResult, m.ReleaseExistsErr
}
