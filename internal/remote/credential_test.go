package remote

import "testing"

func TestFakeCredentialRoundTrip(t *testing.T) {
	f := NewFake()
	if err := f.WriteCredential("db", "secret"); err != nil {
		t.Fatal(err)
	}
	ok, err := f.CredentialExists("db", false)
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if err := f.WriteCredentialEncrypted("db", "secret", "host"); err != nil {
		t.Fatal(err)
	}
	ok, err = f.CredentialExists("db", true)
	if err != nil || !ok {
		t.Fatalf("%v %v", ok, err)
	}
	if err := f.RemoveCredential("db", true); err != nil {
		t.Fatal(err)
	}
	ok, err = f.CredentialExists("db", true)
	if err != nil || ok {
		t.Fatalf("expected removed: %v %v", ok, err)
	}
}
