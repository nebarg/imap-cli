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
	Name      string `json:"name"`
	Delimiter string `json:"delimiter,omitempty"`
	Flags     []string `json:"flags,omitempty"`
}

// MessageSummary is the lightweight representation returned by search.
type MessageSummary struct {
	UID     uint32    `json:"uid"`
	Folder  string    `json:"folder"`
	From    []Address `json:"from"`
	To      []Address `json:"to"`
	Subject string    `json:"subject"`
	Date    string    `json:"date,omitempty"` // RFC 3339
	Flags   []string  `json:"flags"`
	Seen    bool      `json:"seen"`
	Size    int64     `json:"size"`
	Snippet string    `json:"snippet,omitempty"`
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
