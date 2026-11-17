package pgp

import (
	"bufio"
	"bytes"
	"errors"
	"mime"
	"strings"

	"github.com/ProtonMail/go-crypto/openpgp"
	"github.com/emersion/go-message/textproto"
)

// Opened is what Open found in a message.
type Opened struct {
	// Kind is "encrypted", "signed" or "" when the message is neither.
	Kind string
	// Inner is the MIME entity inside: decrypted, or the part that was
	// signed. Nil when an encrypted message couldn't be decrypted.
	Inner     []byte
	Signature *Signature
	// Err says why an encrypted message couldn't be read.
	Err error
}

// split cuts a multipart body into its parts, each exactly as sent
// (headers and body), as a signature covers them.
func split(body []byte, boundary string) [][]byte {
	delim := []byte("--" + boundary)
	var parts [][]byte
	// Every delimiter starts a line.
	idx := []int{}
	for i := 0; i < len(body); {
		j := bytes.Index(body[i:], delim)
		if j < 0 {
			break
		}
		at := i + j
		if at == 0 || body[at-1] == '\n' {
			idx = append(idx, at)
		}
		i = at + len(delim)
	}
	for k := 0; k+1 < len(idx); k++ {
		start := idx[k] + len(delim)
		// Skip the rest of the delimiter line.
		if nl := bytes.IndexByte(body[start:], '\n'); nl >= 0 {
			start += nl + 1
		}
		end := idx[k+1]
		// The line break before a delimiter belongs to the delimiter.
		if end > 0 && body[end-1] == '\n' {
			end--
			if end > 0 && body[end-1] == '\r' {
				end--
			}
		}
		if end >= start {
			parts = append(parts, body[start:end])
		}
	}
	return parts
}

// header reads a MIME entity's header and returns its body.
func header(entity []byte) (textproto.Header, []byte, error) {
	br := bufio.NewReader(bytes.NewReader(entity))
	h, err := textproto.ReadHeader(br)
	if err != nil {
		return textproto.Header{}, nil, err
	}
	rest := entity[len(entity)-br.Buffered():]
	return h, rest, nil
}

// Open reads a raw message as PGP/MIME: decrypting it with private, and
// checking signatures against keyring.
func Open(raw []byte, private, keyring openpgp.EntityList) Opened {
	h, body, err := header(raw)
	if err != nil {
		return Opened{}
	}
	ct, params, err := mime.ParseMediaType(h.Get("Content-Type"))
	if err != nil {
		return Opened{}
	}
	switch {
	case ct == "multipart/encrypted" && strings.EqualFold(params["protocol"], "application/pgp-encrypted"):
		o := Opened{Kind: "encrypted"}
		parts := split(body, params["boundary"])
		if len(parts) < 2 {
			o.Err = errors.New("pgp: the encrypted message is incomplete")
			return o
		}
		_, data, err := header(parts[1])
		if err != nil {
			o.Err = err
			return o
		}
		if len(private) == 0 {
			o.Err = ErrNoKey
			return o
		}
		inner, sig, err := Decrypt(data, private, keyring)
		if err != nil {
			o.Err = err
			return o
		}
		o.Inner, o.Signature = inner, sig
		// Encrypted, then signed inside as multipart/signed (rather than in
		// the encryption): check that too.
		if in := Open(inner, private, keyring); in.Kind == "signed" {
			o.Inner, o.Signature = in.Inner, in.Signature
		}
		return o
	case ct == "multipart/signed" && strings.EqualFold(params["protocol"], "application/pgp-signature"):
		parts := split(body, params["boundary"])
		if len(parts) < 2 {
			return Opened{}
		}
		_, sig, err := header(parts[1])
		if err != nil {
			return Opened{}
		}
		return Opened{Kind: "signed", Inner: parts[0], Signature: VerifyDetached(crlf(parts[0]), sig, keyring)}
	}
	return Opened{}
}
