// Package sdprops exposes the systemd unit directive catalog generated from
// the systemd load-fragment gperf table (see cmd/genprops).
//
// Regenerate the catalog with:
//
//	go generate ./internal/sdprops
package sdprops

//go:generate go run ../../cmd/genprops -out catalog_gen.go data/link-config-v249.gperf data/link-config-v250.gperf data/link-config-v251.gperf data/link-config-v252.gperf data/link-config-v253.gperf data/link-config-v254.gperf data/link-config-v255.gperf data/link-config-v256.gperf data/link-config-v257.gperf data/resolved-gperf-v249.gperf data/resolved-gperf-v250.gperf data/resolved-gperf-v251.gperf data/resolved-gperf-v252.gperf data/resolved-gperf-v253.gperf data/resolved-gperf-v254.gperf data/resolved-gperf-v255.gperf data/resolved-gperf-v256.gperf data/resolved-gperf-v257.gperf data/load-fragment-v249.gperf data/load-fragment-v250.gperf data/load-fragment-v251.gperf data/load-fragment-v252.gperf data/load-fragment-v253.gperf data/load-fragment-v254.gperf data/load-fragment-v255.gperf data/load-fragment-v256.gperf data/load-fragment-v257.gperf data/netdev-v249.gperf data/netdev-v250.gperf data/netdev-v251.gperf data/netdev-v252.gperf data/netdev-v253.gperf data/netdev-v254.gperf data/netdev-v255.gperf data/netdev-v256.gperf data/netdev-v257.gperf data/networkd-network-v249.gperf data/networkd-network-v250.gperf data/networkd-network-v251.gperf data/networkd-network-v252.gperf data/networkd-network-v253.gperf data/networkd-network-v254.gperf data/networkd-network-v255.gperf data/networkd-network-v256.gperf data/networkd-network-v257.gperf

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
	"resolve_support":              {"yes", "no", "resolve"},
	"dnssec_mode":                  {"yes", "no", "allow-downgrade"},
	"dns_over_tls_mode":            {"opportunistic", "no", "yes"},
	"dns_cache_mode":               {"no", "no-negative", "yes"},
	"dns_stub_listener_mode":       {"no", "yes", "udp", "tcp"},
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

// Directive is a single systemd unit directive.
type Directive struct {
	Name  string // directive name as written in unit files (e.g. ExecStart)
	Attr  string // HCL attribute name (snake_case)
	Class string // value class driving type and validation
}

// SectionGroup holds the directives of one section. Attr is the HCL block
// name for the section (snake_case).
type SectionGroup struct {
	Section    string
	Attr       string
	Directives []Directive
}

// kindCatalogs maps a catalog kind to its per-release lists.
var kindCatalogs = map[string][]VersionCatalog{
	"unit":     UnitCatalogs,
	"network":  NetworkCatalogs,
	"netdev":   NetdevCatalogs,
	"link":     LinkCatalogs,
	"resolved": ResolvedCatalogs,
}

// latest returns the newest bundled catalog of a kind.
func latest(kind string) *VersionCatalog {
	cats := kindCatalogs[kind]
	if len(cats) == 0 {
		return nil
	}
	return &cats[len(cats)-1]
}

func sectionsOf(cat *VersionCatalog) []SectionGroup {
	if cat == nil {
		return nil
	}
	return cat.Sections
}

// MinVersion is the oldest systemd release bundled in the unit catalog.
func MinVersion() int {
	if len(UnitCatalogs) == 0 {
		return 0
	}
	return UnitCatalogs[0].Version
}

// CatalogFor returns the newest unit catalog whose release is <= version.
// For a version below the oldest bundled release, the oldest catalog is
// returned (conservative: directives absent from it are rejected).
func CatalogFor(version int) *VersionCatalog {
	return NetCatalogFor("unit", version)
}

// NetCatalogFor returns the newest catalog of the given kind whose release
// is <= version. For a version below the oldest bundled release, the
// oldest catalog is returned (conservative).
func NetCatalogFor(kind string, version int) *VersionCatalog {
	cats := kindCatalogs[kind]
	var best *VersionCatalog
	for i := range cats {
		if cats[i].Version <= version {
			best = &cats[i]
		}
	}
	if best == nil && len(cats) > 0 {
		best = &cats[0]
	}
	return best
}

