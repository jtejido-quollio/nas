package controllers

import (
	"context"
	"strings"
	"time"

	nasv1 "mnemosyne/api/v1alpha1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ZSnapshotRestoreReconciler struct {
	client.Client
	Cfg Config
}

func (r *ZSnapshotRestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var obj nasv1.ZSnapshotRestore
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	mode := obj.Spec.Mode
	mode = strings.ToLower(strings.TrimSpace(mode))

	if mode == "csi" {
		return r.reconcileCSI(ctx, &obj)
	}

	// default/legacy: clone
	source := obj.Spec.SourceSnapshot
	target := obj.Spec.TargetDataset
	if mode == "" {
		mode = "clone"
	}
	if mode != "clone" {
		obj.Status.Phase = "Failed"
		obj.Status.Message = "mode must be 'clone' or 'csi'"
		_ = r.Status().Update(ctx, &obj)
		return ctrl.Result{}, nil
	}
	if source == "" || target == "" {
		obj.Status.Phase = "Pending"
		obj.Status.Message = "sourceSnapshot and targetDataset required for clone mode"
		_ = r.Status().Update(ctx, &obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	phase := obj.Status.Phase
	if phase == "Succeeded" {
		return ctrl.Result{RequeueAfter: 10 * time.Minute}, nil
	}

	na := NewNodeAgentClient(r.Cfg)
	body := map[string]any{"sourceSnapshot": source, "targetDataset": target}
	var out any
	if err := na.do(ctx, "POST", "/v1/zfs/snapshot/clone", body, &out, nil); err != nil {
		obj.Status.Phase = "Failed"
		obj.Status.Message = err.Error()
		_ = r.Status().Update(ctx, &obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	obj.Status.Phase = "Succeeded"
	obj.Status.Message = "OK"
	obj.Status.ResultDataset = target
	_ = r.Status().Update(ctx, &obj)
	return ctrl.Result{RequeueAfter: 10 * time.Minute}, nil
}

func (r *ZSnapshotRestoreReconciler) reconcileCSI(ctx context.Context, obj *nasv1.ZSnapshotRestore) (ctrl.Result, error) {
	src := obj.Spec.SourceVolumeSnapshot
	tgt := obj.Spec.TargetPVC
	sc := obj.Spec.StorageClassName
	if sc == "" {
		sc = "nas-zfspv"
	}
	if src == "" || tgt == "" {
		obj.Status.Phase = "Pending"
		obj.Status.Message = "sourceVolumeSnapshot and targetPVC required for csi mode"
		_ = r.Status().Update(ctx, obj)
		return ctrl.Result{RequeueAfter: 20 * time.Second}, nil
	}

	// Create PVC if missing
	pvcGVK := schema.GroupVersionKind{Group: "", Version: "v1", Kind: "PersistentVolumeClaim"}
	pvc := &unstructured.Unstructured{}
	pvc.SetGroupVersionKind(pvcGVK)
	err := r.Get(ctx, client.ObjectKey{Namespace: obj.GetNamespace(), Name: tgt}, pvc)
	if err != nil {
		// Build PVC manifest
		pvc = &unstructured.Unstructured{}
		pvc.SetGroupVersionKind(pvcGVK)
		pvc.SetNamespace(obj.GetNamespace())
		pvc.SetName(tgt)
		_ = unstructured.SetNestedField(pvc.Object, sc, "spec", "storageClassName")

		accessModes := obj.Spec.AccessModes
		if len(accessModes) == 0 {
			accessModes = []string{"ReadWriteOnce"}
		}
		modes := make([]any, 0, len(accessModes))
		for _, mode := range accessModes {
			modes = append(modes, mode)
		}
		_ = unstructured.SetNestedField(pvc.Object, modes, "spec", "accessModes")

		resources := obj.Spec.Resources
		if resources == nil {
			resources = map[string]any{"requests": map[string]any{"storage": "1Gi"}}
		}
		_ = unstructured.SetNestedField(pvc.Object, resources, "spec", "resources")
		_ = unstructured.SetNestedField(pvc.Object, map[string]any{
			"apiGroup": "snapshot.storage.k8s.io",
			"kind":     "VolumeSnapshot",
			"name":     src,
		}, "spec", "dataSource")
		if err2 := r.Create(ctx, pvc); err2 != nil {
			obj.Status.Phase = "Failed"
			obj.Status.Message = err2.Error()
			_ = r.Status().Update(ctx, obj)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	}

	bound, _, _ := unstructured.NestedString(pvc.Object, "status", "phase")
	if bound == "Bound" {
		obj.Status.Phase = "Succeeded"
		obj.Status.Message = "OK"
		obj.Status.ResultPVC = tgt
		_ = r.Status().Update(ctx, obj)
		return ctrl.Result{RequeueAfter: 10 * time.Minute}, nil
	}

	obj.Status.Phase = "Restoring"
	obj.Status.Message = "PVC creation in progress"
	obj.Status.ResultPVC = tgt
	_ = r.Status().Update(ctx, obj)
	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

func (r *ZSnapshotRestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nasv1.ZSnapshotRestore{}).
		Complete(r)
}
