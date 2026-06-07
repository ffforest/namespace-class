package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func (in *NamespaceClassSpec) DeepCopyInto(out *NamespaceClassSpec) {
	*out = *in
	if in.Resources != nil {
		out.Resources = make([]runtime.RawExtension, len(in.Resources))
		for i := range in.Resources {
			in.Resources[i].DeepCopyInto(&out.Resources[i])
		}
	}
}

func (in *NamespaceClassSpec) DeepCopy() *NamespaceClassSpec {
	if in == nil {
		return nil
	}
	out := new(NamespaceClassSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *NamespaceClassStatus) DeepCopyInto(out *NamespaceClassStatus) {
	*out = *in
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		copy(out.Conditions, in.Conditions)
	}
}

func (in *NamespaceClassStatus) DeepCopy() *NamespaceClassStatus {
	if in == nil {
		return nil
	}
	out := new(NamespaceClassStatus)
	in.DeepCopyInto(out)
	return out
}

func (in *NamespaceClass) DeepCopyInto(out *NamespaceClass) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

func (in *NamespaceClass) DeepCopy() *NamespaceClass {
	if in == nil {
		return nil
	}
	out := new(NamespaceClass)
	in.DeepCopyInto(out)
	return out
}

func (in *NamespaceClass) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *NamespaceClassList) DeepCopyInto(out *NamespaceClassList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]NamespaceClass, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *NamespaceClassList) DeepCopy() *NamespaceClassList {
	if in == nil {
		return nil
	}
	out := new(NamespaceClassList)
	in.DeepCopyInto(out)
	return out
}

func (in *NamespaceClassList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *NamespaceClassBindingSpec) DeepCopyInto(out *NamespaceClassBindingSpec) {
	*out = *in
}

func (in *NamespaceClassBindingSpec) DeepCopy() *NamespaceClassBindingSpec {
	if in == nil {
		return nil
	}
	out := new(NamespaceClassBindingSpec)
	in.DeepCopyInto(out)
	return out
}

func (in *ResourceRef) DeepCopyInto(out *ResourceRef) {
	*out = *in
}

func (in *ResourceRef) DeepCopy() *ResourceRef {
	if in == nil {
		return nil
	}
	out := new(ResourceRef)
	in.DeepCopyInto(out)
	return out
}

func (in *NamespaceClassBindingStatus) DeepCopyInto(out *NamespaceClassBindingStatus) {
	*out = *in
	if in.Inventory != nil {
		out.Inventory = make([]ResourceRef, len(in.Inventory))
		copy(out.Inventory, in.Inventory)
	}
	if in.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(in.Conditions))
		copy(out.Conditions, in.Conditions)
	}
}

func (in *NamespaceClassBindingStatus) DeepCopy() *NamespaceClassBindingStatus {
	if in == nil {
		return nil
	}
	out := new(NamespaceClassBindingStatus)
	in.DeepCopyInto(out)
	return out
}

func (in *NamespaceClassBinding) DeepCopyInto(out *NamespaceClassBinding) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	out.Spec = in.Spec
	in.Status.DeepCopyInto(&out.Status)
}

func (in *NamespaceClassBinding) DeepCopy() *NamespaceClassBinding {
	if in == nil {
		return nil
	}
	out := new(NamespaceClassBinding)
	in.DeepCopyInto(out)
	return out
}

func (in *NamespaceClassBinding) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

func (in *NamespaceClassBindingList) DeepCopyInto(out *NamespaceClassBindingList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]NamespaceClassBinding, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

func (in *NamespaceClassBindingList) DeepCopy() *NamespaceClassBindingList {
	if in == nil {
		return nil
	}
	out := new(NamespaceClassBindingList)
	in.DeepCopyInto(out)
	return out
}

func (in *NamespaceClassBindingList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}
