package remote

import (
	"fmt"
	"path"
	"strings"
)

const DefaultNspawnDir = "/etc/systemd/nspawn"

// MachineImageType selects how EnsureMachineImage obtains an image.
type MachineImageType string

const (
	MachineImageLocal MachineImageType = "local"
	MachineImageTar   MachineImageType = "tar"
	MachineImageRaw   MachineImageType = "raw"
	MachineImageOCI   MachineImageType = "oci"
)

// MachineStatus is a light view of an nspawn machine image + unit + settings.
type MachineStatus struct {
	ImagePresent bool
	Unit         UnitStatus
	Settings     string // empty if no .nspawn file
}

// ParseMachineImageType validates image.type.
func ParseMachineImageType(s string) (MachineImageType, error) {
	switch MachineImageType(s) {
	case MachineImageLocal, MachineImageTar, MachineImageRaw, MachineImageOCI:
		return MachineImageType(s), nil
	default:
		return "", fmt.Errorf("invalid image.type %q (want local|tar|raw|oci)", s)
	}
}

// NspawnUnitName returns the systemd unit that boots a machine.
func NspawnUnitName(machine string) string {
	return "systemd-nspawn@" + machine + ".service"
}

func nspawnSettingsPath(name string) (string, error) {
	if err := safeName(name); err != nil {
		return "", err
	}
	return path.Join(DefaultNspawnDir, name+".nspawn"), nil
}

// MachinectlEnsureArgs returns machinectl subcommand argv (without "machinectl").
func MachinectlEnsureArgs(name string, t MachineImageType, source string) ([]string, error) {
	if err := safeName(name); err != nil {
		return nil, err
	}
	if source == "" {
		return nil, fmt.Errorf("image.source is required")
	}
	switch t {
	case MachineImageTar:
		return []string{"pull-tar", source, name}, nil
	case MachineImageRaw:
		return []string{"pull-raw", source, name}, nil
	case MachineImageOCI:
		return []string{"pull-dkr", source, name}, nil
	case MachineImageLocal:
		lower := strings.ToLower(source)
		switch {
		case strings.HasSuffix(lower, ".raw"):
			return []string{"import-raw", source, name}, nil
		case strings.HasSuffix(lower, ".tar"),
			strings.HasSuffix(lower, ".tar.gz"),
			strings.HasSuffix(lower, ".tgz"),
			strings.HasSuffix(lower, ".tar.xz"):
			return []string{"import-tar", source, name}, nil
		default:
			return nil, fmt.Errorf("local image.source %q must be a .tar/.tar.gz/.tgz/.tar.xz/.raw file path", source)
		}
	default:
		return nil, fmt.Errorf("unsupported image type %q", t)
	}
}
