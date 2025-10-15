package rbac

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// TelegramBotPermission is the Schema for the telegrambotpermissions API
type TelegramBotPermission struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec TelegramBotPermissionSpec `json:"spec,omitempty"`
}

// TelegramBotPermissionSpec defines the desired state of TelegramBotPermission
type TelegramBotPermissionSpec struct {
	TelegramUserID int64        `json:"telegramUserId"`
	Role           string       `json:"role"`
	Permissions    []Permission `json:"permissions,omitempty"`
}

// Permission defines granular access control
type Permission struct {
	Namespace string   `json:"namespace"`
	Resources []string `json:"resources"`
	Verbs     []string `json:"verbs"`
	Selector  string   `json:"selector,omitempty"`
}

// TelegramBotPermissionList contains a list of TelegramBotPermission
type TelegramBotPermissionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []TelegramBotPermission `json:"items"`
}

// DeepCopyInto copies all properties of this object into another object of the same type
func (in *TelegramBotPermission) DeepCopyInto(out *TelegramBotPermission) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
}

// DeepCopy creates a deep copy
func (in *TelegramBotPermission) DeepCopy() *TelegramBotPermission {
	if in == nil {
		return nil
	}
	out := new(TelegramBotPermission)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject creates a deep copy object
func (in *TelegramBotPermission) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

// DeepCopyInto copies spec
func (in *TelegramBotPermissionSpec) DeepCopyInto(out *TelegramBotPermissionSpec) {
	*out = *in
	if in.Permissions != nil {
		in, out := &in.Permissions, &out.Permissions
		*out = make([]Permission, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopyInto copies permission
func (in *Permission) DeepCopyInto(out *Permission) {
	*out = *in
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.Verbs != nil {
		in, out := &in.Verbs, &out.Verbs
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
}

// DeepCopyInto copies list
func (in *TelegramBotPermissionList) DeepCopyInto(out *TelegramBotPermissionList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]TelegramBotPermission, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy creates a deep copy of list
func (in *TelegramBotPermissionList) DeepCopy() *TelegramBotPermissionList {
	if in == nil {
		return nil
	}
	out := new(TelegramBotPermissionList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject creates a deep copy object of list
func (in *TelegramBotPermissionList) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}
