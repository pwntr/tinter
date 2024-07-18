package tinter

import "sync"

const (
	initialBufferSize = 1 << 10  // 1 KB
	maxBufferSize     = 16 << 10 // 16 KB
)

type buffer []byte

var bufPool = sync.Pool{
	New: func() any {
		b := make(buffer, 0, initialBufferSize)
		return &b
	},
}

// newBuffer returns a new buffer from the pool
func newBuffer() *buffer {
	return bufPool.Get().(*buffer)
}

// Free resets the buffer and returns it to the pool
func (b *buffer) Free() {
	if cap(*b) <= maxBufferSize { // to reduce peak allocation, only return smaller buffers to the pool
		*b = (*b)[:0]  // reset buffer
		bufPool.Put(b) // return buffer to the pool
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
