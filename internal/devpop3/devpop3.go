// Package devpop3 is a POP3 server for development and tests. It serves an
// IMAP server's inboxes: each POP3 session signs in to IMAP with the same
// username and password and reads INBOX, so one in-memory IMAP server
// (cmd/devmail) can be read over both protocols.
package devpop3

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"net"
	"slices"
	"strconv"
	"strings"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// Serve answers POP3 on ln, reading mail from the IMAP server at imapAddr.
func Serve(ln net.Listener, imapAddr string) error {
	for {
		c, err := ln.Accept()
		if err != nil {
			return err
		}
		go session(c, imapAddr)
	}
}

type msg struct {
	uid  imap.UID
	size int64
}

func session(conn net.Conn, imapAddr string) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	say := func(format string, args ...any) {
		fmt.Fprintf(w, format+"\r\n", args...)
		w.Flush()
	}
	say("+OK devpop3 ready")
	var user string
	var cl *imapclient.Client
	var msgs []msg
	deleted := map[int]bool{}
	defer func() {
		if cl != nil {
			cl.Close()
		}
	}()
	num := func(arg string) (int, bool) {
		n, err := strconv.Atoi(arg)
		if err != nil || n < 1 || n > len(msgs) || deleted[n] {
			say("-ERR no such message")
			return 0, false
		}
		return n, true
	}
	fetch := func(n int) []byte {
		bufs, err := cl.Fetch(imap.UIDSetNum(msgs[n-1].uid), &imap.FetchOptions{BodySection: []*imap.FetchItemBodySection{{Peek: true}}}).Collect()
		if err != nil || len(bufs) == 0 || len(bufs[0].BodySection) == 0 {
			return nil
		}
		return bufs[0].BodySection[0].Bytes
	}
	multiline := func(b []byte) {
		for _, line := range strings.SplitAfter(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n") {
			if line == "" {
				continue
			}
			line = strings.TrimSuffix(line, "\n")
			if strings.HasPrefix(line, ".") {
				line = "." + line
			}
			w.WriteString(line + "\r\n")
		}
		w.WriteString(".\r\n")
		w.Flush()
	}
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		cmd, arg, _ := strings.Cut(strings.TrimSpace(line), " ")
		switch strings.ToUpper(cmd) {
		case "CAPA":
			say("+OK")
			multiline([]byte("USER\nUIDL\nTOP\n"))
		case "USER":
			user = arg
			say("+OK")
		case "PASS":
			c, err := imapclient.DialInsecure(imapAddr, nil)
			if err == nil {
				err = c.Login(user, arg).Wait()
			}
			if err == nil {
				_, err = c.Select("INBOX", nil).Wait()
			}
			var data *imap.SearchData
			if err == nil {
				data, err = c.UIDSearch(&imap.SearchCriteria{}, nil).Wait()
			}
			if err != nil {
				if c != nil {
					c.Close()
				}
				say("-ERR invalid username or password")
				continue
			}
			cl = c
			uids := data.AllUIDs()
			slices.Sort(uids)
			if len(uids) > 0 {
				bufs, _ := cl.Fetch(imap.UIDSetNum(uids...), &imap.FetchOptions{UID: true, RFC822Size: true}).Collect()
				for _, b := range bufs {
					msgs = append(msgs, msg{uid: b.UID, size: b.RFC822Size})
				}
				slices.SortFunc(msgs, func(a, b msg) int { return int(a.uid) - int(b.uid) })
			}
			say("+OK %d messages", len(msgs))
		case "STAT":
			n, size := 0, int64(0)
			for i, m := range msgs {
				if !deleted[i+1] {
					n++
					size += m.size
				}
			}
			say("+OK %d %d", n, size)
		case "LIST", "UIDL":
			value := func(i int) string {
				if strings.EqualFold(cmd, "UIDL") {
					return strconv.FormatUint(uint64(msgs[i].uid), 10)
				}
				return strconv.FormatInt(msgs[i].size, 10)
			}
			if arg != "" {
				if n, ok := num(arg); ok {
					say("+OK %d %s", n, value(n-1))
				}
				continue
			}
			say("+OK")
			var b bytes.Buffer
			for i := range msgs {
				if !deleted[i+1] {
					fmt.Fprintf(&b, "%d %s\n", i+1, value(i))
				}
			}
			multiline(b.Bytes())
		case "RETR":
			if n, ok := num(arg); ok {
				say("+OK")
				multiline(fetch(n))
			}
		case "TOP":
			a, b, _ := strings.Cut(arg, " ")
			lines, _ := strconv.Atoi(b)
			if n, ok := num(a); ok {
				raw := strings.ReplaceAll(string(fetch(n)), "\r\n", "\n")
				head, body, _ := strings.Cut(raw, "\n\n")
				bl := strings.Split(body, "\n")
				if len(bl) > lines {
					bl = bl[:lines]
				}
				say("+OK")
				multiline([]byte(head + "\n\n" + strings.Join(bl, "\n")))
			}
		case "DELE":
			if n, ok := num(arg); ok {
				deleted[n] = true
				say("+OK")
			}
		case "RSET":
			deleted = map[int]bool{}
			say("+OK")
		case "NOOP":
			say("+OK")
		case "QUIT":
			if cl != nil && len(deleted) > 0 {
				var set imap.UIDSet
				for n := range deleted {
					set.AddNum(msgs[n-1].uid)
				}
				_ = cl.Store(set, &imap.StoreFlags{Op: imap.StoreFlagsAdd, Silent: true, Flags: []imap.Flag{imap.FlagDeleted}}, nil).Close()
				_ = cl.UIDExpunge(set).Close()
			}
			say("+OK bye")
			return
		default:
			say("-ERR unknown command")
		}
	}
}

var _ = io.EOF
