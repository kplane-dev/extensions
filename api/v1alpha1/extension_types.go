package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Extension is a user-defined extension that can be enabled for control planes
// within a project. Unlike PlatformExtension it is fully user-owned.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:subresource:status
type Extension struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExtensionSpec   `json:"spec,omitempty"`
	Status ExtensionStatus `json:"status,omitempty"`
}

// ExtensionSpec defines the desired state of Extension.
// +kubebuilder:object:generate=true
type ExtensionSpec struct {
	// DisplayName is the human-readable name shown in the UI.
	DisplayName string `json:"displayName"`

	// Description explains what the extension provides.
	Description string `json:"description"`

	// Tags are short labels used for filtering in the UI.
	// +optional
	Tags []string `json:"tags,omitempty"`

	// CRDs is a list of URLs to CRD manifests to install into nested control
	// planes when the extension is enabled.
	// +optional
	CRDs []string `json:"crds,omitempty"`
}

// ExtensionStatus defines the observed state of Extension.
// +kubebuilder:object:generate=true
type ExtensionStatus struct {
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ExtensionList contains a list of Extension.
// +kubebuilder:object:root=true
type ExtensionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Extension `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Extension{}, &ExtensionList{})
}
