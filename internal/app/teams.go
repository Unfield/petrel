package app

import (
	"strings"

	"github.com/Unfield/Tern/accounts"
	"github.com/Unfield/Tern/kit"
)

// PersonalTeam is the signed-in user's current team. Someone without one
// (signed in with their mailbox or Google, or after leaving their last
// team) gets a team of their own and is put in it, rather than being sent
// to make one: a webmail user shouldn't have to know teams exist.
func PersonalTeam(ctx *kit.Ctx) (*accounts.Membership, error) {
	acc := Current.Accounts
	u, err := acc.Require(ctx)
	if err != nil {
		return nil, err
	}
	if m := accounts.CurrentTeam(ctx); m != nil {
		return m, nil
	}
	// CurrentTeam is also nil when loading failed; only make a team when
	// the user truly has none.
	teams, err := acc.Teams(ctx)
	if err != nil {
		return nil, err
	}
	if len(teams) > 0 {
		if err := acc.SwitchTeam(ctx, teams[0].Team.ID); err != nil {
			return nil, err
		}
		return &teams[0], nil
	}
	return acc.CreateTeam(ctx, TeamName(u.Name, u.Email))
}

// TeamName is the name of a user's own team.
func TeamName(name, email string) string {
	if name = strings.TrimSpace(name); name == "" {
		name, _, _ = strings.Cut(email, "@")
	}
	return name + "’s team"
}
