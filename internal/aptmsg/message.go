// Package aptmsg implements the RFC822-style message framing APT's method
// IPC protocol uses over stdin/stdout: a "NNN Description" status line
// followed by "Key: Value" header lines, terminated by a blank line.
package aptmsg

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Header is a single "Key: Value" line. Order is preserved and keys may
// repeat (e.g. multiple Config-Item lines in a 101 Configuration message).
type Header struct {
	Key   string
	Value string
}

// Message is one framed APT method protocol message.
type Message struct {
	Code        int
	Description string
	Headers     []Header
}

// Get returns the value of the first header matching key (case-insensitive),
// and whether it was found.
func (m *Message) Get(key string) (string, bool) {
	for _, h := range m.Headers {
		if strings.EqualFold(h.Key, key) {
			return h.Value, true
		}
	}
	return "", false
}

// All returns the values of every header matching key (case-insensitive), in order.
func (m *Message) All(key string) []string {
	var out []string
	for _, h := range m.Headers {
		if strings.EqualFold(h.Key, key) {
			out = append(out, h.Value)
		}
	}
	return out
}

// Reader reads a sequence of Messages from an APT method's stdin.
type Reader struct {
	br *bufio.Reader
}

func NewReader(r io.Reader) *Reader {
	return &Reader{br: bufio.NewReader(r)}
}

// ReadMessage reads one message. It returns io.EOF only if the stream ends
// before any status line of a new message is seen.
func (r *Reader) ReadMessage() (*Message, error) {
	statusLine, err := r.readLine()
	if err != nil {
		return nil, err
	}
	for statusLine == "" {
		// Tolerate stray blank lines between messages.
		statusLine, err = r.readLine()
		if err != nil {
			return nil, err
		}
	}

	code, desc, err := parseStatusLine(statusLine)
	if err != nil {
		return nil, err
	}
	msg := &Message{Code: code, Description: desc}

	for {
		line, err := r.readLine()
		if err != nil {
			if err == io.EOF && line == "" {
				// A message must be blank-line terminated; a stream that
				// ends mid-message is malformed, but return what we have
				// rather than losing the caller's already-parsed status line.
				return msg, nil
			}
			return nil, err
		}
		if line == "" {
			break
		}
		key, value, err := parseHeaderLine(line)
		if err != nil {
			return nil, err
		}
		msg.Headers = append(msg.Headers, Header{Key: key, Value: value})
	}
	return msg, nil
}

func (r *Reader) readLine() (string, error) {
	line, err := r.br.ReadString('\n')
	if err != nil {
		if err == io.EOF && line != "" {
			return strings.TrimRight(line, "\r\n"), nil
		}
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func parseStatusLine(line string) (int, string, error) {
	parts := strings.SplitN(line, " ", 2)
	code, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, "", fmt.Errorf("aptmsg: invalid status line %q: %w", line, err)
	}
	desc := ""
	if len(parts) == 2 {
		desc = parts[1]
	}
	return code, desc, nil
}

func parseHeaderLine(line string) (key, value string, err error) {
	idx := strings.Index(line, ":")
	if idx < 0 {
		return "", "", fmt.Errorf("aptmsg: invalid header line %q", line)
	}
	key = line[:idx]
	value = strings.TrimPrefix(line[idx+1:], " ")
	return key, value, nil
}

// Writer writes framed Messages to an APT method's stdout.
type Writer struct {
	w io.Writer
}

func NewWriter(w io.Writer) *Writer {
	return &Writer{w: w}
}

// WriteMessage writes a status line, the given headers in order, and the
// terminating blank line.
func (w *Writer) WriteMessage(code int, description string, headers ...Header) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%d %s\n", code, description)
	for _, h := range headers {
		fmt.Fprintf(&b, "%s: %s\n", h.Key, h.Value)
	}
	b.WriteString("\n")
	_, err := io.WriteString(w.w, b.String())
	return err
}

// H is a small constructor to keep call sites in method.go readable:
// w.WriteMessage(201, "URI Done", aptmsg.H("URI", uri), aptmsg.H("Filename", fn))
func H(key, value string) Header {
	return Header{Key: key, Value: value}
}
