package provider

import (
	"context"
	"testing"

	"github.com/dlarochette/terraform-provider-systemd/internal/remote"
)

func TestPutDeletePortable(t *testing.T) {
	t.Parallel()
	f := remote.NewFake()
	c := &Client{Host: f}
	en, act := true, false
	if err := c.PutPortable(context.Background(), "app", "local", "/tmp/app.tar", &en, &act); err != nil {
		t.Fatal(err)
	}
	st, err := c.GetPortable(context.Background(), "app")
	if err != nil || !st.ImagePresent || !st.Attached {
		t.Fatalf("%+v %v", st, err)
	}
	if err := c.DeletePortable(context.Background(), "app", true); err != nil {
		t.Fatal(err)
	}
	st, _ = c.GetPortable(context.Background(), "app")
	if st.ImagePresent || st.Attached {
		t.Fatalf("expected gone: %+v", st)
	}
}

func TestPutPortableRejectsOCI(t *testing.T) {
	t.Parallel()
	f := remote.NewFake()
	c := &Client{Host: f}
	err := c.PutPortable(context.Background(), "app", "oci", "docker.io/library/debian:bookworm", nil, nil)
	if err == nil {
		t.Fatal("expected oci rejection")
	}
}
