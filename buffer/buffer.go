// Package buffer implements a text-editing buffer.
package buffer

import "io"

// Buffer is a text buffer that supports efficient insertion of text at the current insertion point.
// It implements the io.ReaderFrom and io.WriterTo interfaces.
type Buffer struct {
	data             []byte
	gapStart, gapEnd int
}

const defaultGapLen = 128

func New(initialSize int) *Buffer {
	buf := make([]byte, initialSize)
	return &Buffer{data: buf, gapStart: 0, gapEnd: initialSize}
}

func (b *Buffer) gap() []byte { return b.data[b.gapStart:b.gapEnd] }
func (b *Buffer) gapLen() int { return b.gapEnd - b.gapStart }

// moveGap moves the gap to pos, growing it to the default length if it's shorter than that.
func (b *Buffer) moveGap(pos int) {
	copy(b.data[b.gapStart:], b.data[b.gapEnd:])
	gapLen := b.gapLen()
	b.gapStart = len(b.data) - gapLen
	b.gapEnd = len(b.data)
	if gapLen < defaultGapLen {
		b.data = append(b.data, make([]byte, defaultGapLen-gapLen)...)
		b.gapEnd = len(b.data)
	}
}

// ReadFrom reads data from r until EOF, splicing it in at the current insertion point position.
func (b *Buffer) ReadFrom(r io.Reader) (n int64, err error) {
	var nr int
	for {
		for b.gapLen() > 0 {
			nr, err = r.Read(b.gap())
			n += int64(nr)
			b.gapStart += nr
			if err != nil {
				if err == io.EOF {
					err = nil
				}
				return
			}
		}
		b.moveGap(b.gapStart)
	}
}

// WriteTo writes the full content of the buffer to w.
func (b *Buffer) WriteTo(w io.Writer) (int64, error) {
	nw, err := w.Write(b.data[:b.gapStart])
	if err != nil {
		return int64(nw), err
	}
	nw2, err := w.Write(b.data[b.gapEnd:])
	return int64(nw + nw2), err
}
