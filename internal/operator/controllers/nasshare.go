package controllers

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"time"

	nasv1 "mnemosyne/api/v1alpha1"
	"mnemosyne/internal/smbconf"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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
	dirName := strings.TrimSpace(spec.DirectoryRef)
	if dirName == "" {
		dirName = "local"
	}
	dir, err := resolveDirectory(ctx, r.Client, ns, dirName)
	if err != nil {
		obj.Status.Phase = "Error"
		obj.Status.Message = fmt.Sprintf("directory %s not found: %v", dirName, err)
		_ = r.Status().Update(ctx, obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	dirType := directoryType(dir)
	if dirType == "ldap" {
		obj.Status.Phase = "Error"
		obj.Status.Message = "SMB requires directory type local or activeDirectory"
		_ = r.Status().Update(ctx, obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

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

	dirCMName := fmt.Sprintf("nasdirectory-%s-smb", dir.GetName())
	var dirCM corev1.ConfigMap
	if err := r.Get(ctx, client.ObjectKey{Namespace: ns, Name: dirCMName}, &dirCM); err != nil {
		if !errors.IsNotFound(err) {
			obj.Status.Phase = "Error"
			obj.Status.Message = err.Error()
			_ = r.Status().Update(ctx, obj)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		obj.Status.Phase = "Error"
		obj.Status.Message = fmt.Sprintf("directory configmap %s not found", dirCMName)
		_ = r.Status().Update(ctx, obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	var adJoinUser, adJoinSecret string
	if dirType == "activeDirectory" {
		adJoinUser, adJoinSecret = directoryBindCredentials(dir)
		if strings.TrimSpace(adJoinUser) == "" || strings.TrimSpace(adJoinSecret) == "" {
			obj.Status.Phase = "Error"
			obj.Status.Message = "activeDirectory requires bind.username and bind.secretRef"
			_ = r.Status().Update(ctx, obj)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	}

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
	if opts.GlobalOptions == nil {
		opts.GlobalOptions = map[string]string{}
	}
	if _, ok := opts.GlobalOptions["include"]; !ok {
		opts.GlobalOptions["include"] = "/etc/smb/directory/smb.conf"
	}

	var allowSel, roSel nasv1.NASSharePrincipalSelector
	if spec.Permissions != nil {
		allowSel = spec.Permissions.Allow
		roSel = spec.Permissions.ReadOnly
	}
	var allowUsers, roUsers []smbUser
	var allowNames, roNames []string
	if dirType == "local" {
		var err error
		allowUsers, allowNames, err = resolveLocalUsers(ctx, r.Client, ns, dir.GetName(), allowSel)
		if err != nil {
			obj.Status.Phase = "Error"
			obj.Status.Message = err.Error()
			_ = r.Status().Update(ctx, obj)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		roUsers, roNames, err = resolveLocalUsers(ctx, r.Client, ns, dir.GetName(), roSel)
		if err != nil {
			obj.Status.Phase = "Error"
			obj.Status.Message = err.Error()
			_ = r.Status().Update(ctx, obj)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	} else {
		allowUsers, allowGroups := selectorPrincipals(allowSel)
		roUsers, roGroups := selectorPrincipals(roSel)
		allowNames = formatSMBPrincipals(allowUsers, allowGroups)
		roNames = formatSMBPrincipals(roUsers, roGroups)
	}

	if len(allowNames)+len(roNames) > 0 {
		if len(opts.ValidUsers) == 0 {
			opts.ValidUsers = uniqueStrings(append(append([]string{}, allowNames...), roNames...))
		}
		if len(opts.WriteList) == 0 && len(allowNames) > 0 {
			opts.WriteList = uniqueStrings(append([]string{}, allowNames...))
		}
	}

	if dirType == "local" {
		if len(allowUsers)+len(roUsers) == 0 {
			if opts.GuestOk == nil || !*opts.GuestOk {
				obj.Status.Phase = "Error"
				obj.Status.Message = "no local users resolved for SMB share"
				_ = r.Status().Update(ctx, obj)
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
		}
	} else {
		if len(opts.ValidUsers) == 0 && (opts.GuestOk == nil || !*opts.GuestOk) {
			obj.Status.Phase = "Error"
			obj.Status.Message = "no directory principals configured for SMB share"
			_ = r.Status().Update(ctx, obj)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	}

	conf, err := smbconf.Render(shareName, mountPath, readOnly, opts)
	if err != nil {
		obj.Status.Phase = "Error"
		obj.Status.Message = err.Error()
		_ = r.Status().Update(ctx, obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	var userScript string
	if dirType == "local" {
		users := mergeSMBUsers(allowUsers, roUsers)
		userScript, err = buildUserScript(ctx, r.Client, ns, users)
		if err != nil {
			obj.Status.Phase = "Error"
			obj.Status.Message = err.Error()
			_ = r.Status().Update(ctx, obj)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	} else {
		userScript = "#!/bin/sh\nset -e\n"
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
	volumeMounts := []corev1.VolumeMount{
		{Name: "conf", MountPath: "/etc/smb"},
		{Name: "directory", MountPath: "/etc/smb/directory", ReadOnly: true},
		{Name: "data", MountPath: mountPath, ReadOnly: readOnly},
	}
	volumes := []corev1.Volume{
		{
			Name: "conf",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: cmName},
				},
			},
		},
		{
			Name: "directory",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: dirCMName},
				},
			},
		},
		dataVolume,
	}

	var initContainers []corev1.Container
	if dirType == "activeDirectory" {
		statePath := getStringOption(spec.Options, "adJoinStatePath")
		if statePath == "" {
			statePath = filepath.Join("/var/lib/nas/samba", obj.GetName())
		}
		volumeMounts = append(volumeMounts,
			corev1.VolumeMount{Name: "directory", MountPath: "/etc/krb5.conf", SubPath: "krb5.conf", ReadOnly: true},
			corev1.VolumeMount{Name: "samba-state", MountPath: "/var/lib/samba"},
		)
		volumes = append(volumes, corev1.Volume{
			Name: "samba-state",
			VolumeSource: corev1.VolumeSource{
				HostPath: &corev1.HostPathVolumeSource{
					Path: statePath,
					Type: hostPathTypePtr(corev1.HostPathDirectoryOrCreate),
				},
			},
		})
		initContainers = append(initContainers, corev1.Container{
			Name:            "smb-join",
			Image:           "dperson/samba:latest",
			ImagePullPolicy: corev1.PullIfNotPresent,
			Command:         []string{"/bin/sh", "-c"},
			Args: []string{
				"net ads testjoin -s /etc/smb/smb.conf -k >/dev/null 2>&1 || net ads join -s /etc/smb/smb.conf -U \"$AD_JOIN_USER%$AD_JOIN_PASS\"",
			},
			Env: []corev1.EnvVar{
				{Name: "AD_JOIN_USER", Value: adJoinUser},
				{
					Name: "AD_JOIN_PASS",
					ValueFrom: &corev1.EnvVarSource{
						SecretKeyRef: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: adJoinSecret},
							Key:                  "password",
						},
					},
				},
			},
			VolumeMounts: []corev1.VolumeMount{
				{Name: "conf", MountPath: "/etc/smb"},
				{Name: "directory", MountPath: "/etc/smb/directory", ReadOnly: true},
				{Name: "directory", MountPath: "/etc/krb5.conf", SubPath: "krb5.conf", ReadOnly: true},
				{Name: "samba-state", MountPath: "/var/lib/samba"},
			},
		})
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
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": depName},
					Annotations: map[string]string{
						fmt.Sprintf("nas.io/directory-%s", dir.GetName()): strings.TrimSpace(dir.Status.AppliedHash),
					},
				},
				Spec: corev1.PodSpec{
					InitContainers: initContainers,
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
							VolumeMounts: volumeMounts,
						},
					},
					Volumes: volumes,
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
	dirName := strings.TrimSpace(spec.DirectoryRef)
	if dirName == "" {
		dirName = "local"
	}
	dir, err := resolveDirectory(ctx, r.Client, ns, dirName)
	if err != nil {
		obj.Status.Phase = "Error"
		obj.Status.Message = fmt.Sprintf("directory %s not found: %v", dirName, err)
		_ = r.Status().Update(ctx, obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}
	dirType := directoryType(dir)
	if dirType != "local" {
		if err := r.applyNFSDirectoryConfig(ctx, ns, dir); err != nil {
			obj.Status.Phase = "Error"
			obj.Status.Message = err.Error()
			_ = r.Status().Update(ctx, obj)
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
	}

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

func (r *NASShareReconciler) applyNFSDirectoryConfig(ctx context.Context, ns string, dir *nasv1.NASDirectory) error {
	secretName := fmt.Sprintf("nasdirectory-%s-nfs-sssd", dir.GetName())
	var sec corev1.Secret
	if err := r.Get(ctx, client.ObjectKey{Namespace: ns, Name: secretName}, &sec); err != nil {
		return fmt.Errorf("sssd secret %s not found: %w", secretName, err)
	}
	conf := strings.TrimSpace(string(sec.Data["sssd.conf"]))
	if conf == "" {
		return fmt.Errorf("sssd.conf missing in %s", secretName)
	}
	body := map[string]any{
		"config": conf,
	}
	if ca := sec.Data["ca.crt"]; len(ca) > 0 {
		body["caBundle"] = string(ca)
	}
	na := NewNodeAgentClient(r.Cfg)
	return na.do(ctx, "POST", "/v1/nfs/sssd/apply", body, nil, nil)
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

func getStringOption(opts map[string]any, key string) string {
	if opts == nil {
		return ""
	}
	val, ok := opts[key]
	if !ok || val == nil {
		return ""
	}
	switch v := val.(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}

func hostPathTypePtr(t corev1.HostPathType) *corev1.HostPathType {
	return &t
}

func (r *NASShareReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nasv1.NASShare{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
