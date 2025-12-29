package controllers

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	nasv1 "mnemosyne/api/v1alpha1"
	"mnemosyne/internal/smbconf"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type smbUser struct {
	Username           string
	PasswordSecretName string
}

func resolveLocalUsers(ctx context.Context, c client.Client, ns string, directory string, sel nasv1.NASSharePrincipalSelector) ([]smbUser, []string, error) {
	userNames, err := resolveLocalUsernames(ctx, c, ns, directory, sel)
	if err != nil {
		return nil, nil, err
	}
	users := make([]smbUser, 0, len(userNames))
	seen := map[string]struct{}{}
	var smbNames []string
	for _, name := range userNames {
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		var u nasv1.NASUser
		if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, &u); err != nil {
			return nil, nil, fmt.Errorf("nasuser %s not found: %w", name, err)
		}
		if !directoryMatches(directory, u.Spec.DirectoryRef) {
			return nil, nil, fmt.Errorf("nasuser %s not in directory %s", name, directory)
		}
		username := strings.TrimSpace(u.Spec.Username)
		secName := strings.TrimSpace(u.Spec.PasswordSecretRef.Name)
		if username == "" || secName == "" {
			continue
		}
		users = append(users, smbUser{
			Username:           username,
			PasswordSecretName: secName,
		})
		smbNames = append(smbNames, username)
	}
	return users, uniqueStrings(smbNames), nil
}

func resolveLocalUsernames(ctx context.Context, c client.Client, ns string, directory string, sel nasv1.NASSharePrincipalSelector) ([]string, error) {
	var out []string
	for _, name := range sel.Users {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	for _, groupName := range sel.Groups {
		groupName = strings.TrimSpace(groupName)
		if groupName == "" {
			continue
		}
		var g nasv1.NASGroup
		if err := c.Get(ctx, client.ObjectKey{Namespace: ns, Name: groupName}, &g); err != nil {
			return nil, fmt.Errorf("nasgroup %s not found: %w", groupName, err)
		}
		if !directoryMatches(directory, g.Spec.DirectoryRef) {
			return nil, fmt.Errorf("nasgroup %s not in directory %s", groupName, directory)
		}
		for _, member := range g.Spec.Members {
			member = strings.TrimSpace(member)
			if member == "" {
				continue
			}
			out = append(out, member)
		}
	}
	return uniqueStrings(out), nil
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

func uniqueStrings(in []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func mergeSMBUsers(a, b []smbUser) []smbUser {
	out := []smbUser{}
	seen := map[string]struct{}{}
	for _, u := range append(a, b...) {
		if u.Username == "" {
			continue
		}
		if _, ok := seen[u.Username]; ok {
			continue
		}
		seen[u.Username] = struct{}{}
		out = append(out, u)
	}
	return out
}

func directoryMatches(expected, actual string) bool {
	expected = strings.TrimSpace(expected)
	actual = strings.TrimSpace(actual)
	if expected == "" {
		expected = "local"
	}
	if actual == "" {
		actual = "local"
	}
	return expected == actual
}
