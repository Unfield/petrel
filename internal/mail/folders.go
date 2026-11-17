package mail

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/emersion/go-imap/v2/imapclient"
	gomail "github.com/emersion/go-message/mail"
	"github.com/knadh/go-pop3"
)

// FolderProvider is a provider that can make, rename and delete mailboxes:
// IMAP and JMAP, not POP3.
type FolderProvider interface {
	// CreateMailbox makes a mailbox, inside parent if it isn't "".
	CreateMailbox(ctx context.Context, name, parent string) (string, error)
	// RenameMailbox gives a mailbox a new name in the same place, and
	// returns its ID, which IMAP changes.
	RenameMailbox(ctx context.Context, id, name string) (string, error)
	// DeleteMailbox deletes a mailbox with the messages in it.
	DeleteMailbox(ctx context.Context, id string) error
}

// RawProvider gives a message as it was sent, headers and all.
type RawProvider interface {
	Raw(ctx context.Context, id string) (io.ReadCloser, error)
}

// ErrName is a mailbox name the server can't take.
var ErrName = fmt.Errorf("mail: that folder name can't be used")

func checkName(name string) error {
	if name = strings.TrimSpace(name); name == "" || len(name) > 200 || strings.ContainsAny(name, "\r\n\x00") {
		return ErrName
	}
	return nil
}

// delim is the server's hierarchy separator, such as "/" or ".".
func (p *imapProvider) delim(cl *imapclient.Client) string {
	list, err := cl.List("", "", nil).Collect()
	if err == nil && len(list) > 0 && list[0].Delim != 0 {
		return string(list[0].Delim)
	}
	return "/"
}

func (p *imapProvider) CreateMailbox(ctx context.Context, name, parent string) (string, error) {
	if err := checkName(name); err != nil {
		return "", err
	}
	var id string
	err := p.do(ctx, func(cl *imapclient.Client) error {
		d := p.delim(cl)
		if strings.Contains(name, d) {
			return ErrName
		}
		path := strings.TrimSpace(name)
		if parent != "" {
			pb, err := decBox(parent)
			if err != nil {
				return ErrNotFound
			}
			path = pb + d + path
		}
		if err := cl.Create(path, nil).Wait(); err != nil {
			return err
		}
		_ = cl.Subscribe(path).Wait()
		id = encBox(path)
		return nil
	})
	return id, err
}

func (p *imapProvider) RenameMailbox(ctx context.Context, id, name string) (string, error) {
	if err := checkName(name); err != nil {
		return "", err
	}
	old, err := decBox(id)
	if err != nil {
		return "", ErrNotFound
	}
	var newID string
	err = p.do(ctx, func(cl *imapclient.Client) error {
		d := p.delim(cl)
		if strings.Contains(name, d) {
			return ErrName
		}
		path := strings.TrimSpace(name)
		if i := strings.LastIndex(old, d); i >= 0 {
			path = old[:i+1] + path
		}
		if err := cl.Rename(old, path, nil).Wait(); err != nil {
			return err
		}
		_ = cl.Unsubscribe(old).Wait()
		_ = cl.Subscribe(path).Wait()
		newID = encBox(path)
		return nil
	})
	return newID, err
}

func (p *imapProvider) DeleteMailbox(ctx context.Context, id string) error {
	box, err := decBox(id)
	if err != nil {
		return ErrNotFound
	}
	return p.do(ctx, func(cl *imapclient.Client) error {
		_ = cl.Unsubscribe(box).Wait()
		return cl.Delete(box).Wait()
	})
}

func (p *imapProvider) Raw(ctx context.Context, id string) (io.ReadCloser, error) {
	var raw []byte
	err := p.do(ctx, func(cl *imapclient.Client) error {
		_, _, b, err := p.fetchRaw(cl, id)
		raw = b
		return err
	})
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(raw)), nil
}

func (p *jmapProvider) mailboxSet(ctx context.Context, args map[string]any) (map[string]json.RawMessage, error) {
	acc, err := p.accountID(ctx)
	if err != nil {
		return nil, err
	}
	args["accountId"] = acc
	res, err := p.call(ctx, nil, invocation{"Mailbox/set", args, "s"})
	if err != nil {
		return nil, err
	}
	if err := methodError(res["s"]); err != nil {
		return nil, err
	}
	var sr struct {
		Created                              map[string]struct{ ID string } `json:"created"`
		NotCreated, NotUpdated, NotDestroyed map[string]struct {
			Type        string `json:"type"`
			Description string `json:"description"`
		}
	}
	_ = json.Unmarshal(res["s"], &sr)
	for _, m := range []map[string]struct {
		Type        string `json:"type"`
		Description string `json:"description"`
	}{sr.NotCreated, sr.NotUpdated, sr.NotDestroyed} {
		for _, e := range m {
			if e.Type == "notFound" {
				return nil, ErrNotFound
			}
			return nil, fmt.Errorf("mail: the server refused: %s %s", e.Type, e.Description)
		}
	}
	out := map[string]json.RawMessage{}
	for k, v := range sr.Created {
		out[k], _ = json.Marshal(v.ID)
	}
	return out, nil
}

