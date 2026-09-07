package remote

import (
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/dlarochette/terraform-provider-systemd/internal/unitfile"
	"sync"
)

// Fake is an in-memory Host for unit tests.
type Fake struct {
	mu               sync.Mutex
	Files            map[string]string
	Commands         []string
	FailCmd          string // if a command contains this substring, it fails
	Statuses         map[string]UnitStatus
	Links            map[string]LinkStatus
	Images           map[string]bool
	Portables        map[string]bool // name → image present under /var/lib/portables
	PortableAttached map[string]bool
	ResolveLinks     map[string]*fakeResolveLink
	UnitDir          string
	NetDir           string
}

type fakeResolveLink struct {
	DNS          []string
	Domains      []string
	DefaultRoute *bool
	LLMNR        string
	MDNS         string
	DNSSEC       string
	DNSOverTLS   string
}

// NewFake returns an empty fake host.
func NewFake() *Fake {
	return &Fake{
		Files:            map[string]string{},
		Statuses:         map[string]UnitStatus{},
		Links:            map[string]LinkStatus{},
		Images:           map[string]bool{},
		Portables:        map[string]bool{},
		PortableAttached: map[string]bool{},
		ResolveLinks:     map[string]*fakeResolveLink{},
		UnitDir:          DefaultUnitDir,
		NetDir:           DefaultNetworkDir,
	}
}

func (f *Fake) Close() error { return nil }

func (f *Fake) Exec(args ...string) error {
	return f.note("exec " + strings.Join(args, " "))
}

func (f *Fake) write(p, content string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Files[p] = content
	return nil
}

func (f *Fake) read(p string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.Files[p]
	if !ok {
		return "", os.ErrNotExist
	}
	return v, nil
}

func (f *Fake) remove(p string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Files, p)
	return nil
}

func (f *Fake) note(cmd string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Commands = append(f.Commands, cmd)
	if f.FailCmd != "" && strings.Contains(cmd, f.FailCmd) {
		return fmt.Errorf("fake fail: %s", cmd)
	}
	return nil
}

func (f *Fake) WriteUnit(name, content string) error {
	p, err := unitPath(f.UnitDir, name)
	if err != nil {
		return err
	}
	return f.write(p, content)
}

func (f *Fake) ReadUnit(name string) (string, error) {
	p, err := unitPath(f.UnitDir, name)
	if err != nil {
		return "", err
	}
	return f.read(p)
}

func (f *Fake) RemoveUnit(name string) error {
	p, err := unitPath(f.UnitDir, name)
	if err != nil {
		return err
	}
	return f.remove(p)
}

func (f *Fake) WriteDropin(unit, dropin, content string) error {
	p, err := dropinPath(f.UnitDir, unit, dropin)
	if err != nil {
		return err
	}
	return f.write(p, content)
}

func (f *Fake) ReadDropin(unit, dropin string) (string, error) {
	p, err := dropinPath(f.UnitDir, unit, dropin)
	if err != nil {
		return "", err
	}
	return f.read(p)
}

func (f *Fake) RemoveDropin(unit, dropin string) error {
	p, err := dropinPath(f.UnitDir, unit, dropin)
	if err != nil {
		return err
	}
	return f.remove(p)
}

func (f *Fake) WriteNetwork(filename, content string) error {
	p, err := networkPath(f.NetDir, filename)
	if err != nil {
		return err
	}
	return f.write(p, content)
}

func (f *Fake) ReadNetwork(filename string) (string, error) {
	p, err := networkPath(f.NetDir, filename)
	if err != nil {
		return "", err
	}
	return f.read(p)
}

func (f *Fake) RemoveNetwork(filename string) error {
	p, err := networkPath(f.NetDir, filename)
	if err != nil {
		return err
	}
	return f.remove(p)
}

func (f *Fake) DaemonReload() error        { return f.note("systemctl daemon-reload") }
func (f *Fake) EnableUnit(n string) error  { return f.note("systemctl enable " + n) }
func (f *Fake) DisableUnit(n string) error { return f.note("systemctl disable " + n) }
func (f *Fake) StartUnit(n string) error   { return f.note("systemctl start " + n) }
func (f *Fake) StopUnit(n string) error    { return f.note("systemctl stop " + n) }

func (f *Fake) UnitStatus(name string) (UnitStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if st, ok := f.Statuses[name]; ok {
		return st, nil
	}
	return UnitStatus{LoadState: "loaded", ActiveState: "inactive", SubState: "dead"}, nil
}

func (f *Fake) NetworkReload() error { return f.note("networkctl reload") }

func (f *Fake) WriteCredential(name, data string) error {
	p, err := credPath(false, name)
	if err != nil {
		return err
	}
	return f.write(p, data)
}

func (f *Fake) WriteCredentialEncrypted(name, data, withKey string) error {
	p, err := credPath(true, name)
	if err != nil {
		return err
	}
	cmd := "systemd-creds encrypt --name=" + name
	if withKey != "" && withKey != "auto" {
		cmd += " --with-key=" + withKey
	}
	cmd += " - " + p
	if err := f.note(cmd); err != nil {
		return err
	}
	return f.write(p, "encrypted:"+data)
}

