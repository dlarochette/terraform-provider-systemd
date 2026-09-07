// Command genprops regenerates internal/sdprops/catalog_gen.go from the
// systemd load-fragment gperf table.
//
//	go generate ./internal/sdprops
//	go run ./cmd/genprops -in internal/sdprops/data/load-fragment-v257.gperf -out internal/sdprops/catalog_gen.go
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type directive struct {
	Section string
	Name    string
	Attr    string
	Class   string
}

// fnClass maps systemd config_parse_* functions to a property class.
// Unlisted functions default to "string".
var fnClass = map[string]string{
	// booleans
	"config_parse_bool":     "bool",
	"config_parse_tristate": "bool",
	// integers
	"config_parse_int":                   "int",
	"config_parse_long":                  "int",
	"config_parse_unsigned":              "int",
	"config_parse_swap_priority":         "int",
	"config_parse_exec_nice":             "int",
	"config_parse_exec_oom_score_adjust": "int",
	"config_parse_exec_io_priority":      "int",
	"config_parse_exec_cpu_sched_prio":   "int",
	"config_parse_tty_size":              "int",
	"config_parse_cpu_shares":            "int",
	// time spans
	"config_parse_sec":                                   "timespan",
	"config_parse_sec_fix_0":                             "timespan",
	"config_parse_sec_def_infinity":                      "timespan",
	"config_parse_nsec":                                  "timespan",
	"config_parse_service_timeout":                       "timespan",
	"config_parse_service_timeout_abort":                 "timespan",
	"config_parse_job_timeout_sec":                       "timespan",
	"config_parse_job_running_timeout_sec":               "timespan",
	"config_parse_managed_oom_mem_pressure_duration_sec": "timespan",
	// byte sizes / memory limits
	"config_parse_memory_limit":      "size",
	"config_parse_io_limit":          "size",
	"config_parse_iec_size":          "size",
	"config_parse_blockio_bandwidth": "size",
	"config_parse_tasks_max":         "size",
	// octal modes
	"config_parse_mode": "mode",
	// unix signals
	"config_parse_signal": "signal",
	// resource limits (soft:hard)
	"config_parse_rlimit": "rlimit",
	// lists (space separated or repeatable)
	"config_parse_exec":                        "list",
	"config_parse_exec_directories":            "list",
	"config_parse_exec_secure_bits":            "list",
	"config_parse_environ":                     "list",
	"config_parse_pass_environ":                "list",
	"config_parse_unset_environ":               "list",
	"config_parse_unit_env_file":               "list",
	"config_parse_namespace_path_strv":         "list",
	"config_parse_bind_paths":                  "list",
	"config_parse_temporary_filesystems":       "list",
	"config_parse_mount_images":                "list",
	"config_parse_extension_images":            "list",
	"config_parse_root_image_options":          "list",
	"config_parse_import_credential":           "list",
	"config_parse_load_credential":             "list",
	"config_parse_set_credential":              "list",
	"config_parse_user_group_strv_compat":      "list",
	"config_parse_colon_separated_paths":       "list",
	"config_parse_unit_deps":                   "list",
	"config_parse_obsolete_unit_deps":          "list",
	"config_parse_unit_mounts_for":             "list",
	"config_parse_unit_condition_string":       "list",
	"config_parse_unit_condition_path":         "list",
	"config_parse_documentation":               "list",
	"config_parse_socket_listen":               "list",
	"config_parse_timer":                       "list",
	"config_parse_open_file":                   "list",
	"config_parse_log_extra_fields":            "list",
	"config_parse_log_filter_patterns":         "list",
	"config_parse_capability_set":              "list",
	"config_parse_address_families":            "list",
	"config_parse_syscall_filter":              "list",
	"config_parse_syscall_log":                 "list",
	"config_parse_syscall_archs":               "list",
	"config_parse_in_addr_prefixes":            "list",
	"config_parse_ip_filter_bpf_progs":         "list",
	"config_parse_cgroup_socket_bind":          "list",
	"config_parse_cgroup_nft_set":              "list",
	"config_parse_bpf_foreign_program":         "list",
	"config_parse_device_allow":                "list",
	"config_parse_disable_controllers":         "list",
	"config_parse_restrict_network_interfaces": "list",
	"config_parse_blockio_device_weight":       "list",
	"config_parse_io_device_weight":            "list",
	"config_parse_io_device_latency":           "list",
	// enum strings
	"config_parse_service_type":                 "enum:service_type",
	"config_parse_service_restart":              "enum:service_restart",
	"config_parse_service_restart_mode":         "enum:service_restart_mode",
	"config_parse_service_exit_type":            "enum:service_exit_type",
	"config_parse_service_timeout_failure_mode": "enum:service_timeout_failure_mode",
	"config_parse_kill_mode":                    "enum:kill_mode",
	"config_parse_notify_access":                "enum:notify_access",
	"config_parse_collect_mode":                 "enum:collect_mode",
	"config_parse_oom_policy":                   "enum:oom_policy",
	"config_parse_emergency_action":             "enum:emergency_action",
	"config_parse_job_mode":                     "enum:job_mode",
	"config_parse_job_mode_isolate":             "enum:job_mode",
	"config_parse_device_policy":                "enum:device_policy",
	"config_parse_managed_oom_mode":             "enum:managed_oom_mode",
	"config_parse_managed_oom_preference":       "enum:managed_oom_preference",
	"config_parse_socket_bind":                  "enum:socket_bind",
	"config_parse_socket_protocol":              "enum:socket_protocol",
	"config_parse_protect_system":               "enum:protect_system",
	"config_parse_protect_home":                 "enum:protect_home",
	"config_parse_protect_proc":                 "enum:protect_proc",
	"config_parse_protect_control_groups":       "enum:protect_control_groups",
	"config_parse_proc_subset":                  "enum:proc_subset",
	"config_parse_exec_mount_propagation_flag":  "enum:mount_propagation_flag",
	"config_parse_exec_keyring_mode":            "enum:keyring_mode",
	"config_parse_exec_utmp_mode":               "enum:exec_utmp_mode",
	// weight strings (allow "max" / "idle")
	"config_parse_cg_cpu_weight":  "weight",
	"config_parse_cg_weight":      "weight",
	"config_parse_blockio_weight": "blockio_weight",
	// special strings
	"config_parse_private_tmp":           "string",
	"config_parse_private_users":         "string",
	"config_parse_delegate":              "string",
	"config_parse_memory_pressure_watch": "string",
	"config_parse_exec_preserve_mode":    "string",
}

