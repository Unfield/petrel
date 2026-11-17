// Package pgp is Petrel's OpenPGP: keys, encrypting and signing outgoing
// mail as PGP/MIME (RFC 3156), and reading encrypted and signed mail, in
// PGP/MIME or inline. It uses ProtonMail's go-crypto.
package pgp

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/ProtonMail/go-crypto/openpgp/armor"
	"github.com/ProtonMail/go-crypto/openpgp/clearsign"
	pgperrors "github.com/ProtonMail/go-crypto/openpgp/errors"
	"github.com/ProtonMail/go-crypto/openpgp/packet"
)

// Errors users can act on.
var (
	ErrNoKey      = errors.New("pgp: no key can read this")
	ErrPassphrase = errors.New("pgp: wrong passphrase")
	ErrBadKey     = errors.New("pgp: that isn't an OpenPGP key")
)

// maxSize is the largest message or key Petrel reads.
const maxSize = 50 << 20

func config() *packet.Config {
	return &packet.Config{Algorithm: packet.PubKeyAlgoEdDSA, DefaultHash: 0}
}

// Key describes an OpenPGP key.
type Key struct {
	Fingerprint string    `json:"fingerprint"`
	Emails      []string  `json:"emails"`
	Name        string    `json:"name"`
	Created     time.Time `json:"created"`
	Expires     time.Time `json:"expires,omitempty"`
	Private     bool      `json:"private"`
	// CanEncrypt: the key has a usable encryption subkey.
	CanEncrypt bool `json:"canEncrypt"`
}

func describe(e *openpgp.Entity) Key {
	k := Key{Fingerprint: strings.ToUpper(hex.EncodeToString(e.PrimaryKey.Fingerprint)), Created: e.PrimaryKey.CreationTime, Private: e.PrivateKey != nil}
	for _, id := range e.Identities {
		if id.UserId == nil {
			continue
		}
		if id.UserId.Email != "" {
			k.Emails = append(k.Emails, strings.ToLower(id.UserId.Email))
		}
		if k.Name == "" {
			k.Name = id.UserId.Name
		}
	}
	if id := e.PrimaryIdentity(); id != nil && id.SelfSignature != nil && id.SelfSignature.KeyLifetimeSecs != nil && *id.SelfSignature.KeyLifetimeSecs > 0 {
		k.Expires = e.PrimaryKey.CreationTime.Add(time.Duration(*id.SelfSignature.KeyLifetimeSecs) * time.Second)
	}
	_, k.CanEncrypt = e.EncryptionKey(time.Now())
	return k
}

// Generate makes a key for a name and address, Curve25519, locked with
// passphrase, and returns it armored, private and public.
func Generate(name, email, passphrase string) (private, public string, k Key, err error) {
	e, err := openpgp.NewEntity(name, "", email, config())
	if err != nil {
		return "", "", Key{}, err
	}
	k = describe(e)
	if public, err = armorKey(e, false); err != nil {
		return "", "", Key{}, err
	}
	if err := e.EncryptPrivateKeys([]byte(passphrase), nil); err != nil {
		return "", "", Key{}, err
	}
	private, err = armorKey(e, true)
	return private, public, k, err
}

