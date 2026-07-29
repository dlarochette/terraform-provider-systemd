package remote

import (
	"fmt"
	"strings"
)

// parseResolvectlList parses `resolvectl dns|domain LINK` output into tokens.
// Typical forms:
//
//	Link 2 (eth0): 1.1.1.1 1.0.0.1
//	1.1.1.1
//	1.0.0.1
func parseResolvectlList(out string) []string {
	var tokens []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if i := strings.Index(line, ":"); i >= 0 && strings.Contains(line[:i], "Link") {
			line = strings.TrimSpace(line[i+1:])
		}
		for _, t := range strings.Fields(line) {
			tokens = append(tokens, t)
		}
	}
	return tokens
}

func boolWord(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func validateLinkName(link string) error {
	if err := safeName(link); err != nil {
		return fmt.Errorf("invalid link %q: %w", link, err)
	}
	return nil
}
