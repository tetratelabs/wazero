package leb128

import (
	"fmt"
	"io"
)

func DecodeUint32(r io.Reader) (ret uint32, num uint64, err error) {
	const (
		uint32Mask  uint32 = 1 << 7
		uint32Mask2        = ^uint32Mask
	)

	for shift := 0; shift < 35; shift += 7 {
		b, err := readByteAsUint32(r)
		if err != nil {
			return 0, 0, fmt.Errorf("readByte failed: %w", err)
		}
		num++
		ret |= (b & uint32Mask2) << shift
		if b&uint32Mask == 0 {
			break
		}
	}
	return
}

func DecodeUint64(r io.Reader) (ret uint64, num uint64, err error) {
	const (
		uint64Mask  uint64 = 1 << 7
		uint64Mask2        = ^uint64Mask
	)
	for shift := 0; shift < 64; shift += 7 {
		b, err := readByteAsUint64(r)
		if err != nil {
			return 0, 0, fmt.Errorf("readByte failed: %w", err)
		}
		num++
		ret |= (b & uint64Mask2) << shift
		if b&uint64Mask == 0 {
			break
		}
	}
	return
}

func DecodeInt32(r io.Reader) (ret int32, num uint64, err error) {
	const (
		int32Mask  int32 = 1 << 7
		int32Mask2       = ^int32Mask
		int32Mask3       = 1 << 6
		int32Mask4       = ^0
	)
	var shift int
	var b int32
	for shift < 35 {
		b, err = readByteAsInt32(r)
		if err != nil {
			return 0, 0, fmt.Errorf("readByte failed: %w", err)
		}
		num++
		ret |= (b & int32Mask2) << shift
		shift += 7
		if b&int32Mask == 0 {
			break
		}
	}

	if shift < 32 && (b&int32Mask3) == int32Mask3 {
		ret |= int32Mask4 << shift
	}
	return
}

func DecodeInt33AsInt64(r io.Reader) (ret int64, num uint64, err error) {
	const (
		int33Mask  int64 = 1 << 7
		int33Mask2       = ^int33Mask
		int33Mask3       = 1 << 6
		int33Mask4       = 8589934591 // 2^33-1
		int33Mask5       = 1 << 32
		int33Mask6       = int33Mask4 + 1 // 2^33
	)
	var shift int
	var b int64
	for shift < 35 {
		b, err = readByteAsInt64(r)
		num++
		if err != nil {
			return 0, 0, fmt.Errorf("readByte failed: %w", err)
		}
		ret |= (b & int33Mask2) << shift
		shift += 7
		if b&int33Mask == 0 {
			break
		}
	}

	// fixme: can be optimized
	if shift < 33 && (b&int33Mask3) == int33Mask3 {
		ret |= int33Mask4 << shift
	}
	ret = ret & int33Mask4

	// if 33rd bit == 1, we translate it as a corresponding signed-33bit minus value
	if ret&int33Mask5 > 0 {
		ret = ret - int33Mask6
	}
	return ret, num, nil
}

func DecodeInt64(r io.Reader) (ret int64, num uint64, err error) {
	const (
		int64Mask  int64 = 1 << 7
		int64Mask2       = ^int64Mask
		int64Mask3       = 1 << 6
		int64Mask4       = ^0
	)
	var shift int
	var b int64
	for shift < 64 {
		b, err = readByteAsInt64(r)
		if err != nil {
			return 0, 0, fmt.Errorf("readByte failed: %w", err)
		}
		num++
		ret |= (b & int64Mask2) << shift
		shift += 7
		if b&int64Mask == 0 {
			break
		}
	}

	if shift < 64 && (b&int64Mask3) == int64Mask3 {
		ret |= int64Mask4 << shift
	}
	return
}

func readByteAsUint32(r io.Reader) (uint32, error) {
	b := make([]byte, 1)
	_, err := io.ReadFull(r, b)
	return uint32(b[0]), err
}

func readByteAsInt32(r io.Reader) (int32, error) {
	b := make([]byte, 1)
	_, err := io.ReadFull(r, b)
	return int32(b[0]), err
}

func readByteAsUint64(r io.Reader) (uint64, error) {
	b := make([]byte, 1)
	_, err := io.ReadFull(r, b)
	return uint64(b[0]), err
}

func readByteAsInt64(r io.Reader) (int64, error) {
	b := make([]byte, 1)
	_, err := io.ReadFull(r, b)
	return int64(b[0]), err
}
