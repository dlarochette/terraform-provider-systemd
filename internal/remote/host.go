// Package remote manages systemd on a host over SSH (SFTP + systemctl/networkctl).
package remote

import (
	"fmt"
	"path"
	"strings"
)

const (
	DefaultUnitDir               = "/etc/systemd/system"
	DefaultNetworkDir            = "/etc/systemd/network"
	DefaultCredstoreDir          = "/etc/credstore"
	DefaultCredstoreEncryptedDir = "/etc/credstore.encrypted"
)

// UnitStatus mirrors selected systemctl show properties.
type UnitStatus struct {
	LoadState     string
	ActiveState   string
	SubState      string
	UnitFileState string
}

// LinkStatus mirrors a light networkctl view.
type LinkStatus struct {
	Name             string
	OperationalState string
	SetupState       string
}

// Host is the remote operations surface used by the provider (SSH or fake).
type Host interface {
	Close() error

	WriteUnit(name, content string) error
	ReadUnit(name string) (string, error)
	RemoveUnit(name string) error

	WriteDropin(unit, dropin, content string) error
	ReadDropin(unit, dropin string) (string, error)
	RemoveDropin(unit, dropin string) error

	WriteNetwork(filename, content string) error
	ReadNetwork(filename string) (string, error)
	RemoveNetwork(filename string) error

	DaemonReload() error
	EnableUnit(name string) error
	DisableUnit(name string) error
	StartUnit(name string) error
	StopUnit(name string) error
	UnitStatus(name string) (UnitStatus, error)

	NetworkReload() error
	LinkStatus(ifname string) (LinkStatus, error)

	WriteCredential(name, data string) error
	WriteCredentialEncrypted(name, data, withKey string) error
	CredentialExists(name string, encrypted bool) (bool, error)
	RemoveCredential(name string, encrypted bool) error

	// Machine (systemd-nspawn) image + /etc/systemd/nspawn settings.
	EnsureMachineImage(name, imageType, source string) error
	RemoveMachineImage(name string) error
	WriteNspawnFile(name, content string) error
	ReadNspawnFile(name string) (string, error)
	RemoveNspawnFile(name string) error
	ShowMachine(name string) (MachineStatus, error)
}

func safeName(name string) error {
	if name == "" || name == "." || name == ".." || strings.Contains(name, "/") || strings.Contains(name, "..") || path.Clean(name) != name {
		return fmt.Errorf("invalid name %q", name)
	}
	return nil
}

func unitPath(unitDir, name string) (string, error) {
	if err := safeName(name); err != nil {
		return "", err
	}
	return path.Join(unitDir, name), nil
}

func dropinPath(unitDir, unit, dropin string) (string, error) {
	if err := safeName(unit); err != nil {
		return "", err
	}
	if err := safeName(dropin); err != nil {
		return "", err
	}
	if !strings.HasSuffix(dropin, ".conf") {
		dropin += ".conf"
	}
	return path.Join(unitDir, unit+".d", dropin), nil
}

func networkPath(networkDir, filename string) (string, error) {
	if err := safeName(filename); err != nil {
		return "", err
	}
	return path.Join(networkDir, filename), nil
}

// credPath returns the credstore path for a credential name, plaintext or encrypted.
func credPath(encrypted bool, name string) (string, error) {
	if err := safeName(name); err != nil {
		return "", err
	}
	if encrypted {
		return path.Join(DefaultCredstoreEncryptedDir, name), nil
	}
	return path.Join(DefaultCredstoreDir, name), nil
}