func armorKey(e *openpgp.Entity, private bool) (string, error) {
	var buf bytes.Buffer
	kind := openpgp.PublicKeyType
	if private {
		kind = openpgp.PrivateKeyType
	}
	w, err := armor.Encode(&buf, kind, nil)
	if err != nil {
		return "", err
	}
	if private {
		// Serialize without signing again: the private key is locked.
		err = e.SerializePrivateWithoutSigning(w, nil)
	} else {
		err = e.Serialize(w)
	}
	if err != nil {
		return "", err
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// Read reads one key, armored or binary.
func Read(data string) (*openpgp.Entity, error) {
	var list openpgp.EntityList
	var err error
	// Only armored keys are text: binary ones are read byte for byte.
	if t := strings.TrimSpace(data); strings.HasPrefix(t, "-----BEGIN") {
		list, err = openpgp.ReadArmoredKeyRing(strings.NewReader(t))
	} else {
		list, err = openpgp.ReadKeyRing(strings.NewReader(data))
	}
	if err != nil || len(list) == 0 {
		return nil, ErrBadKey
	}
	return list[0], nil
}

// Inspect describes a key, and returns its public part armored.
func Inspect(data string) (Key, string, error) {
	e, err := Read(data)
	if err != nil {
		return Key{}, "", err
	}
	pub, err := armorKey(e, false)
	return describe(e), pub, err
}

// Unlock opens a private key with its passphrase.
func Unlock(private, passphrase string) (*openpgp.Entity, error) {
	e, err := Read(private)
	if err != nil {
		return nil, err
	}
	if e.PrivateKey == nil {
		return nil, ErrBadKey
	}
	if e.PrivateKey.Encrypted {
		if err := e.DecryptPrivateKeys([]byte(passphrase)); err != nil {
			return nil, ErrPassphrase
		}
	}
	return e, nil
}

// Relock locks an unlocked private key with a passphrase, for storing one
// that was imported.
func Relock(private, oldPassphrase, newPassphrase string) (string, error) {
	e, err := Unlock(private, oldPassphrase)
	if err != nil {
		return "", err
	}
	if err := e.EncryptPrivateKeys([]byte(newPassphrase), nil); err != nil {
		return "", err
	}
	return armorKey(e, true)
}

// AutocryptKeyData is a public key as Autocrypt's keydata: base64 of the
// binary key.
func AutocryptKeyData(public string) (string, error) {
	e, err := Read(public)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := e.Serialize(&buf); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes()), nil
}

// FromAutocrypt reads an Autocrypt header's address and key.
func FromAutocrypt(header string) (email, public string, err error) {
	var keydata string
	for _, part := range strings.Split(header, ";") {
		k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch strings.ToLower(k) {
		case "addr":
			email = strings.ToLower(strings.TrimSpace(v))
		case "keydata":
			keydata = strings.Join(strings.Fields(v), "")
		}
	}
	raw, err := base64.StdEncoding.DecodeString(keydata)
	if err != nil || email == "" {
		return "", "", ErrBadKey
	}
	e, err := Read(string(raw))
	if err != nil {
		return "", "", err
	}
	public, err = armorKey(e, false)
	return email, public, err
}

// Signature is what a message's signature says.
type Signature struct {
	// Status: good, bad, or unknown (signed by a key Petrel doesn't have).
	Status      string `json:"status"`
	Fingerprint string `json:"fingerprint,omitempty"`
	Email       string `json:"email,omitempty"`
}

func signature(signer *openpgp.Entity, keyID uint64, err error) *Signature {
	switch {
	case err == nil && signer != nil:
		k := describe(signer)
		s := &Signature{Status: "good", Fingerprint: k.Fingerprint}
		if len(k.Emails) > 0 {
			s.Email = k.Emails[0]
		}
		return s
	case errors.Is(err, pgperrors.ErrUnknownIssuer) || (signer == nil && err == nil):
		return &Signature{Status: "unknown", Fingerprint: fmt.Sprintf("%016X", keyID)}
	default:
		return &Signature{Status: "bad"}
	}
}

// Decrypt reads an encrypted message, armored or binary, with the private
// keys, checking any signature inside against keyring.
func Decrypt(data []byte, private openpgp.EntityList, keyring openpgp.EntityList) ([]byte, *Signature, error) {
	r := io.Reader(bytes.NewReader(data))
	if bytes.Contains(data[:min(len(data), 200)], []byte("-----BEGIN PGP MESSAGE-----")) {
		start := bytes.Index(data, []byte("-----BEGIN PGP MESSAGE-----"))
		block, err := armor.Decode(bytes.NewReader(data[start:]))
		if err != nil {
			return nil, nil, err
		}
		r = block.Body
	}
	keys := append(append(openpgp.EntityList{}, private...), keyring...)
	md, err := openpgp.ReadMessage(r, keys, nil, nil)
	if err != nil {
		if errors.Is(err, pgperrors.ErrKeyIncorrect) {
			return nil, nil, ErrNoKey
		}
		return nil, nil, err
	}
	plain, err := io.ReadAll(io.LimitReader(md.UnverifiedBody, maxSize))
	if err != nil && md.SignatureError == nil {
		return nil, nil, err
	}
	var sig *Signature
	if md.IsSigned {
		var signer *openpgp.Entity
		if md.SignedBy != nil {
			signer = md.SignedBy.Entity
		}
		sig = signature(signer, md.SignedByKeyId, md.SignatureError)
	}
	return plain, sig, nil
}

// VerifyDetached checks a detached signature (multipart/signed) over the
// signed part's bytes.
func VerifyDetached(signed, sig []byte, keyring openpgp.EntityList) *Signature {
	var signer *openpgp.Entity
	var err error
	if bytes.Contains(sig, []byte("-----BEGIN PGP SIGNATURE-----")) {
		signer, err = openpgp.CheckArmoredDetachedSignature(keyring, bytes.NewReader(signed), bytes.NewReader(sig), nil)
	} else {
		signer, err = openpgp.CheckDetachedSignature(keyring, bytes.NewReader(signed), bytes.NewReader(sig), nil)
	}
	return signature(signer, issuer(sig), err)
}

// issuer is the key ID a signature names, for "signed by an unknown key".
func issuer(sig []byte) uint64 {
	r := io.Reader(bytes.NewReader(sig))
	if bytes.Contains(sig, []byte("-----BEGIN")) {
		if b, err := armor.Decode(bytes.NewReader(sig)); err == nil {
			r = b.Body
		}
	}
	p, err := packet.Read(r)
	if s, ok := p.(*packet.Signature); err == nil && ok && s.IssuerKeyId != nil {
		return *s.IssuerKeyId
	}
	return 0
}

// VerifyClearsigned checks inline signed text and returns the text.
func VerifyClearsigned(data []byte, keyring openpgp.EntityList) ([]byte, *Signature, bool) {
	b, _ := clearsign.Decode(data)
	if b == nil {
		return nil, nil, false
	}
	signer, err := b.VerifySignature(keyring, nil)
	var id uint64
	if b.ArmoredSignature != nil {
		if p, perr := packet.Read(b.ArmoredSignature.Body); perr == nil {
			if s, ok := p.(*packet.Signature); ok && s.IssuerKeyId != nil {
				id = *s.IssuerKeyId
			}
		}
	}
	return b.Plaintext, signature(signer, id, err), true
}

// boundary is a random MIME boundary.
func boundary() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "petrel-" + hex.EncodeToString(b)
}

