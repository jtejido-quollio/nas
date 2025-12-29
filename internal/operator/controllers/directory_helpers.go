package controllers

import (
	"context"
	"fmt"
	"strings"

	nasv1 "mnemosyne/api/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

func resolveDirectory(ctx context.Context, c client.Client, ns, name string) (*nasv1.NASDirectory, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "local"
	}
	var dir nasv1.NASDirectory
	if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &dir); err != nil {
		return nil, err
	}
	return &dir, nil
}

func directoryType(dir *nasv1.NASDirectory) string {
	if dir == nil {
		return ""
	}
	t := strings.ToLower(strings.TrimSpace(dir.Spec.Type))
	if t == "" {
		return "local"
	}
	return t
}

func requireLocalDirectory(dir *nasv1.NASDirectory) error {
	if directoryType(dir) != "local" {
		return fmt.Errorf("directory type %q not supported yet", dir.Spec.Type)
	}
	return nil
}

func directoryBindCredentials(dir *nasv1.NASDirectory) (string, string) {
	if dir == nil || dir.Spec.Bind == nil || dir.Spec.Bind.SecretRef == nil {
		return "", ""
	}
	user := strings.TrimSpace(dir.Spec.Bind.Username)
	secret := strings.TrimSpace(dir.Spec.Bind.SecretRef.Name)
	return user, secret
}
