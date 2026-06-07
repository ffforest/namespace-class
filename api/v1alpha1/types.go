package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	ClassLabelKey = "namespaceclass.akuity.io/name"

	ManagedLabelKey          = "namespaceclass.akuity.io/managed"
	ClassLabelOwnerKey       = "namespaceclass.akuity.io/class"
	NamespaceLabelOwnerKey   = "namespaceclass.akuity.io/namespace"
	OwnerNamespaceUIDAnnoKey = "namespaceclass.akuity.io/owner-namespace-uid"

	ConditionReady        = "Ready"
	ReasonBindingRecorded = "BindingRecorded"
	ReasonClassNotFound   = "ClassNotFound"
	ReasonCleanupFailed   = "CleanupFailed"
)

type NamespaceClassSpec struct {
	Resources []runtime.RawExtension `json:"resources"`
}

type NamespaceClassStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

type NamespaceClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NamespaceClassSpec   `json:"spec,omitempty"`
	Status NamespaceClassStatus `json:"status,omitempty"`
}

type NamespaceClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []NamespaceClass `json:"items"`
}

type NamespaceClassBindingSpec struct {
	NamespaceName string `json:"namespaceName"`
	ClassName     string `json:"className"`
}

type ResourceRef struct {
	APIVersion string `json:"apiVersion"`
	Kind       string `json:"kind"`
	Namespace  string `json:"namespace,omitempty"`
	Name       string `json:"name"`
}

type NamespaceClassBindingStatus struct {
	ObservedNamespaceUID    string             `json:"observedNamespaceUID,omitempty"`
	ObservedClassGeneration int64              `json:"observedClassGeneration,omitempty"`
	Inventory               []ResourceRef      `json:"inventory,omitempty"`
	Conditions              []metav1.Condition `json:"conditions,omitempty"`
}

type NamespaceClassBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   NamespaceClassBindingSpec   `json:"spec,omitempty"`
	Status NamespaceClassBindingStatus `json:"status,omitempty"`
}

type NamespaceClassBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []NamespaceClassBinding `json:"items"`
}
