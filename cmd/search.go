package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"go-imap-cli/internal/mailbox"
)

var searchFlags struct {
	folders    []string
	from       []string
	to         []string
	subject    []string
	body       []string
	contains   []string
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

Every filter is repeatable. Repeating the SAME filter ORs its values; DIFFERENT
filters are ANDed together. So:
  --from amazon --from ebay              -> from amazon OR from ebay
  --from amazon --contains order         -> from amazon AND contains order
  --from amazon --contains order --contains receipt
                                         -> from amazon AND (order OR receipt)

--from/--to/--subject match those headers; --contains matches any header or the
body; --body matches the body only.

--folder is also repeatable: each named mailbox is searched and the results are
merged (ordered by received time, since UIDs aren't comparable across folders).

Examples:
  imap-cli search --from alice@example.com --limit 20
  imap-cli search --subject invoice --since 2026-01-01
  imap-cli search --folder INBOX --folder "[Gmail]/Sent Mail" --contains invoice

  # Orders from Amazon (or eBay) in the last 24 hours:
  imap-cli search --from amazon --from ebay --since-hours 24 \
      --contains order --contains receipt --contains dispatched`,
	RunE: func(cmd *cobra.Command, args []string) error {
		p := mailbox.SearchParams{
			Folders:     searchFlags.folders,
			From:        searchFlags.from,
			To:          searchFlags.to,
			Subject:     searchFlags.subject,
			Body:        searchFlags.body,
			Contains:    searchFlags.contains,
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
	f.StringArrayVar(&searchFlags.folders, "folder", nil, "mailbox to search; repeatable to search several, results merged (default INBOX)")
	f.StringArrayVar(&searchFlags.from, "from", nil, "match the From header (repeatable; ORed)")
	f.StringArrayVar(&searchFlags.to, "to", nil, "match the To header (repeatable; ORed)")
	f.StringArrayVar(&searchFlags.subject, "subject", nil, "match the Subject header (repeatable; ORed)")
	f.StringArrayVar(&searchFlags.body, "body", nil, "match text in the body (repeatable; ORed)")
	f.StringArrayVar(&searchFlags.contains, "contains", nil, "match text in any header or the body (repeatable; ORed)")
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
