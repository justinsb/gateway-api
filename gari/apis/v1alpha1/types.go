package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +kubebuilder:object:root=true

// OIDCAuth configures oidc authentication.
type OIDCAuth struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Spec defines the desired state of OIDCAuth.
	Spec OIDCAuthSpec `json:"spec"`

	// Status defines the current state of OIDCAuth.
	Status OIDCAuthStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OIDCAuthList contains a list of OIDCAuth.
type OIDCAuthList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OIDCAuth `json:"items"`
}

// OIDCAuthSpec defines the desired state of OIDCAuth.
type OIDCAuthSpec struct {
	LoginURL string `json:"loginURL,omitempty"`
	Audience string `json:"audience,omitempty"`
	Issuer   string `json:"issuer,omitempty"`
}

// OIDCAuthStatus defines the observed state of OIDCAuth.
type OIDCAuthStatus struct {
}
