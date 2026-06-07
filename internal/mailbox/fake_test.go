package mailbox

import (
	"fmt"

	"github.com/emersion/go-imap/v2"
	"github.com/emersion/go-imap/v2/imapclient"
)

// fakeSession is a scripted session for testing command orchestration without a
// live server. Fetch responses are returned from a queue in call order, and
// every Fetch is recorded so tests can assert which parts were (and weren't)
// requested.
type fakeSession struct {
	selectErr  error
	searchUIDs []imap.UID
	searchErr  error
	fetches    []fetchResponse
	fetchCalls []*imap.FetchOptions
}

// imapMsg is a short alias for the verbose fetch buffer type used throughout
// the test fixtures.
type imapMsg = imapclient.FetchMessageBuffer

type fetchResponse struct {
	msgs []*imapMsg
	err  error
}

func (f *fakeSession) Select(string) error { return f.selectErr }

func (f *fakeSession) UIDSearch(*imap.SearchCriteria) ([]imap.UID, error) {
	return f.searchUIDs, f.searchErr
}

func (f *fakeSession) Fetch(_ imap.NumSet, opts *imap.FetchOptions) ([]*imapclient.FetchMessageBuffer, error) {
	f.fetchCalls = append(f.fetchCalls, opts)
	if len(f.fetches) == 0 {
		return nil, fmt.Errorf("fakeSession: unexpected Fetch call #%d", len(f.fetchCalls))
	}
	r := f.fetches[0]
	f.fetches = f.fetches[1:]
	return r.msgs, r.err
}

func (f *fakeSession) List() ([]*imap.ListData, error) { return nil, nil }
func (f *fakeSession) Close() error                    { return nil }

// requestedBodyPaths returns every body-section part path the fake was asked to
// fetch across all calls, formatted like "[1]" or "[1 2]".
func (f *fakeSession) requestedBodyPaths() []string {
	var paths []string
	for _, opts := range f.fetchCalls {
		for _, s := range opts.BodySection {
			paths = append(paths, fmt.Sprint(s.Part))
		}
	}
	return paths
}

// --- fixture builders -------------------------------------------------------

// structMsg is a phase-1 (BODYSTRUCTURE) result: metadata, no body bytes.
func structMsg(uid uint32, subject string, bs imap.BodyStructure) *imapclient.FetchMessageBuffer {
	return &imapclient.FetchMessageBuffer{
		UID:           imap.UID(uid),
		Envelope:      &imap.Envelope{Subject: subject},
		BodyStructure: bs,
		RFC822Size:    1000,
	}
}

// bodyMsg is a phase-2 result carrying the bytes of one or more sections.
func bodyMsg(uid uint32, sections ...imapclient.FetchBodySectionBuffer) *imapclient.FetchMessageBuffer {
	return &imapclient.FetchMessageBuffer{
		UID:         imap.UID(uid),
		BodySection: sections,
	}
}

// sectionBytes pairs a part path with its returned bytes. The stored section
// only needs Part set: FindBodySection matches on the path (ignoring Peek).
func sectionBytes(path []int, data string) imapclient.FetchBodySectionBuffer {
	return imapclient.FetchBodySectionBuffer{
		Section: &imap.FetchItemBodySection{Part: path},
		Bytes:   []byte(data),
	}
}

// textSP builds a text/* single part with the given subtype and 7bit/utf-8.
func textSP(subtype string) *imap.BodyStructureSinglePart {
	return &imap.BodyStructureSinglePart{
		Type: "text", Subtype: subtype,
		Encoding: "7bit",
		Params:   map[string]string{"charset": "utf-8"},
	}
}

// fileSP builds an attachment single part (disposition attachment).
func fileSP(typ, subtype, filename, encoding string, size uint32) *imap.BodyStructureSinglePart {
	return &imap.BodyStructureSinglePart{
		Type: typ, Subtype: subtype,
		Encoding: encoding,
		Size:     size,
		Extended: &imap.BodyStructureSinglePartExt{
			Disposition: &imap.BodyStructureDisposition{
				Value:  "attachment",
				Params: map[string]string{"filename": filename},
			},
		},
	}
}
