package remote

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
	"golang.org/x/crypto/ssh/knownhosts"
)

// Config holds SSH connection settings.
type Config struct {
	Host                  string
	User                  string
	Port                  int
	PrivateKey            string
	PrivateKeyPath        string
	UseSSHAgent           bool
	BastionHost           string
	BastionUser           string
	BastionPort           int
	InsecureIgnoreHostKey bool
	UnitDir               string
	NetworkDir            string
}

func (c *Config) defaults() {
	if c.User == "" {
		c.User = "root"
	}
	if c.Port == 0 {
		c.Port = 22
	}
	if c.BastionPort == 0 {
		c.BastionPort = 22
	}
	if c.UnitDir == "" {
		c.UnitDir = DefaultUnitDir
	}
	if c.NetworkDir == "" {
		c.NetworkDir = DefaultNetworkDir
	}
}

// Client is an SSH+SFTP Host implementation.
type Client struct {
	cfg     Config
	ssh     *ssh.Client
	bastion *ssh.Client
	sftp    *sftp.Client
}

// Dial opens SSH (+ optional bastion) and an SFTP session.
func Dial(cfg Config) (*Client, error) {
	cfg.defaults()
	auth, err := cfg.authMethods()
	if err != nil {
		return nil, err
	}
	hostKey, err := cfg.hostKeyCallback()
	if err != nil {
		return nil, err
	}
	sshCfg := &ssh.ClientConfig{
		User:            cfg.User,
		Auth:            auth,
		HostKeyCallback: hostKey,
		Timeout:         30 * time.Second,
	}

	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	c := &Client{cfg: cfg}

	if cfg.BastionHost != "" {
		bUser := cfg.BastionUser
		if bUser == "" {
			bUser = cfg.User
		}
		bCfg := *sshCfg
		bCfg.User = bUser
		bastion, err := ssh.Dial("tcp", net.JoinHostPort(cfg.BastionHost, strconv.Itoa(cfg.BastionPort)), &bCfg)
		if err != nil {
			return nil, fmt.Errorf("bastion: %w", err)
		}
		conn, err := bastion.Dial("tcp", addr)
		if err != nil {
			_ = bastion.Close()
			return nil, fmt.Errorf("bastion dial target: %w", err)
		}
		ncc, chans, reqs, err := ssh.NewClientConn(conn, addr, sshCfg)
		if err != nil {
			_ = conn.Close()
			_ = bastion.Close()
			return nil, err
		}
		c.bastion = bastion
		c.ssh = ssh.NewClient(ncc, chans, reqs)
	} else {
		client, err := ssh.Dial("tcp", addr, sshCfg)
		if err != nil {
			return nil, err
		}
		c.ssh = client
	}

	sc, err := sftp.NewClient(c.ssh)
	if err != nil {
		_ = c.Close()
		return nil, fmt.Errorf("sftp: %w", err)
	}
	c.sftp = sc
	return c, nil
}

// Close closes SFTP and SSH.
func (c *Client) Close() error {
	var err error
	if c.sftp != nil {
		err = c.sftp.Close()
	}
	if c.ssh != nil {
		if e := c.ssh.Close(); e != nil && err == nil {
			err = e
		}
	}
	if c.bastion != nil {
		if e := c.bastion.Close(); e != nil && err == nil {
			err = e
		}
	}
	return err
}

func (c Config) authMethods() ([]ssh.AuthMethod, error) {
	var methods []ssh.AuthMethod
	if c.PrivateKey != "" || c.PrivateKeyPath != "" {
		var key []byte
		var err error
		if c.PrivateKey != "" {
			key = []byte(c.PrivateKey)
		} else {
			key, err = os.ReadFile(c.PrivateKeyPath)
			if err != nil {
				return nil, err
			}
		}
		signer, err := ssh.ParsePrivateKey(key)
		if err != nil {
			return nil, err
		}
		methods = append(methods, ssh.PublicKeys(signer))
	}
	if c.UseSSHAgent || len(methods) == 0 {
		if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
			conn, err := net.Dial("unix", sock)
			if err == nil {
				ag := agent.NewClient(conn)
				methods = append(methods, ssh.PublicKeysCallback(ag.Signers))
			}
		}
	}
	if len(methods) == 0 {
		return nil, fmt.Errorf("no SSH authentication method available")
	}
	return methods, nil
}

