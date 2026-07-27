package provider

import "regexp"

func mustCompile(expr string) *regexp.Regexp {
	return regexp.MustCompile(expr)
}
