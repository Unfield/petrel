package mail

import "github.com/emersion/go-sasl"

// xoauth2 is the SASL mechanism Gmail and Outlook use for OAuth tokens.
type xoauth2 struct{ user, token string }

func (x xoauth2) Start() (string, []byte, error) {
	return "XOAUTH2", []byte("user=" + x.user + "\x01auth=Bearer " + x.token + "\x01\x01"), nil
}

// Next answers an error challenge with an empty response, as the
// mechanism requires; the server then fails the command.
func (x xoauth2) Next([]byte) ([]byte, error) { return []byte{}, nil }

var _ sasl.Client = xoauth2{}
