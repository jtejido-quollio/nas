package smbconf

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

type SnapshotExposure struct {
	Enabled   bool
	Mode      string
	Format    string
	LocalTime *bool
}

type TimeMachine struct {
	Enabled                bool
	VolumeSizeLimitBytes   *int64
	AdvertiseAsTimeMachine *bool
}

type Options struct {
	MacOSCompat *bool
	Encryption  *string

	Browseable *bool
	GuestOk    *bool
	ValidUsers []string
	WriteList  []string

	CreateMask    *string
	DirectoryMask *string
	InheritPerms  *bool

	SnapshotExposure *SnapshotExposure
	TimeMachine      *TimeMachine
}

var (
	maskRe = regexp.MustCompile(`^[0-7]{3,4}$`)
)

func Render(shareName, path string, readOnly bool, o Options) (string, error) {
	// conservative validation
	if o.CreateMask != nil && !maskRe.MatchString(*o.CreateMask) {
		return "", fmt.Errorf("invalid createMask: %q", *o.CreateMask)
	}
	if o.DirectoryMask != nil && !maskRe.MatchString(*o.DirectoryMask) {
		return "", fmt.Errorf("invalid directoryMask: %q", *o.DirectoryMask)
	}

	global := `[global]
  server role = standalone server
  map to guest = never
  disable netbios = yes
  smb ports = 445
  log level = 1
  load printers = no
  printing = bsd
  printcap name = /dev/null
  disable spoolss = yes
`

	yesno := func(b bool) string {
		if b {
			return "yes"
		}
		return "no"
	}

	browseable := "yes"
	if o.Browseable != nil {
		browseable = yesno(*o.Browseable)
	}
	guestOk := "no"
	if o.GuestOk != nil {
		guestOk = yesno(*o.GuestOk)
	}

	createMask := "0664"
	if o.CreateMask != nil {
		createMask = *o.CreateMask
	}
	dirMask := "0775"
	if o.DirectoryMask != nil {
		dirMask = *o.DirectoryMask
	}

	vfs := []string{}
	if o.MacOSCompat != nil && *o.MacOSCompat {
		vfs = append(vfs, "fruit", "catia", "streams_xattr")
	}
	if o.SnapshotExposure != nil && o.SnapshotExposure.Enabled {
		vfs = append(vfs, "shadow_copy2")
	}
	if o.TimeMachine != nil && o.TimeMachine.Enabled {
		// ensure fruit
		found := false
		for _, x := range vfs {
			if x == "fruit" {
				found = true
				break
			}
		}
		if !found {
			vfs = append(vfs, "fruit")
		}
	}
	vfs = uniqStable(vfs)

	var vfsLine string
	if len(vfs) > 0 {
		vfsLine = fmt.Sprintf("  vfs objects = %s", strings.Join(vfs, " "))
	}

	var fruitLines string
	if slices.Contains(vfs, "fruit") {
		fruitLines = "" + "  fruit:metadata = stream" + "  fruit:resource = xattr" + "  fruit:posix_rename = yes"
	}

	var shadowLines string
	if o.SnapshotExposure != nil && o.SnapshotExposure.Enabled {
		localTime := "yes"
		if o.SnapshotExposure.LocalTime != nil && !*o.SnapshotExposure.LocalTime {
			localTime = "no"
		}
		shadowLines = "  shadow:snapdir = .zfs/snapshot" +
			fmt.Sprintf("  shadow:format = %s", o.SnapshotExposure.Format) + "  shadow:localtime = " + localTime + " shadow:sort = desc"
	}

	var tmLines string
	if o.TimeMachine != nil && o.TimeMachine.Enabled {
		adv := true
		if o.TimeMachine.AdvertiseAsTimeMachine != nil {
			adv = *o.TimeMachine.AdvertiseAsTimeMachine
		}
		if adv {
			tmLines += "  fruit:time machine = yes"
		}
		if o.TimeMachine.VolumeSizeLimitBytes != nil {
			tmLines += fmt.Sprintf("  fruit:time machine max size = %d", *o.TimeMachine.VolumeSizeLimitBytes)
		}
		tmLines += "  ea support = yes inherit acls = yes"
	}

	encLine := ""
	if o.Encryption != nil {
		switch strings.ToLower(strings.TrimSpace(*o.Encryption)) {
		case "disabled":
			encLine = "  smb encrypt = off"
		case "desired":
			encLine = "  smb encrypt = desired"
		case "required":
			encLine = "  smb encrypt = required"
		}
	}

	validUsers := ""
	if len(o.ValidUsers) > 0 {
		validUsers = fmt.Sprintf("  valid users = %s", strings.Join(o.ValidUsers, " "))
	}
	writeList := ""
	if len(o.WriteList) > 0 {
		writeList = fmt.Sprintf("  write list = %s", strings.Join(o.WriteList, " "))
	}

	inheritPerms := ""
	if o.InheritPerms != nil {
		inheritPerms = fmt.Sprintf("  inherit permissions = %s", yesno(*o.InheritPerms))
	}

	share := fmt.Sprintf(`
[%s]
  path = %s
  browseable = %s
  guest ok = %s
  read only = %s
  create mask = %s
  directory mask = %s
%s%s%s%s%s%s%s%s`,
		shareName,
		path,
		browseable,
		guestOk,
		yesno(readOnly),
		createMask,
		dirMask,
		inheritPerms,
		vfsLine,
		fruitLines,
		shadowLines,
		tmLines,
		encLine,
		validUsers,
		writeList,
	)
	return global + share, nil
}

func uniqStable(in []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, x := range in {
		x = strings.TrimSpace(x)
		if x == "" {
			continue
		}
		if _, ok := seen[x]; ok {
			continue
		}
		seen[x] = struct{}{}
		out = append(out, x)
	}
	return out
}
