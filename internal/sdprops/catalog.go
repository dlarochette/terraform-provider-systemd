// Package sdprops exposes the systemd unit directive catalog generated from
// the systemd load-fragment gperf table (see cmd/genprops).
//
// Regenerate the catalog with:
//
//	go generate ./internal/sdprops
package sdprops

//go:generate go run ../../cmd/genprops -in data/load-fragment-v257.gperf -out catalog_gen.go -version v257

import "strings"

// Value classes.
const (
	ClassBool      = "bool"
	ClassInt       = "int"
	ClassTimespan  = "timespan"
	ClassSize      = "size"
	ClassMode      = "mode"
	ClassSignal    = "signal"
	ClassRlimit    = "rlimit"
	ClassWeight    = "weight"
	ClassBlkWeight = "blockio_weight"
	ClassString    = "string"
	ClassList      = "list"
	// "enum:<name>" — values in EnumValues.
)

// EnumValues holds the accepted values for enum-class directives, mirroring
// the systemd parsers.
var EnumValues = map[string][]string{
	"service_type":                 {"simple", "forking", "exec", "dbus", "notify", "notify-reload", "oneshot"},
	"service_restart":              {"no", "on-success", "on-failure", "on-abnormal", "on-watchdog", "on-abort", "always"},
	"service_restart_mode":         {"direct", "manual"},
	"service_exit_type":            {"main", "group"},
	"service_timeout_failure_mode": {"terminate", "abort", "kill"},
	"kill_mode":                    {"control-group", "process", "mixed", "none"},
	"notify_access":                {"none", "main", "exec", "all"},
	"collect_mode":                 {"inactive", "inactive-or-failed"},
	"oom_policy":                   {"stop", "kill", "continue"},
	"emergency_action":             {"none", "reboot", "reboot-force", "reboot-immediate", "poweroff", "poweroff-force", "poweroff-immediate", "exit", "exit-force"},
	"job_mode":                     {"fail", "replace", "replace-irreversibly", "isolate", "ignore-dependencies", "ignore-requirements", "flush"},
	"device_policy":                {"auto", "closed", "strict"},
	"managed_oom_mode":             {"auto", "kill"},
	"managed_oom_preference":       {"none", "prefer", "avoid"},
	"socket_bind":                  {"default", "both", "ipv6-only"},
	"socket_protocol":              {"sctp", "udp", "udplite", "tcp"},
	"protect_system":               {"no", "yes", "full", "strict"},
	"protect_home":                 {"no", "yes", "read-only", "tmpfs"},
	"protect_proc":                 {"no", "invisible", "ptraceable", "strict"},
	"protect_control_groups":       {"no", "yes", "read-only"},
	"proc_subset":                  {"all", "pid"},
	"mount_propagation_flag":       {"shared", "slave", "private", "rshared", "rslave", "rprivate", "unbindable", "runbindable", "rrunbindable"},
	"keyring_mode":                 {"inherit", "private", "shared"},
	"exec_utmp_mode":               {"init", "login", "user"},
}

// enumPrefix is the class prefix for enum classes.
const enumPrefix = "enum:"

// EnumClass returns the enum name for a class, or "" when the class is not
// an enum.
func EnumClass(class string) string {
	if strings.HasPrefix(class, enumPrefix) {
		return strings.TrimPrefix(class, enumPrefix)
	}
	return ""
}

// EnumOf returns the accepted values for an enum class, or nil.
func EnumOf(class string) []string {
	name := EnumClass(class)
	if name == "" {
		return nil
	}
	return EnumValues[name]
}

type group struct {
	Section    string
	Directives []Directive
}

// SectionNames returns the catalog sections in order.
func SectionNames() []string {
	out := make([]string, 0, len(SectionGroups))
	for _, g := range SectionGroups {
		out = append(out, g.Section)
	}
	return out
}

// Directives returns the catalog directives of one section.
func Directives(section string) []Directive {
	for _, g := range SectionGroups {
		if g.Section == section {
			return g.Directives
		}
	}
	return nil
}

// DirectiveNames returns the directive names of one section.
func DirectiveNames(section string) []string {
	ds := Directives(section)
	out := make([]string, 0, len(ds))
	for _, d := range ds {
		out = append(out, d.Name)
	}
	return out
}

// UnitSections lists every section usable in a unit file, in documentation
// order. [Scope] is excluded: scope units cannot be written as unit files.
func UnitSections() []string {
	var out []string
	for _, g := range SectionGroups {
		if g.Section != "Scope" {
			out = append(out, g.Section)
		}
	}
	return out
}

// unitTypeSection maps a unit file suffix to its dedicated section.
var unitTypeSection = map[string]string{
	".service":   "Service",
	".socket":    "Socket",
	".mount":     "Mount",
	".automount": "Automount",
	".swap":      "Swap",
	".timer":     "Timer",
	".path":      "Path",
	".slice":     "Slice",
	".target":    "Target",
}

// SectionsForUnitType returns the sections a typed unit resource accepts:
// the common [Unit]/[Install] sections plus the section dedicated to the
// unit type.
func SectionsForUnitType(suffix string) []string {
	sec, ok := unitTypeSection[suffix]
	if !ok {
		return UnitSections()
	}
	return []string{"Unit", "Install", sec}
}
