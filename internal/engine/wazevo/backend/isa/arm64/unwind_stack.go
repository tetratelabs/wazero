package arm64

import (
	"encoding/binary"
	"reflect"
	"unsafe"
)

// UnwindStack is a function to unwind the stack.
// The implementation must be aligned with the ABI/Calling convention as in machine_pro_epi_logue.go/abi.go.
func UnwindStack(sp, top uintptr) (returnAddresses []uintptr) {
	l := int(top - sp)

	var stackBuf []byte
	{
		// TODO: use unsafe.Slice after floor version is set to Go 1.20.
		hdr := (*reflect.SliceHeader)(unsafe.Pointer(&stackBuf))
		hdr.Data = sp
		hdr.Len = l
		hdr.Cap = l
	}

	for i := uint64(0); i < uint64(l); {
		//       (high address)
		//    +-----------------+
		//    |     .......     |
		//    |      ret Y      |  <----+
		//    |     .......     |       |
		//    |      ret 0      |       |
		//    |      arg X      |       |  size_of_arg_ret
		//    |     .......     |       |
		//    |      arg 1      |       |
		//    |      arg 0      |  <----+
		//    | size_of_arg_ret |
		//    |  ReturnAddress  |
		//    +-----------------+ <----+
		//    |   ...........   |      |
		//    |   spill slot M  |      |
		//    |   ............  |      |
		//    |   spill slot 2  |      |
		//    |   spill slot 1  |      | frame size
		//    |   spill slot 1  |      |
		//    |   clobbered N   |      |
		//    |   ............  |      |
		//    |   clobbered 0   | <----+
		//    |     xxxxxx      |  ;; unused space to make it 16-byte aligned.
		//    |   frame_size    |
		//    +-----------------+ <---- SP
		//       (low address)

		frameSize := binary.LittleEndian.Uint64(stackBuf[i:])
		i += frameSize +
			16 // frame size + aligned space.
		retAddr := binary.LittleEndian.Uint64(stackBuf[i:])
		i += 8 // ret addr.
		sizeOfArgRet := binary.LittleEndian.Uint64(stackBuf[i:])
		i += 8 + sizeOfArgRet
		returnAddresses = append(returnAddresses, uintptr(retAddr))
	}
	return
}
