package amd64

import (
	"encoding/binary"
	"reflect"
	"unsafe"
)

func UnwindStack(sp, top uintptr, returnAddresses []uintptr) []uintptr {
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
		//    |      ret Y      |
		//    |     .......     |
		//    |      ret 0      |
		//    |      arg X      |
		//    |     .......     |
		//    |      arg 1      |
		//    |      arg 0      |
		//    |  ReturnAddress  |
		//    |  Caller_RBP     |
		//    +-----------------+ <---- RBP
		//    |   ...........   |
		//    |   clobbered  M  |
		//    |   ............  |
		//    |   clobbered  0  |
		//    |   spill slot N  |
		//    |   ............  |
		//    |   spill slot 0  |
		//    |  ReturnAddress  |
		//    |  Caller_RBP     |
		//    +-----------------+ <---- SP
		//       (low address)

		callerRBP := binary.LittleEndian.Uint64(stackBuf[i:])
		retAddr := binary.LittleEndian.Uint64(stackBuf[i+8:])
		returnAddresses = append(returnAddresses, uintptr(retAddr))
		i = callerRBP - uint64(sp)
	}
	return returnAddresses
}

// GoCallStackView is a function to get a view of the stack before a Go call, which
// is the view of the stack allocated in CompileGoFunctionTrampoline.
func GoCallStackView(stackPointerBeforeGoCall *uint64) []uint64 {
	panic("implement me")
}
