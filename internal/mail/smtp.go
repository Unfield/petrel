package mail

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/emersion/go-sasl"
	"github.com/emersion/go-smtp"
)

// sendSMTP submits a message through the account's SMTP server and returns
// the raw message, to file in the Sent mailbox.
func sendSMTP(ctx context.Context, s Server, c Credentials, allowPrivate bool, m Outgoing) ([]byte, error) {
	raw, _, err := build(m, domainOf(m.From.Email))
	if err != nil {
		return nil, err
	}
	conn, err := dial(ctx, s, allowPrivate)
	if err != nil {
		return nil, fmt.Errorf("mail: connecting to %s: %w", s.Host, err)
	}
	_ = conn.SetDeadline(time.Now().Add(2 * time.Minute))
	var client *smtp.Client
	if s.Security == StartTLS {
		client, err = smtp.NewClientStartTLS(conn, tlsConfig(s.Host))
		if err != nil {
			conn.Close()
			return nil, err
		}
	} else {
		client = smtp.NewClient(conn)
	}
	defer client.Close()
	if err := client.Hello("petrel.local"); err != nil {
		return nil, err
	}
	if ok, _ := client.Extension("AUTH"); ok && c.Password != "" {
		if err := client.Auth(sasl.NewPlainClient("", c.Username, c.Password)); err != nil {
			var se *smtp.SMTPError
			if errors.As(err, &se) && se.Code == 535 {
				return nil, ErrAuth
			}
			return nil, err
		}
	}
	var to []string
	for _, list := range [][]Address{m.To, m.Cc, m.Bcc} {
		for _, a := range list {
			to = append(to, a.Email)
		}
	}
	if len(to) == 0 {
		return nil, errors.New("mail: add at least one recipient")
	}
	if err := client.SendMail(m.From.Email, to, bytes.NewReader(raw)); err != nil {
		return nil, err
	}
	_ = client.Quit()
	return raw, nil
}

// savesSent says whether a provider files sent mail itself, so Petrel
// mustn't add a second copy.
func savesSent(host string) bool {
	host = strings.ToLower(host)
	return strings.HasSuffix(host, "gmail.com") || strings.HasSuffix(host, "googlemail.com") ||
		strings.HasSuffix(host, "office365.com") || strings.HasSuffix(host, "outlook.com")
}
