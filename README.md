<p align="center"><img src="brand/icon.svg" width="72" height="72" alt=""></p>
<h1 align="center">Petrel</h1>
<p align="center">Modern, open-source webmail. JMAP, IMAP, POP3 and SMTP, in one fast inbox you can self-host.</p>

> **Status: early development.** Accounts, sign-in and the app shell work. Connecting mailboxes and the mail view are being built next. The clickable UI prototype lives in [`mockup/`](mockup/).

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

To try the UI prototype without the server:

```sh
python3 -m http.server 4321 --directory mockup
```

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
| `APP_NAME` | the name shown by authenticator apps and passkey prompts |

## Project layout

```
main.go              plugins: migrations, jobs, accounts, sign-in with other providers
internal/app/        database, accounts, connected mail accounts
migrations/          SQL migrations, applied at start
src/routes/          pages (Svelte) and their server code (Go)
  +page.svelte       landing page
  auth.group/        sign in, sign up, two-step code, password reset
  app.group/         the signed-in app: mail, settings, security
src/locales/         translations (en, de)
src/lib/             shared components and Petrel's theme
brand/               logo and app icon (SVG)
mockup/              clickable UI prototype (static HTML)
docs/                design notes
```

## Contributing

Issues and pull requests are welcome. Petrel is early, so for anything bigger than a fix, please open an issue first so we can agree on the approach.

## License

[MIT](LICENSE)
