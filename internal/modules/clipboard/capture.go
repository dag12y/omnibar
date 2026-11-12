// Package clipboard implements bounded text history. Collection starts only
// after an explicit request; only explicitly saved favorites are written to disk.
package clipboard

import (
	"encoding/json"
	"io"
	"time"
	"unicode"
	"unicode/utf8"
)

const MaxEntries = 50
const MaxItemBytes = 16 * 1024
const MaxTotalBytes = 128 * 1024

type Capture struct {
	State string `json:"state"`
	Text  string `json:"text,omitempty"`
}

func validText(text string) bool {
	if text == "" || len(text) > MaxItemBytes || !utf8.ValidString(text) {
		return false
	}
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return false
		}
	}
	return true
}
func readCapture(input io.Reader, state string) Capture {
	if state != "data" {
		switch state {
		case "nil", "clear", "sensitive":
			return Capture{State: state}
		}
		return Capture{State: "skipped"}
	}
	data, err := io.ReadAll(io.LimitReader(input, MaxItemBytes+1))
	if err != nil || !validText(string(data)) {
		return Capture{State: "skipped"}
	}
	return Capture{State: "data", Text: string(data)}
}

// CaptureCommand is an internal wl-paste --watch child mode. Only the private
// stdout pipe receives bounded JSON; text never appears in arguments or logs.
func CaptureCommand(input io.Reader, output io.Writer, state string) int {
	result := make(chan Capture, 1)
	go func() { result <- readCapture(input, state) }()
	var capture Capture
	select {
	case capture = <-result:
	case <-time.After(2 * time.Second):
		capture = Capture{State: "skipped"}
	}
	if err := json.NewEncoder(output).Encode(capture); err != nil {
		return 1
	}
	return 0
}
