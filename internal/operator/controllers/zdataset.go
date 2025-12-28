package controllers

import (
	"context"
	"time"

	nasv1 "mnemosyne/api/v1alpha1"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ZDatasetReconciler struct {
	client.Client
	Cfg Config
}

func (r *ZDatasetReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var obj nasv1.ZDataset
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	ds := obj.Spec.DatasetName
	props := obj.Spec.Properties

	na := NewNodeAgentClient(r.Cfg)
	body := map[string]any{
		"dataset":    ds,
		"properties": props,
	}
	var out any
	if err := na.do(ctx, "POST", "/v1/zfs/dataset/ensure", body, &out, nil); err != nil {
		obj.Status.Phase = "Error"
		obj.Status.Message = err.Error()
		_ = r.Status().Update(ctx, &obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	obj.Status.Phase = "Ready"
	obj.Status.Message = "OK"
	_ = r.Status().Update(ctx, &obj)
	return ctrl.Result{RequeueAfter: 10 * time.Minute}, nil
}

func (r *ZDatasetReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nasv1.ZDataset{}).
		Complete(r)
}
