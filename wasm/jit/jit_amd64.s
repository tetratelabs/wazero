//go:build amd64
// +build amd64

#include "funcdata.h"
#include "textflag.h"

// jitcall(codeSegment, engine, memory uintptr)
TEXT ·jitcall(SB),NOSPLIT|NOFRAME,$0-24
        MOVQ codeSegment+0(FP),AX  // Load the address of native code.
        MOVQ engine+8(FP),R12      // Load the address of engine.
        MOVQ memory+16(FP),R15     // Load the address of memory instance.
        JMP AX                     // Jump to native code.
