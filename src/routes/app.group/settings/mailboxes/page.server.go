package mailboxes

import (
	"net/http"
	"strings"

	"github.com/Unfield/Tern/kit"

	"petrel/internal/app"
	"petrel/internal/mail"
)

type Page struct {
	Accounts     []app.MailAccount  `json:"accounts"`
	AllowPrivate bool               `json:"allowPrivate"`
	Direct       bool               `json:"direct"`
	OAuth        []app.MailProvider `json:"oauth"`
	Connected    string             `json:"connected"`
	Error        string             `json:"error"`
}

func Load(ctx *kit.Ctx) (Page, error) {
	list, err := app.Current.Mail.List(ctx, app.UserID(ctx))
	q := ctx.URL().Query()
	return Page{Accounts: list, AllowPrivate: app.Current.Mail.AllowPrivate(), Direct: app.Current.Direct(),
		OAuth: app.Current.OAuthProviders(), Connected: q.Get("connected"), Error: q.Get("error")}, err
}

type Connect struct {
	Email        string `form:"email" validate:"required,email,max=254" label:"Email address"`
	Name         string `form:"name" validate:"max=80" label:"Your name"`
	Password     string `form:"password" validate:"required,max=500" label:"Password"`
	Protocol     string `form:"protocol" validate:"required,oneof=imap|jmap|pop3"`
	Username     string `form:"username" validate:"max=254"`
	Host         string `form:"host" validate:"max=253"`
	Port         int    `form:"port" validate:"max=65535"`
	Security     string `form:"security" validate:"oneof=tls|starttls|plain"`
	SMTPHost     string `form:"smtpHost" validate:"max=253" label:"SMTP server"`
	SMTPPort     int    `form:"smtpPort" validate:"max=65535" label:"SMTP port"`
	SMTPSecurity string `form:"smtpSecurity" validate:"oneof=tls|starttls|plain"`
	JMAPURL      string `form:"jmapUrl" validate:"max=500" label:"JMAP session URL"`
}

type Remove struct {
	ID int64 `form:"id" validate:"required"`
}

type Done struct {
	Done  string `json:"done"`
	Email string `json:"email"`
}

var Actions = kit.Actions{
	"connect": kit.FormAction(func(ctx *kit.Ctx, in Connect) (Done, error) {
		if app.Current.Direct() {
			return Done{}, kit.FormError("This Petrel reads the mailbox you signed in with.")
		}
		s := mail.Settings{Protocol: mail.Protocol(in.Protocol), Username: strings.TrimSpace(in.Username)}
		if s.Username == "" {
			s.Username = in.Email
		}
		if s.Protocol == mail.JMAP {
			if !strings.HasPrefix(in.JMAPURL, "https://") && !(app.Current.Mail.AllowPrivate() && strings.HasPrefix(in.JMAPURL, "http://")) {
				return Done{}, kit.FieldError("jmapUrl", "Enter the JMAP session URL, starting with https://.")
			}
			s.JMAPURL = in.JMAPURL
		} else {
			if in.Host == "" || in.Port == 0 {
				return Done{}, kit.FieldError("host", "Enter the incoming server and its port.")
			}
			if in.SMTPHost == "" || in.SMTPPort == 0 {
				return Done{}, kit.FieldError("smtpHost", "Enter the SMTP server and its port, to send mail.")
			}
			s.Incoming = mail.Server{Host: strings.TrimSpace(in.Host), Port: in.Port, Security: mail.Security(in.Security)}
			s.SMTP = mail.Server{Host: strings.TrimSpace(in.SMTPHost), Port: in.SMTPPort, Security: mail.Security(in.SMTPSecurity)}
		}
		c := mail.Credentials{Username: s.Username, Password: in.Password}
		if err := app.Current.Mail.Test(ctx, s, c); err != nil {
			var ke *kit.HTTPError
			if e := app.MailError(err); asHTTP(e, &ke) {
				return Done{}, kit.FormError(ke.Message)
			}
			return Done{}, kit.FormError("Couldn’t connect: " + err.Error())
		}
		if _, err := app.Current.Mail.Create(ctx, app.UserID(ctx), in.Email, strings.TrimSpace(in.Name), s, c); err != nil {
			return Done{}, kit.FormError(err.Error())
		}
		return Done{Done: "connected", Email: in.Email}, nil
	}),
	"remove": kit.FormAction(func(ctx *kit.Ctx, in Remove) (Done, error) {
		if err := app.Current.Mail.Delete(ctx, app.UserID(ctx), in.ID); err != nil {
			return Done{}, kit.Error(http.StatusNotFound, "That mailbox isn’t connected.")
		}
		return Done{Done: "removed"}, nil
	}),
}

func asHTTP(err error, target **kit.HTTPError) bool {
	e, ok := err.(*kit.HTTPError)
	if ok {
		*target = e
	}
	return ok
}
