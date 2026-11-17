# Petrel (working name)

Modern webmail on Tern (Svelte 5 + Go). Speaks JMAP, IMAP4rev2, POP3 and SMTP submission.

## Decided
- Framework: Tern (single Go binary), starting from the `saas` template
- Backend: one JMAP-shaped `MailProvider` interface with JMAP, IMAP+SMTP and POP3+SMTP adapters
- i18n: Tern translations (JSON per locale in `src/locales`, ICU plural/select). Start with `en` + `de`.
- RTL: ready but not shipped. Use logical CSS only (`margin-inline-start`, `inset-inline-end`,
  `text-align: start`), set `dir` from the locale, test with a pseudo-RTL locale. No Arabic/Hebrew translations yet.
- Theming (level 3):
  - Mode: light / dark / system (`data-theme`)
  - Presets: one CSS file of `--tern-*` + `--petrel-*` tokens each (e.g. Paper, Midnight, High contrast)
  - Accent color picker on top of any preset
  - Density: compact / comfortable / spacious (`data-density`, drives row height, padding, font size)
  - Stored per user; applied before first paint (no flash)

## Sign-in modes (instance setting: `auth.mode` in config / env `PETREL_AUTH_MODE`)
- `direct`: log in with mailbox credentials (Roundcube-style). Petrel checks them against the configured
  server (JMAP session or IMAP LOGIN), creates a Petrel session with exactly one mail account.
  Credentials kept encrypted in the server-side session only, gone at logout/expiry.
- `accounts`: Petrel user accounts (Tern accounts: passwords, passkeys, OAuth). Each user connects one or
  more mail accounts; credentials encrypted at rest (AES-GCM, key from env), OAuth2/XOAUTH2 where possible.
- Both modes resolve to the same thing: `session -> []MailAccount`. Everything after login is shared.
- Settings/preferences: stored per Petrel user in `accounts` mode; per mailbox address in `direct` mode.

## Server discovery
- `direct` mode: admin-configured server list only, matched by email domain
  (e.g. `example.com -> imap.example.com:993 + smtp.example.com:587`, or a JMAP session URL).
  One entry = users just type email + password. No autodiscovery, so the instance can't be used
  to probe arbitrary hosts or brute-force other people's mail servers.
- `accounts` mode: autodiscovery to prefill "connect mailbox" settings, user can edit:
  1. JMAP `/.well-known/jmap` (RFC 8620)
  2. SRV records `_jmap._tcp`, `_imaps._tcp`, `_submissions._tcp`, `_pop3s._tcp` (RFC 6186 / 8314)
  3. Thunderbird autoconfig (domain, then ISPDB)
  4. Common guesses (`imap.<domain>`, `mail.<domain>`)
  Outbound connections guarded: block private/loopback IPs, mail ports only, rate-limited, TLS required by default.

## Built
- Providers: IMAP (IDLE, MOVE/COPYUID, special-use + name heuristics), JMAP (Mailbox, Email, Identity, EmailSubmission, push), POP3 (flags in Petrel's DB), SMTP submission, XOAUTH2; one `Provider` interface. Tested against in-memory servers and a real Stalwart (`scripts/stalwart-test.sh`).
- Both sign-in modes. Accounts mode: connect flow with autodiscovery and Gmail/Outlook OAuth. Direct mode: admin-listed servers by domain.
- Mail app at /mail ported from the mockup: three layouts, density, presets, accent, light/dark/system via light-dark(), en/de, RTL toggle, ⌘K palette, shortcuts, threads, unified inbox, undo, live updates over SSE.
- Writing: compose with attachments, address suggestions, server-side drafts that autosave, delayed send with undo.
- Organizing: labels as keywords, snooze (a Snoozed folder plus a job that wakes mail), move, spam.
- Mail HTML: bluemonday + sandboxed iframe with CSP; remote images opt-in per message or per sender.
- Account pages, server errors and Tern UI texts in en/de.
- Limits: failed mailbox sign-ins per address and client, settings lookups and connection tests per user.

## Decisions made along the way
- Appearance prefs in a cookie (not localStorage) so the server renders the right theme: no flash, no inline script under Tern's CSP.
- Undo send = the browser waits 5 s before posting.
- Starred = flagged messages across Inbox, Archive and Sent (IMAP) / everything but Trash and Junk (JMAP).
- No auto-opening the first message: opening marks as read, which should be the reader's choice.
- `cmd/devmail` instead of Docker for local development.
- Everyone gets a personal team on first use; teams stay hidden unless `PETREL_TEAMS=true`. A webmail user shouldn't have to know what a team is.
- In direct mode the mailbox's provider owns the address and password, so Petrel hides email change, password, passkeys and two-step.
- Deleting an account deletes Petrel's data for it (mailboxes and their passwords, contacts, prefs, snoozes, uploads); Tern has no hook for this, so the delete action does it.
