package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PlatformExtension is a kplane-managed extension available for enablement in
// project control planes. Defined centrally in GKE and replicated read-only
// into every project VCP. Third parties contribute entries via catalog/.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
type PlatformExtension struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PlatformExtensionSpec   `json:"spec,omitempty"`
	Status PlatformExtensionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:generate=true
type PlatformExtensionSpec struct {
	// DisplayName is the human-readable name shown in the UI.
	DisplayName string `json:"displayName"`

	// Description explains what the extension provides.
	Description string `json:"description"`

	// Tags are short labels used for filtering in the UI (e.g. "DNS", "VMs").
	// +optional
	Tags []string `json:"tags,omitempty"`

	// CRDs is a list of URLs to CRD manifests to install into nested control
	// planes when the extension is enabled.
	// +optional
	CRDs []string `json:"crds,omitempty"`
}

// +kubebuilder:object:generate=true
type PlatformExtensionStatus struct {
	// Conditions reflect replication health.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type PlatformExtensionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PlatformExtension `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PlatformExtension{}, &PlatformExtensionList{})
}
