package tinter

import "sync"

const maxBufferSize = 16 << 10 // 16 KB

type buffer []byte

var bufPool = sync.Pool{
	New: func() any {
		b := make(buffer, 0, 1024) // 1 KB initial buffer size
		return (*buffer)(&b)
	},
}

// newBuffer returns a new buffer from the pool
func newBuffer() *buffer {
	return bufPool.Get().(*buffer)
}

// Free resets the buffer and returns it to the pool
func (b *buffer) Free() {
	// to reduce peak allocation, return only smaller buffers to the pool.
	if cap(*b) <= maxBufferSize {
		*b = (*b)[:0]
		bufPool.Put(b)
	}
}

// WriteChar appends a char to the buffer
func (b *buffer) WriteChar(char byte) {
	*b = append(*b, char)
}

// WriteString appends a string to the buffer
func (b *buffer) WriteString(str string) {
	*b = append(*b, str...)
}

// WriteStringIf appends a string to the buffer if the condition is true, otherwise does nothing
func (b *buffer) WriteStringIf(ok bool, str string) {
	if ok {
		b.WriteString(str)
	}
}
