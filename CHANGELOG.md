# Changelog

All notable changes to Petrel are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and Petrel adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html). Before v1.0.0, minor versions may contain breaking changes; a release that needs steps from you when updating lists them first.

## [Unreleased]

### Added

- `PETREL_EXTRA_PORTS` allows more ports on public addresses, for servers outside the usual ports, such as a calendar server on 8443.

### Fixed

- An empty folder's "All done" message was squeezed into 30 pixels, with its icon and text on top of each other.
- The To field in Compose showed a focus box that the other fields don't have.
- Spacing on the settings pages:
  - **Settings:** the resend-confirmation button sits beside the unconfirmed address, not pressed against the field under it.
  - **Encryption:** "Import private key" sits beside "Create key".
  - **Contacts:** the toolbar buttons have the same height.

## [0.1.0] - 2026-11-17

The first release: a webmail client for any IMAP, JMAP or POP3 mailbox, in English and German. See the [README](README.md#features) for the full feature list and settings.

### Added

- **Mail:**
  - IMAP with IDLE push, JMAP with EventSource push, POP3 with flags kept by Petrel, SMTP submission
  - Gmail and Outlook through OAuth
  - setup finds a mailbox's servers from its address
- **Reading:** a unified inbox, conversations, search with filters, and sanitized HTML with remote images blocked until you allow them.
- **Writing:**
  - rich text, attachments, address suggestions, and drafts saved to the server as you type
  - undo send
  - addresses to send as, each with its own signature and Reply-To
- **Organizing:**
  - archive, delete, spam, move, star, labels and snooze
  - make, rename and delete folders; empty Trash and Spam; mark a folder as read
  - unsubscribe (one click where the list allows it) and block senders
  - print, show the original, download as .eml
- **Filters and vacation replies,** run by the mail server as Sieve: ManageSieve for IMAP, JMAP Sieve and VacationResponse for JMAP.
- **OpenPGP:** encrypt, sign, decrypt and verify, with keys from Web Key Directories, Autocrypt and attachments. Private keys are unlocked on the server; the README says what that means.
- **Calendar:** your provider's CalDAV calendars, with day, week, month and list views. Answer invitations from mail, and Petrel replies to the organizer.
- **Contacts:** an address book with vCard import and export.
- **The rest:**
  - desktop notifications for new mail
  - the storage used and the quota, for each mailbox
  - themes, accent colors, densities and layouts
  - a right-to-left layout
- **Sign-in:** Petrel accounts with passwords, passkeys, two-step codes and GitHub or Google, or direct mode, where people sign in with the mailbox itself (`PETREL_AUTH_MODE=direct`).
- **Running it in public:**
  - mailbox passwords are encrypted at rest
  - no connections to private addresses or non-mail ports
  - failed sign-ins, connection tests and settings lookups are rate-limited
- **Teams,** off unless `PETREL_TEAMS=true`.

### Known limitations

- Filters, vacation replies and calendars don't work for Gmail and Outlook mailboxes: Petrel's sign-in with them covers mail only, and neither offers Sieve.
- POP3 mailboxes have no folders, quota or server-side filters, because POP3 has none.

[Unreleased]: https://github.com/Unfield/petrel/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/Unfield/petrel/releases/tag/v0.1.0
