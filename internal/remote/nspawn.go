package remote

import (
	"bytes"
	"errors"
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

// copyTo copies localPath into the machine at remotePath, overwriting an existing
// destination (--force) — writeFile is used for both initial writes and updates
// (e.g. Terraform re-applying the same unit/network/credential path), and plain
// `machinectl copy-to` refuses to overwrite an existing file ("Failed to copy:
// File exists").
func (n *Nspawn) copyTo(localPath, remotePath string) error {
	cmd := exec.Command("machinectl", "copy-to", "--force", n.Machine, localPath, remotePath)
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

// stageDir returns a directory for staging local temp files that `machinectl
// copy-to`/`copy-from` will read from or write into.
//
// On SELinux-enforcing hosts, plain files under the default os.TempDir()
// (usually /tmp, labeled tmp_t) are not readable/writable by the
// systemd_machined_t domain that `machinectl copy-to`/`copy-from` runs its
// "(sd-copy)" helper as — every copy fails with "Failed to copy: Access
// denied" (confirmed via `ausearch -m avc`: `avc: denied { open } ...
// scontext=system_u:system_r:systemd_machined_t:s0
// tcontext=unconfined_u:object_r:user_tmp_t:s0`). Files created directly
// under /var/lib/machines inherit that directory's own
// systemd_machined_var_lib_t label instead, which the same domain is
// allowed to open — so stage there rather than in the system temp dir.
// Falls back to os.TempDir() if /var/lib/machines isn't writable (e.g. non-root).
func stageDir() string {
	const machinesDir = "/var/lib/machines"
	dir := path.Join(machinesDir, ".tf-provider-stage")
	if err := os.MkdirAll(dir, 0o700); err == nil {
		return dir
	}
	return os.TempDir()
}

// writeFile writes content to remotePath inside the machine: stage it in a local temp
// file, machinectl copy-to it into place, then chmod to mode. The parent directory is
// created first so unit/dropin/network/credential writes all work regardless of layout.
func (n *Nspawn) writeFile(remotePath, content string, mode os.FileMode) error {
	tmp, err := os.CreateTemp(stageDir(), "nspawn-write-*")
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
	tmp, err := os.CreateTemp(stageDir(), "nspawn-read-*")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	_ = tmp.Close()
	// `machinectl copy-from` refuses to overwrite an existing destination
	// (fails with "Failed to copy: File exists") — it creates the file
	// itself. os.CreateTemp only exists to reserve a unique name; remove it
	// immediately so copy-from can (re)create it.
	if err := os.Remove(tmpPath); err != nil {
		return "", err
	}
	defer os.Remove(tmpPath)
	if err := n.copyFrom(remotePath, tmpPath); err != nil {
		return "", err
	}
	b, err := os.ReadFile(tmpPath)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

// removeFile removes remotePath inside the machine. `rm -f` already exits 0 when the
// path is missing (mirrors SSH semantics: a missing file is not an error), so any
// remaining error here is a genuine transport/exec failure (systemd-run/machine
// unreachable, etc.) and must be propagated rather than swallowed.
func (n *Nspawn) removeFile(remotePath string) error {
	return n.mustOK("rm", "-f", remotePath)
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

// exitCode extracts the process exit code from err, or -1 if err is nil or is not an
// *exec.ExitError (e.g. the systemd-run/machinectl binary itself could not be started,
// or the connection to the machine failed) — such errors must not be mistaken for a
// clean "command ran and reported false".
func exitCode(err error) int {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func (n *Nspawn) CredentialExists(name string, encrypted bool) (bool, error) {
	p, err := credPath(encrypted, name)
	if err != nil {
		return false, err
	}
	_, err = n.run("test", "-e", p)
	if err == nil {
		return true, nil
	}
	// `test -e` exits 1 when the path is absent; any other failure (transport error,
	// systemd-run/machinectl not found, machine unreachable, etc.) must be propagated
	// rather than silently reported as "credential does not exist".
	if exitCode(err) == 1 {
		return false, nil
	}
	return false, fmt.Errorf("test -e %s: %w", p, err)
}

func (n *Nspawn) RemoveCredential(name string, encrypted bool) error {
	p, err := credPath(encrypted, name)
	if err != nil {
		return err
	}
	return n.removeFile(p)
}
