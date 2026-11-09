// Package ipc provides newline-delimited message framing for the
// Omnibar IPC transport.
//
// One message per line. Multiple messages can be sent over one
// connection. A message is not processed until its newline delimiter
// is received. Writes append exactly one newline delimiter.
//
// ReadFrame requires a persistent *bufio.Reader. The caller must retain
// the same reader across calls; using a new reader for each frame can
// silently lose buffered data.
package ipc

import (
	"bufio"
	"bytes"
	"errors"
	"io"
)

// MaxFrameSize is the maximum size of a single message frame.
// 1 MiB is conservative and appropriate for future JSON-RPC messages.
const MaxFrameSize = 1 * 1024 * 1024

// ErrFrameTooLarge is returned when a message exceeds MaxFrameSize.
var ErrFrameTooLarge = errors.New("frame exceeds maximum size")

// ReadFrame reads a single newline-delimited frame from r, returning
// the frame data without the trailing newline.
//
// r must be a *bufio.Reader that the caller retains across calls.
// Using a new reader for each frame can lose buffered data.
//
// Returns io.EOF on clean connection close with no pending data.
// Returns io.ErrUnexpectedEOF if there is pending data but the
// connection closes before a newline delimiter.
// Returns ErrFrameTooLarge if the frame exceeds MaxFrameSize.
func ReadFrame(r *bufio.Reader) ([]byte, error) {
	var buf bytes.Buffer
	for {
		b, err := r.ReadByte()
		if err != nil {
			if err == io.EOF {
				if buf.Len() > 0 {
					return nil, io.ErrUnexpectedEOF
				}
				return nil, io.EOF
			}
			return nil, err
		}
		if b == '\n' {
			return buf.Bytes(), nil
		}
		if buf.Len() >= MaxFrameSize {
			return nil, ErrFrameTooLarge
		}
		buf.WriteByte(b)
	}
}

// WriteFrame writes msg to w followed by a single newline delimiter.
func WriteFrame(w io.Writer, msg []byte) error {
	if _, err := w.Write(msg); err != nil {
		return err
	}
	_, err := w.Write([]byte{'\n'})
	return err
}
