// Package mailbox wraps the IMAP client with read-only operations and
// JSON-friendly result types intended for consumption by an LLM tool.
package mailbox

// Address is a JSON-friendly email address.
type Address struct {
	Name  string `json:"name,omitempty"`
	Email string `json:"email"`
}

// Folder describes a single mailbox/folder.
type Folder struct {
	Name string `json:"name"`
	// Role is a normalized, provider-independent purpose derived from the
	// mailbox's special-use attributes (RFC 6154): one of inbox, sent, drafts,
	// trash, junk, archive, all, flagged, important — or "" if unknown.
	Role string `json:"role,omitempty"`
	// Selectable reports whether the folder can be opened/searched. Container
	// placeholders (e.g. "[Gmail]") are not selectable.
	Selectable bool `json:"selectable"`
	// Delimiter is the hierarchy separator used to nest sub-folders.
	Delimiter string `json:"delimiter,omitempty"`
	// Flags are the raw IMAP mailbox attributes, kept for completeness.
	Flags []string `json:"flags,omitempty"`
}

// MessageSummary is the lightweight representation returned by search.
type MessageSummary struct {
	UID      uint32    `json:"uid"`
	Folder   string    `json:"folder"`
	From     []Address `json:"from"`
	To       []Address `json:"to"`
	Subject  string    `json:"subject"`
	Date     string    `json:"date,omitempty"`     // RFC 3339; sender's Date header
	Received string    `json:"received,omitempty"` // RFC 3339; server received time (INTERNALDATE)
	Flags    []string  `json:"flags"`
	Seen     bool      `json:"seen"`
	Size     int64     `json:"size"`
	Snippet  string    `json:"snippet,omitempty"`
}

// Attachment is attachment metadata (no payload is downloaded).
type Attachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type,omitempty"`
	Size        int64  `json:"size,omitempty"`
}

// Message is the full representation returned by read.
type Message struct {
	UID         uint32       `json:"uid"`
	Folder      string       `json:"folder"`
	From        []Address    `json:"from"`
	To          []Address    `json:"to"`
	Cc          []Address    `json:"cc,omitempty"`
	ReplyTo     []Address    `json:"reply_to,omitempty"`
	Subject     string       `json:"subject"`
	Date        string       `json:"date,omitempty"` // RFC 3339
	MessageID   string       `json:"message_id,omitempty"`
	Flags       []string     `json:"flags"`
	Seen        bool         `json:"seen"`
	Size        int64        `json:"size"`
	BodyText    string       `json:"body_text"`
	BodyHTML    string       `json:"body_html,omitempty"`
	Attachments []Attachment `json:"attachments"`
}
