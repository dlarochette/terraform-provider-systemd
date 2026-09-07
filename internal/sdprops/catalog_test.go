package sdprops

import (
	"regexp"
	"testing"
)

func TestCatalogSanity(t *testing.T) {
	sections := SectionNames()
	if len(sections) == 0 {
		t.Fatal("empty catalog")
	}
	attrRe := regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	seen := map[string]bool{}
	total := 0
	for _, sec := range sections {
		if seen[sec] {
			t.Fatalf("duplicate section %s", sec)
		}
		seen[sec] = true
		ds := Directives(sec)
		if len(ds) == 0 {
			t.Fatalf("section %s has no directives", sec)
		}
		used := map[string]bool{}
		for _, d := range ds {
			if !attrRe.MatchString(d.Attr) {
				t.Errorf("%s.%s: invalid attr name %q", sec, d.Name, d.Attr)
			}
			if used[d.Attr] {
				t.Errorf("%s: duplicate attr %q", sec, d.Attr)
			}
			used[d.Attr] = true
			// reserved names clash with resource attributes/blocks
			switch d.Attr {
			case "name", "content", "id", "enable", "active", "section":
				t.Errorf("%s: reserved attr name %q", sec, d.Attr)
			}
		}
	}
	total = 0
	for _, sec := range sections {
		total += len(Directives(sec))
	}
	if total < 1000 {
		t.Fatalf("catalog too small: %d directives", total)
	}
}

func TestSnakeSpotChecks(t *testing.T) {
	cases := map[string]string{
		"Description":          "description",
		"ExecStart":            "exec_start",
		"ExecStartPre":         "exec_start_pre",
		"CPUSchedulingPolicy":  "cpu_scheduling_policy",
		"AllowedCPUs":          "allowed_cpus",
		"StartupAllowedCPUs":   "startup_allowed_cpus",
		"UMask":                "umask",
		"TimerSlackNSec":       "timer_slack_nsec",
		"MemoryZSwapMax":       "memory_zswap_max",
		"BindIPv6Only":         "bind_ipv6_only",
		"SendSIGKILL":          "send_sigkill",
		"LimitNOFILE":          "limit_nofile",
		"IOReadIOPSMax":        "io_read_iops_max",
		"PrivatePIDs":          "private_pids",
		"WantedBy":             "wanted_by",
		"OnCalendar":           "on_calendar",
		"IOSchedulingClass":    "io_scheduling_class",
		"SELinuxContext":       "selinux_context",
		"ConditionACPower":     "condition_ac_power",
		"MemoryZSwapWriteback": "memory_zswap_writeback",
		"SysVStartPriority":    "sysv_start_priority",
		"SmackLabelIPIn":       "smack_label_ip_in",
		"ListenUSBFunction":    "listen_usb_function",
		"AssertOSRelease":      "assert_os_release",
		"RootImagePolicy":      "root_image_policy",
		"OOMScoreAdjust":       "oom_score_adjust",
		"CPUQuotaPeriodSec":    "cpu_quota_period_sec",
		"IPAddressDeny":        "ip_address_deny",
		"MemoryKSM":            "memory_ksm",
		"UtmpMode":             "utmp_mode",
		"BlockIODeviceWeight":  "block_io_device_weight",
		"DefaultDependencies":  "default_dependencies",
	}
	dirs := Directives("Unit")
	lookup := func(name string) (string, bool) {
		for _, d := range dirs {
			if d.Name == name {
				return d.Attr, true
			}
		}
		return "", false
	}
	for name, want := range cases {
		got, ok := lookup(name)
		if !ok {
			// not every name lives in [Unit]; find it anywhere
			found := false
			for _, sec := range SectionNames() {
				if a, ok2 := func() (string, bool) {
					for _, d := range Directives(sec) {
						if d.Name == name {
							return d.Attr, true
						}
					}
					return "", false
				}(); ok2 {
					got, found = a, true
					break
				}
			}
			if !found {
				t.Fatalf("%s not in catalog", name)
			}
		}
		if got != want {
			t.Errorf("%s: attr = %q, want %q", name, got, want)
		}
	}
}

func TestSectionsForUnitType(t *testing.T) {
	if got := SectionsForUnitType(".timer"); len(got) != 3 || got[2] != "Timer" {
		t.Errorf("timer sections = %v", got)
	}
	if got := SectionsForUnitType(".slice"); got[2] != "Slice" {
		t.Errorf("slice sections = %v", got)
	}
	all := SectionsForUnitType(".unknown")
	if len(all) != len(UnitSections()) {
		t.Errorf("unknown suffix should return all sections")
	}
}

func TestEnumOf(t *testing.T) {
	if v := EnumOf("enum:service_restart"); v == nil || v[0] != "no" {
		t.Errorf("service_restart enum = %v", v)
	}
	if EnumOf("bool") != nil {
		t.Error("bool must not be an enum")
	}
}

func TestSinceVersion(t *testing.T) {
	// sanity: a few directives and their introduction version
	type tc struct {
		section, name string
		min, max      int
	}
	cases := []tc{
		{"Service", "ExecStart", 249, 249},
	}
	// every directive must be available in the latest version
	for _, sec := range SectionNames() {
		for _, d := range Directives(sec) {
			if SinceVersion(sec, d.Name) > LatestVersion {
				t.Errorf("%s.%s: since %d > latest %d", sec, d.Name, SinceVersion(sec, d.Name), LatestVersion)
			}
		}
	}
	// ExecStart exists in every bundled version
	for _, cat := range UnitCatalogs {
		found := false
		for _, d := range DirectivesIn(&cat, "Service") {
			if d.Name == "ExecStart" {
				found = true
			}
		}
		if !found {
			t.Errorf("v%d: ExecStart missing", cat.Version)
		}
	}
	_ = cases
}

func TestCatalogFor(t *testing.T) {
	if CatalogFor(250).Version != 250 {
		t.Error("250 should select v250")
	}
	if CatalogFor(251).Version != 251 {
		t.Error("251 should select v251")
	}
	if CatalogFor(400).Version != LatestVersion {
		t.Error("400 should clamp to latest")
	}
	if CatalogFor(100).Version != MinVersion() {
		t.Error("below min should clamp to oldest")
	}
	if CatalogFor(256).Version != 256 {
		t.Error("256 should select v256")
	}
}
