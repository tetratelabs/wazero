#include "funcdata.h"
#include "textflag.h"

// entrypoint(preambleExecutable, functionExecutable *byte, executionContextPtr uintptr, moduleContextPtr *byte, paramResultPtr *uint64, goAllocatedStackSlicePtr uintptr
TEXT ·entrypoint(SB), NOSPLIT|NOFRAME, $0-48
	MOVD preambleExecutable+0(FP), R11
	MOVQ functionExectuable+8(FP), R14
	MOVQ executionContextPtr+16(FP), AX // First argument is passed in AX.
	MOVD moduleContextPtr+24(FP), CX // Second argument is passed in CX.
	MOVD paramResultSlicePtr+32(FP), R12
	MOVD goAllocatedStackSlicePtr+40(FP), BX
	JMP  R11

TEXT ·afterGoFunctionCallEntrypoint(SB), NOSPLIT|NOFRAME, $0-24
	UD2 // TODO!
