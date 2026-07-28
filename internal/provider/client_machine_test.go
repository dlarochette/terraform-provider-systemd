package provider

import (
	"context"
	"testing"

	"github.com/dlarochette/terraform-provider-systemd/internal/remote"
)

func TestPutDeleteMachine(t *testing.T) {
	t.Parallel()
	f := remote.NewFake()
	c := &Client{Host: f}
	settings := "[Exec]\nBoot=no\n"
	en, act := true, true
	if err := c.PutMachine(context.Background(), "web", "local", "/tmp/a.tar", &settings, &en, &act); err != nil {
		t.Fatal(err)
	}
	st, err := c.GetMachine(context.Background(), "web")
	if err != nil || !st.ImagePresent || st.Settings == "" {
		t.Fatalf("status=%+v err=%v", st, err)
	}
	if err := c.DeleteMachine(context.Background(), "web", true); err != nil {
		t.Fatal(err)
	}
	st, _ = c.GetMachine(context.Background(), "web")
	if st.ImagePresent {
		t.Fatal("image should be gone")
	}
}

func TestPutMachineNilSettingsLeavesFile(t *testing.T) {
	t.Parallel()
	f := remote.NewFake()
	c := &Client{Host: f}
	body := "[Exec]\nBoot=no\n"
	if err := f.WriteNspawnFile("web", body); err != nil {
		t.Fatal(err)
	}
	if err := c.PutMachine(context.Background(), "web", "local", "/tmp/a.tar", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	st, err := c.GetMachine(context.Background(), "web")
	if err != nil || st.Settings != body {
		t.Fatalf("settings should be untouched: %+v %v", st, err)
	}
}
