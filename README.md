# imap-cli

A small, **read-only** IMAP client that prints results as JSON. Built to be used
as a tool by LLMs — e.g. "find all emails from a given sender" — but it's a
perfectly usable CLI on its own.

- **Read-only.** Never deletes, moves, or modifies messages. Bodies are fetched
  with IMAP `PEEK`, so reading a message does **not** mark it as seen.
- **JSON everywhere.** Every invocation prints one JSON object to stdout:
  `{"ok": true, "data": ...}` or `{"ok": false, "error": "..."}`. Diagnostics go
  to stderr; a failed command also exits non-zero.
- **Server-side search.** Filtering happens on the IMAP server, so it works on
  large mailboxes without downloading everything.

## Setup

```sh
cp .env.example .env   # then edit it
go build -o imap-cli .
```

`.env` (see `.env.example`):

```
IMAP_HOST=imap.gmail.com
IMAP_PORT=993
IMAP_USERNAME=you@example.com
IMAP_PASSWORD=your-app-specific-password
IMAP_TLS=true          # true = implicit TLS (993); false = STARTTLS (143)
```

For Gmail/Outlook, create an **app-specific password** — your normal account
password won't work with IMAP when 2FA is on.

## Commands

### `folders` — list mailboxes

```sh
./imap-cli folders          # selectable folders only
./imap-cli folders --all     # include unselectable container folders
```

Each folder has a `name`, a normalized `role` (`inbox`, `sent`, `trash`,
`drafts`, `junk`, `archive`, `all`, `flagged`, `important`, or `""`), a
`selectable` flag, the hierarchy `delimiter`, and the raw IMAP `flags`. By
default, unselectable structural containers (e.g. Gmail's `[Gmail]`) are hidden
since they can't hold mail or be searched; `--all` shows them.

### `search` — find messages (newest first)

```sh
./imap-cli search --from alice@example.com --limit 20
./imap-cli search --subject invoice --since 2026-01-01 --before 2026-04-01
./imap-cli search --folder "[Gmail]/Sent Mail" --to bob@example.com --snippet
./imap-cli search --contains refund --unseen

# Orders from Amazon (or eBay) in the last 24 hours:
./imap-cli search --from amazon --from ebay --since-hours 24 \
    --contains order --contains receipt --contains dispatched
```

Filters: `--from`, `--to`, `--subject` (those headers), `--contains` (any header
or body), `--body` (body only), `--since`/`--before` (`YYYY-MM-DD`),
`--seen`/`--unseen`, `--flagged`.
`--folder` is repeatable: each named mailbox is searched and the results are
merged. (Across folders, results are ordered by received time rather than UID,
since UIDs are only meaningful within a single mailbox.)
Paging: `--limit` (default 50, `0` = no limit), `--offset`.
`--snippet` adds a short body preview (fetches bodies, so it's slower).

**Combining filters — one rule.** Every text/header filter is repeatable.
Repeating the **same** filter ORs its values; **different** filters are ANDed
together. So:

| Command | Meaning |
|---|---|
| `--from amazon --from ebay` | from amazon **OR** from ebay |
| `--from amazon --contains order` | from amazon **AND** contains order |
| `--from amazon --contains order --contains receipt` | from amazon **AND** (order **OR** receipt) |

**Requiring multiple terms (AND).** Repeating a filter ORs by default. For
`--contains`, you can flip that with `--contains-match-all`, which requires
*every* term:

```sh
# messages mentioning BOTH refund AND order, anywhere
./imap-cli search --contains refund --contains order --contains-match-all
```

A single query is either all-OR or all-AND — mixing the two (e.g.
`(a OR b) AND c`) is intentionally unsupported to keep the flags simple. Run
separate queries for that; it's cheap and an LLM can combine the results.

**Time windows.** `--since`/`--before` are date-granular (an IMAP limitation).
For a precise rolling window use `--since-hours N`: it narrows the server-side
scan to the relevant day(s), then trims to the exact cutoff using each message's
server received time.

Each result contains: `uid`, `folder`, `from`, `to`, `subject`, `date`
(sender's Date header), `received` (server receipt time, RFC 3339), `flags`,
`seen`, `size_bytes`, and optionally `snippet`.

### `read` — fetch full messages by UID

```sh
./imap-cli read --uid 4213 --folder INBOX
./imap-cli read --uid 4213 --include-html
./imap-cli read --uid 4213 --uid 4214        # several at once
./imap-cli read --uid 4213,4214,4220         # comma list, one round-trip
```

Returns a JSON array with one entry per requested UID, ordered to match the UIDs
you asked for. Each entry is `{"uid": N, "found": true, "message": {…}}`; a UID
with no matching message comes back as `{"uid": N, "found": false}` rather than
being dropped, so you can always tell which UIDs resolved.

```json
[
  { "uid": 4213, "found": true,  "message": { "subject": "...", "body_text": "..." } },
  { "uid": 4214, "found": false }
]
```

Each `message` has full headers, `body_text` (plain text; HTML is stripped to
text when no plain-text part exists), and `attachments` metadata — `filename`,
`content_type`, and `encoded_size_bytes` (the on-the-wire size IMAP reports;
payloads are **not** downloaded, so the decoded file size isn't known here).
`--include-html` adds the raw HTML body.

Reading is bandwidth-light: only the structure and the text part are fetched, so
attachment payloads never come down the wire. `--uid` is repeatable (or
comma-separated) to read several messages from the same `--folder` at once.

UIDs come from `search` and are stable within a mailbox, so the usual flow is
*search → pick a UID → read → (optionally) fetch an attachment*.

### `attachment` — download one attachment by UID and index

```sh
./imap-cli attachment --uid 4213 --index 1                 # save to a file
./imap-cli attachment --uid 4213 --index 1 --out-dir ~/dl  # choose the directory
./imap-cli attachment --uid 4213 --index 1 --base64        # inline base64 in JSON
./imap-cli attachment --uid 4213 --index 1 --as-text       # extract readable text
```

`--index` is 1-based and refers to the attachment's position in that message's
`attachments` list from `read` (so `--index 1` is the first). Only that one MIME
part is fetched — not the body or the other attachments — and the message is
never marked as seen.

By default the decoded bytes are written under `<temp>/imap-cli` (or `--out-dir`)
and the `path` is returned. `--base64` returns the bytes inline instead.
`--as-text` extracts readable text (PDF and `text/*` formats) so an LLM can read
the content directly:

```json
{ "ok": true, "data": {
    "uid": 4213, "index": 1,
    "filename": "statement.pdf", "content_type": "application/pdf",
    "decoded_size_bytes": 81876,
    "text": "CUSTOMER ID 12570184 ... Activity statement ..." } }
```

`decoded_size_bytes` is the actual size of the bytes produced (the real file
size). It differs from `read`'s `encoded_size_bytes`, which is IMAP's on-the-wire
part size — for base64 attachments the encoded size is ~33% larger.

## Global flags

- `--env <path>` — config file path (default `.env`).
- `--timeout <seconds>` — connection timeout (default 30).
- `--account <name>` — use a named account. Set `IMAP_<NAME>_HOST`, etc. in
  `.env`; any unset value falls back to the default `IMAP_*` vars. Single-account
  setups can ignore this.

## Notes for LLM/tool use

- Parse stdout as JSON; check the top-level `ok` field. On `ok: false`, read
  `error`. The process also exits non-zero on failure.
- Prefer `search` first (cheap, returns headers), then `read` a specific UID for
  full content. Keep `--limit` modest to bound output size.
