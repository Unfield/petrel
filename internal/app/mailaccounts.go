package app

import (
	"context"
	"database/sql"
)

// MailAccount is a mailbox connected to Petrel. Its server settings and
// encrypted credentials stay on the server and are never sent to the page.
type MailAccount struct {
	ID          int64  `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"displayName"`
	Protocol    string `json:"protocol"`
	Color       string `json:"color"`
}

// MailAccounts stores the mail_accounts table (migrations/).
type MailAccounts struct{ db *sql.DB }

// List returns the user's mailboxes in the order they arranged them.
func (m *MailAccounts) List(ctx context.Context, userID int64) ([]MailAccount, error) {
	rows, err := m.db.QueryContext(ctx,
		`SELECT id, email, display_name, protocol, color FROM mail_accounts WHERE user_id = ? ORDER BY position, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []MailAccount{}
	for rows.Next() {
		var a MailAccount
		if err := rows.Scan(&a.ID, &a.Email, &a.DisplayName, &a.Protocol, &a.Color); err != nil {
			return nil, err
		}
		list = append(list, a)
	}
	return list, rows.Err()
}
