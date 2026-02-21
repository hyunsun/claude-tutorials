package v1alpha1

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Phase represents the current lifecycle phase of a HelmRelease.
type Phase string

const (
	PhaseInstalling   Phase = "Installing"
	PhaseUpgrading    Phase = "Upgrading"
	PhaseReady        Phase = "Ready"
	PhaseFailed       Phase = "Failed"
	PhaseUninstalling Phase = "Uninstalling"
)

// HelmReleaseSpec defines the desired state of HelmRelease.
// +kubebuilder:object:generate=true
type HelmReleaseSpec struct {
	// Chart is the name of the Helm chart to deploy.
	// +kubebuilder:validation:Required
	Chart string `json:"chart"`

	// RepoURL is the URL of the Helm chart repository.
	// +kubebuilder:validation:Required
	RepoURL string `json:"repoURL"`

	// Version is the version of the Helm chart to deploy.
	// +kubebuilder:validation:Required
	Version string `json:"version"`

	// TargetNamespace is the Kubernetes namespace where the Helm release will be installed.
	// +kubebuilder:validation:Required
	TargetNamespace string `json:"targetNamespace"`

	// ReleaseName overrides the Helm release name. Defaults to metadata.name.
	// +kubebuilder:validation:Optional
	// +optional
	ReleaseName string `json:"releaseName,omitempty"`

	// Values contains Helm values to pass to the chart during install/upgrade.
	// +kubebuilder:validation:Optional
	// +optional
	Values *apiextensionsv1.JSON `json:"values,omitempty"`
}

// HelmReleaseStatus defines the observed state of HelmRelease.
// +kubebuilder:object:generate=true
type HelmReleaseStatus struct {
	// Phase is the current lifecycle phase of the release.
	// +kubebuilder:validation:Enum=Installing;Upgrading;Ready;Failed;Uninstalling
	// +optional
	Phase Phase `json:"phase,omitempty"`

	// Conditions represent the latest observations of the HelmRelease's state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// DeployedVersion is the chart version currently deployed.
	// +optional
	DeployedVersion string `json:"deployedVersion,omitempty"`

	// HelmRevision is the Helm release revision number.
	// +optional
	HelmRevision int `json:"helmRevision,omitempty"`

	// LastDeployedAt is the timestamp of the last successful Helm operation.
	// +optional
	LastDeployedAt *metav1.Time `json:"lastDeployedAt,omitempty"`

	// ObservedGeneration is the last generation the controller successfully reconciled.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// HelmRelease is the Schema for the helmreleases API.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=hr
// +kubebuilder:printcolumn:name="Chart",type=string,JSONPath=`.spec.chart`
// +kubebuilder:printcolumn:name="Version",type=string,JSONPath=`.spec.version`
// +kubebuilder:printcolumn:name="Namespace",type=string,JSONPath=`.spec.targetNamespace`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
type HelmRelease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HelmReleaseSpec   `json:"spec,omitempty"`
	Status HelmReleaseStatus `json:"status,omitempty"`
}

// HelmReleaseList contains a list of HelmRelease.
// +kubebuilder:object:root=true
type HelmReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HelmRelease `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HelmRelease{}, &HelmReleaseList{})
}
