package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"go-imap-cli/internal/mailbox"
)

var searchFlags struct {
	folders     []string
	from        []string
	to          []string
	subject     []string
	body        []string
	contains    []string
	containsAll bool
	since       string
	before      string
	sinceHours  int
	seen        bool
	unseen      bool
	flagged     bool
	limit       int
	offset      int
	snippet     bool
}

const dateLayout = "2006-01-02"

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Search messages and return summaries (newest first)",
	Long: `Search messages with server-side IMAP criteria and print summaries
(newest first).

What each filter matches:
  --from / --to / --subject   that header
  --contains                  any header OR the body
  --body                      the body only
  --folder                    mailbox to search (default INBOX)
  --since / --before          received-date range (YYYY-MM-DD)
  --since-hours               received within the last N hours
  --seen / --unseen           read state
  --flagged                   starred/flagged state

Combining filters:
  Repeating the SAME filter ORs its values; DIFFERENT filters are ANDed.
    --from amazon --from ebay              from amazon OR from ebay
    --from amazon --contains order         from amazon AND contains order
    --from amazon --contains a --contains b
                                           from amazon AND (a OR b)

  Repeated --contains terms OR by default; add --contains-match-all to require
  every term. A query is all-OR or all-AND — for mixed logic, run separate
  searches.

  --folder repeats too: each mailbox is searched and the results are merged
  (ordered by received time, since UIDs aren't comparable across folders).

Examples:
  imap-cli search --from alice@example.com --limit 20
  imap-cli search --subject invoice --since 2026-01-01
  imap-cli search --folder INBOX --folder "[Gmail]/Sent Mail" --contains invoice
  imap-cli search --contains refund --contains order --contains-match-all

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
			ContainsAll: searchFlags.containsAll,
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
	f.StringArrayVar(&searchFlags.folders, "folder", nil, "mailbox `name` to search; repeatable, results merged (default INBOX)")
	f.StringArrayVar(&searchFlags.from, "from", nil, "match the From header against `text`; repeatable (ORed)")
	f.StringArrayVar(&searchFlags.to, "to", nil, "match the To header against `text`; repeatable (ORed)")
	f.StringArrayVar(&searchFlags.subject, "subject", nil, "match the Subject header against `text`; repeatable (ORed)")
	f.StringArrayVar(&searchFlags.body, "body", nil, "match `text` in the body; repeatable (ORed)")
	f.StringArrayVar(&searchFlags.contains, "contains", nil, "match `text` in any header or the body; repeatable (ORed)")
	f.BoolVar(&searchFlags.containsAll, "contains-match-all", false, "require every --contains term (AND) instead of any (OR)")
	f.StringVar(&searchFlags.since, "since", "", "messages on/after this `date` (YYYY-MM-DD)")
	f.StringVar(&searchFlags.before, "before", "", "messages before this `date` (YYYY-MM-DD)")
	f.IntVar(&searchFlags.sinceHours, "since-hours", 0, "only messages received within the last N hours")
	f.BoolVar(&searchFlags.seen, "seen", false, "only messages marked as read")
	f.BoolVar(&searchFlags.unseen, "unseen", false, "only unread messages")
	f.BoolVar(&searchFlags.flagged, "flagged", false, "filter by flagged/starred state")
	f.IntVar(&searchFlags.limit, "limit", 50, "maximum number of results (0 = no limit)")
	f.IntVar(&searchFlags.offset, "offset", 0, "number of results to skip (for paging)")
	f.BoolVar(&searchFlags.snippet, "snippet", false, "include a short body preview (fetches the text part only, not attachments)")
}
