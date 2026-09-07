package provider

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/dlarochette/terraform-provider-systemd/internal/remote"
)

// Verify modes for the provider `verify` attribute. The unit file is
// validated with the target host's own systemd parser (systemd-analyze
// verify), so the rules always match the remote systemd version.
const (
	VerifyOff   = "off"
	VerifyWarn  = "warn"
	VerifyError = "error"
)

// Client wraps a remote.Host with Terraform-oriented unit/network helpers.
type Client struct {
	Host   remote.Host
	Verify string
	// SystemdVersion pins the target systemd release for directive
	// availability checks; 0 (unset) means detect on the host once.
	SystemdVersion int

	detectedVersion *int
}

// hostSystemdVersion returns the pinned release, or the host release
// detected once (and cached) via `systemctl --version`.
func (c *Client) hostSystemdVersion() (int, error) {
	if c.SystemdVersion > 0 {
		return c.SystemdVersion, nil
	}
	if c.detectedVersion != nil {
		return *c.detectedVersion, nil
	}
	line, err := c.Host.SystemdVersion()
	if err != nil {
		return 0, err
	}
	f := strings.Fields(line)
	if len(f) < 2 {
		return 0, fmt.Errorf("cannot parse systemd version from %q", line)
	}
	v, err := strconv.Atoi(f[1])
	if err != nil {
		return 0, fmt.Errorf("cannot parse systemd version from %q: %w", line, err)
	}
	c.detectedVersion = &v
	return v, nil
}

func (c *Client) verifyMode() string {
	switch c.Verify {
	case VerifyOff, VerifyError:
		return c.Verify
	default:
		return VerifyWarn
	}
}

func (c *Client) PutUnit(ctx context.Context, name, content string, enable, active *bool) error {
	_, err := c.PutUnitVerified(ctx, name, content, enable, active)
	return err
}

// PutUnitVerified writes the unit file, validates it with the remote
// systemd parser according to the configured verify mode, then reloads and
// applies the lifecycle. In error mode a verification failure rolls the
// file back (restored or removed) so the host is left untouched. Warnings
// (mode warn, or systemd-analyze diagnostics) are returned for the caller
// to surface.
func (c *Client) PutUnitVerified(ctx context.Context, name, content string, enable, active *bool) (warnings []string, err error) {
	mode := c.verifyMode()
	if mode == VerifyOff {
		if err := c.Host.WriteUnit(name, content); err != nil {
			return nil, err
		}
		if err := c.Host.DaemonReload(); err != nil {
			return nil, err
		}
		return nil, c.ApplyUnitLifecycle(ctx, name, enable, active)
	}

	var prev string
	prevExisted := false
	if old, err := c.Host.ReadUnit(name); err == nil {
		prev, prevExisted = old, true
	}
	if err := c.Host.WriteUnit(name, content); err != nil {
		return nil, err
	}

	out, verr := c.Host.VerifyUnit(name)
	if verr != nil {
		if isMissingCommand(out, verr) {
			// systemd-analyze unavailable on this host: degrade to a warning.
			warnings = append(warnings, fmt.Sprintf("systemd-analyze is not available on the host; unit %s was written without verification", name))
		} else {
			diag := fmt.Sprintf("systemd-analyze verify %s failed (systemd %s):\n%s", name, c.versionOrUnknown(), strings.TrimSpace(out))
			if mode == VerifyError {
				if rbErr := c.rollbackUnit(name, prev, prevExisted); rbErr != nil {
					diag += fmt.Sprintf("\nrollback failed: %v", rbErr)
				}
				return warnings, fmt.Errorf("%s", diag)
			}
			warnings = append(warnings, diag)
		}
	} else if strings.TrimSpace(out) != "" {
		warnings = append(warnings, fmt.Sprintf("systemd-analyze verify %s: %s", name, strings.TrimSpace(out)))
	}

	if err := c.Host.DaemonReload(); err != nil {
		return warnings, err
	}
	return warnings, c.ApplyUnitLifecycle(ctx, name, enable, active)
}

// rollbackUnit restores the previous unit content, or removes the unit when
// it did not exist before.
func (c *Client) rollbackUnit(name, prev string, prevExisted bool) error {
	if !prevExisted {
		return c.Host.RemoveUnit(name)
	}
	return c.Host.WriteUnit(name, prev)
}

func isMissingCommand(out string, err error) bool {
	s := strings.TrimSpace(out)
	return err != nil && strings.Contains(s, "command not found")
}

func (c *Client) versionOrUnknown() string {
	line, err := c.Host.SystemdVersion()
	if err != nil {
		return "unknown"
	}
	return line
}

// systemdVersionInfo carries the parsed systemd release.
type systemdVersionInfo struct {
	Version string
	Line    string
}

func (c *Client) systemdVersion() (systemdVersionInfo, error) {
	line, err := c.Host.SystemdVersion()
	if err != nil {
		return systemdVersionInfo{}, err
	}
	f := strings.Fields(line)
	if len(f) < 2 {
		return systemdVersionInfo{}, fmt.Errorf("cannot parse systemd version from %q", line)
	}
	return systemdVersionInfo{Version: f[1], Line: line}, nil
}

