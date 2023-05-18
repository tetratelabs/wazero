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
//
// The zero value is a valid, empty code segment, equivalent to being
// constructed by calling NewCodeSegment(nil).
type CodeSegment struct {
	code []byte
	size int
}

// NewCodeSegment constructs a CodeSegment value from a byte slice.
//
// No validation is made that the byte slice is a memory mapped region which can
// be unmapped on Close.
func NewCodeSegment(code []byte) *CodeSegment {
	return &CodeSegment{code: code, size: len(code)}
}

// Map allocates a memory mapping of the given size to the code segment.
//
// Note that programs only need to use this method to initialize the code
// segment to a specific content (e.g. when loading pre-compiled code from a
// file), otherwise the backing memory mapping is allocated on demand when code
// is written to the code segment via Buffers returned by calls to Next.
//
// The method errors is the segment is already backed by a memory mapping.
func (seg *CodeSegment) Map(size int) error {
	if seg.code != nil {
		return fmt.Errorf("code segment already initialized to memory mapping of size %d", len(seg.code))
	}
	b, err := platform.MmapCodeSegment(size)
	if err != nil {
		return err
	}
	seg.code = b
	seg.size = size
	return nil
}

// Close unmaps the underlying memory region held by the code segment, clearing
// its state back to an empty code segment.
//
// The value is still usable after unmapping its memory, a new memory area can
// be allocated by calling Map or writing to the segment.
func (seg *CodeSegment) Unmap() error {
	if seg.code != nil {
		if err := platform.MunmapCodeSegment(seg.code[:cap(seg.code)]); err != nil {
			return err
		}
		seg.code = nil
		seg.size = 0
	}
	return nil
}

// Addr returns the address of the beginning of the code segment as a uintptr.
func (seg *CodeSegment) Addr() uintptr {
	if len(seg.code) > 0 {
		return uintptr(unsafe.Pointer(&seg.code[0]))
	}
	return 0
}

// Size returns the size of code segment, which is less or equal to the length
// of the byte slice returned by Len or Bytes.
func (seg *CodeSegment) Size() uintptr {
	return uintptr(seg.size)
}

// Len returns the length of the byte slice referencing the memory mapping of
// the code segment.
func (seg *CodeSegment) Len() int {
	return len(seg.code)
}

// Bytes returns a byte slice to the memory mapping of the code segment.
//
// The returned slice remains valid until more bytes are written to a buffer
// of the code segment, or Unmap is called.
func (seg *CodeSegment) Bytes() []byte {
	return seg.code
}

// Next returns a buffer pointed at the end of the code segment to support
// writing more code instructions to it.
//
// Buffers are passed by value, but they hold a reference to the code segment
// that they were created from.
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

func (buf Buffer) WriteByte(b byte) {
	buf.seg.writeByte(b)
}

func (buf Buffer) WriteUint32(u uint32) {
	buf.seg.writeUint32(u)
}

func (buf Buffer) Write4Bytes(a, b, c, d byte) {
	buf.seg.writeUint32(uint32(a) | uint32(b)<<8 | uint32(c)<<16 | uint32(d)<<24)
}

func (buf Buffer) Write(b []byte) (int, error) {
	buf.seg.write(b)
	return len(b), nil
}