// EncryptMIME encrypts a MIME entity to the recipients, signed by signer if
// given, as the body of a multipart/encrypted message (RFC 3156 §4).
func EncryptMIME(entity []byte, to openpgp.EntityList, signer *openpgp.Entity) (contentType string, params map[string]string, body []byte, err error) {
	var ct bytes.Buffer
	aw, err := armor.Encode(&ct, "PGP MESSAGE", nil)
	if err != nil {
		return "", nil, nil, err
	}
	pw, err := openpgp.Encrypt(aw, to, signer, nil, nil)
	if err != nil {
		return "", nil, nil, err
	}
	if _, err := pw.Write(entity); err != nil {
		return "", nil, nil, err
	}
	if err := pw.Close(); err != nil {
		return "", nil, nil, err
	}
	if err := aw.Close(); err != nil {
		return "", nil, nil, err
	}
	b := boundary()
	var out bytes.Buffer
	fmt.Fprintf(&out, "This is an OpenPGP/MIME encrypted message (RFC 4880 and 3156)\r\n--%s\r\n", b)
	fmt.Fprintf(&out, "Content-Type: application/pgp-encrypted\r\nContent-Description: PGP/MIME version identification\r\n\r\nVersion: 1\r\n\r\n--%s\r\n", b)
	fmt.Fprintf(&out, "Content-Type: application/octet-stream; name=\"encrypted.asc\"\r\nContent-Description: OpenPGP encrypted message\r\nContent-Disposition: inline; filename=\"encrypted.asc\"\r\n\r\n")
	out.Write(crlf(ct.Bytes()))
	fmt.Fprintf(&out, "\r\n--%s--\r\n", b)
	return "multipart/encrypted", map[string]string{"protocol": "application/pgp-encrypted", "boundary": b}, out.Bytes(), nil
}

// SignMIME signs a MIME entity as multipart/signed (RFC 3156 §5).
func SignMIME(entity []byte, signer *openpgp.Entity) (contentType string, params map[string]string, body []byte, err error) {
	entity = crlf(entity)
	var sig bytes.Buffer
	if err := openpgp.ArmoredDetachSign(&sig, signer, bytes.NewReader(entity), nil); err != nil {
		return "", nil, nil, err
	}
	b := boundary()
	var out bytes.Buffer
	fmt.Fprintf(&out, "This is an OpenPGP/MIME signed message (RFC 4880 and 3156)\r\n--%s\r\n", b)
	out.Write(entity)
	fmt.Fprintf(&out, "\r\n--%s\r\nContent-Type: application/pgp-signature; name=\"signature.asc\"\r\nContent-Description: OpenPGP digital signature\r\nContent-Disposition: attachment; filename=\"signature.asc\"\r\n\r\n", b)
	out.Write(crlf(sig.Bytes()))
	fmt.Fprintf(&out, "\r\n--%s--\r\n", b)
	return "multipart/signed", map[string]string{"protocol": "application/pgp-signature", "micalg": "pgp-sha256", "boundary": b}, out.Bytes(), nil
}

// crlf makes every line end in CRLF, as signed MIME must.
func crlf(b []byte) []byte {
	b = bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
	return bytes.ReplaceAll(b, []byte("\n"), []byte("\r\n"))
}
