// Package folders makes, renames, deletes and empties folders.
package folders

import (
	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
)

var Access = kit.SignedIn

type In struct {
	Op string `json:"op" validate:"required,oneof=create|rename|delete|empty|readAll"`
	// Account is where a folder is created.
	Account int64 `json:"account"`
	// Folder is the global ID of the folder to change.
	Folder string `json:"folder"`
	// Parent is the folder to create one in, if any.
	Parent string `json:"parent"`
	Name   string `json:"name" validate:"max=200"`
}

type Out struct {
	Folder  string `json:"folder,omitempty"`
	Changed int    `json:"changed"`
}

func POST(ctx *kit.Ctx, in In) (Out, error) {
	m, user := app.Current.Mail, app.UserID(ctx)
	switch in.Op {
	case "create":
		id, err := m.CreateFolder(ctx, user, in.Account, in.Name, in.Parent)
		return Out{Folder: id}, err
	case "rename":
		id, err := m.RenameFolder(ctx, user, in.Folder, in.Name)
		return Out{Folder: id}, err
	case "delete":
		return Out{}, m.DeleteFolder(ctx, user, in.Folder)
	case "empty":
		n, err := m.EmptyFolder(ctx, user, in.Folder)
		return Out{Changed: n}, err
	default:
		n, err := m.MarkAllRead(ctx, user, in.Folder)
		return Out{Changed: n}, err
	}
}
