package controllers

import (
	"context"
	"fmt"
	"time"

	nasv1 "mnemosyne/api/v1alpha1"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ZSnapshot creates a CSI VolumeSnapshot for a PVC.

type ZSnapshotReconciler struct {
	client.Client
	Cfg Config
}

var volumeSnapshotGVK = schema.GroupVersionKind{Group: "snapshot.storage.k8s.io", Version: "v1", Kind: "VolumeSnapshot"}

func (r *ZSnapshotReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var obj nasv1.ZSnapshot
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	pvcName := obj.Spec.PVCName
	if pvcName == "" {
		obj.Status.Phase = "Pending"
		obj.Status.Message = "spec.pvcName is required"
		_ = r.Status().Update(ctx, &obj)
		return ctrl.Result{RequeueAfter: 20 * time.Second}, nil
	}

	snapClass := obj.Spec.SnapshotClassName
	if snapClass == "" {
		snapClass = "nas-zfspv-snapclass"
	}

	// Desired VolumeSnapshot name: <zsnapshot name>
	vsName := req.Name

	// Create/patch VolumeSnapshot
	vs := &unstructured.Unstructured{}
	vs.SetGroupVersionKind(volumeSnapshotGVK)
	vs.SetNamespace(req.Namespace)
	vs.SetName(vsName)

	mutate := func() error {
		// spec
		_ = unstructured.SetNestedField(vs.Object, snapClass, "spec", "volumeSnapshotClassName")
		_ = unstructured.SetNestedField(vs.Object, map[string]any{"persistentVolumeClaimName": pvcName}, "spec", "source")
		return nil
	}

	op, err := controllerutil.CreateOrPatch(ctx, r.Client, vs, mutate)
	if err != nil {
		obj.Status.Phase = "Failed"
		obj.Status.Message = fmt.Sprintf("create VolumeSnapshot: %v", err)
		_ = r.Status().Update(ctx, &obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Read status.readyToUse
	ready, _, _ := unstructured.NestedBool(vs.Object, "status", "readyToUse")
	if ready {
		obj.Status.Phase = "Succeeded"
		obj.Status.Message = "Ready"
	} else {
		obj.Status.Phase = "Creating"
		obj.Status.Message = fmt.Sprintf("VolumeSnapshot %s (%s)", vsName, op)
	}
	obj.Status.VolumeSnapshotName = vsName
	_ = r.Status().Update(ctx, &obj)
	return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
}

func (r *ZSnapshotReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nasv1.ZSnapshot{}).
		Complete(r)
}
