package mailbox

import (
	"testing"

	"github.com/emersion/go-imap/v2"
)

func TestFolderRole(t *testing.T) {
	cases := []struct {
		name  string
		attrs []imap.MailboxAttr
		want  string
	}{
		{"INBOX", nil, "inbox"},
		{"inbox", nil, "inbox"}, // matched case-insensitively
		{"[Gmail]/Sent Mail", []imap.MailboxAttr{imap.MailboxAttrSent}, "sent"},
		{"[Gmail]/Trash", []imap.MailboxAttr{imap.MailboxAttrTrash}, "trash"},
		{"Receipts", nil, ""}, // ordinary user folder
	}
	for _, tc := range cases {
		if got := folderRole(tc.name, tc.attrs); got != tc.want {
			t.Errorf("folderRole(%q, %v) = %q, want %q", tc.name, tc.attrs, got, tc.want)
		}
	}
}

func TestIsSelectable(t *testing.T) {
	if !isSelectable([]imap.MailboxAttr{imap.MailboxAttrHasChildren}) {
		t.Error("a normal folder should be selectable")
	}
	if isSelectable([]imap.MailboxAttr{imap.MailboxAttrNoSelect}) {
		t.Error("\\Noselect folder should not be selectable")
	}
	if isSelectable([]imap.MailboxAttr{imap.MailboxAttrNonExistent}) {
		t.Error("\\NonExistent folder should not be selectable")
	}
}