func (f *Fake) CredentialExists(name string, encrypted bool) (bool, error) {
	p, err := credPath(encrypted, name)
	if err != nil {
		return false, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.Files[p]
	return ok, nil
}

func (f *Fake) RemoveCredential(name string, encrypted bool) error {
	p, err := credPath(encrypted, name)
	if err != nil {
		return err
	}
	return f.remove(p)
}

func (f *Fake) LinkStatus(ifname string) (LinkStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if st, ok := f.Links[ifname]; ok {
		return st, nil
	}
	return LinkStatus{Name: ifname, OperationalState: "off", SetupState: "unmanaged"}, nil
}

func (f *Fake) EnsureMachineImage(name, imageType, source string) error {
	t, err := ParseMachineImageType(imageType)
	if err != nil {
		return err
	}
	args, err := MachinectlEnsureArgs(name, t, source)
	if err != nil {
		return err
	}
	if err := f.note("machinectl " + strings.Join(args, " ")); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Images == nil {
		f.Images = map[string]bool{}
	}
	f.Images[name] = true
	return nil
}

func (f *Fake) RemoveMachineImage(name string) error {
	if err := safeName(name); err != nil {
		return err
	}
	if err := f.note("machinectl remove " + name); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Images, name)
	return nil
}

func (f *Fake) WriteNspawnFile(name, content string) error {
	p, err := nspawnSettingsPath(name)
	if err != nil {
		return err
	}
	return f.write(p, content)
}

func (f *Fake) ReadNspawnFile(name string) (string, error) {
	p, err := nspawnSettingsPath(name)
	if err != nil {
		return "", err
	}
	return f.read(p)
}

func (f *Fake) RemoveNspawnFile(name string) error {
	p, err := nspawnSettingsPath(name)
	if err != nil {
		return err
	}
	return f.remove(p)
}

func (f *Fake) ShowMachine(name string) (MachineStatus, error) {
	if err := safeName(name); err != nil {
		return MachineStatus{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	st := MachineStatus{ImagePresent: f.Images[name]}
	if u, ok := f.Statuses[NspawnUnitName(name)]; ok {
		st.Unit = u
	}
	if c, ok := f.Files[path.Join(DefaultNspawnDir, name+".nspawn")]; ok {
		st.Settings = c
	}
	return st, nil
}

func (f *Fake) EnsurePortableImage(name, imageType, source string) error {
	if _, err := ValidatePortableImageType(imageType); err != nil {
		return err
	}
	if err := safeName(name); err != nil {
		return err
	}
	if source == "" {
		return fmt.Errorf("image.source is required")
	}
	if err := f.note("portable ensure " + name + " " + imageType + " " + source); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Portables == nil {
		f.Portables = map[string]bool{}
	}
	f.Portables[name] = true
	return nil
}

func (f *Fake) RemovePortableImage(name string) error {
	if err := safeName(name); err != nil {
		return err
	}
	if err := f.note("portablectl remove " + name); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.Portables, name)
	delete(f.PortableAttached, name)
	return nil
}

func (f *Fake) AttachPortable(name string, enable, active bool) error {
	if err := safeName(name); err != nil {
		return err
	}
	cmd := "portablectl attach " + name
	if enable {
		cmd += " --enable"
	}
	if active {
		cmd += " --now"
	}
	if err := f.note(cmd); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.PortableAttached == nil {
		f.PortableAttached = map[string]bool{}
	}
	f.PortableAttached[name] = true
	if enable || active {
		st := f.Statuses[PortablePrimaryUnit(name)]
		if enable {
			st.UnitFileState = "enabled"
		}
		if active {
			st.ActiveState = "active"
			st.LoadState = "loaded"
		}
		f.Statuses[PortablePrimaryUnit(name)] = st
	}
	return nil
}

func (f *Fake) DetachPortable(name string, enable, active bool) error {
	if err := safeName(name); err != nil {
		return err
	}
	cmd := "portablectl detach " + name
	if enable {
		cmd += " --enable"
	}
	if active {
		cmd += " --now"
	}
	if err := f.note(cmd); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.PortableAttached, name)
	return nil
}

