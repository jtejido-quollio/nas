package controllers

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func upsert(ctx context.Context, c client.Client, obj client.Object) error {
	err := c.Create(ctx, obj)
	if err == nil {
		return nil
	}
	if errors.IsAlreadyExists(err) {
		return c.Update(ctx, obj)
	}
	return err
}
