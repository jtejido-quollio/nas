package controllers

import (
	"context"
	"encoding/base64"
	"fmt"
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

type SMBShareReconciler struct {
	client.Client
	Cfg Config
}

func (r *SMBShareReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var obj nasv1.SMBShare
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	ns := obj.GetNamespace()
	if ns == "" {
		ns = r.Cfg.Namespace
	}

	spec := obj.Spec
	shareName := spec.ShareName
	mountPath := spec.MountPath
	readOnly := spec.ReadOnly
	svcType := spec.ServiceType
	nodePort64 := int64(spec.NodePort)

	// Options - best-effort map into our allowlisted renderer
	opts := parseOptions(spec.Options)

	conf, err := smbconf.Render(shareName, mountPath, readOnly, opts)
	if err != nil {
		obj.Status.Phase = "Error"
		obj.Status.Message = err.Error()
		_ = r.Status().Update(ctx, &obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	cmName := fmt.Sprintf("smbshare-%s-conf", obj.GetName())
	depName := fmt.Sprintf("smbshare-%s", obj.GetName())
	svcName := fmt.Sprintf("smbshare-%s", obj.GetName())

	// Build users script from Secret refs
	userScript, err := r.buildUserScript(ctx, ns, spec.Users)
	if err != nil {
		obj.Status.Phase = "Error"
		obj.Status.Message = err.Error()
		_ = r.Status().Update(ctx, &obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// ConfigMap
	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: ns,
		},
		Data: map[string]string{
			"smb.conf": conf,
			"users.sh": userScript,
		},
	}
	_ = upsert(ctx, r.Client, &cm)

	// Deployment (uses dperson/samba; runs users.sh then starts samba)
	replicas := int32(1)
	dep := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: depName, Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": depName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": depName},
				},
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
								"sh /etc/smb/users.sh && exec /usr/sbin/smbd -F -s /etc/smb/smb.conf",
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "conf", MountPath: "/etc/smb"},
								{Name: "data", MountPath: mountPath},
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
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{Path: mountPath},
							},
						},
					},
				},
			},
		},
	}
	_ = upsert(ctx, r.Client, &dep)

	// Service
	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: svcName, Namespace: ns},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": depName},
			Ports: []corev1.ServicePort{
				{
					Name:       "smb",
					Port:       445,
					TargetPort: intstr.FromInt(445),
				},
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

	// Set status
	obj.Status.Phase = "Ready"
	obj.Status.Message = "OK"
	if svc.Spec.Type == corev1.ServiceTypeNodePort {
		obj.Status.Endpoint = fmt.Sprintf("NodePort:%d", svc.Spec.Ports[0].NodePort)
	}
	_ = r.Status().Update(ctx, &obj)

	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

func (r *SMBShareReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nasv1.SMBShare{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}

func (r *SMBShareReconciler) buildUserScript(ctx context.Context, ns string, users []nasv1.SMBShareUser) (string, error) {
	var lines []string
	lines = append(lines, "#!/bin/sh", "set -e")

	for _, u := range users {
		username := strings.TrimSpace(u.Username)
		secName := strings.TrimSpace(u.PasswordSecretRef.Name)
		if username == "" || secName == "" {
			continue
		}
		var sec corev1.Secret
		if err := r.Get(ctx, client.ObjectKey{Namespace: ns, Name: secName}, &sec); err != nil {
			return "", err
		}
		pw := string(sec.Data["password"])
		if pw == "" {
			// allow stringData in cluster - but it will be in Data already
			pw = string(sec.StringData["password"])
		}
		// dperson/samba expects `smbpasswd -a user` but we can do:
		// useradd + smbpasswd
		enc := base64.StdEncoding.EncodeToString([]byte(pw))
		lines = append(lines,
			fmt.Sprintf("id -u %s >/dev/null 2>&1 || adduser -D %s", username, username),
			fmt.Sprintf("pw=$(echo %s | base64 -d)", enc),
			fmt.Sprintf("printf '%%s\\n%%s\\n' \"$pw\" \"$pw\" | smbpasswd -a -s %s", username),
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

func boolPtr(b bool) *bool { return &b }