func (p *jmapProvider) CreateMailbox(ctx context.Context, name, parent string) (string, error) {
	if err := checkName(name); err != nil {
		return "", err
	}
	m := map[string]any{"name": strings.TrimSpace(name)}
	if parent != "" {
		m["parentId"] = parent
	}
	res, err := p.mailboxSet(ctx, map[string]any{"create": map[string]any{"m": m}})
	if err != nil {
		return "", err
	}
	var id string
	_ = json.Unmarshal(res["m"], &id)
	return id, nil
}

func (p *jmapProvider) RenameMailbox(ctx context.Context, id, name string) (string, error) {
	if err := checkName(name); err != nil {
		return "", err
	}
	_, err := p.mailboxSet(ctx, map[string]any{"update": map[string]any{id: map[string]any{"name": strings.TrimSpace(name)}}})
	return id, err
}

func (p *jmapProvider) DeleteMailbox(ctx context.Context, id string) error {
	_, err := p.mailboxSet(ctx, map[string]any{"destroy": []string{id}, "onDestroyRemoveEmails": true})
	return err
}

func (p *jmapProvider) Raw(ctx context.Context, id string) (io.ReadCloser, error) {
	e, err := p.getEmail(ctx, id)
	if err != nil {
		return nil, err
	}
	return p.download(ctx, e.BlobID, "message/rfc822", "message.eml")
}

func (p *pop3Provider) Raw(ctx context.Context, id string) (io.ReadCloser, error) {
	uidl := decUIDL(id)
	var raw []byte
	err := p.session(ctx, func(c *pop3.Conn) error {
		n, err := find(c, uidl)
		if err != nil {
			return err
		}
		buf, err := c.RetrRaw(n)
		if err != nil {
			return err
		}
		raw = buf.Bytes()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(raw)), nil
}

// unsubscribeOf reads List-Unsubscribe and List-Unsubscribe-Post.
func unsubscribeOf(h gomail.Header) *Unsubscribe {
	v := h.Get("List-Unsubscribe")
	if v == "" {
		return nil
	}
	var urls []string
	for _, part := range strings.Split(v, ",") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "<") && strings.HasSuffix(part, ">") {
			urls = append(urls, part[1:len(part)-1])
		}
	}
	return unsubscribe(urls, h.Get("List-Unsubscribe-Post"))
}

// unsubscribe picks an https address and a mailto one from the list's.
func unsubscribe(urls []string, post string) *Unsubscribe {
	u := &Unsubscribe{}
	for _, s := range urls {
		s = strings.TrimSpace(s)
		p, err := url.Parse(s)
		if err != nil {
			continue
		}
		switch strings.ToLower(p.Scheme) {
		case "https", "http":
			if u.URL == "" {
				u.URL = s
			}
		case "mailto":
			if u.Mailto == "" {
				u.Mailto = s
			}
		}
	}
	if u.URL == "" && u.Mailto == "" {
		return nil
	}
	// RFC 8058: one click only over https, and only when the list says so.
	u.OneClick = strings.HasPrefix(strings.ToLower(u.URL), "https://") && strings.EqualFold(strings.ReplaceAll(strings.TrimSpace(post), " ", ""), "List-Unsubscribe=One-Click")
	return u
}

// OneClickUnsubscribe asks a mailing list to drop the address, as RFC 8058
// says: a POST to its https address, through the same guard as everything
// else Petrel connects to.
func OneClickUnsubscribe(ctx context.Context, u string, o Options) error {
	if !strings.HasPrefix(strings.ToLower(u), "https://") {
		return ErrUnsupported
	}
	h := httpClient(o.AllowPrivate, 20*time.Second)
	if o.TLS != nil {
		h.Transport.(*http.Transport).TLSClientConfig = o.TLS.Clone()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader("List-Unsubscribe=One-Click"))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Petrel")
	res, err := h.Do(req)
	if err != nil {
		return err
	}
	res.Body.Close()
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("mail: the list answered %s", res.Status)
	}
	return nil
}
