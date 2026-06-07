package mailbox

import (
	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// session is the narrow set of IMAP operations the mailbox commands depend on.
// It exists as a seam so the two-phase read/search/attachment orchestration can
// be unit-tested against a fake, without a live server. imapSession is the real
// adapter over an imapclient.Client; tests supply their own implementation.
type session interface {
	// Select opens a mailbox read-only.
	Select(folder string) error
	// UIDSearch runs a UID SEARCH and returns the matching UIDs.
	UIDSearch(criteria *imap.SearchCriteria) ([]imap.UID, error)
	// Fetch runs a FETCH and collects the per-message results.
	Fetch(numSet imap.NumSet, options *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error)
	// List enumerates mailboxes (LIST "" "*").
	List() ([]*imap.ListData, error)
	// Close logs out and tears down the connection.
	Close() error
}

// imapSession adapts a live imapclient.Client to the session interface,
// collapsing each command's builder/Wait/Collect dance into one call.
type imapSession struct {
	c *imapclient.Client
}

func (s imapSession) Select(folder string) error {
	_, err := s.c.Select(folder, &imap.SelectOptions{ReadOnly: true}).Wait()
	return err
}

func (s imapSession) UIDSearch(criteria *imap.SearchCriteria) ([]imap.UID, error) {
	data, err := s.c.UIDSearch(criteria, nil).Wait()
	if err != nil {
		return nil, err
	}
	return data.AllUIDs(), nil
}

func (s imapSession) Fetch(numSet imap.NumSet, options *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
	return s.c.Fetch(numSet, options).Collect()
}

func (s imapSession) List() ([]*imap.ListData, error) {
	return s.c.List("", "*", nil).Collect()
}

func (s imapSession) Close() error {
	_ = s.c.Logout().Wait() // best-effort logout; always close the socket
	return s.c.Close()
}
