package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	nasv1 "mnemosyne/api/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type NASDirectoryReconciler struct {
	client.Client
	Cfg Config
}

func (r *NASDirectoryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var obj nasv1.NASDirectory
	if err := r.Get(ctx, req.NamespacedName, &obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	if !obj.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, nil
	}

	dirType, ok := normalizeDirectoryType(obj.Spec.Type)
	if !ok {
		obj.Status.Phase = "Error"
		obj.Status.Message = fmt.Sprintf("unsupported type: %s", obj.Spec.Type)
		obj.Status.ObservedGeneration = obj.Generation
		_ = r.Status().Update(ctx, &obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	errs, usesLDAPS := validateDirectorySpec(obj.Spec, dirType)
	if len(errs) > 0 {
		obj.Status.Phase = "Error"
		obj.Status.Message = strings.Join(errs, "; ")
		obj.Status.ObservedGeneration = obj.Generation
		_ = r.Status().Update(ctx, &obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	var bindSecret, caSecret *corev1.Secret
	if dirType != "local" {
		if name := secretName(obj.Spec.Bind); name != "" {
			var sec corev1.Secret
			if err := r.Get(ctx, client.ObjectKey{Namespace: obj.Namespace, Name: name}, &sec); err != nil {
				obj.Status.Phase = "Error"
				obj.Status.Message = fmt.Sprintf("bind secret %s not found: %v", name, err)
				obj.Status.ObservedGeneration = obj.Generation
				_ = r.Status().Update(ctx, &obj)
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
			bindSecret = &sec
		}
		if specTLS := obj.Spec.TLS; specTLS != nil {
			if specTLS.CABundleSecretRef != nil && strings.TrimSpace(specTLS.CABundleSecretRef.Name) != "" {
				var sec corev1.Secret
				if err := r.Get(ctx, client.ObjectKey{Namespace: obj.Namespace, Name: specTLS.CABundleSecretRef.Name}, &sec); err != nil {
					obj.Status.Phase = "Error"
					obj.Status.Message = fmt.Sprintf("ca bundle secret %s not found: %v", specTLS.CABundleSecretRef.Name, err)
					obj.Status.ObservedGeneration = obj.Generation
					_ = r.Status().Update(ctx, &obj)
					return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
				}
				caSecret = &sec
			} else if specTLS.Verify && usesLDAPS {
				obj.Status.Phase = "Error"
				obj.Status.Message = "tls.verify=true requires caBundleSecretRef for ldaps servers"
				obj.Status.ObservedGeneration = obj.Generation
				_ = r.Status().Update(ctx, &obj)
				return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
			}
		}
	}

	dirJSON, err := renderDirectoryJSON(&obj, dirType)
	if err != nil {
		obj.Status.Phase = "Error"
		obj.Status.Message = err.Error()
		obj.Status.ObservedGeneration = obj.Generation
		_ = r.Status().Update(ctx, &obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	smbConf, krb5Conf, err := renderSMBDirectoryConf(&obj, dirType)
	if err != nil {
		obj.Status.Phase = "Error"
		obj.Status.Message = err.Error()
		obj.Status.ObservedGeneration = obj.Generation
		_ = r.Status().Update(ctx, &obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	sssdConf, caBundle, err := renderSSSDConf(&obj, dirType, bindSecret, caSecret)
	if err != nil {
		obj.Status.Phase = "Error"
		obj.Status.Message = err.Error()
		obj.Status.ObservedGeneration = obj.Generation
		_ = r.Status().Update(ctx, &obj)
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	hash := directoryHash(dirJSON, smbConf, krb5Conf, sssdConf, caBundle, bindSecret, caSecret)

	ownerRef := *metav1.NewControllerRef(&obj, nasv1.GroupVersion.WithKind("NASDirectory"))
	ns := obj.Namespace

	smbData := map[string]string{
		"directory.json": dirJSON,
		"smb.conf":       smbConf,
	}
	if strings.TrimSpace(krb5Conf) != "" {
		smbData["krb5.conf"] = krb5Conf
	}
	smbCM := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            fmt.Sprintf("nasdirectory-%s-smb", obj.Name),
			Namespace:       ns,
			OwnerReferences: []metav1.OwnerReference{ownerRef},
			Annotations:     map[string]string{"nas.io/applied-hash": hash},
		},
		Data: smbData,
	}
	_ = upsert(ctx, r.Client, &smbCM)

	nfsCM := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            fmt.Sprintf("nasdirectory-%s-nfs", obj.Name),
			Namespace:       ns,
			OwnerReferences: []metav1.OwnerReference{ownerRef},
			Annotations:     map[string]string{"nas.io/applied-hash": hash},
		},
		Data: map[string]string{
			"directory.json": dirJSON,
		},
	}
	_ = upsert(ctx, r.Client, &nfsCM)

	if strings.TrimSpace(sssdConf) != "" {
		sssdSecret := corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:            fmt.Sprintf("nasdirectory-%s-nfs-sssd", obj.Name),
				Namespace:       ns,
				OwnerReferences: []metav1.OwnerReference{ownerRef},
				Annotations:     map[string]string{"nas.io/applied-hash": hash},
			},
			Type: corev1.SecretTypeOpaque,
			StringData: map[string]string{
				"sssd.conf": sssdConf,
			},
		}
		if len(caBundle) > 0 {
			sssdSecret.Data = map[string][]byte{"ca.crt": caBundle}
		}
		_ = upsert(ctx, r.Client, &sssdSecret)
	}

	obj.Status.Phase = "Ready"
	obj.Status.Message = "OK"
	obj.Status.AppliedHash = hash
	obj.Status.ObservedGeneration = obj.Generation
	_ = r.Status().Update(ctx, &obj)

	return ctrl.Result{RequeueAfter: 10 * time.Minute}, nil
}

func (r *NASDirectoryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nasv1.NASDirectory{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Watches(&corev1.Secret{}, handler.EnqueueRequestsFromMapFunc(
			func(ctx context.Context, obj client.Object) []reconcile.Request {
				sec, ok := obj.(*corev1.Secret)
				if !ok {
					return nil
				}
				var dirs nasv1.NASDirectoryList
				if err := r.List(ctx, &dirs, client.InNamespace(sec.Namespace)); err != nil {
					return nil
				}
				var out []reconcile.Request
				for i := range dirs.Items {
					dir := &dirs.Items[i]
					if directoryUsesSecret(dir, sec.Name) {
						out = append(out, reconcile.Request{
							NamespacedName: types.NamespacedName{
								Name:      dir.Name,
								Namespace: dir.Namespace,
							},
						})
					}
				}
				return out
			}),
		).
		Complete(r)
}

func normalizeDirectoryType(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "local", true
	}
	switch strings.ToLower(raw) {
	case "local":
		return "local", true
	case "ldap":
		return "ldap", true
	case "active-directory", "activedirectory", "ad":
		return "activeDirectory", true
	default:
		return "", false
	}
}

func validateDirectorySpec(spec nasv1.NASDirectorySpec, dirType string) ([]string, bool) {
	var errs []string
	usesLDAPS := false
	if dirType != "local" {
		if len(spec.Servers) == 0 {
			errs = append(errs, "servers required for non-local directory")
		}
		if strings.TrimSpace(spec.BaseDN) == "" {
			errs = append(errs, "baseDN required for non-local directory")
		}
		if spec.Bind == nil || spec.Bind.SecretRef == nil || strings.TrimSpace(spec.Bind.SecretRef.Name) == "" {
			errs = append(errs, "bind.secretRef required for non-local directory")
		}
		if spec.Bind != nil && strings.TrimSpace(spec.Bind.Username) == "" {
			errs = append(errs, "bind.username required for non-local directory")
		}
		for _, raw := range spec.Servers {
			raw = strings.TrimSpace(raw)
			if raw == "" {
				continue
			}
			parsed, err := url.Parse(raw)
			if err != nil || parsed.Scheme == "" || parsed.Host == "" {
				errs = append(errs, fmt.Sprintf("invalid server url: %s", raw))
				continue
			}
			switch strings.ToLower(parsed.Scheme) {
			case "ldap":
			case "ldaps":
				usesLDAPS = true
			default:
				errs = append(errs, fmt.Sprintf("unsupported server scheme: %s", parsed.Scheme))
			}
		}
	}
	return errs, usesLDAPS
}

func secretName(bind *nasv1.NASDirectoryBind) string {
	if bind == nil || bind.SecretRef == nil {
		return ""
	}
	return strings.TrimSpace(bind.SecretRef.Name)
}

func renderDirectoryJSON(dir *nasv1.NASDirectory, dirType string) (string, error) {
	spec := dir.Spec
	out := struct {
		Type      string   `json:"type"`
		Servers   []string `json:"servers,omitempty"`
		BaseDN    string   `json:"baseDN,omitempty"`
		Realm     string   `json:"realm,omitempty"`
		Workgroup string   `json:"workgroup,omitempty"`
		Bind      *struct {
			Username   string `json:"username,omitempty"`
			SecretName string `json:"secretName,omitempty"`
		} `json:"bind,omitempty"`
		TLS *struct {
			CABundleSecretName string `json:"caBundleSecretName,omitempty"`
			Verify             bool   `json:"verify,omitempty"`
		} `json:"tls,omitempty"`
		IDMapping       *nasv1.NASDirectoryIDMapping       `json:"idMapping,omitempty"`
		GroupResolution *nasv1.NASDirectoryGroupResolution `json:"groupResolution,omitempty"`
		Local           *nasv1.NASDirectoryLocal           `json:"local,omitempty"`
	}{
		Type:            dirType,
		Servers:         spec.Servers,
		BaseDN:          spec.BaseDN,
		Realm:           spec.Realm,
		Workgroup:       spec.Workgroup,
		IDMapping:       spec.IDMapping,
		GroupResolution: spec.GroupResolution,
		Local:           spec.Local,
	}
	if spec.Bind != nil {
		out.Bind = &struct {
			Username   string `json:"username,omitempty"`
			SecretName string `json:"secretName,omitempty"`
		}{
			Username:   strings.TrimSpace(spec.Bind.Username),
			SecretName: secretName(spec.Bind),
		}
	}
	if spec.TLS != nil {
		name := ""
		if spec.TLS.CABundleSecretRef != nil {
			name = strings.TrimSpace(spec.TLS.CABundleSecretRef.Name)
		}
		out.TLS = &struct {
			CABundleSecretName string `json:"caBundleSecretName,omitempty"`
			Verify             bool   `json:"verify,omitempty"`
		}{
			CABundleSecretName: name,
			Verify:             spec.TLS.Verify,
		}
	}
	raw, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return "", err
	}
	return string(raw) + "\n", nil
}

func directoryHash(dirJSON, smbConf, krb5Conf, sssdConf string, caBundle []byte, bindSecret, caSecret *corev1.Secret) string {
	h := sha256.New()
	h.Write([]byte(dirJSON))
	h.Write([]byte(smbConf))
	h.Write([]byte(krb5Conf))
	h.Write([]byte(sssdConf))
	if len(caBundle) > 0 {
		h.Write(caBundle)
	}
	if bindSecret != nil {
		h.Write([]byte(bindSecret.ResourceVersion))
	}
	if caSecret != nil {
		h.Write([]byte(caSecret.ResourceVersion))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func directoryUsesSecret(dir *nasv1.NASDirectory, secretName string) bool {
	if secretName == "" {
		return false
	}
	if dir.Spec.Bind != nil && dir.Spec.Bind.SecretRef != nil {
		if strings.TrimSpace(dir.Spec.Bind.SecretRef.Name) == secretName {
			return true
		}
	}
	if dir.Spec.TLS != nil && dir.Spec.TLS.CABundleSecretRef != nil {
		if strings.TrimSpace(dir.Spec.TLS.CABundleSecretRef.Name) == secretName {
			return true
		}
	}
	return false
}

func renderSMBDirectoryConf(dir *nasv1.NASDirectory, dirType string) (string, string, error) {
	if dirType != "activeDirectory" {
		return "# directory: local/ldap (no SMB settings)\n", "", nil
	}

	realm, workgroup, domain, err := deriveADNames(dir.Spec)
	if err != nil {
		return "", "", err
	}
	kdcHost := firstServerHost(dir.Spec.Servers)
	if kdcHost == "" {
		return "", "", fmt.Errorf("activeDirectory requires at least one server host")
	}

	var lines []string
	lines = append(lines,
		"[global]",
		"  security = ads",
		fmt.Sprintf("  realm = %s", realm),
		fmt.Sprintf("  workgroup = %s", workgroup),
		"  kerberos method = secrets and keytab",
		"  winbind use default domain = yes",
		"  winbind refresh tickets = yes",
		"  winbind offline logon = yes",
		"  template shell = /bin/bash",
		"  template homedir = /home/%U",
	)

	start, end := idmapRange(dir.Spec.IDMapping)
	strategy := ""
	if dir.Spec.IDMapping != nil {
		strategy = dir.Spec.IDMapping.Strategy
	}
	if strings.EqualFold(strategy, "autorid") {
		lines = append(lines,
			"  idmap config * : backend = tdb",
			"  idmap config * : range = 3000-7999",
			fmt.Sprintf("  idmap config %s : backend = autorid", workgroup),
			fmt.Sprintf("  idmap config %s : range = %d-%d", workgroup, start, end),
		)
	} else {
		lines = append(lines,
			"  idmap config * : backend = tdb",
			"  idmap config * : range = 3000-7999",
			fmt.Sprintf("  idmap config %s : backend = ad", workgroup),
			fmt.Sprintf("  idmap config %s : schema_mode = rfc2307", workgroup),
			fmt.Sprintf("  idmap config %s : range = %d-%d", workgroup, start, end),
			"  winbind nss info = rfc2307",
		)
	}

	conf := strings.Join(lines, "\n") + "\n"

	krb5 := strings.Join([]string{
		"[libdefaults]",
		fmt.Sprintf("  default_realm = %s", realm),
		"  dns_lookup_realm = false",
		"  dns_lookup_kdc = true",
		"",
		"[realms]",
		fmt.Sprintf("%s = {", realm),
		fmt.Sprintf("  kdc = %s", kdcHost),
		fmt.Sprintf("  admin_server = %s", kdcHost),
		"}",
		"",
		"[domain_realm]",
		fmt.Sprintf("  .%s = %s", domain, realm),
		fmt.Sprintf("  %s = %s", domain, realm),
		"",
	}, "\n")

	return conf, krb5, nil
}

func renderSSSDConf(dir *nasv1.NASDirectory, dirType string, bindSecret, caSecret *corev1.Secret) (string, []byte, error) {
	if dirType == "local" {
		return "", nil, nil
	}

	if bindSecret == nil {
		return "", nil, fmt.Errorf("bind secret required for %s directory", dirType)
	}
	bindPass := secretValue(bindSecret, "password")
	if bindPass == "" {
		return "", nil, fmt.Errorf("bind secret missing password")
	}
	bindUser := ""
	if dir.Spec.Bind != nil {
		bindUser = strings.TrimSpace(dir.Spec.Bind.Username)
	}
	if bindUser == "" {
		return "", nil, fmt.Errorf("bind.username required for %s directory", dirType)
	}

	_, _, domain, err := deriveADNames(dir.Spec)
	if err != nil && strings.TrimSpace(dir.Spec.BaseDN) == "" {
		return "", nil, fmt.Errorf("baseDN or realm required to render sssd.conf")
	}
	if err != nil {
		domain = strings.ToLower(strings.TrimSpace(realmFromBaseDN(dir.Spec.BaseDN)))
	}
	if domain == "" {
		return "", nil, fmt.Errorf("unable to determine domain for sssd.conf")
	}

	uris := cleanServers(dir.Spec.Servers)
	uriLine := strings.Join(uris, ",")
	if uriLine == "" {
		return "", nil, fmt.Errorf("servers required for sssd.conf")
	}

	caBundle := caBundleBytes(caSecret)
	useTLS := len(caBundle) > 0

	lines := []string{
		"[sssd]",
		"services = nss, pam",
		fmt.Sprintf("domains = %s", domain),
		"",
		fmt.Sprintf("[domain/%s]", domain),
		"id_provider = ldap",
		"auth_provider = ldap",
		fmt.Sprintf("ldap_uri = %s", uriLine),
		fmt.Sprintf("ldap_search_base = %s", dir.Spec.BaseDN),
		fmt.Sprintf("ldap_default_bind_dn = %s", bindUser),
		fmt.Sprintf("ldap_default_authtok = %s", bindPass),
		"ldap_default_authtok_type = password",
		"cache_credentials = True",
		"enumerate = True",
	}

	strategy := ""
	if dir.Spec.IDMapping != nil {
		strategy = dir.Spec.IDMapping.Strategy
	}
	if strings.EqualFold(strategy, "rfc2307") || strategy == "" {
		lines = append(lines,
			"ldap_schema = rfc2307",
			"ldap_id_mapping = False",
		)
	}
	if dirType == "activeDirectory" {
		lines = append(lines, "ldap_referrals = False")
	}
	if useTLS {
		lines = append(lines,
			"ldap_tls_reqcert = demand",
			"ldap_tls_cacert = /etc/sssd/certs/ca.crt",
		)
	}

	return strings.Join(lines, "\n") + "\n", caBundle, nil
}

func deriveADNames(spec nasv1.NASDirectorySpec) (string, string, string, error) {
	realm := strings.TrimSpace(spec.Realm)
	if realm == "" {
		realm = realmFromBaseDN(spec.BaseDN)
	}
	if realm == "" {
		return "", "", "", fmt.Errorf("realm required for activeDirectory")
	}
	realm = strings.ToUpper(realm)

	workgroup := strings.TrimSpace(spec.Workgroup)
	if workgroup == "" {
		workgroup = workgroupFromRealm(realm)
	}
	if workgroup == "" {
		return "", "", "", fmt.Errorf("workgroup required for activeDirectory")
	}

	domain := strings.ToLower(realm)
	if strings.Contains(domain, " ") {
		domain = strings.ReplaceAll(domain, " ", "")
	}
	return realm, strings.ToUpper(workgroup), domain, nil
}

func realmFromBaseDN(baseDN string) string {
	parts := strings.Split(baseDN, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(strings.ToLower(p), "dc=") {
			out = append(out, strings.TrimSpace(p[3:]))
		}
	}
	if len(out) == 0 {
		return ""
	}
	return strings.ToUpper(strings.Join(out, "."))
}

func workgroupFromRealm(realm string) string {
	realm = strings.TrimSpace(realm)
	if realm == "" {
		return ""
	}
	parts := strings.Split(realm, ".")
	if len(parts) == 0 {
		return ""
	}
	return strings.ToUpper(parts[0])
}

func idmapRange(idmap *nasv1.NASDirectoryIDMapping) (int64, int64) {
	start := int64(10000)
	if idmap != nil {
		if idmap.UIDStart > 0 {
			start = idmap.UIDStart
		}
		if idmap.GIDStart > 0 && idmap.GIDStart < start {
			start = idmap.GIDStart
		}
	}
	end := start + 899999
	return start, end
}

func firstServerHost(servers []string) string {
	for _, raw := range servers {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" {
			continue
		}
		host := parsed.Hostname()
		if host != "" {
			return host
		}
	}
	return ""
}

func cleanServers(in []string) []string {
	var out []string
	for _, raw := range in {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		out = append(out, raw)
	}
	return out
}

func caBundleBytes(sec *corev1.Secret) []byte {
	if sec == nil {
		return nil
	}
	if b, ok := sec.Data["ca.crt"]; ok {
		return b
	}
	for _, v := range sec.Data {
		if len(v) > 0 {
			return v
		}
	}
	return nil
}

func secretValue(sec *corev1.Secret, key string) string {
	if sec == nil {
		return ""
	}
	if b, ok := sec.Data[key]; ok {
		return strings.TrimSpace(string(b))
	}
	return ""
}
