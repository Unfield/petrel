package mail

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// ManageSieve (RFC 5804) is how IMAP servers take Sieve scripts. Petrel
// keeps one script, named "petrel", and makes it the active one.

// ScriptName is the name of the script Petrel writes.
const ScriptName = "petrel"

// SieveStatus is what a server says about its scripts.
type SieveStatus struct {
	// Active is the name of the active script, "" if none.
	Active string `json:"active"`
	// Extensions the server's Sieve supports, such as "vacation".
	Extensions []string `json:"extensions"`
}

// SieveProvider is a provider whose server runs Sieve: IMAP with
// ManageSieve, JMAP with Sieve (RFC 9661).
type SieveProvider interface {
	SieveStatus(ctx context.Context) (*SieveStatus, error)
	// PutSieve checks, stores and activates Petrel's script.
	PutSieve(ctx context.Context, script string) error
}

// ErrSieve is a script the server refused, with its reason.
type ErrSieve struct{ Reason string }

func (e *ErrSieve) Error() string { return "mail: the server refused the filters: " + e.Reason }

// sieveServer is where an IMAP account's ManageSieve is: Settings.Sieve,
// else the incoming server's host on port 4190.
func (p *imapProvider) sieveServer() Server {
	if p.s.Sieve != nil && p.s.Sieve.Host != "" {
		return *p.s.Sieve
	}
	return Server{Host: p.s.Incoming.Host, Port: 4190, Security: StartTLS}
}

type sieveConn struct {
	c    net.Conn
	r    *bufio.Reader
	caps map[string]string
}

func (p *imapProvider) sieve(ctx context.Context) (*sieveConn, error) {
	s := p.sieveServer()
	conn, err := dial(ctx, s, p.o.AllowPrivate, p.o)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupported, err)
	}
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	}
	sc := &sieveConn{c: conn, r: bufio.NewReader(conn)}
	if err := sc.capabilities(); err != nil {
		conn.Close()
		return nil, err
	}
	if s.Security == StartTLS {
		if _, ok := sc.caps["STARTTLS"]; !ok {
			conn.Close()
			return nil, fmt.Errorf("%w: the Sieve server doesn't offer STARTTLS", ErrUnsupported)
		}
		if err := sc.cmd("STARTTLS"); err != nil {
			conn.Close()
			return nil, err
		}
		tc := tls.Client(conn, tlsConfig(s.Host, p.o))
		if err := tc.HandshakeContext(ctx); err != nil {
			conn.Close()
			return nil, err
		}
		sc.c, sc.r = tc, bufio.NewReader(tc)
		// The server says its capabilities again after STARTTLS.
		if err := sc.capabilities(); err != nil {
			tc.Close()
			return nil, err
		}
	}
	var mech, initial string
	if p.c.OAuth != nil {
		tok, err := token(ctx, &p.c, p.o)
		if err != nil {
			sc.close()
			return nil, err
		}
		mech, initial = "XOAUTH2", "user="+p.c.Username+"\x01auth=Bearer "+tok+"\x01\x01"
	} else {
		mech, initial = "PLAIN", "\x00"+p.c.Username+"\x00"+p.c.Password
	}
	if err := sc.cmd(`AUTHENTICATE "` + mech + `" "` + base64.StdEncoding.EncodeToString([]byte(initial)) + `"`); err != nil {
		sc.close()
		if errors.Is(err, errNo) {
			return nil, ErrAuth
		}
		return nil, err
	}
	return sc, nil
}

var errNo = errors.New("managesieve: NO")

func (sc *sieveConn) close() {
	fmt.Fprintf(sc.c, "LOGOUT\r\n")
	sc.c.Close()
}

// line reads a line without its CRLF.
func (sc *sieveConn) line() (string, error) {
	l, err := sc.r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(l, "\r\n"), nil
}

