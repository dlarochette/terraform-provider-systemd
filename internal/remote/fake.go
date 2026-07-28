package remote

import (
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
)

// Fake is an in-memory Host for unit tests.
type Fake struct {
	mu       sync.Mutex
	Files    map[string]string
	Commands []string
	FailCmd  string // if a command contains this substring, it fails
	Statuses map[string]UnitStatus
	Links    map[string]LinkStatus
	Images   map[string]bool
	UnitDir  string
	NetDir   string
}

// NewFake returns an empty fake host.
func NewFake() *Fake {
	return &Fake{
		Files:    map[string]string{},
		Statuses: map[string]UnitStatus{},
		Links:    map[string]LinkStatus{},
		Images:   map[string]bool{},
		UnitDir:  DefaultUnitDir,
		NetDir:   DefaultNetworkDir,
	}
}

func (f *Fake) Close() error { return nil }

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
