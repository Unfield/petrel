package mail

import (
	"context"
	"encoding/xml"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Discovered is what autodiscovery found for an address, for the user to
// check before connecting.
type Discovered struct {
	Settings Settings `json:"settings"`
	// Source says where the settings came from: "jmap", "autoconfig",
	// "ispdb", "srv" or "guess".
	Source string `json:"source"`
}

// Discover finds the servers for an email address. It tries, in order:
// JMAP's well-known address, the domain's own autoconfig file, Thunderbird's
// database of providers, DNS SRV records, and finally common host names.
func Discover(ctx context.Context, email string, allowPrivate bool) Discovered {
	ctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	domain := domainOf(email)
	local := email
	if i := strings.LastIndexByte(email, '@'); i >= 0 {
		local = email[:i]
	}
	client := httpClient(allowPrivate, 5*time.Second)

	if u, ok := findJMAP(ctx, client, domain); ok {
		return Discovered{Source: "jmap", Settings: Settings{Protocol: JMAP, Username: email, JMAPURL: u}}
	}
	for _, src := range []struct{ name, url string }{
		{"autoconfig", "https://autoconfig." + domain + "/mail/config-v1.1.xml?emailaddress=" + url.QueryEscape(email)},
		{"autoconfig", "https://" + domain + "/.well-known/autoconfig/mail/config-v1.1.xml?emailaddress=" + url.QueryEscape(email)},
		{"ispdb", "https://autoconfig.thunderbird.net/v1.1/" + url.PathEscape(domain)},
	} {
		if s, ok := autoconfig(ctx, client, src.url, email, local, domain); ok {
			return Discovered{Source: src.name, Settings: s}
		}
	}
	if s, ok := srv(ctx, domain, email); ok {
		return Discovered{Source: "srv", Settings: s}
	}
	return Discovered{Source: "guess", Settings: Settings{
		Protocol: IMAP, Username: email,
		Incoming: Server{Host: "imap." + domain, Port: 993, Security: TLS},
		SMTP:     Server{Host: "smtp." + domain, Port: 587, Security: StartTLS},
	}}
}

func findJMAP(ctx context.Context, client *http.Client, domain string) (string, bool) {
	u := "https://" + domain + "/.well-known/jmap"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", false
	}
	res, err := client.Do(req)
	if err != nil {
		return "", false
	}
	res.Body.Close()
	// Without credentials a JMAP server answers 401 and asks for them.
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusOK {
		if strings.Contains(strings.ToLower(res.Header.Get("WWW-Authenticate")+res.Header.Get("Content-Type")), "basic") ||
			strings.Contains(res.Header.Get("Content-Type"), "json") || res.StatusCode == http.StatusUnauthorized {
			return res.Request.URL.String(), true
		}
	}
	return "", false
}

type clientConfig struct {
	Provider struct {
		Incoming []xmlServer `xml:"incomingServer"`
		Outgoing []xmlServer `xml:"outgoingServer"`
	} `xml:"emailProvider"`
}

type xmlServer struct {
	Type     string `xml:"type,attr"`
	Hostname string `xml:"hostname"`
	Port     int    `xml:"port"`
	Socket   string `xml:"socketType"`
	Username string `xml:"username"`
}

func autoconfig(ctx context.Context, client *http.Client, u, email, local, domain string) (Settings, bool) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Settings{}, false
	}
	res, err := client.Do(req)
	if err != nil {
		return Settings{}, false
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Settings{}, false
	}
	var cfg clientConfig
	if err := xml.NewDecoder(limited(res.Body, 256<<10)).Decode(&cfg); err != nil {
		return Settings{}, false
	}
	fill := func(s string) string {
		return strings.NewReplacer("%EMAILADDRESS%", email, "%EMAILLOCALPART%", local, "%EMAILDOMAIN%", domain).Replace(s)
	}
	sec := func(s string) Security {
		switch strings.ToUpper(s) {
		case "SSL", "TLS":
			return TLS
		case "STARTTLS":
			return StartTLS
		}
		return Plain
	}
	var out Settings
	// Prefer IMAP over POP3, and encrypted servers over plain ones.
	for _, want := range []string{"imap", "pop3"} {
		for _, in := range cfg.Provider.Incoming {
			if in.Type == want && sec(in.Socket) != Plain && out.Protocol == "" {
				out.Protocol = Protocol(want)
				out.Incoming = Server{Host: fill(in.Hostname), Port: in.Port, Security: sec(in.Socket)}
				out.Username = fill(in.Username)
			}
		}
	}
	for _, o := range cfg.Provider.Outgoing {
		if o.Type == "smtp" && sec(o.Socket) != Plain && out.SMTP.Host == "" {
			out.SMTP = Server{Host: fill(o.Hostname), Port: o.Port, Security: sec(o.Socket)}
		}
	}
	if out.Protocol == "" || out.SMTP.Host == "" {
		return Settings{}, false
	}
	if out.Username == "" {
		out.Username = email
	}
	return out, true
}

// srv reads RFC 6186 and RFC 8314 records.
func srv(ctx context.Context, domain, email string) (Settings, bool) {
	var r net.Resolver
	look := func(service string) (string, int, bool) {
		_, recs, err := r.LookupSRV(ctx, service, "tcp", domain)
		if err != nil || len(recs) == 0 || recs[0].Target == "." {
			return "", 0, false
		}
		return strings.TrimSuffix(recs[0].Target, "."), int(recs[0].Port), true
	}
	s := Settings{Username: email}
	if h, p, ok := look("imaps"); ok {
		s.Protocol, s.Incoming = IMAP, Server{Host: h, Port: p, Security: TLS}
	} else if h, p, ok := look("imap"); ok {
		s.Protocol, s.Incoming = IMAP, Server{Host: h, Port: p, Security: StartTLS}
	} else if h, p, ok := look("pop3s"); ok {
		s.Protocol, s.Incoming = POP3, Server{Host: h, Port: p, Security: TLS}
	} else {
		return Settings{}, false
	}
	if h, p, ok := look("submissions"); ok {
		s.SMTP = Server{Host: h, Port: p, Security: TLS}
	} else if h, p, ok := look("submission"); ok {
		s.SMTP = Server{Host: h, Port: p, Security: StartTLS}
	} else {
		s.SMTP = Server{Host: s.Incoming.Host, Port: 587, Security: StartTLS}
	}
	return s, true
}

// PortSecurity guesses the security a well-known port uses.
func PortSecurity(port int) Security {
	switch port {
	case 993, 995, 465:
		return TLS
	case 143, 110, 587, 25:
		return StartTLS
	}
	return TLS
}

var _ = strconv.Itoa