// str reads a string at the start of s: quoted, or a literal {n} whose
// bytes follow the line. It returns the string and what's after it.
func (sc *sieveConn) str(s string) (string, string, error) {
	s = strings.TrimLeft(s, " ")
	switch {
	case strings.HasPrefix(s, `"`):
		var b strings.Builder
		for i := 1; i < len(s); i++ {
			switch s[i] {
			case '\\':
				if i+1 < len(s) {
					i++
					b.WriteByte(s[i])
				}
			case '"':
				return b.String(), s[i+1:], nil
			default:
				b.WriteByte(s[i])
			}
		}
		return "", "", errors.New("managesieve: unterminated string")
	case strings.HasPrefix(s, "{"):
		end := strings.IndexByte(s, '}')
		n, err := strconv.Atoi(strings.TrimSuffix(s[1:end], "+"))
		if end < 0 || err != nil || n < 0 || n > 1<<20 {
			return "", "", errors.New("managesieve: bad literal")
		}
		buf := make([]byte, n)
		if _, err := io.ReadFull(sc.r, buf); err != nil {
			return "", "", err
		}
		rest, err := sc.line()
		return string(buf), rest, err
	}
	return "", s, nil
}

// response reads lines until OK, NO or BYE, returning the other lines and
// the reason given with NO or BYE.
func (sc *sieveConn) response() ([]string, error) {
	var lines []string
	for {
		l, err := sc.line()
		if err != nil {
			return lines, err
		}
		word, rest, _ := strings.Cut(l, " ")
		switch strings.ToUpper(word) {
		case "OK":
			return lines, nil
		case "NO", "BYE":
			// NO (CODE) "reason", where the reason may be a literal.
			if strings.HasPrefix(rest, "(") {
				if i := strings.IndexByte(rest, ')'); i >= 0 {
					rest = rest[i+1:]
				}
			}
			reason, _, _ := sc.str(rest)
			return lines, fmt.Errorf("%w: %s", errNo, reason)
		}
		// A literal's bytes come after the line that announces it.
		if strings.HasSuffix(l, "}") {
			if i := strings.LastIndexByte(l, '{'); i >= 0 {
				s, _, err := sc.str(l[i:])
				if err != nil {
					return lines, err
				}
				l = l[:i] + strconv.Quote(s)
			}
		}
		lines = append(lines, l)
	}
}

func (sc *sieveConn) capabilities() error {
	lines, err := sc.response()
	if err != nil {
		return err
	}
	sc.caps = map[string]string{}
	for _, l := range lines {
		k, rest, _ := sc.str(l)
		v, _, _ := sc.str(rest)
		sc.caps[strings.ToUpper(k)] = v
	}
	return nil
}

func (sc *sieveConn) cmd(c string) error {
	if _, err := fmt.Fprintf(sc.c, "%s\r\n", c); err != nil {
		return err
	}
	_, err := sc.response()
	return err
}

// literal is a client literal: {n+} with the bytes after it.
func literal(s string) string { return "{" + strconv.Itoa(len(s)) + "+}\r\n" + s }

func (p *imapProvider) SieveStatus(ctx context.Context) (*SieveStatus, error) {
	sc, err := p.sieve(ctx)
	if err != nil {
		return nil, err
	}
	defer sc.close()
	st := &SieveStatus{Extensions: strings.Fields(strings.ToLower(sc.caps["SIEVE"]))}
	fmt.Fprintf(sc.c, "LISTSCRIPTS\r\n")
	lines, err := sc.response()
	if err != nil {
		return nil, err
	}
	for _, l := range lines {
		name, rest, _ := sc.str(l)
		if strings.EqualFold(strings.TrimSpace(rest), "ACTIVE") {
			st.Active = name
		}
	}
	return st, nil
}

func (p *imapProvider) PutSieve(ctx context.Context, script string) error {
	sc, err := p.sieve(ctx)
	if err != nil {
		return err
	}
	defer sc.close()
	if _, ok := sc.caps["VERSION"]; ok { // RFC 5804 servers check first
		if err := sc.cmd("CHECKSCRIPT " + literal(script)); err != nil {
			if errors.Is(err, errNo) {
				return &ErrSieve{Reason: strings.TrimPrefix(err.Error(), errNo.Error()+": ")}
			}
			return err
		}
	}
	if err := sc.cmd(`PUTSCRIPT "` + ScriptName + `" ` + literal(script)); err != nil {
		if errors.Is(err, errNo) {
			return &ErrSieve{Reason: strings.TrimPrefix(err.Error(), errNo.Error()+": ")}
		}
		return err
	}
	return sc.cmd(`SETACTIVE "` + ScriptName + `"`)
}
