package asm

import (
	"encoding/binary"
	"fmt"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/platform"
)

var zero [16]byte

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

func (seg *CodeSegment) write(b []byte) {
	i := seg.size
	j := seg.size + len(b)
	if j > len(seg.code) {
		seg.grow(len(b))
	}
	seg.size += copy(seg.code[i:j], b)
}

func (seg *CodeSegment) writeByte(b byte) {
	seg.size++
	if seg.size > len(seg.code) {
		seg.grow(0)
	}
	seg.code[seg.size-1] = b
}

func (seg *CodeSegment) writeUint16(u uint16) {
	seg.size += 2
	if seg.size > len(seg.code) {
		seg.grow(0)
	}
	binary.LittleEndian.PutUint16(seg.code[seg.size-2:seg.size], u)
}

func (seg *CodeSegment) writeUint32(u uint32) {
	seg.size += 4
	if seg.size > len(seg.code) {
		seg.grow(0)
	}
	binary.LittleEndian.PutUint32(seg.code[seg.size-4:seg.size], u)
}

func (seg *CodeSegment) writeUint64(u uint64) {
	seg.size += 8
	if seg.size > len(seg.code) {
		seg.grow(0)
	}
	binary.LittleEndian.PutUint64(seg.code[seg.size-8:seg.size], u)
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
	if len(seg.code) == size {
		panic(fmt.Errorf("remapping to same segment size: %d", size))
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

func (buf Buffer) Write(b []byte) (int, error) {
	buf.seg.write(b)
	return len(b), nil
}

func (buf Buffer) WriteByte(b byte) error {
	buf.seg.writeByte(b)
	return nil
}

func (buf Buffer) WriteUint16(u uint16) error {
	buf.seg.writeUint16(u)
	return nil
}

func (buf Buffer) WriteUint32(u uint32) error {
	buf.seg.writeUint32(u)
	return nil
}

func (buf Buffer) WriteUint64(u uint64) error {
	buf.seg.writeUint64(u)
	return nil
}

func (buf Buffer) Write2Bytes(a, b byte) error {
	buf.seg.writeUint16(uint16(a) | uint16(b)<<8)
	return nil
}

func (buf Buffer) Write3Bytes(a, b, c byte) error {
	buf.Write4Bytes(a, b, c, 0)
	buf.seg.size--
	return nil
}

func (buf Buffer) Write4Bytes(a, b, c, d byte) error {
	buf.seg.writeUint32(uint32(a) | uint32(b)<<8 | uint32(c)<<16 | uint32(d)<<24)
	return nil
}

func (buf Buffer) Grow(n int) {
	buf.seg.grow(n)
}

func (buf Buffer) Reset() {
	buf.seg.size = buf.off
}

func (buf Buffer) Truncate(n int) {
	buf.seg.size = buf.off + n
}
