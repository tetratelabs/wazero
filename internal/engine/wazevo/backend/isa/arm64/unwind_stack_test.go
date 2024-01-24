package arm64

import (
	"encoding/binary"
	"testing"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestUnwindStack(t *testing.T) {
	for _, tc := range []struct {
		name     string
		contents []uint64
		exp      []uintptr
	}{
		{
			name: "top / with frame and arg-ret-space",
			contents: []uint64{
				32,                 // Frame size: 16 byte-aligned.
				0,                  // reserved.
				0xffffffffffffffff, // in frame.
				0xffffffffffffffff, // in frame.
				0xffffffffffffffff, // in frame.
				0xffffffffffffffff, // in frame.
				0xaa,               // return address.
				48,                 // size_of_arg_ret: 16-byte aligned.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
			},
			exp: []uintptr{0xaa},
		},
		{
			name: "top / without frame / with arg-ret-space",
			contents: []uint64{
				0,                  // Frame size: 16 byte-aligned.
				0,                  // reserved.
				0xaa,               // return address.
				48,                 // size_of_arg_ret: 16-byte aligned.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
			},
			exp: []uintptr{0xaa},
		},
		{
			name: "top / without frame and arg-ret-space",
			contents: []uint64{
				0,    // Frame size: 16 byte-aligned.
				0,    // reserved.
				0xaa, // return address.
				0,    // size_of_arg_ret: 16-byte aligned.
			},
			exp: []uintptr{0xaa},
		},
		{
			name: "three frames",
			contents: []uint64{
				// ------------ first frame -------------
				32,                 // Frame size: 16 byte-aligned.
				0,                  // reserved.
				0xffffffffffffffff, // in frame.
				0xffffffffffffffff, // in frame.
				0xffffffffffffffff, // in frame.
				0xffffffffffffffff, // in frame.
				0xaa,               // return address.
				48,                 // size_of_arg_ret: 16-byte aligned.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
				// ------------ second frame -----------------
				0,                  // Frame size: 16 byte-aligned.
				0,                  // reserved.
				0xbb,               // return address.
				48,                 // size_of_arg_ret: 16-byte aligned.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
				0xeeeeeeeeeeeeeeee, // in arg-ret space.
				// ------------ third frame -----------------
				0,    // Frame size: 16 byte-aligned.
				0,    // reserved.
				0xcc, // return address.
				0,    // size_of_arg_ret: 16-byte aligned.
			},
			exp: []uintptr{0xaa, 0xbb, 0xcc},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			buf := make([]byte, len(tc.contents)*8+1)
			for i, v := range tc.contents {
				binary.LittleEndian.PutUint64(buf[i*8:], v)
			}
			sp := uintptr(unsafe.Pointer(&buf[0]))
			returnAddresses := UnwindStack(sp, 0, uintptr(unsafe.Pointer(&buf[len(buf)-1])), nil)
			require.Equal(t, tc.exp, returnAddresses)
		})
	}
}
