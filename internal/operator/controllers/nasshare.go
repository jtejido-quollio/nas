package controllers

import (
	"context"
	"encoding/base64"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	nasv1 "mnemosyne/api/v1alpha1"
	"mnemosyne/internal/smbconf"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
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
		if slices.Contains(obj.Finalizers, nasshareFinalizer) {
			if proto == "nfs" {
				if err := r.deleteNFSExport(ctx, &obj); err != nil {
					obj.Status.Phase = "Error"
					obj.Status.Message = err.Error()
					_ = r.Status().Update(ctx, &obj)
					return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
				}
			}
			obj.Finalizers = slices.DeleteFunc(obj.Finalizers, func(n string) bool {
				return n == nasshareFinalizer
			})
			_ = r.Update(ctx, &obj)
		}
		return ctrl.Result{}, nil
	}

	if proto == "nfs" && !slices.Contains(obj.Finalizers, nasshareFinalizer) {
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

	spec := obj.Spec
	if strings.TrimSpace(spec.DatasetName) == "" && strings.TrimSpace(spec.PVCName) == "" {
		obj.Status.Phase = "Error"
		obj.Status.Message = "datasetName or pvcName required for SMB shares"
		_ = r.Status().Update(ctx, obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	shareName := spec.ShareName
	mountPath := spec.MountPath
	readOnly := spec.ReadOnly
	svcType := spec.ServiceType
	nodePort64 := int64(spec.NodePort)

	if strings.TrimSpace(spec.PVCName) == "" && strings.TrimSpace(spec.DatasetName) != "" {
		na := NewNodeAgentClient(r.Cfg)
		body := map[string]any{"dataset": spec.DatasetName}
		if strings.TrimSpace(mountPath) != "" {
			body["mountpoint"] = mountPath
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

	// Options - best-effort map into our allowlisted renderer.
	opts := parseOptions(spec.Options)
	conf, err := smbconf.Render(shareName, mountPath, readOnly, opts)
	if err != nil {
		obj.Status.Phase = "Error"
		obj.Status.Message = err.Error()
		_ = r.Status().Update(ctx, obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	users, err := resolveNASUsers(ctx, r.Client, ns, spec.Users)
	if err != nil {
		obj.Status.Phase = "Error"
		obj.Status.Message = err.Error()
		_ = r.Status().Update(ctx, obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	userScript, err := buildUserScript(ctx, r.Client, ns, users)
	if err != nil {
		obj.Status.Phase = "Error"
		obj.Status.Message = err.Error()
		_ = r.Status().Update(ctx, obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	cmName := fmt.Sprintf("smbshare-%s-conf", obj.GetName())
	depName := fmt.Sprintf("smbshare-%s", obj.GetName())
	svcName := fmt.Sprintf("smbshare-%s", obj.GetName())
	ownerRef := *metav1.NewControllerRef(obj, nasv1.GroupVersion.WithKind("NASShare"))

	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            cmName,
			Namespace:       ns,
			OwnerReferences: []metav1.OwnerReference{ownerRef},
		},
		Data: map[string]string{
			"smb.conf": conf,
			"users.sh": userScript,
		},
	}
	_ = upsert(ctx, r.Client, &cm)

	replicas := int32(1)
	dataVolume := corev1.Volume{Name: "data"}
	if strings.TrimSpace(spec.PVCName) != "" {
		dataVolume.VolumeSource = corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: spec.PVCName,
				ReadOnly:  readOnly,
			},
		}
	} else {
		dataVolume.VolumeSource = corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{Path: mountPath},
		}
	}
	dep := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            depName,
			Namespace:       ns,
			OwnerReferences: []metav1.OwnerReference{ownerRef},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": depName}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": depName}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "samba",
							Image: "dperson/samba:latest",
							SecurityContext: &corev1.SecurityContext{
								Privileged: boolPtr(true),
							},
							Ports: []corev1.ContainerPort{
								{Name: "smb", ContainerPort: 445},
							},
							Command: []string{"/bin/sh", "-c"},
							Args: []string{
								"sh /etc/smb/users.sh && if command -v samba.sh >/dev/null 2>&1; then exec samba.sh -I /etc/smb/smb.conf; else exec /usr/sbin/smbd -F -s /etc/smb/smb.conf; fi",
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "conf", MountPath: "/etc/smb"},
								{Name: "data", MountPath: mountPath, ReadOnly: readOnly},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "conf",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: cmName},
								},
							},
						},
						dataVolume,
					},
				},
			},
		},
	}
	_ = upsert(ctx, r.Client, &dep)

	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            svcName,
			Namespace:       ns,
			OwnerReferences: []metav1.OwnerReference{ownerRef},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": depName},
			Ports: []corev1.ServicePort{
				{Name: "smb", Port: 445, TargetPort: intstr.FromInt(445)},
			},
		},
	}
	if strings.EqualFold(svcType, "NodePort") {
		svc.Spec.Type = corev1.ServiceTypeNodePort
		if nodePort64 > 0 {
			svc.Spec.Ports[0].NodePort = int32(nodePort64)
		}
	} else {
		svc.Spec.Type = corev1.ServiceTypeClusterIP
	}
	_ = upsert(ctx, r.Client, &svc)

	obj.Status.Phase = "Ready"
	obj.Status.Message = "OK"
	if svc.Spec.Type == corev1.ServiceTypeNodePort {
		obj.Status.Endpoint = fmt.Sprintf("NodePort:%d", svc.Spec.Ports[0].NodePort)
	}
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

