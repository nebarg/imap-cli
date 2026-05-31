package mailbox

import (
	"fmt"
	"net"
	"time"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"

	"go-imap-cli/internal/config"
)

// Client is a connected, authenticated read-only IMAP session.
type Client struct {
	imap *imapclient.Client
}

// Connect dials the account's server, negotiates TLS and logs in. The caller
// must call Close when done.
func Connect(acc config.Account, timeout time.Duration) (*Client, error) {
	opts := &imapclient.Options{}
	if timeout > 0 {
		opts.Dialer = &net.Dialer{Timeout: timeout}
	}

	var (
		c   *imapclient.Client
		err error
	)
	if acc.TLS {
		c, err = imapclient.DialTLS(acc.Addr(), opts)
	} else {
		c, err = imapclient.DialStartTLS(acc.Addr(), opts)
	}
	if err != nil {
		return nil, fmt.Errorf("connecting to %s: %w", acc.Addr(), err)
	}

	if err := c.Login(acc.Username, acc.Password).Wait(); err != nil {
		c.Close()
		return nil, fmt.Errorf("login failed for %s: %w", acc.Username, err)
	}

	return &Client{imap: c}, nil
}

// Close logs out and tears down the connection.
func (c *Client) Close() error {
	if c.imap == nil {
		return nil
	}
	// Best-effort logout; always close the socket.
	_ = c.imap.Logout().Wait()
	return c.imap.Close()
}

// Folders lists all mailboxes available to the account.
func (c *Client) Folders() ([]Folder, error) {
	mboxes, err := c.imap.List("", "*", nil).Collect()
	if err != nil {
		return nil, fmt.Errorf("listing mailboxes: %w", err)
	}

	folders := make([]Folder, 0, len(mboxes))
	for _, m := range mboxes {
		folders = append(folders, Folder{
			Name:      m.Mailbox,
			Delimiter: string(m.Delim),
			Flags:     mailboxAttrs(m.Attrs),
		})
	}
	return folders, nil
}

func mailboxAttrs(attrs []imap.MailboxAttr) []string {
	out := make([]string, 0, len(attrs))
	for _, a := range attrs {
		out = append(out, string(a))
	}
	return out
}
