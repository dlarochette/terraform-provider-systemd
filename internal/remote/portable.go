package remote

import (
	"fmt"
	"path"
	"strings"
)

const DefaultPortablesDir = "/var/lib/portables"

// PortableStatus is a light view of a portable service image.
type PortableStatus struct {
	ImagePresent bool
	Attached     bool
	ImagePath    string
	Unit         UnitStatus // primary <name>.service when attached
}

// PortableImagePath returns the on-disk path for a materialized portable image.
// Directory trees use /var/lib/portables/<name>; raw images use <name>.raw.
func PortableImagePath(name string, raw bool) (string, error) {
	if err := safeName(name); err != nil {
		return "", err
	}
	if raw {
		return path.Join(DefaultPortablesDir, name+".raw"), nil
	}
	return path.Join(DefaultPortablesDir, name), nil
}

// PortablePrimaryUnit is the conventional main unit for a portable image named name.
func PortablePrimaryUnit(name string) string {
	return name + ".service"
}

// IsPortableRawSource reports whether source should materialize as a .raw file.
func IsPortableRawSource(imageType, source string) bool {
	t, err := ParseMachineImageType(imageType)
	if err != nil {
		return false
	}
	switch t {
	case MachineImageRaw:
		return true
	case MachineImageLocal:
		return strings.HasSuffix(strings.ToLower(source), ".raw")
	default:
		return false
	}
}

// PortableMaterializeHint documents how EnsurePortableImage should obtain an image.
// OCI is rejected: portable services need an OS tree / .raw, not a machinectl OCI import.
func ValidatePortableImageType(imageType string) (MachineImageType, error) {
	t, err := ParseMachineImageType(imageType)
	if err != nil {
		return "", err
	}
	if t == MachineImageOCI {
		return "", fmt.Errorf("image.type oci is not supported for portable services; use local, tar, or raw")
	}
	return t, nil
}
