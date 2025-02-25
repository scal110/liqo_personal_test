package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// VirtualNodeConnectionSpec definisce i dati di input
type VirtualNodeConnectionSpec struct {
	VirtualNodeA string `json:"virtualNodeA"`
	KubeconfigA  string `json:"kubeconfigA"`
	VirtualNodeB string `json:"virtualNodeB"`
	KubeconfigB  string `json:"kubeconfigB"`
}

// VirtualNodeConnectionStatus definisce lo stato della connessione
type VirtualNodeConnectionStatus struct {
	IsConnected  bool   `json:"isConnected"`
	LastUpdated  string `json:"lastUpdated,omitempty"`
	Phase        string `json:"phase,omitempty"`
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
type VirtualNodeConnection struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              VirtualNodeConnectionSpec   `json:"spec,omitempty"`
	Status            VirtualNodeConnectionStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true
type VirtualNodeConnectionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualNodeConnection `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VirtualNodeConnection{}, &VirtualNodeConnectionList{})
}
