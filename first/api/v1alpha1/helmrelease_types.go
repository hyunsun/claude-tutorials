package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// HelmReleaseSpec defines the desired state of HelmRelease
type HelmReleaseSpec struct {
	// Chart is the name of the Helm chart
	Chart string `json:"chart"`
	// RepoURL is the Helm repository URL
	RepoURL string `json:"repoURL"`
	// Version is the chart version
	Version string `json:"version"`
	// No targetNamespace — can't install into a specific namespace
	// No values — can't customize the chart
}

// HelmReleaseStatus defines the observed state of HelmRelease
type HelmReleaseStatus struct {
	// Installed indicates whether the chart has been installed
	// No phase, conditions, revision, or last deployed time
	Installed bool `json:"installed,omitempty"`
}

// +kubebuilder:object:root=true
type HelmRelease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HelmReleaseSpec   `json:"spec,omitempty"`
	Status HelmReleaseStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type HelmReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HelmRelease `json:"items"`
}

// Missing: DeepCopyObject and DeepCopyInto methods — won't compile without controller-gen

var (
	GroupVersion  = schema.GroupVersion{Group: "helm.example.com", Version: "v1alpha1"}
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	AddToScheme   = SchemeBuilder.AddToScheme
)

func init() {
	SchemeBuilder.Register(&HelmRelease{}, &HelmReleaseList{})
}

// Ensure HelmRelease implements runtime.Object — this will fail at compile time
// because DeepCopyObject is not implemented.
var _ runtime.Object = &HelmRelease{}
