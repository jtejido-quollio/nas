package v1alpha1

// PasswordSecretRef references a Secret that contains a "password" key.
type PasswordSecretRef struct {
	Name string `json:"name"`
}

func (in *PasswordSecretRef) DeepCopyInto(out *PasswordSecretRef) { *out = *in }

func (in *PasswordSecretRef) DeepCopy() *PasswordSecretRef {
	if in == nil {
		return nil
	}
	out := new(PasswordSecretRef)
	in.DeepCopyInto(out)
	return out
}
