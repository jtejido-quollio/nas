package controllers

import (
	"context"
	"net/url"
	"time"

	nasv1 "mnemosyne/api/v1alpha1"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ZPoolReconciler struct {
	client.Client
	Cfg Config
}

func (r *ZPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var obj nasv1.ZPool
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	poolName := obj.Spec.PoolName
	vdevs := obj.Spec.Vdevs

	na := NewNodeAgentClient(r.Cfg)

	var list struct {
		OK    bool     `json:"ok"`
		Items []string `json:"items"`
	}
	_ = na.do(ctx, "GET", "/v1/zfs/pool/list", nil, &list, nil)
	exists := false
	for _, n := range list.Items {
		if n == poolName {
			exists = true
			break
		}
	}
	if !exists {
		body := map[string]any{
			"poolName": poolName,
			"vdevs":    vdevs,
		}
		var out any
		if err := na.do(ctx, "POST", "/v1/zfs/pool/create", body, &out, nil); err != nil {
			obj.Status.Phase = "Error"
			obj.Status.Message = err.Error()
			_ = r.Status().Update(ctx, &obj)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	}

	var statusResp struct {
		OK   bool `json:"ok"`
		Pool *struct {
			Usage *nasv1.ZPoolUsage `json:"usage,omitempty"`
		} `json:"pool,omitempty"`
		Error string `json:"error,omitempty"`
	}
	if err := na.do(ctx, "GET", "/v1/zfs/zpools/status?name="+url.QueryEscape(poolName), nil, &statusResp, nil); err == nil {
		if statusResp.Pool != nil && statusResp.Pool.Usage != nil {
			obj.Status.Usage = statusResp.Pool.Usage
		}
	}

	obj.Status.Phase = "Ready"
	obj.Status.Message = "OK"
	_ = r.Status().Update(ctx, &obj)
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *ZPoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nasv1.ZPool{}).
		Complete(r)
}
