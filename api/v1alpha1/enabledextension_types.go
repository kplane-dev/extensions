package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnabledExtension activates an Extension or PlatformExtension for one or more
// nested control planes within a project VCP.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:subresource:status
type EnabledExtension struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EnabledExtensionSpec   `json:"spec,omitempty"`
	Status EnabledExtensionStatus `json:"status,omitempty"`
}

// EnabledExtensionSpec defines the desired state of EnabledExtension.
// +kubebuilder:object:generate=true
type EnabledExtensionSpec struct {
	// ExtensionRef references the Extension or PlatformExtension to enable.
	ExtensionRef ExtensionRef `json:"extensionRef"`

	// ControlPlanes lists the nested control plane names to enable the extension
	// for. Use ["*"] to target all control planes in the project.
	// +kubebuilder:validation:MinItems=1
	ControlPlanes []string `json:"controlPlanes"`
}

type ExtensionRef struct {
	// Kind is either "Extension" or "PlatformExtension".
	// +kubebuilder:validation:Enum=Extension;PlatformExtension
	Kind string `json:"kind"`

	// Name of the Extension or PlatformExtension.
	Name string `json:"name"`
}

// EnabledExtensionStatus defines the observed state of EnabledExtension.
// +kubebuilder:object:generate=true
// +kubebuilder:printcolumn:name="Accepted",type=string,JSONPath=`.status.conditions[?(@.type=="Accepted")].status`
// +kubebuilder:printcolumn:name="Programmed",type=string,JSONPath=`.status.conditions[?(@.type=="Programmed")].status`
type EnabledExtensionStatus struct {
	// Conditions includes Accepted (spec is valid) and Programmed (CRDs
	// installed in all currently-targeted nested CPs).
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

const (
	// ConditionAccepted is True when the ExtensionRef resolves and the spec is valid.
	ConditionAccepted = "Accepted"
	// ConditionProgrammed is True when CRDs have been installed into every
	// currently-targeted nested control plane.
	ConditionProgrammed = "Programmed"
)

// EnabledExtensionList contains a list of EnabledExtension.
// +kubebuilder:object:root=true
type EnabledExtensionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []EnabledExtension `json:"items"`
}

func init() {
	SchemeBuilder.Register(&EnabledExtension{}, &EnabledExtensionList{})
}