func (c Config) hostKeyCallback() (ssh.HostKeyCallback, error) {
	if c.InsecureIgnoreHostKey {
		return ssh.InsecureIgnoreHostKey(), nil
	}
	kh := filepath.Join(os.Getenv("HOME"), ".ssh", "known_hosts")
	cb, err := knownhosts.New(kh)
	if err != nil {
		return nil, fmt.Errorf("known_hosts %s: %w (set insecure_ignore_host_key=true for lab)", kh, err)
	}
	return cb, nil
}

func (c *Client) run(cmd string) (string, error) {
	session, err := c.ssh.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	var buf bytes.Buffer
	session.Stdout = &buf
	session.Stderr = &buf
	err = session.Run(cmd)
	return buf.String(), err
}

func (c *Client) mustOK(cmd string) error {
	out, err := c.run(cmd)
	if err != nil {
		return fmt.Errorf("%s: %w (%s)", cmd, err, strings.TrimSpace(out))
	}
	return nil
}

// runWithStdin runs cmd on a new session, feeding stdin to it. Output (stdout+stderr) is
// captured for error reporting only; callers must never log stdin (it may hold secret data).
func (c *Client) runWithStdin(cmd string, stdin []byte) (string, error) {
	session, err := c.ssh.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	session.Stdin = bytes.NewReader(stdin)
	var buf bytes.Buffer
	session.Stdout = &buf
	session.Stderr = &buf
	err = session.Run(cmd)
	return buf.String(), err
}

func (c *Client) atomicWrite(remotePath string, content string) error {
	return c.atomicWriteMode(remotePath, content, 0o644)
}

// atomicWriteMode writes content to remotePath atomically (write to a temp file, chmod,
// then rename), applying mode to the temp file *before* it is renamed into place. This
// avoids a window where the destination path is briefly visible with the default (looser)
// permissions — important for secret material such as credentials.
func (c *Client) atomicWriteMode(remotePath string, content string, mode os.FileMode) error {
	dir := path.Dir(remotePath)
	if err := c.sftp.MkdirAll(dir); err != nil {
		return err
	}
	tmp := remotePath + ".tmp." + strconv.FormatInt(time.Now().UnixNano(), 36)
	f, err := c.sftp.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, strings.NewReader(content)); err != nil {
		_ = f.Close()
		_ = c.sftp.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = c.sftp.Remove(tmp)
		return err
	}
	if err := c.sftp.Chmod(tmp, mode); err != nil {
		_ = c.sftp.Remove(tmp)
		return err
	}
	if err := c.sftp.PosixRename(tmp, remotePath); err != nil {
		// fallback if PosixRename unsupported
		_ = c.sftp.Remove(remotePath)
		if err2 := c.sftp.Rename(tmp, remotePath); err2 != nil {
			_ = c.sftp.Remove(tmp)
			return fmt.Errorf("rename: %v / %v", err, err2)
		}
	}
	return nil
}

// ensureCredstoreDir creates a credstore directory (if missing) and locks it down to 0700,
// since it holds secret material. Best-effort on the chmod: some servers may already manage
// this directory's ownership/mode (e.g. pre-created by systemd-creds tooling).
func (c *Client) ensureCredstoreDir(dir string) error {
	if err := c.sftp.MkdirAll(dir); err != nil {
		return err
	}
	_ = c.sftp.Chmod(dir, 0o700)
	return nil
}

