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
