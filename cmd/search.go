package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"go-imap-cli/internal/mailbox"
)

var searchFlags struct {
	folder     string
	from       string
	to         string
	subject    string
	body       string
	text       string
	or         []string
	since      string
	before     string
	sinceHours int
	seen       bool
	unseen     bool
	flagged    bool
	limit      int
	offset     int
	snippet    bool
}

const dateLayout = "2006-01-02"

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search messages and return JSON summaries (newest first)",
	Long: `Search a folder using server-side IMAP criteria.

Filters of different kinds are combined with AND. --from/--to/--subject match
those headers; --text/--body match anywhere; --or is repeatable and matches if
ANY of its terms appears (the terms are ORed together, then ANDed with the rest).

Examples:
  imap-cli search --from alice@example.com --limit 20
  imap-cli search --subject invoice --since 2026-01-01

  # Orders from Amazon in the last 24 hours (amazon anywhere in the From header):
  imap-cli search --from amazon --since-hours 24 \
      --or order --or receipt --or dispatched --or "order confirmation"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		p := mailbox.SearchParams{
			Folder:      searchFlags.folder,
			From:        searchFlags.from,
			To:          searchFlags.to,
			Subject:     searchFlags.subject,
			Body:        searchFlags.body,
			Text:        searchFlags.text,
			Or:          searchFlags.or,
			SinceHours:  searchFlags.sinceHours,
			Limit:       searchFlags.limit,
			Offset:      searchFlags.offset,
			WithSnippet: searchFlags.snippet,
		}

		if searchFlags.seen && searchFlags.unseen {
			return fmt.Errorf("--seen and --unseen are mutually exclusive")
		}
		if searchFlags.seen {
			t := true
			p.Seen = &t
		} else if searchFlags.unseen {
			f := false
			p.Seen = &f
		}
		if cmd.Flags().Changed("flagged") {
			p.Flagged = &searchFlags.flagged
		}

		var err error
		if p.Since, err = parseDate(searchFlags.since); err != nil {
			return fmt.Errorf("invalid --since: %w", err)
		}
		if p.Before, err = parseDate(searchFlags.before); err != nil {
			return fmt.Errorf("invalid --before: %w", err)
		}

		withClient(func(c *mailbox.Client) (any, error) {
			return c.Search(p)
		})
		return nil
	},
}

func parseDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(dateLayout, s)
}

func init() {
	f := searchCmd.Flags()
	f.StringVar(&searchFlags.folder, "folder", "INBOX", "mailbox to search")
	f.StringVar(&searchFlags.from, "from", "", "match the From header")
	f.StringVar(&searchFlags.to, "to", "", "match the To header")
	f.StringVar(&searchFlags.subject, "subject", "", "match the Subject header")
	f.StringVar(&searchFlags.body, "body", "", "match text in the message body")
	f.StringVar(&searchFlags.text, "text", "", "match text in any header or the body")
	f.StringArrayVar(&searchFlags.or, "or", nil, "match if ANY of these terms appears in a header or body (repeatable)")
	f.StringVar(&searchFlags.since, "since", "", "messages on/after this date (YYYY-MM-DD)")
	f.StringVar(&searchFlags.before, "before", "", "messages before this date (YYYY-MM-DD)")
	f.IntVar(&searchFlags.sinceHours, "since-hours", 0, "only messages received within the last N hours (exact)")
	f.BoolVar(&searchFlags.seen, "seen", false, "only messages marked as read")
	f.BoolVar(&searchFlags.unseen, "unseen", false, "only unread messages")
	f.BoolVar(&searchFlags.flagged, "flagged", false, "filter by flagged/starred state")
	f.IntVar(&searchFlags.limit, "limit", 50, "maximum number of results (0 = no limit)")
	f.IntVar(&searchFlags.offset, "offset", 0, "number of results to skip (for paging)")
	f.BoolVar(&searchFlags.snippet, "snippet", false, "include a short body preview (fetches bodies)")
}