// ApplyUnitLifecycle enables/disables and starts/stops a unit according to the
// desired (possibly nil/unset) enable and active flags.
func (c *Client) ApplyUnitLifecycle(ctx context.Context, name string, enable, active *bool) error {
	_ = ctx
	if enable != nil {
		if *enable {
			if err := c.Host.EnableUnit(name); err != nil {
				return err
			}
		} else if err := c.Host.DisableUnit(name); err != nil {
			return err
		}
	}
	if active != nil {
		if *active {
			if err := c.Host.StartUnit(name); err != nil {
				return fmt.Errorf("start %s: %w", name, err)
			}
		} else if err := c.Host.StopUnit(name); err != nil {
			return err
		}
	}
	return nil
}

// DeleteUnitLifecycle best-effort stops and disables a unit before removal.
func (c *Client) DeleteUnitLifecycle(ctx context.Context, name string) {
	_ = ctx
	_ = c.Host.StopUnit(name)
	_ = c.Host.DisableUnit(name)
}

func (c *Client) GetUnit(ctx context.Context, name string) (string, error) {
	_ = ctx
	return c.Host.ReadUnit(name)
}

func (c *Client) DeleteUnit(ctx context.Context, name string) error {
	c.DeleteUnitLifecycle(ctx, name)
	if err := c.Host.RemoveUnit(name); err != nil {
		return err
	}
	return c.Host.DaemonReload()
}

func (c *Client) UnitStatus(ctx context.Context, name string) (remote.UnitStatus, error) {
	_ = ctx
	return c.Host.UnitStatus(name)
}

func (c *Client) PutDropin(ctx context.Context, unit, dropin, content string) error {
	_ = ctx
	if err := c.Host.WriteDropin(unit, dropin, content); err != nil {
		return err
	}
	return c.Host.DaemonReload()
}

func (c *Client) GetDropin(ctx context.Context, unit, dropin string) (string, error) {
	_ = ctx
	return c.Host.ReadDropin(unit, dropin)
}

func (c *Client) DeleteDropin(ctx context.Context, unit, dropin string) error {
	_ = ctx
	if err := c.Host.RemoveDropin(unit, dropin); err != nil {
		return err
	}
	return c.Host.DaemonReload()
}

func (c *Client) PutNetwork(ctx context.Context, filename, content string) error {
	_ = ctx
	if err := c.Host.WriteNetwork(filename, content); err != nil {
		return err
	}
	return c.Host.NetworkReload()
}

func (c *Client) GetNetwork(ctx context.Context, filename string) (string, error) {
	_ = ctx
	return c.Host.ReadNetwork(filename)
}

func (c *Client) DeleteNetwork(ctx context.Context, filename string) error {
	_ = ctx
	if err := c.Host.RemoveNetwork(filename); err != nil {
		return err
	}
	return c.Host.NetworkReload()
}

func (c *Client) LinkStatus(ctx context.Context, ifname string) (remote.LinkStatus, error) {
	_ = ctx
	return c.Host.LinkStatus(ifname)
}

// PutCredential writes a credential to the host, plaintext under /etc/credstore or
// encrypted (via systemd-creds) under /etc/credstore.encrypted.
func (c *Client) PutCredential(ctx context.Context, name, data string, encrypted bool, withKey string) error {
	_ = ctx
	if encrypted {
		return c.Host.WriteCredentialEncrypted(name, data, withKey)
	}
	return c.Host.WriteCredential(name, data)
}

func (c *Client) HasCredential(ctx context.Context, name string, encrypted bool) (bool, error) {
	_ = ctx
	return c.Host.CredentialExists(name, encrypted)
}

func (c *Client) DeleteCredential(ctx context.Context, name string, encrypted bool) error {
	_ = ctx
	return c.Host.RemoveCredential(name, encrypted)
}

// PutMachine ensures the image, optionally writes/clears .nspawn settings, then
// applies enable/active on systemd-nspawn@<name>.service.
//
// settings:
//   - nil: leave .nspawn file untouched
//   - non-nil empty string: remove .nspawn if present
//   - non-nil non-empty: write settings
func (c *Client) PutMachine(ctx context.Context, name, imageType, source string, settings *string, enable, active *bool) error {
	if err := c.Host.EnsureMachineImage(name, imageType, source); err != nil {
		return err
	}
	if settings != nil {
		if *settings == "" {
			if err := c.Host.RemoveNspawnFile(name); err != nil {
				return err
			}
		} else if err := c.Host.WriteNspawnFile(name, *settings); err != nil {
			return err
		}
	}
	if err := c.Host.DaemonReload(); err != nil {
		return err
	}
	return c.ApplyUnitLifecycle(ctx, remote.NspawnUnitName(name), enable, active)
}

func (c *Client) GetMachine(ctx context.Context, name string) (remote.MachineStatus, error) {
	_ = ctx
	return c.Host.ShowMachine(name)
}

