// Package actions changes messages: read, flag, move, delete.
package actions

import (
	"errors"
	"net/http"

	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
	"petrel/internal/mail"
)

var Access = kit.SignedIn

type In struct {
	IDs    []string `json:"ids"`
	Action string   `json:"action" validate:"required,oneof=read|unread|flag|unflag|archive|trash|junk|inbox|move|delete"`
	// Mailbox is the folder to move to, for "move".
	Mailbox string `json:"mailbox"`
}

type Out struct {
	Changed int `json:"changed"`
	// Moved gives messages' new IDs, where moving changed them.
	Moved map[string]string `json:"moved"`
}

func POST(ctx *kit.Ctx, in In) (Out, error) {
	user := app.UserID(ctx)
	byAccount := map[int64][]string{}
	for _, id := range in.IDs {
		if a, pid, ok := app.SplitID(id); ok {
			byAccount[a] = append(byAccount[a], pid)
		}
	}
	t, f := true, false
	out := Out{Changed: len(in.IDs), Moved: map[string]string{}}
	note := func(account int64, moved map[string]string) {
		for old, now := range moved {
			out.Moved[app.GlobalID(account, old)] = app.GlobalID(account, now)
		}
	}
	for account, ids := range byAccount {
		var moved map[string]string
		p, _, err := app.Current.Mail.Provider(ctx, user, account)
		if err != nil {
			return Out{}, app.MailError(err)
		}
		switch in.Action {
		case "read":
			err = p.SetFlags(ctx, ids, &t, nil)
		case "unread":
			err = p.SetFlags(ctx, ids, &f, nil)
		case "flag":
			err = p.SetFlags(ctx, ids, nil, &t)
		case "unflag":
			err = p.SetFlags(ctx, ids, nil, &f)
		case "delete":
			err = p.Destroy(ctx, ids)
		case "move":
			dest, box, ok := app.SplitID(in.Mailbox)
			if !ok || dest != account {
				return Out{}, kit.Error(http.StatusBadRequest, "Messages can only move within their account.")
			}
			moved, err = p.Move(ctx, ids, box)
		default: // archive, trash, junk, inbox: the account's folder with that role
			role := map[string]string{"archive": mail.RoleArchive, "trash": mail.RoleTrash, "junk": mail.RoleJunk, "inbox": mail.RoleInbox}[in.Action]
			var box string
			box, err = app.Current.Mail.RoleBox(ctx, user, account, role)
			if err == nil {
				moved, err = p.Move(ctx, ids, box)
			}
			// POP3 can't move: putting a message in the trash deletes it.
			if errors.Is(err, mail.ErrUnsupported) && in.Action == "trash" {
				err = p.Destroy(ctx, ids)
			}
			var ke *kit.HTTPError
			if errors.As(err, &ke) && in.Action == "trash" {
				err = p.Destroy(ctx, ids) // no Trash folder: delete
			}
		}
		if err != nil {
			return Out{}, app.MailError(err)
		}
		note(account, moved)
	}
	return out, nil
}
