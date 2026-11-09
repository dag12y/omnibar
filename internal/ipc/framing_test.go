package ipc

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"testing"
)

// smallChunkReader returns data one or a few bytes at a time to
// simulate partial delivery across multiple reads.
type smallChunkReader struct {
	data   []byte
	offset int
	chunk  int
}

func newSmallChunkReader(data []byte) *smallChunkReader {
	return &smallChunkReader{data: data, chunk: 3}
}

func (r *smallChunkReader) Read(p []byte) (int, error) {
	if r.offset >= len(r.data) {
		return 0, io.EOF
	}
	n := copy(p, r.data[r.offset:])
	if n > r.chunk {
		n = r.chunk
	}
	r.offset += n
	return n, nil
}

func TestReadFrameComplete(t *testing.T) {
	msg := []byte("hello")
	data := append(msg, '\n')
	r := bufio.NewReader(bytes.NewReader(data))

	frame, err := ReadFrame(r)
	if err != nil {
		t.Fatalf("ReadFrame() returned error: %v", err)
	}
	if !bytes.Equal(frame, msg) {
		t.Errorf("ReadFrame() = %q, want %q", frame, msg)
	}
}

func TestReadFrameMultiple(t *testing.T) {
	r := bufio.NewReader(bytes.NewReader([]byte("msg1\nmsg2\nmsg3\n")))

	f1, err := ReadFrame(r)
	if err != nil || string(f1) != "msg1" {
		t.Fatalf("first frame error: %v, got %q", err, f1)
	}
	f2, err := ReadFrame(r)
	if err != nil || string(f2) != "msg2" {
		t.Fatalf("second frame error: %v, got %q", err, f2)
	}
	f3, err := ReadFrame(r)
	if err != nil || string(f3) != "msg3" {
		t.Fatalf("third frame error: %v, got %q", err, f3)
	}
}

func TestReadFramePartialSmallChunks(t *testing.T) {
	// A frame delivered one or a few bytes at a time across multiple
	// Read calls. The persistent bufio.Reader must buffer and reassemble
	// it without data loss.
	full := []byte("partial message\n")
	r := bufio.NewReader(newSmallChunkReader(full))

	frame, err := ReadFrame(r)
	if err != nil {
		t.Fatalf("ReadFrame() returned error: %v", err)
	}
	if !bytes.Equal(frame, []byte("partial message")) {
		t.Errorf("ReadFrame() = %q, want %q", frame, "partial message")
	}
}

func TestReadFrameMultipleInOneRead(t *testing.T) {
	// Two complete messages delivered in a single buffer.
	r := bufio.NewReader(bytes.NewReader([]byte("first\nsecond\n")))

	f1, err := ReadFrame(r)
	if err != nil || string(f1) != "first" {
		t.Fatalf("first frame error: %v, got %q", err, f1)
	}
	f2, err := ReadFrame(r)
	if err != nil || string(f2) != "second" {
		t.Fatalf("second frame error: %v, got %q", err, f2)
	}
}

func TestReadFrameMultiplePersistentReader(t *testing.T) {
	// Multiple frames read through the same persistent reader; no data
	// may be lost between calls.
	r := bufio.NewReader(bytes.NewReader([]byte("alpha\nbeta\ngamma\n")))

	expected := []string{"alpha", "beta", "gamma"}
	for i, exp := range expected {
		frame, err := ReadFrame(r)
		if err != nil {
			t.Fatalf("frame %d error: %v", i, err)
		}
		if string(frame) != exp {
			t.Errorf("frame %d = %q, want %q", i, frame, exp)
		}
	}
}

func TestReadFrameEmpty(t *testing.T) {
	// An empty message is just a newline.
	r := bufio.NewReader(bytes.NewReader([]byte("\n")))

	frame, err := ReadFrame(r)
	if err != nil {
		t.Fatalf("ReadFrame() returned error: %v", err)
	}
	if len(frame) != 0 {
		t.Errorf("ReadFrame() = %q, want empty", frame)
	}
}

func TestReadFrameOversized(t *testing.T) {
	// Build a message larger than MaxFrameSize.
	big := make([]byte, MaxFrameSize+1)
	for i := range big {
		big[i] = 'x'
	}
	data := append(big, '\n')
	r := bufio.NewReader(bytes.NewReader(data))

	_, err := ReadFrame(r)
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Errorf("ReadFrame() error = %v, want ErrFrameTooLarge", err)
	}
}

func TestReadFrameMaxSizeAllowed(t *testing.T) {
	// A message exactly MaxFrameSize bytes (plus newline) must be accepted.
	big := make([]byte, MaxFrameSize)
	for i := range big {
		big[i] = 'x'
	}
	data := append(big, '\n')
	r := bufio.NewReader(bytes.NewReader(data))

	frame, err := ReadFrame(r)
	if err != nil {
		t.Fatalf("ReadFrame() returned error: %v", err)
	}
	if len(frame) != MaxFrameSize {
		t.Errorf("ReadFrame() len = %d, want %d", len(frame), MaxFrameSize)
	}
}

func TestReadFrameEOF(t *testing.T) {
	// Clean EOF with no pending data.
	r := bufio.NewReader(bytes.NewReader([]byte{}))

	_, err := ReadFrame(r)
	if !errors.Is(err, io.EOF) {
		t.Errorf("ReadFrame() error = %v, want io.EOF", err)
	}
}

func TestReadFrameUnexpectedEOF(t *testing.T) {
	// Connection closes in the middle of a message.
	r := bufio.NewReader(bytes.NewReader([]byte("incomplete")))

	_, err := ReadFrame(r)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("ReadFrame() error = %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestReadFrameIncompleteAtEOF(t *testing.T) {
	// Partial frame data in the buffer when EOF arrives.
	r := bufio.NewReader(bytes.NewReader([]byte("partial")))

	_, err := ReadFrame(r)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("ReadFrame() error = %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestWriteFrame(t *testing.T) {
	var buf bytes.Buffer

	msg := []byte("hello world")
	if err := WriteFrame(&buf, msg); err != nil {
		t.Fatalf("WriteFrame() returned error: %v", err)
	}

	expected := append([]byte("hello world"), '\n')
	if !bytes.Equal(buf.Bytes(), expected) {
		t.Errorf("WriteFrame() = %q, want %q", buf.Bytes(), expected)
	}
}

func TestWriteFrameError(t *testing.T) {
	// A writer that always fails.
	err := WriteFrame(&errorWriter{}, []byte("test"))
	if err == nil {
		t.Fatal("expected error from WriteFrame")
	}
}

type errorWriter struct{}

func (e *errorWriter) Write(p []byte) (int, error) {
	return 0, errors.New("write error")
}