func (c *Client) DeleteMachine(ctx context.Context, name string, deleteImage bool) error {
	c.DeleteUnitLifecycle(ctx, remote.NspawnUnitName(name))
	_ = c.Host.RemoveNspawnFile(name)
	if deleteImage {
		if err := c.Host.RemoveMachineImage(name); err != nil {
			return err
		}
	}
	return nil
}

// PutPortable ensures the portable image, attaches it, then applies enable/active
// on the primary <name>.service unit when flags are set after attach.
func (c *Client) PutPortable(ctx context.Context, name, imageType, source string, enable, active *bool) error {
	if err := c.Host.EnsurePortableImage(name, imageType, source); err != nil {
		return err
	}
	en, act := false, false
	if enable != nil {
		en = *enable
	}
	if active != nil {
		act = *active
	}
	// Detach first so re-attach picks up a replaced image.
	_ = c.Host.DetachPortable(name, true, true)
	if err := c.Host.AttachPortable(name, en, act); err != nil {
		return err
	}
	// portablectl --enable/--now covers initial attach; still apply systemctl for explicit nil-safe updates.
	return c.ApplyUnitLifecycle(ctx, remote.PortablePrimaryUnit(name), enable, active)
}

func (c *Client) GetPortable(ctx context.Context, name string) (remote.PortableStatus, error) {
	_ = ctx
	return c.Host.ShowPortable(name)
}

func (c *Client) DeletePortable(ctx context.Context, name string, deleteImage bool) error {
	_ = ctx
	_ = c.Host.DetachPortable(name, true, true)
	if deleteImage {
		return c.Host.RemovePortableImage(name)
	}
	return nil
}

func (c *Client) PutResolvedConf(ctx context.Context, content string) error {
	_ = ctx
	if err := c.Host.WriteResolvedConf(content); err != nil {
		return err
	}
	return c.Host.ResolvedRestart()
}

func (c *Client) GetResolvedConf(ctx context.Context) (string, error) {
	_ = ctx
	return c.Host.ReadResolvedConf()
}

func (c *Client) DeleteResolvedConf(ctx context.Context) error {
	_ = ctx
	if err := c.Host.RemoveResolvedConf(); err != nil {
		return err
	}
	return c.Host.ResolvedRestart()
}

func (c *Client) PutResolvedDropin(ctx context.Context, name, content string) error {
	_ = ctx
	if err := c.Host.WriteResolvedDropin(name, content); err != nil {
		return err
	}
	return c.Host.ResolvedRestart()
}

func (c *Client) GetResolvedDropin(ctx context.Context, name string) (string, error) {
	_ = ctx
	return c.Host.ReadResolvedDropin(name)
}

func (c *Client) DeleteResolvedDropin(ctx context.Context, name string) error {
	_ = ctx
	if err := c.Host.RemoveResolvedDropin(name); err != nil {
		return err
	}
	return c.Host.ResolvedRestart()
}

// ResolveLinkDesired is the desired per-link resolvectl configuration.
type ResolveLinkDesired struct {
	DNS          []string
	Domains      []string
	DefaultRoute *bool
	LLMNR        string
	MDNS         string
	DNSSEC       string
	DNSOverTLS   string
}

func (c *Client) PutResolveLink(ctx context.Context, link string, d ResolveLinkDesired) error {
	_ = ctx
	if d.DNS != nil {
		if err := c.Host.ResolvectlDNS(link, d.DNS); err != nil {
			return err
		}
	}
	if d.Domains != nil {
		if err := c.Host.ResolvectlDomain(link, d.Domains); err != nil {
			return err
		}
	}
	if d.DefaultRoute != nil {
		if err := c.Host.ResolvectlDefaultRoute(link, *d.DefaultRoute); err != nil {
			return err
		}
	}
	if d.LLMNR != "" {
		if err := c.Host.ResolvectlLLMNR(link, d.LLMNR); err != nil {
			return err
		}
	}
	if d.MDNS != "" {
		if err := c.Host.ResolvectlMDNS(link, d.MDNS); err != nil {
			return err
		}
	}
	if d.DNSSEC != "" {
		if err := c.Host.ResolvectlDNSSEC(link, d.DNSSEC); err != nil {
			return err
		}
	}
	if d.DNSOverTLS != "" {
		if err := c.Host.ResolvectlDNSOverTLS(link, d.DNSOverTLS); err != nil {
			return err
		}
	}
	return nil
}

func (c *Client) GetResolveLinkDNS(ctx context.Context, link string) ([]string, error) {
	_ = ctx
	return c.Host.ResolvectlDNSGet(link)
}

func (c *Client) GetResolveLinkDomains(ctx context.Context, link string) ([]string, error) {
	_ = ctx
	return c.Host.ResolvectlDomainGet(link)
}

func (c *Client) DeleteResolveLink(ctx context.Context, link string) error {
	_ = ctx
	return c.Host.ResolvectlRevert(link)
}

func (c *Client) ResolveStatus(ctx context.Context, link string) (string, error) {
	_ = ctx
	return c.Host.ResolvectlStatus(link)
}
