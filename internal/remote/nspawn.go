package remote

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"
)

// Nspawn implements Host against a systemd-nspawn machine (ACC only). It talks to the
// machine exclusively via `machinectl copy-to/copy-from` and `systemd-run -M` — never SSH.
type Nspawn struct {
	Machine string
}

// NewNspawn returns a Host bound to the given nspawn machine name. An empty name
// defaults to "tf-systemd-acc".
func NewNspawn(machine string) *Nspawn {
	if machine == "" {
		machine = "tf-systemd-acc"
	}
	return &Nspawn{Machine: machine}
}

var _ Host = (*Nspawn)(nil)

func (n *Nspawn) Close() error { return nil }

// run executes args inside the machine via `systemd-run -M <machine> -P --wait --`.
func (n *Nspawn) run(args ...string) (string, error) {
	cmd := exec.Command("systemd-run", append([]string{
		"-M", n.Machine, "-P", "--wait", "--",
	}, args...)...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// runWithStdin is like run but feeds stdin to the invoked command. Callers must never
// log stdin (it may hold secret data).
func (n *Nspawn) runWithStdin(stdin []byte, args ...string) (string, error) {
	cmd := exec.Command("systemd-run", append([]string{
		"-M", n.Machine, "-P", "--wait", "--",
	}, args...)...)
	cmd.Stdin = bytes.NewReader(stdin)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

func (n *Nspawn) mustOK(args ...string) error {
	out, err := n.run(args...)
	if err != nil {
		return fmt.Errorf("%v: %w (%s)", args, err, strings.TrimSpace(out))
	}
	return nil
}

func (n *Nspawn) copyTo(localPath, remotePath string) error {
	cmd := exec.Command("machinectl", "copy-to", n.Machine, localPath, remotePath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("machinectl copy-to: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (n *Nspawn) copyFrom(remotePath, localPath string) error {
	cmd := exec.Command("machinectl", "copy-from", n.Machine, remotePath, localPath)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("machinectl copy-from: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// writeFile writes content to remotePath inside the machine: stage it in a local temp
// file, machinectl copy-to it into place, then chmod to mode. The parent directory is
// created first so unit/dropin/network/credential writes all work regardless of layout.
func (n *Nspawn) writeFile(remotePath, content string, mode os.FileMode) error {
	tmp, err := os.CreateTemp("", "nspawn-write-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(content); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := n.mustOK("mkdir", "-p", path.Dir(remotePath)); err != nil {
		return err
	}
	if err := n.copyTo(tmp.Name(), remotePath); err != nil {
		return err
	}
	return n.mustOK("chmod", fmt.Sprintf("%04o", mode), remotePath)
}

// readFile reads remotePath from inside the machine via machinectl copy-from into a
// local temp file.
func (n *Nspawn) readFile(remotePath string) (string, error) {
	tmp, err := os.CreateTemp("", "nspawn-read-*")
	if err != nil {
		return "", err
	}
	_ = tmp.Close()
	defer os.Remove(tmp.Name())
	if err := n.copyFrom(remotePath, tmp.Name()); err != nil {
		return "", err
	}
	b, err := os.ReadFile(tmp.Name())
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// removeFile removes remotePath inside the machine, best-effort (mirrors SSH semantics:
// a missing file is not an error).
func (n *Nspawn) removeFile(remotePath string) error {
	_, _ = n.run("rm", "-f", remotePath)
	return nil
}

func (n *Nspawn) WriteUnit(name, content string) error {
	p, err := unitPath(DefaultUnitDir, name)
	if err != nil {
		return err
	}
	return n.writeFile(p, content, 0o644)
}

func (n *Nspawn) ReadUnit(name string) (string, error) {
	p, err := unitPath(DefaultUnitDir, name)
	if err != nil {
		return "", err
	}
	return n.readFile(p)
}

func (n *Nspawn) RemoveUnit(name string) error {
	p, err := unitPath(DefaultUnitDir, name)
	if err != nil {
		return err
	}
	return n.removeFile(p)
}

func (n *Nspawn) WriteDropin(unit, dropin, content string) error {
	p, err := dropinPath(DefaultUnitDir, unit, dropin)
	if err != nil {
		return err
	}
	return n.writeFile(p, content, 0o644)
}

func (n *Nspawn) ReadDropin(unit, dropin string) (string, error) {
	p, err := dropinPath(DefaultUnitDir, unit, dropin)
	if err != nil {
		return "", err
	}
	return n.readFile(p)
}

func (n *Nspawn) RemoveDropin(unit, dropin string) error {
	p, err := dropinPath(DefaultUnitDir, unit, dropin)
	if err != nil {
		return err
	}
	return n.removeFile(p)
}

func (n *Nspawn) WriteNetwork(filename, content string) error {
	p, err := networkPath(DefaultNetworkDir, filename)
	if err != nil {
		return err
	}
	return n.writeFile(p, content, 0o644)
}

func (n *Nspawn) ReadNetwork(filename string) (string, error) {
	p, err := networkPath(DefaultNetworkDir, filename)
	if err != nil {
		return "", err
	}
	return n.readFile(p)
}

func (n *Nspawn) RemoveNetwork(filename string) error {
	p, err := networkPath(DefaultNetworkDir, filename)
	if err != nil {
		return err
	}
	return n.removeFile(p)
}

func (n *Nspawn) DaemonReload() error {
	return n.mustOK("systemctl", "daemon-reload")
}

func (n *Nspawn) EnableUnit(name string) error {
	return n.mustOK("systemctl", "enable", "--", name)
}

func (n *Nspawn) DisableUnit(name string) error {
	return n.mustOK("systemctl", "disable", "--", name)
}

func (n *Nspawn) StartUnit(name string) error {
	return n.mustOK("systemctl", "start", "--", name)
}

func (n *Nspawn) StopUnit(name string) error {
	// best-effort stop, mirrors SSH semantics
	_, _ = n.run("systemctl", "stop", "--", name)
	return nil
}

func (n *Nspawn) UnitStatus(name string) (UnitStatus, error) {
	out, err := n.run("systemctl", "show",
		"--property=LoadState,ActiveState,SubState,UnitFileState", "--", name)
	if err != nil {
		return UnitStatus{}, fmt.Errorf("systemctl show: %w (%s)", err, strings.TrimSpace(out))
	}
	return parseUnitStatus(out), nil
}

func (n *Nspawn) NetworkReload() error {
	return n.mustOK("networkctl", "reload")
}

func (n *Nspawn) LinkStatus(ifname string) (LinkStatus, error) {
	if err := safeName(ifname); err != nil {
		return LinkStatus{}, err
	}
	out, err := n.run("networkctl", "status", "--", ifname)
	if err != nil {
		return LinkStatus{}, fmt.Errorf("networkctl status: %w (%s)", err, strings.TrimSpace(out))
	}
	return parseLinkStatus(ifname, out), nil
}

func (n *Nspawn) WriteCredential(name, data string) error {
	p, err := credPath(false, name)
	if err != nil {
		return err
	}
	return n.writeFile(p, data, 0o600)
}

func (n *Nspawn) WriteCredentialEncrypted(name, data, withKey string) error {
	p, err := credPath(true, name)
	if err != nil {
		return err
	}
	if err := n.mustOK("mkdir", "-p", path.Dir(p)); err != nil {
		return err
	}
	args := []string{"systemd-creds", "encrypt", "--name=" + name}
	if withKey != "" && withKey != "auto" {
		args = append(args, "--with-key="+withKey)
	}
	args = append(args, "-", p)
	out, err := n.runWithStdin([]byte(data), args...)
	if err != nil {
		return fmt.Errorf("systemd-creds encrypt %s: %w (%s)", name, err, strings.TrimSpace(out))
	}
	return nil
}

func (n *Nspawn) CredentialExists(name string, encrypted bool) (bool, error) {
	p, err := credPath(encrypted, name)
	if err != nil {
		return false, err
	}
	if _, err := n.run("test", "-e", p); err != nil {
		return false, nil
	}
	return true, nil
}

func (n *Nspawn) RemoveCredential(name string, encrypted bool) error {
	p, err := credPath(encrypted, name)
	if err != nil {
		return err
	}
	return n.removeFile(p)
}