func (f *Fake) ShowPortable(name string) (PortableStatus, error) {
	if err := safeName(name); err != nil {
		return PortableStatus{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	st := PortableStatus{
		ImagePresent: f.Portables[name],
		Attached:     f.PortableAttached[name],
	}
	if p, err := PortableImagePath(name, false); err == nil {
		st.ImagePath = p
	}
	st.Unit = f.Statuses[PortablePrimaryUnit(name)]
	return st, nil
}

func (f *Fake) WriteResolvedConf(content string) error {
	return f.write(DefaultResolvedConf, content)
}

func (f *Fake) ReadResolvedConf() (string, error) {
	return f.read(DefaultResolvedConf)
}

func (f *Fake) RemoveResolvedConf() error {
	return f.remove(DefaultResolvedConf)
}

func (f *Fake) WriteResolvedDropin(name, content string) error {
	p, err := resolvedDropinPath(name)
	if err != nil {
		return err
	}
	return f.write(p, content)
}

func (f *Fake) ReadResolvedDropin(name string) (string, error) {
	p, err := resolvedDropinPath(name)
	if err != nil {
		return "", err
	}
	return f.read(p)
}

func (f *Fake) RemoveResolvedDropin(name string) error {
	p, err := resolvedDropinPath(name)
	if err != nil {
		return err
	}
	return f.remove(p)
}

func (f *Fake) ResolvedRestart() error {
	return f.note("systemctl restart systemd-resolved.service")
}

func (f *Fake) resolveLink(link string) (*fakeResolveLink, error) {
	if err := validateLinkName(link); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	st, ok := f.ResolveLinks[link]
	if !ok {
		st = &fakeResolveLink{}
		f.ResolveLinks[link] = st
	}
	return st, nil
}

func (f *Fake) ResolvectlDNS(link string, servers []string) error {
	st, err := f.resolveLink(link)
	if err != nil {
		return err
	}
	if err := f.note("resolvectl dns " + link + " " + strings.Join(servers, " ")); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	st.DNS = append([]string{}, servers...)
	return nil
}

func (f *Fake) ResolvectlDomain(link string, domains []string) error {
	st, err := f.resolveLink(link)
	if err != nil {
		return err
	}
	if err := f.note("resolvectl domain " + link + " " + strings.Join(domains, " ")); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	st.Domains = append([]string{}, domains...)
	return nil
}

func (f *Fake) ResolvectlDefaultRoute(link string, enable bool) error {
	st, err := f.resolveLink(link)
	if err != nil {
		return err
	}
	if err := f.note("resolvectl default-route " + link + " " + boolWord(enable)); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	st.DefaultRoute = &enable
	return nil
}

func (f *Fake) ResolvectlLLMNR(link, mode string) error {
	st, err := f.resolveLink(link)
	if err != nil {
		return err
	}
	if err := f.note("resolvectl llmnr " + link + " " + mode); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	st.LLMNR = mode
	return nil
}

func (f *Fake) ResolvectlMDNS(link, mode string) error {
	st, err := f.resolveLink(link)
	if err != nil {
		return err
	}
	if err := f.note("resolvectl mdns " + link + " " + mode); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	st.MDNS = mode
	return nil
}

func (f *Fake) ResolvectlDNSSEC(link, mode string) error {
	st, err := f.resolveLink(link)
	if err != nil {
		return err
	}
	if err := f.note("resolvectl dnssec " + link + " " + mode); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	st.DNSSEC = mode
	return nil
}

func (f *Fake) ResolvectlDNSOverTLS(link, mode string) error {
	st, err := f.resolveLink(link)
	if err != nil {
		return err
	}
	if err := f.note("resolvectl dnsovertls " + link + " " + mode); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	st.DNSOverTLS = mode
	return nil
}

func (f *Fake) ResolvectlRevert(link string) error {
	if err := validateLinkName(link); err != nil {
		return err
	}
	if err := f.note("resolvectl revert " + link); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.ResolveLinks, link)
	return nil
}

func (f *Fake) ResolvectlStatus(link string) (string, error) {
	if link != "" {
		if err := validateLinkName(link); err != nil {
			return "", err
		}
	}
	cmd := "resolvectl status"
	if link != "" {
		cmd += " " + link
	}
	if err := f.note(cmd); err != nil {
		return "", err
	}
	if link == "" {
		return "Global\n\tDNS Servers: 1.1.1.1\n", nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	st := f.ResolveLinks[link]
	out := "Link " + link + "\n"
	if st != nil && len(st.DNS) > 0 {
		out += "\tCurrent DNS Server: " + st.DNS[0] + "\n\tDNS Servers: " + strings.Join(st.DNS, " ") + "\n"
	}
	if st != nil && len(st.Domains) > 0 {
		out += "\tDNS Domain: " + strings.Join(st.Domains, " ") + "\n"
	}
	return out, nil
}

func (f *Fake) ResolvectlDNSGet(link string) ([]string, error) {
	if err := validateLinkName(link); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	st := f.ResolveLinks[link]
	if st == nil {
		return nil, nil
	}
	return append([]string{}, st.DNS...), nil
}

func (f *Fake) ResolvectlDomainGet(link string) ([]string, error) {
	if err := validateLinkName(link); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	st := f.ResolveLinks[link]
	if st == nil {
		return nil, nil
	}
	return append([]string{}, st.Domains...), nil
}

// SystemdVersion reports a plausible version for the fake host.
func (f *Fake) SystemdVersion() (string, error) {
	return "systemd 257 (257.6-fake)", nil
}

// VerifyUnit validates the unit file with the local INI parser. A
// non-systemd-INI body fails verification, as systemd-analyze would.
func (f *Fake) VerifyUnit(name string) (string, error) {
	content, err := f.ReadUnit(name)
	if err != nil {
		return "", err
	}
	if _, perr := unitfile.Parse(content); perr != nil {
		return fmt.Sprintf("fake verify: %s", perr), fmt.Errorf("systemd-analyze verify %s: %w", name, perr)
	}
	return "", nil
}
