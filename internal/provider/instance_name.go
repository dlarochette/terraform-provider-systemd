package provider

import (
	"fmt"
	"strings"
)

func InstantiateUnit(template, instance string) (string, error) {
	at := strings.Index(template, "@.")
	if at < 0 {
		return "", fmt.Errorf("not a template unit name: %q", template)
	}
	if instance == "" || strings.ContainsAny(instance, "@/") {
		return "", fmt.Errorf("invalid instance %q", instance)
	}
	return template[:at+1] + instance + template[at+1:], nil
}

func ParseInstantiatedUnit(name string) (template, instance string, err error) {
	at := strings.Index(name, "@")
	if at < 0 {
		return "", "", fmt.Errorf("not an instantiated unit: %q", name)
	}
	rest := name[at+1:]
	sufAt := strings.LastIndex(rest, ".")
	if sufAt <= 0 {
		return "", "", fmt.Errorf("not an instantiated unit: %q", name)
	}
	instance = rest[:sufAt]
	if instance == "" {
		return "", "", fmt.Errorf("template name has empty instance: %q", name)
	}
	suffix := rest[sufAt:] // e.g. ".service"
	template = name[:at+1] + suffix
	return template, instance, nil
}
