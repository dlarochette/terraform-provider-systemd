package provider

import (
	"context"
	"fmt"

	"github.com/dlarochette/terraform-provider-systemd/internal/remote"
)

// Client wraps a remote.Host with Terraform-oriented unit/network helpers.
type Client struct {
	Host remote.Host
}

func (c *Client) PutUnit(ctx context.Context, name, content string, enable, active *bool) error {
	if err := c.Host.WriteUnit(name, content); err != nil {
		return err
	}
	if err := c.Host.DaemonReload(); err != nil {
		return err
	}
	return c.ApplyUnitLifecycle(ctx, name, enable, active)
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
