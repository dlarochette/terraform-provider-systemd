//go:build !skip_acc

package provider

import "testing"

// TestAccCredentialPlaintext exercises Client.PutCredential / HasCredential /
// DeleteCredential for a plaintext credential under /etc/credstore.
func TestAccCredentialPlaintext(t *testing.T) {
	c := accClient(t)
	ctx := t.Context()

	const name = "tf-acc-cred-plain"

	if err := c.PutCredential(ctx, name, "s3cr3t", false, ""); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	t.Cleanup(func() {
		if err := c.DeleteCredential(ctx, name, false); err != nil {
			t.Logf("cleanup DeleteCredential %s: %v", name, err)
		}
	})

	exists, err := c.HasCredential(ctx, name, false)
	if err != nil {
		t.Fatalf("HasCredential: %v", err)
	}
	if !exists {
		t.Fatal("expected plaintext credential to exist after PutCredential")
	}

	if err := c.DeleteCredential(ctx, name, false); err != nil {
		t.Fatalf("DeleteCredential: %v", err)
	}
	exists, err = c.HasCredential(ctx, name, false)
	if err != nil {
		t.Fatalf("HasCredential after delete: %v", err)
	}
	if exists {
		t.Fatal("expected credential to be gone after DeleteCredential")
	}
}

// TestAccCredentialEncrypted exercises the encrypted path (systemd-creds
// encrypt under /etc/credstore.encrypted). Encryption depends on machine
// capability (host key auto-provisioning, optional TPM2) that may not be
// available in a minimal nspawn container, so a failure to encrypt skips
// rather than fails the test, per the brief.
func TestAccCredentialEncrypted(t *testing.T) {
	c := accClient(t)
	ctx := t.Context()

	const name = "tf-acc-cred-enc"

	if err := c.PutCredential(ctx, name, "s3cr3t", true, ""); err != nil {
		t.Skipf("systemd-creds encrypt not available on this machine: %v", err)
	}
	t.Cleanup(func() {
		if err := c.DeleteCredential(ctx, name, true); err != nil {
			t.Logf("cleanup DeleteCredential %s: %v", name, err)
		}
	})

	exists, err := c.HasCredential(ctx, name, true)
	if err != nil {
		t.Fatalf("HasCredential: %v", err)
	}
	if !exists {
		t.Fatal("expected encrypted credential to exist after PutCredential")
	}
}
