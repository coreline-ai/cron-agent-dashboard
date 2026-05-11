package worker

// RingBuffer keeps the most recent N bytes written to it. It is used for stderr
// so error_message stays bounded.
type RingBuffer struct {
	buf    []byte
	start  int
	filled bool
}

func NewRingBuffer(size int) *RingBuffer {
	if size < 0 {
		size = 0
	}
	return &RingBuffer{buf: make([]byte, size)}
}

func (r *RingBuffer) Write(p []byte) (int, error) {
	if len(r.buf) == 0 {
		return len(p), nil
	}
	for _, b := range p {
		r.buf[r.start] = b
		r.start = (r.start + 1) % len(r.buf)
		if r.start == 0 {
			r.filled = true
		}
	}
	return len(p), nil
}

func (r *RingBuffer) Bytes() []byte {
	if len(r.buf) == 0 {
		return nil
	}
	if !r.filled {
		return append([]byte(nil), r.buf[:r.start]...)
	}
	out := make([]byte, 0, len(r.buf))
	out = append(out, r.buf[r.start:]...)
	out = append(out, r.buf[:r.start]...)
	return out
}

func (r *RingBuffer) String() string { return string(r.Bytes()) }