func (c *Client) readFile(remotePath string) (string, error) {
	f, err := c.sftp.Open(remotePath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (c *Client) removeFile(remotePath string) error {
	err := c.sftp.Remove(remotePath)
	if err != nil && !os.IsNotExist(err) {
		// sftp may wrap errors
		if !strings.Contains(err.Error(), "no such file") {
			return err
		}
	}
	return nil
}

func (c *Client) WriteUnit(name, content string) error {
	p, err := unitPath(c.cfg.UnitDir, name)
	if err != nil {
		return err
	}
	return c.atomicWrite(p, content)
}

func (c *Client) ReadUnit(name string) (string, error) {
	p, err := unitPath(c.cfg.UnitDir, name)
	if err != nil {
		return "", err
	}
	return c.readFile(p)
}

func (c *Client) RemoveUnit(name string) error {
	p, err := unitPath(c.cfg.UnitDir, name)
	if err != nil {
		return err
	}
	return c.removeFile(p)
}

func (c *Client) WriteDropin(unit, dropin, content string) error {
	p, err := dropinPath(c.cfg.UnitDir, unit, dropin)
	if err != nil {
		return err
	}
	return c.atomicWrite(p, content)
}

func (c *Client) ReadDropin(unit, dropin string) (string, error) {
	p, err := dropinPath(c.cfg.UnitDir, unit, dropin)
	if err != nil {
		return "", err
	}
	return c.readFile(p)
}

func (c *Client) RemoveDropin(unit, dropin string) error {
	p, err := dropinPath(c.cfg.UnitDir, unit, dropin)
	if err != nil {
		return err
	}
	return c.removeFile(p)
}

func (c *Client) WriteNetwork(filename, content string) error {
	p, err := networkPath(c.cfg.NetworkDir, filename)
	if err != nil {
		return err
	}
	return c.atomicWrite(p, content)
}

func (c *Client) ReadNetwork(filename string) (string, error) {
	p, err := networkPath(c.cfg.NetworkDir, filename)
	if err != nil {
		return "", err
	}
	return c.readFile(p)
}

func (c *Client) RemoveNetwork(filename string) error {
	p, err := networkPath(c.cfg.NetworkDir, filename)
	if err != nil {
		return err
	}
	return c.removeFile(p)
}

func (c *Client) DaemonReload() error {
	return c.mustOK("systemctl daemon-reload")
}

func (c *Client) EnableUnit(name string) error {
	return c.mustOK("systemctl enable -- " + shellQuote(name))
}

func (c *Client) DisableUnit(name string) error {
	return c.mustOK("systemctl disable -- " + shellQuote(name))
}

func (c *Client) StartUnit(name string) error {
	return c.mustOK("systemctl start -- " + shellQuote(name))
}

func (c *Client) StopUnit(name string) error {
	// best-effort stop
	_, _ = c.run("systemctl stop -- " + shellQuote(name))
	return nil
}

func (c *Client) UnitStatus(name string) (UnitStatus, error) {
	out, err := c.run("systemctl show --property=LoadState,ActiveState,SubState,UnitFileState -- " + shellQuote(name))
	if err != nil {
		return UnitStatus{}, fmt.Errorf("systemctl show: %w (%s)", err, strings.TrimSpace(out))
	}
	return parseUnitStatus(out), nil
}

func (c *Client) NetworkReload() error {
	return c.mustOK("networkctl reload")
}

func (c *Client) LinkStatus(ifname string) (LinkStatus, error) {
	if err := safeName(ifname); err != nil {
		return LinkStatus{}, err
	}
	out, err := c.run("networkctl status -- " + shellQuote(ifname))
	if err != nil {
		return LinkStatus{}, fmt.Errorf("networkctl status: %w (%s)", err, strings.TrimSpace(out))
	}
	return parseLinkStatus(ifname, out), nil
}

func (c *Client) WriteCredential(name, data string) error {
	p, err := credPath(false, name)
	if err != nil {
		return err
	}
	if err := c.ensureCredstoreDir(path.Dir(p)); err != nil {
		return err
	}
	// mode is applied to the temp file before the atomic rename, so the credential is
	// never briefly visible at the final path with looser (world/group-readable) permissions.
	return c.atomicWriteMode(p, data, 0o600)
}

func (c *Client) WriteCredentialEncrypted(name, data, withKey string) error {
	p, err := credPath(true, name)
	if err != nil {
		return err
	}
	if err := c.ensureCredstoreDir(path.Dir(p)); err != nil {
		return err
	}
	cmd := "systemd-creds encrypt --name=" + shellQuote(name)
	if withKey != "" && withKey != "auto" {
		cmd += " --with-key=" + shellQuote(withKey)
	}
	cmd += " - " + shellQuote(p)
	out, err := c.runWithStdin(cmd, []byte(data))
	if err != nil {
		return fmt.Errorf("systemd-creds encrypt %s: %w (%s)", name, err, strings.TrimSpace(out))
	}
	return nil
}

func (c *Client) CredentialExists(name string, encrypted bool) (bool, error) {
	p, err := credPath(encrypted, name)
	if err != nil {
		return false, err
	}
	if _, err := c.sftp.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (c *Client) RemoveCredential(name string, encrypted bool) error {
	p, err := credPath(encrypted, name)
	if err != nil {
		return err
	}
	return c.removeFile(p)
}

func shellJoin(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = shellQuote(a)
	}
	return strings.Join(parts, " ")
}

// EnsureMachineImage pulls or imports an image. If an image with the same name
// already exists, it is removed first.
func (c *Client) EnsureMachineImage(name, imageType, source string) error {
	t, err := ParseMachineImageType(imageType)
	if err != nil {
		return err
	}
	args, err := MachinectlEnsureArgs(name, t, source)
	if err != nil {
		return err
	}
	if _, err := c.run("machinectl show-image -- " + shellQuote(name)); err == nil {
		_ = c.mustOK("machinectl remove -- " + shellQuote(name))
	}
	return c.mustOK("machinectl " + shellJoin(args))
}

func (c *Client) RemoveMachineImage(name string) error {
	if err := safeName(name); err != nil {
		return err
	}
	return c.mustOK("machinectl remove -- " + shellQuote(name))
}

func (c *Client) WriteNspawnFile(name, content string) error {
	p, err := nspawnSettingsPath(name)
	if err != nil {
		return err
	}
	if err := c.mustOK("mkdir -p " + shellQuote(path.Dir(p))); err != nil {
		return err
	}
	return c.atomicWrite(p, content)
}

func (c *Client) ReadNspawnFile(name string) (string, error) {
	p, err := nspawnSettingsPath(name)
	if err != nil {
		return "", err
	}
	return c.readFile(p)
}

func (c *Client) RemoveNspawnFile(name string) error {
	p, err := nspawnSettingsPath(name)
	if err != nil {
		return err
	}
	return c.removeFile(p)
}

func (c *Client) ShowMachine(name string) (MachineStatus, error) {
	if err := safeName(name); err != nil {
		return MachineStatus{}, err
	}
	st := MachineStatus{}
	if _, err := c.run("machinectl show-image -- " + shellQuote(name)); err == nil {
		st.ImagePresent = true
	}
	unit, err := c.UnitStatus(NspawnUnitName(name))
	if err != nil {
		return MachineStatus{}, err
	}
	st.Unit = unit
	if body, err := c.ReadNspawnFile(name); err == nil {
		st.Settings = body
	} else if !os.IsNotExist(err) {
		return MachineStatus{}, err
	}
	return st, nil
}

// EnsurePortableImage materializes a portable image under /var/lib/portables.
// Existing image with the same name is removed first (best-effort detach+remove).
func (c *Client) EnsurePortableImage(name, imageType, source string) error {
	t, err := ValidatePortableImageType(imageType)
	if err != nil {
		return err
	}
	if err := safeName(name); err != nil {
		return err
	}
	if source == "" {
		return fmt.Errorf("image.source is required")
	}
	_ = c.mustOK("portablectl detach --now --enable -- " + shellQuote(name))
	_ = c.mustOK("portablectl remove -- " + shellQuote(name))
	_ = c.mustOK("rm -rf " + shellQuote(path.Join(DefaultPortablesDir, name)) + " " + shellQuote(path.Join(DefaultPortablesDir, name+".raw")))

	if err := c.mustOK("mkdir -p " + shellQuote(DefaultPortablesDir)); err != nil {
		return err
	}
	raw := IsPortableRawSource(imageType, source)
	dest, err := PortableImagePath(name, raw)
	if err != nil {
		return err
	}
	switch t {
	case MachineImageLocal:
		if raw {
			return c.mustOK("cp -a -- " + shellQuote(source) + " " + shellQuote(dest))
		}
		lower := strings.ToLower(source)
		if strings.HasSuffix(lower, ".tar") || strings.HasSuffix(lower, ".tar.gz") ||
			strings.HasSuffix(lower, ".tgz") || strings.HasSuffix(lower, ".tar.xz") {
			if err := c.mustOK("mkdir -p " + shellQuote(dest)); err != nil {
				return err
			}
			return c.mustOK("tar -C " + shellQuote(dest) + " -xf " + shellQuote(source))
		}
		return fmt.Errorf("local portable image.source %q must be a .tar/.tar.gz/.tgz/.tar.xz/.raw path", source)
	case MachineImageTar:
		if err := c.mustOK("mkdir -p " + shellQuote(dest)); err != nil {
			return err
		}
		inner := "set -euo pipefail; curl -fsSL " + shellQuote(source) + " | tar -C " + shellQuote(dest) + " -x"
		return c.mustOK("bash -c " + shellQuote(inner))
	case MachineImageRaw:
		inner := "set -euo pipefail; curl -fsSL " + shellQuote(source) + " -o " + shellQuote(dest)
		return c.mustOK("bash -c " + shellQuote(inner))
	default:
		return fmt.Errorf("unsupported portable image type %q", t)
	}
}

func (c *Client) RemovePortableImage(name string) error {
	if err := safeName(name); err != nil {
		return err
	}
	_ = c.mustOK("portablectl remove -- " + shellQuote(name))
	_ = c.mustOK("rm -rf " + shellQuote(path.Join(DefaultPortablesDir, name)) + " " + shellQuote(path.Join(DefaultPortablesDir, name+".raw")))
	return nil
}

func (c *Client) AttachPortable(name string, enable, active bool) error {
	if err := safeName(name); err != nil {
		return err
	}
	// Prefer directory, then .raw.
	img := path.Join(DefaultPortablesDir, name)
	if _, err := c.run("test -e " + shellQuote(img)); err != nil {
		img = path.Join(DefaultPortablesDir, name+".raw")
	}
	args := []string{"attach", "--profile=trusted"}
	if enable {
		args = append(args, "--enable")
	}
	if active {
		args = append(args, "--now")
	}
	args = append(args, "--", img)
	return c.mustOK("portablectl " + shellJoin(args))
}

func (c *Client) DetachPortable(name string, enable, active bool) error {
	if err := safeName(name); err != nil {
		return err
	}
	args := []string{"detach"}
	if enable {
		args = append(args, "--enable")
	}
	if active {
		args = append(args, "--now")
	}
	args = append(args, "--")
	for _, id := range []string{
		name,
		path.Join(DefaultPortablesDir, name),
		path.Join(DefaultPortablesDir, name+".raw"),
	} {
		_, _ = c.run("portablectl " + shellJoin(append(append([]string{}, args...), id)))
	}
	return nil
}

func (c *Client) ShowPortable(name string) (PortableStatus, error) {
	if err := safeName(name); err != nil {
		return PortableStatus{}, err
	}
	st := PortableStatus{}
	dir := path.Join(DefaultPortablesDir, name)
	raw := path.Join(DefaultPortablesDir, name+".raw")
	if _, err := c.run("test -e " + shellQuote(dir)); err == nil {
		st.ImagePresent = true
		st.ImagePath = dir
	} else if _, err := c.run("test -e " + shellQuote(raw)); err == nil {
		st.ImagePresent = true
		st.ImagePath = raw
	}
	if st.ImagePresent {
		if _, err := c.run("portablectl is-attached -- " + shellQuote(name)); err == nil {
			st.Attached = true
		} else if _, err := c.run("portablectl is-attached -- " + shellQuote(st.ImagePath)); err == nil {
			st.Attached = true
		}
	}
	unit, err := c.UnitStatus(PortablePrimaryUnit(name))
	if err != nil {
		return PortableStatus{}, err
	}
	st.Unit = unit
	return st, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
