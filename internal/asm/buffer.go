package asm

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/platform"
)

var zero [16]byte

// CodeSegment represents a memory mapped segment where native CPU instructions
// are written.
//
// To construct code segments, the program must call Next to obtain a buffer
// view capable of writing data at the end of the segment. Next must be called
// before generating the code of a function because it aligns the next write on
// 16 bytes.
//
// Instances of CodeSegment hold references to memory which is NOT managed by
// the garbage collector and therefore must be released *manually* by calling
// their Unmap method to prevent memory leaks.
type CodeSegment struct {
	code []byte
	size int
}

func MakeCodeSegment(code []byte) CodeSegment {
	return CodeSegment{code: code}
}

func (seg *CodeSegment) Map(size int) error {
	if seg.code != nil {
		return fmt.Errorf("code segment already initialized to memory mapping of size %d", len(seg.code))
	}
	b, err := platform.MmapCodeSegment(size)
	if err != nil {
		return err
	}
	seg.code = b
	seg.size = 0
	return nil
}

func (seg *CodeSegment) Unmap() error {
	if seg.code != nil {
		if err := platform.MunmapCodeSegment(seg.code[:cap(seg.code)]); err != nil {
			return err
		}
		seg.code = nil
	}
	return nil
}

func (seg *CodeSegment) Addr() uintptr {
	if len(seg.code) > 0 {
		return uintptr(unsafe.Pointer(&seg.code[0]))
	}
	return 0
}

func (seg *CodeSegment) Size() uintptr {
	return uintptr(seg.size)
}

func (seg *CodeSegment) Len() int {
	return len(seg.code)
}

func (seg *CodeSegment) Bytes() []byte {
	return seg.code
}

func (seg *CodeSegment) Next() Buffer {
	// Align 16-bytes boundary.
	seg.write(zero[:seg.size&15])
	return Buffer{seg: seg, off: seg.size}
}

func (seg *CodeSegment) append(n int) []byte {
	i := seg.size
	j := seg.size + n
	if j > len(seg.code) {
		seg.grow(n)
	}
	seg.size = j
	return seg.code[i:j:j]
}

func (seg *CodeSegment) write(b []byte) {
	copy(seg.append(len(b)), b)
}

func (seg *CodeSegment) writeByte(b byte) {
	seg.size++
	if seg.size > len(seg.code) {
		seg.grow(0)
	}
	seg.code[seg.size-1] = b
}

func (seg *CodeSegment) writeUint32(u uint32) {
	seg.size += 4
	if seg.size > len(seg.code) {
		seg.grow(0)
	}
	binary.LittleEndian.PutUint32(seg.code[seg.size-4:seg.size], u)
}

func (seg *CodeSegment) grow(n int) {
	size := len(seg.code)
	want := seg.size + n
	if size >= want {
		return
	}
	if size == 0 {
		size = 65536
	}
	for size < want {
		size *= 2
	}
	b, err := platform.RemapCodeSegment(seg.code, size)
	if err != nil {
		// The only reason for growing the buffer to error is if we run
		// out of memory, so panic for now as it greatly simplifies error
		// handling to assume writing to the buffer would never fail.
		panic(err)
	}
	seg.code = b
}

// Buffer is a reference type representing a section beginning at the end of a
// code segment where new instructions can be written.
type Buffer struct {
	seg *CodeSegment
	off int
}

func (buf Buffer) Cap() int {
	return len(buf.seg.code) - buf.off
}

func (buf Buffer) Len() int {
	return buf.seg.size - buf.off
}

func (buf Buffer) Bytes() []byte {
	i := buf.off
	j := buf.seg.size
	return buf.seg.Bytes()[i:j:j]
}

func (buf Buffer) Grow(n int) {
	buf.seg.grow(n)
}

func (buf Buffer) Reset() {
	buf.seg.size = buf.off
}

func (buf Buffer) Append(n int) []byte {
	return buf.seg.append(n)
}

func (buf Buffer) Truncate(n int) {
	buf.seg.size = buf.off + n
}

func (buf Buffer) Write(b []byte) (int, error) {
	buf.seg.write(b)
	return len(b), nil
}

func (buf Buffer) WriteByte(b byte) error {
	buf.seg.writeByte(b)
	return nil
}

func (buf Buffer) WriteUint32(u uint32) error {
	buf.seg.writeUint32(u)
	return nil
}

func (buf Buffer) Write4Bytes(a, b, c, d byte) error {
	buf.seg.writeUint32(uint32(a) | uint32(b)<<8 | uint32(c)<<16 | uint32(d)<<24)
	return nil
}
