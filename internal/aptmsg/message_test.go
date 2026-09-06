package aptmsg

import (
	"bytes"
	"strings"
	"testing"
)

func TestReadMessage_URIAcquire(t *testing.T) {
	input := "600 URI Acquire\n" +
		"URI: sigstore+https://example.com/dists/stable/InRelease\n" +
		"Filename: /var/lib/apt/lists/partial/example.com_dists_stable_InRelease\n" +
		"\n"

	r := NewReader(strings.NewReader(input))
	msg, err := r.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if msg.Code != 600 || msg.Description != "URI Acquire" {
		t.Fatalf("got code=%d desc=%q", msg.Code, msg.Description)
	}
	uri, ok := msg.Get("URI")
	if !ok || uri != "sigstore+https://example.com/dists/stable/InRelease" {
		t.Fatalf("URI header = %q, ok=%v", uri, ok)
	}
	fn, ok := msg.Get("Filename")
	if !ok || fn != "/var/lib/apt/lists/partial/example.com_dists_stable_InRelease" {
		t.Fatalf("Filename header = %q, ok=%v", fn, ok)
	}
}

func TestReadMessage_RepeatedConfigItem(t *testing.T) {
	input := "101 Configuration\n" +
		"Config-Item: APT::Architecture=amd64\n" +
		"Config-Item: Acquire::sigstore::RekorServer=https://sigstore.dev\n" +
		"\n"

	r := NewReader(strings.NewReader(input))
	msg, err := r.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	items := msg.All("Config-Item")
	if len(items) != 2 {
		t.Fatalf("expected 2 Config-Item headers, got %d: %v", len(items), items)
	}
	if items[0] != "APT::Architecture=amd64" {
		t.Fatalf("unexpected first item: %q", items[0])
	}
}

func TestReadMessage_MultipleMessagesInStream(t *testing.T) {
	input := "100 Capabilities\n" +
		"Version: 1.2\n" +
		"\n" +
		"600 URI Acquire\n" +
		"URI: sigstore+https://debian.org\n" +
		"\n"
	r := NewReader(strings.NewReader(input))

	first, err := r.ReadMessage()
	if err != nil {
		t.Fatalf("first ReadMessage: %v", err)
	}
	if first.Code != 100 {
		t.Fatalf("expected code 100, got %d", first.Code)
	}

	second, err := r.ReadMessage()
	if err != nil {
		t.Fatalf("second ReadMessage: %v", err)
	}
	if second.Code != 600 {
		t.Fatalf("expected code 600, got %d", second.Code)
	}
}

func TestReadMessage_MalformedStatusLine(t *testing.T) {
	r := NewReader(strings.NewReader("not-a-number Foo\n\n"))
	if _, err := r.ReadMessage(); err == nil {
		t.Fatal("expected error for malformed status line")
	}
}

func TestReadMessage_MalformedHeaderLine(t *testing.T) {
	r := NewReader(strings.NewReader("600 URI Acquire\nNoColonHere\n\n"))
	if _, err := r.ReadMessage(); err == nil {
		t.Fatal("expected error for malformed header line")
	}
}

func TestWriteMessage(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	if err := w.WriteMessage(201, "URI Done",
		H("URI", "sigstore+https://example.com"),
		H("Filename", "/tmp/x"),
		H("Size", "4096"),
	); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	want := "201 URI Done\n" +
		"URI: sigstore+https://example.com\n" +
		"Filename: /tmp/x\n" +
		"Size: 4096\n" +
		"\n"
	if buf.String() != want {
		t.Fatalf("got:\n%q\nwant:\n%q", buf.String(), want)
	}
}

func TestRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	w := NewWriter(&buf)
	_ = w.WriteMessage(400, "URI Failure", H("URI", "sigstore+https://x"), H("Message", "bad"))

	r := NewReader(&buf)
	msg, err := r.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}
	if msg.Code != 400 {
		t.Fatalf("code = %d", msg.Code)
	}
	if v, _ := msg.Get("Message"); v != "bad" {
		t.Fatalf("Message = %q", v)
	}
}