// DirectivesFor returns the catalog directives of one section for a
// specific systemd release.
func DirectivesFor(version int, section string) []Directive {
	return DirectivesIn(CatalogFor(version), section)
}

// DirectivesIn returns the catalog directives of one section in the given
// catalog.
func DirectivesIn(cat *VersionCatalog, section string) []Directive {
	if cat == nil {
		return nil
	}
	for _, g := range cat.Sections {
		if g.Section == section {
			return g.Directives
		}
	}
	return nil
}

// SinceVersion returns the first bundled systemd release providing the
// directive in the unit catalog (fallback: LatestVersion).
func SinceVersion(section, name string) int {
	return NetSinceVersion("unit", section, name)
}

// NetSinceVersion returns the first bundled release providing the
// directive in the given catalog kind.
func NetSinceVersion(kind, section, name string) int {
	for _, cat := range kindCatalogs[kind] {
		for _, d := range DirectivesIn(&cat, section) {
			if d.Name == name {
				return cat.Version
			}
		}
	}
	return LatestVersion
}

// SectionNames returns the catalog sections (latest bundled version) in
// order.
func SectionNames() []string {
	var out []string
	for _, g := range sectionsOf(latest("unit")) {
		out = append(out, g.Section)
	}
	return out
}

// Directives returns the catalog directives of one section (latest
// bundled version).
func Directives(section string) []Directive {
	return DirectivesIn(latest("unit"), section)
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
	for _, g := range sectionsOf(latest("unit")) {
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

// NetSections returns the latest sections of a net catalog kind
// ("network", "netdev" or "link").
func NetSections(kind string) []SectionGroup {
	return sectionsOf(latest(kind))
}

// NetDirectives returns the latest directives of one section of a net kind.
func NetDirectives(kind, section string) []Directive {
	return DirectivesIn(latest(kind), section)
}

// NetSectionNames returns the latest section names of a net kind.
func NetSectionNames(kind string) []string {
	var out []string
	for _, g := range NetSections(kind) {
		out = append(out, g.Section)
	}
	return out
}

// repeatableSections lists the sections that may appear several times in a
// net file (each occurrence becomes one block instance / file section).
var repeatableSections = map[string][]string{
	"network": {
		"Address", "Route", "Neighbor", "IPv6AddressLabel", "IPv6PREF64Prefix",
		"IPv6Prefix", "IPv6RoutePrefix", "RoutingPolicyRule", "NextHop",
		"BridgeFDB", "BridgeMDB", "BridgeVLAN", "DHCPServerStaticLease",
		"Token",
		// traffic control: one [QDisc] per qdisc may be repeated
		"QDisc", "NetworkEmulator", "TokenBucketFilter", "ControlledDelay",
		"DeficitRoundRobinScheduler", "DeficitRoundRobinSchedulerClass",
		"EnhancedTransmissionSelection", "FairQueueing",
		"FairQueueingControlledDelay", "FlowQueuePIE",
		"GenericRandomEarlyDetection", "HeavyHitterFilter",
		"HierarchyTokenBucket", "HierarchyTokenBucketClass", "PFIFO",
		"PFIFOFast", "PFIFOHeadDrop", "BFIFO", "PIE", "QuickFairQueueing",
		"QuickFairQueueingClass", "StochasticFairBlue",
		"StochasticFairnessQueueing", "TrivialLinkEqualizer",
		"BandMultiQueueing", "ClassfulMultiQueueing", "CAKE",
		"TrafficControlQueueingDiscipline",
	},
	"netdev": {
		"WireGuardPeer", "L2TPSession", "MACsecReceiveAssociation",
		"MACsecTransmitAssociation", "MACsecReceiveChannel", "Peer", "Xfrm",
	},
	"link":     {},
	"resolved": {},
}

// RepeatableSections returns the set of repeatable section names of a kind.
func RepeatableSections(kind string) map[string]bool {
	out := map[string]bool{}
	for _, s := range repeatableSections[kind] {
		out[s] = true
	}
	return out
}
