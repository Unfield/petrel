package pgp

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ProtonMail/go-crypto/openpgp"
)

func keys(t *testing.T, name, email string) (*openpgp.Entity, string) {
	t.Helper()
	priv, pub, k, err := Generate(name, email, "secret "+name)
	if err != nil {
		t.Fatal(err)
	}
	if k.Emails[0] != email || !k.CanEncrypt || !k.Private {
		t.Fatalf("key: %+v", k)
	}
	if _, err := Unlock(priv, "wrong"); err != ErrPassphrase {
		t.Errorf("wrong passphrase: %v", err)
	}
	e, err := Unlock(priv, "secret "+name)
	if err != nil {
		t.Fatal(err)
	}
	return e, pub
}

func TestRoundTrip(t *testing.T) {
	ada, adaPub := keys(t, "Ada", "ada@example.test")
	bob, bobPub := keys(t, "Bob", "bob@example.test")
	adaKey, _ := Read(adaPub)
	bobKey, _ := Read(bobPub)
	entity := []byte("Content-Type: text/plain; charset=utf-8\r\n\r\nHello Bob,\r\nthe plan is on.\r\n")

	// Encrypted to Bob (and Ada, for her Sent), signed by Ada.
	ct, params, body, err := EncryptMIME(entity, openpgp.EntityList{bobKey, adaKey}, ada)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte("From: ada@example.test\r\nContent-Type: " + ct + "; protocol=\"" + params["protocol"] + "\"; boundary=\"" + params["boundary"] + "\"\r\n\r\n")
	raw = append(raw, body...)
	o := Open(raw, openpgp.EntityList{bob}, openpgp.EntityList{adaKey})
	if o.Kind != "encrypted" || o.Err != nil || !bytes.Equal(o.Inner, entity) {
		t.Fatalf("decrypt: %+v %q", o, o.Inner)
	}
	if o.Signature == nil || o.Signature.Status != "good" || o.Signature.Email != "ada@example.test" {
		t.Errorf("signature: %+v", o.Signature)
	}
	if o := Open(raw, nil, nil); o.Kind != "encrypted" || o.Err != ErrNoKey {
		t.Errorf("locked: %+v", o)
	}
	eve, _ := keys(t, "Eve", "eve@example.test")
	if o := Open(raw, openpgp.EntityList{eve}, nil); o.Err == nil {
		t.Error("someone else read it")
	}

	// Signed only.
	ct, params, body, err = SignMIME(entity, ada)
	if err != nil {
		t.Fatal(err)
	}
	raw = []byte("Content-Type: " + ct + "; micalg=pgp-sha256; protocol=\"" + params["protocol"] + "\"; boundary=\"" + params["boundary"] + "\"\r\n\r\n")
	raw = append(raw, body...)
	if o := Open(raw, nil, openpgp.EntityList{adaKey}); o.Kind != "signed" || o.Signature.Status != "good" || !bytes.Equal(o.Inner, entity) {
		t.Errorf("signed: %+v", o)
	}
	if o := Open(raw, nil, nil); o.Signature.Status != "unknown" {
		t.Errorf("unknown signer: %+v", o.Signature)
	}
	tampered := bytes.Replace(raw, []byte("plan is on"), []byte("plan is off"), 1)
	if o := Open(tampered, nil, openpgp.EntityList{adaKey}); o.Signature.Status != "bad" {
		t.Errorf("tampered: %+v", o.Signature)
	}

	// Autocrypt.
	data, err := AutocryptKeyData(bobPub)
	if err != nil {
		t.Fatal(err)
	}
	email, pub, err := FromAutocrypt("addr=bob@example.test; prefer-encrypt=mutual; keydata=" + data)
	if err != nil || email != "bob@example.test" || !strings.Contains(pub, "PUBLIC KEY") {
		t.Errorf("autocrypt: %q %v", email, err)
	}
	_ = bob
}
