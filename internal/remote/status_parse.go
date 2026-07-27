package remote

import "strings"

// parseUnitStatus parses the key=value output of `systemctl show
// --property=LoadState,ActiveState,SubState,UnitFileState`. Shared by the SSH and
// Nspawn Host implementations so the parsing logic lives in one place.
func parseUnitStatus(out string) UnitStatus {
	st := UnitStatus{}
	for _, line := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), "=")
		if !ok {
			continue
		}
		switch k {
		case "LoadState":
			st.LoadState = v
		case "ActiveState":
			st.ActiveState = v
		case "SubState":
			st.SubState = v
		case "UnitFileState":
			st.UnitFileState = v
		}
	}
	return st
}

// parseLinkStatus parses the `State:` line of `networkctl status <ifname>` output, e.g.
// "State: routable (configured)". Shared by the SSH and Nspawn Host implementations.
func parseLinkStatus(ifname, out string) LinkStatus {
	st := LinkStatus{Name: ifname}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "State:") {
			rest := strings.TrimSpace(strings.TrimPrefix(line, "State:"))
			parts := strings.Fields(rest)
			if len(parts) > 0 {
				st.OperationalState = parts[0]
			}
			if i := strings.Index(rest, "("); i >= 0 {
				st.SetupState = strings.Trim(rest[i:], "()")
			}
		}
	}
	return st
}
