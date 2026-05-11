package worker

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

type chunkReader struct {
	chunk []byte
	left  int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if r.left <= 0 {
		return 0, io.EOF
	}
	n := copy(p, r.chunk)
	if n > r.left {
		n = r.left
	}
	r.left -= n
	return n, nil
}

func TestCopyWithCapAndDrain(t *testing.T) {
	reader := &chunkReader{chunk: []byte(strings.Repeat("a", 7)), left: 25}
	var dst bytes.Buffer
	written, truncated, err := CopyWithCapAndDrain(&dst, reader, 10, []byte("[cut]"))
	if err != nil {
		t.Fatalf("copy failed: %v", err)
	}
	if !truncated {
		t.Fatal("expected truncation")
	}
	if written != 10 {
		t.Fatalf("written payload bytes = %d, want 10", written)
	}
	if reader.left != 0 {
		t.Fatalf("reader was not drained, left=%d", reader.left)
	}
	if got, want := dst.String(), strings.Repeat("a", 10)+"[cut]"; got != want {
		t.Fatalf("dst = %q, want %q", got, want)
	}
}
