<p align="center"><img src="brand/icon.svg" width="72" height="72" alt=""></p>
<h1 align="center">Petrel</h1>
<p align="center">Modern, open-source webmail. JMAP, IMAP, POP3 and SMTP, in one fast inbox you can self-host.</p>

> **Status: feature-complete first version.** Petrel reads and sends real mail over JMAP, IMAP and POP3, with drafts, attachments, labels, snooze, address suggestions, live updates, OAuth for Gmail and Outlook, and two sign-in modes. It's young: try it with the dev mail server first, and report what breaks.

## Goals

- **Every protocol:** JMAP (RFC 8620/8621) for speed, IMAP4rev2 and POP3 for everything else, SMTP submission for sending. Every protocol plugs into one JMAP-shaped interface in Go.
- **Two sign-in modes**, chosen per installation:
  - `direct`: log in with your mailbox credentials, like Roundcube.
  - `accounts`: Petrel accounts with passkeys and two-step sign-in, plus any number of connected mailboxes.
- **Private by default:** HTML mail is sandboxed, remote images are blocked until you allow them, and credentials are encrypted at rest.
- **Fast and keyboard-friendly:** Gmail-style shortcuts, a ⌘K command palette, instant actions with undo, and live updates through JMAP push and IMAP IDLE.
- **Make it yours:** light, dark and system mode, theme presets, accent colors, three density levels, three reading-pane layouts, English and German, with a layout ready for right-to-left languages.
- **One binary:** Go and Svelte built with [Tern](https://github.com/Unfield/Tern), with SQLite next to it. Also available as a Docker image.

See [docs/design.md](docs/design.md) for the design decisions so far.

## Develop

You need Go 1.26+ and Node.js 22+.

```sh
npm install
npm run dev        # or: go tool tern dev
```

Open <http://localhost:5173> and create an account. Without `SMTP_HOST`, account emails (verification, password reset) aren't sent: their links are in the server's log.

### A mail server to develop against

`cmd/devmail` is an IMAP and SMTP server in memory, with two mailboxes of sample mail (threads, HTML newsletters with tracking pixels, attachments, inline images, spam). No Docker needed:

```sh
go run ./cmd/devmail
PETREL_ALLOW_PRIVATE_HOSTS=true npm run dev
```

In Petrel, go to **Settings → Mailboxes** and click **Use the dev mail server**. Sign in as `sam@petrel.test` or `sam.taylor@northwind.test`, password `petrel`. Mail you send to either address arrives in its inbox, and a new message arrives a minute after the server starts, so you can see live updates.

### Tests

```sh
go test ./...      # IMAP, POP3, SMTP and the whole API against in-memory servers
go tool tern check # Svelte and TypeScript
```

To also test JMAP, and IMAP, POP3 and SMTP over TLS, against a real mail server, start [Stalwart](https://stalw.art) in Docker and point the tests at it:

```sh
./scripts/stalwart-test.sh
PETREL_JMAP_URL=http://127.0.0.1:8080/.well-known/jmap \
PETREL_JMAP_USER1=sam@petrel.test PETREL_JMAP_USER2=lena@petrel.test PETREL_JMAP_PASSWORD=petrel-jmap-test \
PETREL_REAL_IMAP=127.0.0.1:2993 PETREL_REAL_SMTP=127.0.0.1:2465 PETREL_REAL_POP3=127.0.0.1:2995 \
go test ./internal/mail -run 'JMAP|RealServers' -v
```

Remove the server afterwards with `docker rm -f petrel-stalwart`.

To try the original UI prototype without the server: `python3 -m http.server 4321 --directory mockup`.

## Build

```sh
npm run build      # or: go tool tern build
./build/petrel
```

The binary listens on `PORT` (default 3000) and keeps its data in `app.db` (`DATABASE_PATH`). The Dockerfile builds the same binary into a small image.

## Settings

Petrel reads its settings from the environment, and `tern dev` also from a `.env` file (copy `.env.example`).

| Variable | |
|---|---|
| `BASE_URL` | Petrel's address, such as `https://mail.example.com`: for links in emails and for passkeys |
| `DATABASE_PATH` | the SQLite database, `app.db` by default |
| `SMTP_HOST`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `MAIL_FROM` | how Petrel sends its own account emails |
| `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET`, `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `OAUTH_SECRET` | sign-in with GitHub or Google (optional) |
| `TRUSTED_PROXIES` | your reverse proxies' addresses, comma-separated |
| `PETREL_SECRET_KEY` | at least 32 random characters: encrypts saved mailbox passwords. Without it, Petrel makes `petrel.key` beside the database; back it up with the database |
| `PETREL_ALLOW_PRIVATE_HOSTS` | `true` lets mailboxes on private addresses and any port connect, for local test servers. Never set it on a public instance |
| `PETREL_AUTH_MODE`, `PETREL_DIRECT_SERVERS` | sign in with the mailbox itself: see [Direct sign-in](#direct-sign-in) |
| `PETREL_TEAMS` | `true` shows teams: invitations, roles and a Team page. Off by default; everyone has a team of their own either way |
| `PETREL_CA_FILE` | a PEM file of extra certificate authorities to trust for mail servers, such as a company's own CA or a local test server's self-signed certificate |
| `APP_NAME` | the name shown by authenticator apps and passkey prompts |

## How it works

- **`internal/mail`** talks to mail servers. JMAP, IMAP and POP3 each implement one `Provider` interface, shaped after JMAP: mailboxes with roles, messages with keywords, paged queries.
  - **IMAP:** keeps a connection per account and a second one in IDLE for push.
  - **JMAP:** batches method calls over HTTPS.
  - **POP3:** keeps seen and starred in Petrel's database, because POP3 has neither.
  - **Sending:** SMTP submission for IMAP and POP3, `EmailSubmission` for JMAP.
- **Security:**
  - Mailbox passwords are encrypted at rest with Tern's `sealed`.
  - Every connection to a user-given server goes through a guard that refuses private addresses and non-mail ports.
  - Mail HTML is sanitized, then shown in a sandboxed frame whose Content-Security-Policy blocks scripts and, until you allow them, remote images and tracking pixels.
- **Live updates:** IMAP IDLE (or polling for JMAP and POP3) publishes changes on Tern's `realtime` hub. The open tabs receive them over server-sent events.
- **The mail app** is `/mail`: Svelte 5, ported from the approved prototype. Appearance settings live in a cookie, so the page renders in your theme from the first byte, with no flash.

```
main.go              plugins: migrations, jobs, accounts, sign-in with other providers
cmd/devmail/         IMAP + SMTP server in memory, for development
internal/mail/       protocols: imap.go, jmap.go, pop3.go, smtp.go, discover.go, sanitize.go
internal/app/        database, accounts, mail accounts, the views the API returns
migrations/          SQL migrations, applied at start
src/routes/
  mail/              the mail app
  api/mail/          JSON API: accounts, messages, message, actions, send, attachment, events, discover
  app.group/         settings: mailboxes, profile, security
  auth.group/        sign in, sign up, two-step code, password reset
src/lib/mail/        the mail app's components, store and styles
src/locales/         translations (en, de)
brand/               logo and app icon (SVG)
mockup/              the clickable UI prototype
docs/                design notes
```

## Features

- **Protocols:**
  - **IMAP:** with IDLE push.
  - **JMAP:** with EventSource push and EmailSubmission.
  - **POP3:** over TLS or STARTTLS, with flags kept by Petrel.
  - **SMTP submission.**
  - **OAuth:** XOAUTH2 for Gmail and Outlook.
- **Reading:**
  - unified inbox and conversations
  - search with filters
  - sanitized HTML with remote images blocked
  - attachments and inline images
- **Writing:**
  - rich text, attachments (picker, drag and drop, paste), address suggestions
  - reply, reply all, forward (forward keeps the attachments)
  - drafts that save to the server as you type
  - several names and addresses per mailbox (aliases), each with its own signature and Reply-To
  - undo send
- **Organizing:**
  - archive, delete and spam with undo; move to any folder
  - star; labels, stored as IMAP and JMAP keywords
  - snooze: messages come back to the inbox, unread, on time
- **Yours:**
  - light, dark and system mode; four themes; accent colors; three densities; three layouts
  - English and German, plus a mirrored right-to-left layout
  - storage used and the quota, from IMAP QUOTA or JMAP Quota
  - preferences saved to your account
- **Sign-in:**
  - **Petrel accounts** (default): passwords, passkeys, two-step, GitHub or Google sign-in.
  - **Direct mode:** sign in with the mailbox itself.
- **Safe to run in public:**
  - mailbox passwords encrypted at rest; deleting an account deletes them
  - no connections to private addresses or non-mail ports
  - failed mailbox sign-ins limited per address and per client; settings lookups and connection tests limited per user

### Direct sign-in

Set `PETREL_AUTH_MODE=direct` and list your servers in `PETREL_DIRECT_SERVERS` (JSON, or `@path/to/file.json`). Domain `*` matches any address:

```json
[{ "domain": "example.com", "protocol": "imap",
   "incoming": { "host": "imap.example.com", "port": 993, "security": "tls" },
   "smtp": { "host": "smtp.example.com", "port": 587, "security": "starttls" } }]
```

People then sign in with their email address and its password. Petrel keeps the password, encrypted, only until they sign out.

### Gmail and Outlook

Register an OAuth app with Google or Microsoft and set `PETREL_GOOGLE_CLIENT_ID` / `PETREL_GOOGLE_CLIENT_SECRET` or `PETREL_MICROSOFT_CLIENT_ID` / `PETREL_MICROSOFT_CLIENT_SECRET`. The redirect URL is `<BASE_URL>/api/mail/oauth/callback`. **Connect Gmail** and **Connect Outlook** buttons then appear in Settings → Mailboxes.

## Known gaps

- **Contacts:** there's no address book yet beyond suggestions learned from your mail.

## Contributing

Issues and pull requests are welcome. Petrel is early, so for anything bigger than a fix, please open an issue first so we can agree on the approach.

## License

[MIT](LICENSE)