func (r *NASShareReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nasv1.NASShare{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

type smbUser struct {
	Username           string
	PasswordSecretName string
}

func resolveNASUsers(ctx context.Context, c client.Client, ns string, names []string) ([]smbUser, error) {
	var out []smbUser
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		var u nasv1.NASUser
		if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &u); err != nil {
			return nil, fmt.Errorf("nasuser %s not found: %w", name, err)
		}
		username := strings.TrimSpace(u.Spec.Username)
		secName := strings.TrimSpace(u.Spec.PasswordSecretRef.Name)
		if username == "" || secName == "" {
			continue
		}
		out = append(out, smbUser{
			Username:           username,
			PasswordSecretName: secName,
		})
	}
	return out, nil
}

func buildUserScript(ctx context.Context, c client.Client, ns string, users []smbUser) (string, error) {
	lines := []string{"#!/bin/sh", "set -e"}
	for _, u := range users {
		if u.Username == "" || u.PasswordSecretName == "" {
			continue
		}
		var sec corev1.Secret
		if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: u.PasswordSecretName}, &sec); err != nil {
			return "", err
		}
		pw := string(sec.Data["password"])
		if pw == "" {
			pw = string(sec.StringData["password"])
		}
		enc := base64.StdEncoding.EncodeToString([]byte(pw))
		lines = append(lines,
			fmt.Sprintf("id -u %s >/dev/null 2>&1 || adduser -D %s", u.Username, u.Username),
			fmt.Sprintf("pw=$(echo %s | base64 -d)", enc),
			fmt.Sprintf("printf '%%s\\n%%s\\n' \"$pw\" \"$pw\" | smbpasswd -a -s %s", u.Username),
		)
	}
	return strings.Join(lines, "\n") + "\n", nil
}

func parseOptions(m map[string]any) smbconf.Options {
	var o smbconf.Options

	if v, ok := m["macosCompat"].(bool); ok {
		o.MacOSCompat = &v
	}
	if v, ok := m["encryption"].(string); ok {
		o.Encryption = &v
	}
	if v, ok := m["browseable"].(bool); ok {
		o.Browseable = &v
	}
	if v, ok := m["guestOk"].(bool); ok {
		o.GuestOk = &v
	}
	if v, ok := m["validUsers"].([]any); ok {
		for _, x := range v {
			if s, ok := x.(string); ok {
				o.ValidUsers = append(o.ValidUsers, s)
			}
		}
	}
	if v, ok := m["writeList"].([]any); ok {
		for _, x := range v {
			if s, ok := x.(string); ok {
				o.WriteList = append(o.WriteList, s)
			}
		}
	}
	if v, ok := m["globalOptions"].(map[string]any); ok {
		o.GlobalOptions = map[string]string{}
		for k, raw := range v {
			if s, ok := raw.(string); ok && strings.TrimSpace(s) != "" {
				o.GlobalOptions[k] = s
			}
		}
	}
	if v, ok := m["createMask"].(string); ok {
		o.CreateMask = &v
	}
	if v, ok := m["directoryMask"].(string); ok {
		o.DirectoryMask = &v
	}
	if v, ok := m["inheritPerms"].(bool); ok {
		o.InheritPerms = &v
	}

	if se, ok := m["snapshotExposure"].(map[string]any); ok {
		enabled, _ := se["enabled"].(bool)
		mode, _ := se["mode"].(string)
		format, _ := se["format"].(string)
		var lt *bool
		if b, ok := se["localTime"].(bool); ok {
			lt = &b
		}
		o.SnapshotExposure = &smbconf.SnapshotExposure{
			Enabled:   enabled,
			Mode:      mode,
			Format:    format,
			LocalTime: lt,
		}
	}
	if tm, ok := m["timeMachine"].(map[string]any); ok {
		enabled, _ := tm["enabled"].(bool)
		var adv *bool
		if b, ok := tm["advertiseAsTimeMachine"].(bool); ok {
			adv = &b
		}
		var lim *int64
		if f, ok := tm["volumeSizeLimitBytes"].(float64); ok {
			v := int64(f)
			lim = &v
		}
		o.TimeMachine = &smbconf.TimeMachine{
			Enabled:                enabled,
			AdvertiseAsTimeMachine: adv,
			VolumeSizeLimitBytes:   lim,
		}
	}

	return o
}

type AutoPermissions struct {
	Mode      string
	Recursive bool
}

func parseAutoPermissions(m map[string]any) *AutoPermissions {
	if m == nil {
		return nil
	}
	raw, ok := m["autoPermissions"]
	if !ok || raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case bool:
		if !v {
			return nil
		}
		return &AutoPermissions{Mode: "0777"}
	case map[string]any:
		if enabled, ok := v["enabled"].(bool); ok && !enabled {
			return nil
		}
		mode := ""
		switch mv := v["mode"].(type) {
		case string:
			mode = strings.TrimSpace(mv)
		case float64:
			mode = strconv.FormatInt(int64(mv), 10)
		case int:
			mode = strconv.Itoa(mv)
		case int64:
			mode = strconv.FormatInt(mv, 10)
		}
		if mode == "" {
			mode = "0777"
		}
		rec, _ := v["recursive"].(bool)
		return &AutoPermissions{Mode: mode, Recursive: rec}
	default:
		return nil
	}
}

func boolPtr(b bool) *bool { return &b }
