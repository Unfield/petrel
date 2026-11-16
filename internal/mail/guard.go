package mail

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"syscall"
	"time"
)

// Users type in the servers Petrel connects to. Without a guard, anyone
// could point Petrel at the cloud's metadata service, the database, or an
// admin panel on localhost, or use it to scan networks. The guard checks
// the address actually dialed, after DNS, so names pointing inward fail.

// ErrBlocked is returned for private addresses and ports that aren't for mail.
var ErrBlocked = errors.New("mail: that server's address isn't allowed")

var mailPorts = map[int]bool{
	25: true, 110: true, 143: true, 465: true, 587: true, 993: true, 995: true, 2525: true,
	80: true, 443: true, // JMAP and autoconfig over HTTPS
}

func dialer(allowPrivate bool) *net.Dialer {
	return &net.Dialer{
		Timeout:   15 * time.Second,
		KeepAlive: 30 * time.Second,
		Control: func(_, address string, _ syscall.RawConn) error {
			if allowPrivate {
				return nil
			}
			ap, err := netip.ParseAddrPort(address)
			if err != nil {
				return ErrBlocked
			}
			ip := ap.Addr().Unmap()
			if !mailPorts[int(ap.Port())] || !ip.IsGlobalUnicast() || ip.IsPrivate() || ip.IsLoopback() ||
				ip.IsLinkLocalUnicast() || ip.IsMulticast() || ip.IsUnspecified() || isShared(ip) {
				return ErrBlocked
			}
			return nil
		},
	}
}

// isShared is 100.64.0.0/10, carrier-grade NAT, which IsPrivate leaves out.
func isShared(ip netip.Addr) bool {
	return ip.Is4() && netip.MustParsePrefix("100.64.0.0/10").Contains(ip)
}

func addr(s Server) string { return net.JoinHostPort(s.Host, strconv.Itoa(s.Port)) }

func tlsConfig(host string, o ...Options) *tls.Config {
	if len(o) > 0 && o[0].TLS != nil {
		c := o[0].TLS.Clone()
		if c.ServerName == "" {
			c.ServerName = host
		}
		return c
	}
	return &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}
}

// dial connects to a server as its Security says.
func dial(ctx context.Context, s Server, allowPrivate bool, o ...Options) (net.Conn, error) {
	if s.Security == Plain && !allowPrivate {
		return nil, fmt.Errorf("mail: unencrypted connections are only allowed for local test servers")
	}
	d := dialer(allowPrivate)
	if s.Security == TLS {
		td := &tls.Dialer{NetDialer: d, Config: tlsConfig(s.Host, o...)}
		return td.DialContext(ctx, "tcp", addr(s))
	}
	return d.DialContext(ctx, "tcp", addr(s))
}

// httpClient is for JMAP and autoconfig: guarded, with time and size
// limits on what it reads.
func httpClient(allowPrivate bool, timeout time.Duration) *http.Client {
	tr := &http.Transport{
		Proxy:               nil,
		DialContext:         dialer(allowPrivate).DialContext,
		TLSHandshakeTimeout: 10 * time.Second,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
	}
	return &http.Client{
		Transport: tr,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("mail: too many redirects")
			}
			if req.URL.Scheme != "https" && !allowPrivate {
				return ErrBlocked
			}
			return nil
		},
	}
}

// limited reads at most n bytes of a response.
func limited(r io.Reader, n int64) io.Reader { return io.LimitReader(r, n) }