var enumValues = map[string][]string{
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

// acronymTokens are CamelCase tokens preserved as single words during
// snake_case conversion. Longer entries win over shorter ones.
var acronymTokens = map[string]string{
	"AC":    "ac",
	"CPU":   "cpu",
	"CPUs":  "cpus",
	"NUMA":  "numa",
	"OOM":   "oom",
	"PID":   "pid",
	"PIDs":  "pids",
	"TTY":   "tty",
	"USB":   "usb",
	"IO":    "io",
	"IOPS":  "iops",
	"IP":    "ip",
	"IPv6":  "ipv6",
	"OS":    "os",
	"FIFO":  "fifo",
	"BPF":   "bpf",
	"IPC":   "ipc",
	"TCP":   "tcp",
	"NFT":   "nft",
	"PAM":   "pam",
	"ZSwap": "zswap",
	"SysV":  "sysv",
	"NSec":  "nsec",
}

// preNormalize fixes names the generic scanner cannot handle.
var preNormalize = []struct{ from, to string }{
	{"IOScheduling", "IoScheduling"},
	{"UMask", "Umask"},
	{"SELinux", "Selinux"},
}

func snake(name string) string {
	name = strings.ReplaceAll(name, "-", "")
	for _, r := range preNormalize {
		name = strings.ReplaceAll(name, r.from, r.to)
	}
	var words []string
	i := 0
	for i < len(name) {
		if match, tok := longestAcronym(name, i); tok {
			words = append(words, match)
			i += len(match)
			continue
		}
		c := name[i]
		switch {
		case c >= 'A' && c <= 'Z':
			j := i
			for j < len(name) && name[j] >= 'A' && name[j] <= 'Z' {
				j++
			}
			run := name[i:j]
			// Single cap followed by lowercase: "Description" → "description".
			if len(run) == 1 && j < len(name) && ((name[j] >= 'a' && name[j] <= 'z') || name[j] == '_') {
				for j < len(name) && ((name[j] >= 'a' && name[j] <= 'z') || name[j] == '_') {
					j++
				}
				words = append(words, strings.ToLower(name[i:j]))
				i = j
				continue
			}
			// A caps run followed by a lowercase letter: the last cap
			// belongs to the next word (e.g. "ExecS" + "tart").
			if len(run) > 1 && j < len(name) && name[j] >= 'a' && name[j] <= 'z' {
				j--
				run = run[:len(run)-1]
			}
			words = append(words, strings.ToLower(run))
			i = j
		case c >= '0' && c <= '9':
			j := i
			for j < len(name) && name[j] >= '0' && name[j] <= '9' {
				j++
			}
			words = append(words, name[i:j])
			i = j
		default:
			j := i
			for j < len(name) && ((name[j] >= 'a' && name[j] <= 'z') || name[j] == '_') {
				j++
			}
			if j == i {
				j++
			}
			words = append(words, name[i:j])
			i = j
		}
	}
	return strings.Join(words, "_")
}

func longestAcronym(s string, i int) (string, bool) {
	best := ""
	for tok := range acronymTokens {
		if i+len(tok) <= len(s) && s[i:i+len(tok)] == tok && len(tok) > len(best) {
			best = tok
		}
	}
	if best == "" {
		return "", false
	}
	return acronymTokens[best], true
}

type macroDef struct{ lines []string }

func parseGperf(src string) []directive {
	macros := map[string]*macroDef{}
	var out []directive
	seen := map[[2]string]bool{}

	var handle func(line, typeVar string)
	var expand func(lines []string, typeVar string)

	handle = func(line, typeVar string) {
		line = strings.ReplaceAll(line, "{{type}}", typeVar)
		line = strings.TrimSpace(strings.TrimSuffix(line, ","))
		parts := splitTop(line)
		if len(parts) < 2 {
			return
		}
		key := strings.TrimSpace(parts[0])
		fn := strings.TrimSpace(parts[1])
		sec, name, ok := strings.Cut(key, ".")
		if !ok {
			return
		}
		k := [2]string{sec, name}
		if seen[k] {
			return
		}
		seen[k] = true
		// [Install] directives have no parse function in the gperf table
		// (they are consumed by systemctl enable, not the manager).
		var class string
		switch {
		case fn == "NULL":
			if name == "DefaultInstance" {
				class = "string"
			} else {
				class = "list"
			}
		case strings.HasPrefix(fn, "config_parse"):
			class = fnClass[fn]
			if class == "" {
				class = "string"
			}
		default:
			return
		}
		out = append(out, directive{Section: sec, Name: name, Attr: snake(name), Class: class})
	}

	expand = func(lines []string, typeVar string) {
		for _, l := range lines {
			l = strings.TrimSpace(l)
			if m := macroCall(l); m != "" {
				expand(macros[m].lines, macroArgs(l))
				continue
			}
			handle(l, typeVar)
		}
	}

	var curMacro string

	for _, raw := range strings.Split(src, "\n") {
		s := strings.TrimSpace(raw)
		if s == "" || strings.HasPrefix(s, "{#") || strings.HasPrefix(s, "#") {
			continue
		}
		if strings.HasPrefix(s, "{%- macro") {
			rest := strings.TrimPrefix(s, "{%- macro")
			if i := strings.Index(rest, "("); i >= 0 {
				curMacro = strings.TrimSpace(rest[:i])
			}
			continue
		}
		if strings.HasPrefix(s, "{%- endmacro") {
			curMacro = ""
			continue
		}
		if curMacro != "" {
			if macros[curMacro] == nil {
				macros[curMacro] = &macroDef{}
			}
			macros[curMacro].lines = append(macros[curMacro].lines, s)
			continue
		}
		if m := macroCall(s); m != "" {
			expand(macros[m].lines, macroArgs(s))
			continue
		}
		handle(s, "")
	}
	return out
}

func macroCall(s string) string {
	if !strings.HasPrefix(s, "{{") {
		return ""
	}
	end := strings.Index(s, "}}")
	if end < 0 {
		return ""
	}
	inner := strings.TrimSpace(s[2:end])
	after := strings.TrimSpace(s[end+2:])
	// Only a whole-line invocation "{{ NAME(args) }}" is a macro call;
	// "{{type}}.Directive, …" lines have content after the closing braces.
	if after != "" || !strings.HasSuffix(inner, ")") {
		return ""
	}
	if i := strings.Index(inner, "("); i > 0 {
		return inner[:i]
	}
	return ""
}

func macroArgs(s string) string {
	end := strings.Index(s, "}}")
	if end < 0 {
		return ""
	}
	inner := strings.TrimSpace(s[2:end])
	i := strings.Index(inner, "(")
	j := strings.LastIndex(inner, ")")
	if i < 0 || j < 0 || j < i {
		return ""
	}
	return strings.Trim(strings.TrimSpace(inner[i+1:j]), "'\"")
}

// splitTop splits a gperf line on commas (no nesting in these tables).
func splitTop(s string) []string {
	return strings.Split(s, ",")
}

const goHeader = `// Code generated by cmd/genprops from systemd load-fragment-gperf.gperf.in files. DO NOT EDIT.

package sdprops

// LatestVersion is the newest systemd release covered by the catalogs.
const LatestVersion = %d

// VersionCatalog holds the directives of one systemd release.
type VersionCatalog struct {
	Version  int
	Sections []SectionGroup
}

`

type Directive struct {
	Name  string
	Attr  string
	Class string
}

type SectionGroup struct {
	Section    string
	Directives []Directive
}

type VersionCatalog struct {
	Version  int
	Sections []SectionGroup
}

var byVersion2 = map[string][]SectionGroup{}
var unitLatestVersions []int

func kindPrefix(k string) string {
	switch k {
	case "unit":
		return "load-fragment-v"
	case "network":
		return "networkd-network-v"
	case "netdev":
		return "netdev-v"
	case "link":
		return "link-config-v"
	}
	return k + "-v"
}

func kindTitle(k string) string {
	switch k {
	case "unit":
		return "Unit"
	case "network":
		return "Network"
	case "netdev":
		return "Netdev"
	case "link":
		return "Link"
	}
	return k
}

func kindOf(base string) (string, error) {
	switch {
	case strings.HasPrefix(base, "load-fragment-v"):
		return "unit", nil
	case strings.HasPrefix(base, "networkd-network-v"):
		return "network", nil
	case strings.HasPrefix(base, "netdev-v"):
		return "netdev", nil
	case strings.HasPrefix(base, "link-config-v"):
		return "link", nil
	}
	return "", fmt.Errorf("unknown gperf file kind: %s", base)
}

func main() {
	out := flag.String("out", "internal/sdprops/catalog_gen.go", "output Go file")
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: genprops -out FILE load-fragment-vNNN.gperf ...")
		os.Exit(1)
	}

	kindFiles := map[string][]string{} // kind → file list
	for _, in := range args {
		kind, err := kindOf(filepath.Base(in))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		kindFiles[kind] = append(kindFiles[kind], in)
	}

	var b strings.Builder
	latest := 0
	for _, kind := range []string{"unit", "network", "netdev", "link"} {
		files := kindFiles[kind]
		if len(files) == 0 {
			continue
		}
		slices.Sort(files)
		versions := []int{}
		for _, in := range files {
			base := filepath.Base(in)
			var v int
			if _, err := fmt.Sscanf(base, kindPrefix(kind)+"%d.gperf", &v); err != nil {
				fmt.Fprintf(os.Stderr, "cannot derive version from %s\n", base)
				os.Exit(1)
			}
			src, err := os.ReadFile(in)
			if err != nil {
				fmt.Fprintln(os.Stderr, "read:", err)
				os.Exit(1)
			}
			ds := parseGperf(string(src))
			bySection := map[string][]directive{}
			var order []string
			for _, d := range ds {
				if !slices.Contains(order, d.Section) {
					order = append(order, d.Section)
				}
				bySection[d.Section] = append(bySection[d.Section], d)
			}
			for sec, list := range bySection {
				used := map[string]string{}
				for _, d := range list {
					if prev, ok := used[d.Attr]; ok {
						fmt.Fprintf(os.Stderr, "attr collision in %s (v%d %s): %s vs %s\n", sec, v, kind, prev, d.Name)
						os.Exit(1)
					}
					used[d.Attr] = d.Name
				}
			}
			groups := make([]SectionGroup, 0, len(bySection))
			for _, sec := range order {
				g := SectionGroup{Section: sec}
				for _, d := range bySection[sec] {
					g.Directives = append(g.Directives, Directive{Name: d.Name, Attr: d.Attr, Class: d.Class})
				}
				groups = append(groups, g)
			}
			byVersion2[kind+"_"+fmt.Sprint(v)] = groups
			versions = append(versions, v)
			byVersion2[kind+"_"+fmt.Sprint(v)] = groups
		}
		slices.Sort(versions)
		if versions[len(versions)-1] > latest {
			latest = versions[len(versions)-1]
		}
		if b.Len() == 0 {
			fmt.Fprintf(&b, goHeader, latest)
		}
		fmt.Fprintf(&b, "// %s directives per systemd release, ascending order.\nvar %sCatalogs = []VersionCatalog{\n", kind, kindTitle(kind))
		for _, v := range versions {
			groups := byVersion2[kind+"_"+fmt.Sprint(v)]
			fmt.Fprintf(&b, "\t{\n\t\tVersion: %d,\n\t\tSections: []SectionGroup{\n", v)
			for _, g := range groups {
				fmt.Fprintf(&b, "\t\t\t{\n\t\t\t\tSection: %q,\n\t\t\t\tDirectives: []Directive{\n", g.Section)
				for _, d := range g.Directives {
					fmt.Fprintf(&b, "\t\t\t\t\t{Name: %q, Attr: %q, Class: %q},\n", d.Name, d.Attr, d.Class)
				}
				b.WriteString("\t\t\t\t},\n\t\t\t},\n")
			}
			b.WriteString("\t\t},\n\t},\n")
		}
		b.WriteString("}\n\n")
		total := 0
		for _, k := range byVersion2 {
			for _, g := range k {
				total += len(g.Directives)
			}
		}
		_ = total
	}

	if err := os.WriteFile(*out, []byte(b.String()), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
	fmt.Printf("wrote %s: %d catalogs, latest v%d\n", *out, len(kindFiles), latest)
}
