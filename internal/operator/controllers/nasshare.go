package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	nasv1 "mnemosyne/api/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const nasshareFinalizer = "nas.io/nasshare-finalizer"

type NASShareReconciler struct {
	client.Client
	Cfg Config
}

func (r *NASShareReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var obj nasv1.NASShare
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	proto := strings.ToLower(strings.TrimSpace(obj.Spec.Protocol))
	if proto == "" {
		obj.Status.Phase = "Error"
		obj.Status.Message = "protocol required"
		_ = r.Status().Update(ctx, &obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	if !obj.DeletionTimestamp.IsZero() {
		if containsString(obj.Finalizers, nasshareFinalizer) {
			if proto == "nfs" {
				if err := r.deleteNFSExport(ctx, &obj); err != nil {
					obj.Status.Phase = "Error"
					obj.Status.Message = err.Error()
					_ = r.Status().Update(ctx, &obj)
					return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
				}
			}
			obj.Finalizers = removeString(obj.Finalizers, nasshareFinalizer)
			_ = r.Update(ctx, &obj)
		}
		return ctrl.Result{}, nil
	}

	if proto == "nfs" && !containsString(obj.Finalizers, nasshareFinalizer) {
		obj.Finalizers = append(obj.Finalizers, nasshareFinalizer)
		if err := r.Update(ctx, &obj); err != nil {
			return ctrl.Result{}, err
		}
	}

	switch proto {
	case "smb":
		return r.reconcileSMB(ctx, &obj)
	case "nfs":
		return r.reconcileNFS(ctx, &obj)
	default:
		obj.Status.Phase = "Error"
		obj.Status.Message = fmt.Sprintf("unsupported protocol: %s", obj.Spec.Protocol)
		_ = r.Status().Update(ctx, &obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
}

func (r *NASShareReconciler) reconcileSMB(ctx context.Context, obj *nasv1.NASShare) (ctrl.Result, error) {
	ns := obj.GetNamespace()
	if ns == "" {
		ns = r.Cfg.Namespace
	}

	var users []nasv1.SMBShareUser
	for _, name := range obj.Spec.Users {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var u nasv1.NASUser
		if err := r.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &u); err != nil {
			obj.Status.Phase = "Error"
			obj.Status.Message = fmt.Sprintf("nasuser %s not found: %v", name, err)
			_ = r.Status().Update(ctx, obj)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		users = append(users, nasv1.SMBShareUser{
			Username: u.Spec.Username,
			PasswordSecretRef: nasv1.SMBShareSecretRef{
				Name: u.Spec.PasswordSecretRef.Name,
			},
		})
	}

	child := nasv1.SMBShare{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.GetName(),
			Namespace: ns,
			Labels: map[string]string{
				"nas.io/managed-by": "nasshare",
			},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(obj, nasv1.GroupVersion.WithKind("NASShare")),
			},
		},
		Spec: nasv1.SMBShareSpec{
			NodeName:    "",
			DatasetName: obj.Spec.DatasetName,
			PVCName:     obj.Spec.PVCName,
			MountPath:   obj.Spec.MountPath,
			ShareName:   obj.Spec.ShareName,
			ReadOnly:    obj.Spec.ReadOnly,
			ServiceType: obj.Spec.ServiceType,
			NodePort:    obj.Spec.NodePort,
			Users:       users,
			Options:     obj.Spec.Options,
		},
	}
	_ = upsert(ctx, r.Client, &child)

	obj.Status.Phase = "Ready"
	obj.Status.Message = "OK"
	_ = r.Status().Update(ctx, obj)
	return ctrl.Result{RequeueAfter: 10 * time.Minute}, nil
}

func (r *NASShareReconciler) reconcileNFS(ctx context.Context, obj *nasv1.NASShare) (ctrl.Result, error) {
	ns := obj.GetNamespace()
	if ns == "" {
		ns = r.Cfg.Namespace
	}

	spec := obj.Spec
	na := NewNodeAgentClient(r.Cfg)
	if strings.TrimSpace(spec.DatasetName) != "" {
		body := map[string]any{"dataset": spec.DatasetName}
		if strings.TrimSpace(spec.MountPath) != "" {
			body["mountpoint"] = spec.MountPath
		}
		if perms := parseAutoPermissions(spec.Options); perms != nil {
			if strings.TrimSpace(perms.Mode) != "" {
				body["mode"] = perms.Mode
			}
			if perms.Recursive {
				body["recursive"] = true
			}
		}
		var out map[string]any
		if err := na.do(ctx, "POST", "/v1/zfs/dataset/mount", body, &out, nil); err != nil {
			obj.Status.Phase = "Error"
			obj.Status.Message = err.Error()
			_ = r.Status().Update(ctx, obj)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	}

	clients := []string{}
	options := ""
	if spec.NFS != nil {
		clients = append(clients, spec.NFS.Clients...)
		options = spec.NFS.Options
	}
	options = normalizeNFSOptions(options, spec.ReadOnly)
	if len(clients) == 0 {
		clients = []string{"*"}
	}

	body := map[string]any{
		"path":    spec.MountPath,
		"clients": clients,
		"options": options,
	}
	var out map[string]any
	if err := na.do(ctx, "POST", "/v1/nfs/export/ensure", body, &out, nil); err != nil {
		obj.Status.Phase = "Error"
		obj.Status.Message = err.Error()
		_ = r.Status().Update(ctx, obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	obj.Status.Phase = "Ready"
	obj.Status.Message = "OK"
	obj.Status.Endpoint = spec.MountPath
	_ = r.Status().Update(ctx, obj)
	return ctrl.Result{RequeueAfter: 10 * time.Minute}, nil
}

func (r *NASShareReconciler) deleteNFSExport(ctx context.Context, obj *nasv1.NASShare) error {
	if strings.ToLower(strings.TrimSpace(obj.Spec.Protocol)) != "nfs" {
		return nil
	}
	if strings.TrimSpace(obj.Spec.MountPath) == "" {
		return nil
	}
	na := NewNodeAgentClient(r.Cfg)
	body := map[string]any{"path": obj.Spec.MountPath}
	return na.do(ctx, "POST", "/v1/nfs/export/delete", body, nil, nil)
}

func normalizeNFSOptions(raw string, readOnly bool) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if readOnly {
			return "ro,sync,no_subtree_check"
		}
		return "rw,sync,no_subtree_check"
	}
	parts := strings.Split(raw, ",")
	var out []string
	hasRO := false
	hasRW := false
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if p == "ro" {
			hasRO = true
		}
		if p == "rw" {
			hasRW = true
		}
		out = append(out, p)
	}
	if readOnly && !hasRO {
		out = append(out, "ro")
	}
	if !readOnly && !hasRW {
		out = append(out, "rw")
	}
	return strings.Join(out, ",")
}

func containsString(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func removeString(list []string, s string) []string {
	out := []string{}
	for _, v := range list {
		if v == s {
			continue
		}
		out = append(out, v)
	}
	return out
}

func (r *NASShareReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nasv1.NASShare{}).
		Owns(&nasv1.SMBShare{}).
		Complete(r)
}
