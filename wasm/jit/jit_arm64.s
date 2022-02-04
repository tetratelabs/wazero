//go:build arm64
// +build arm64

#include "funcdata.h"
#include "textflag.h"

// jitcall(codeSegment, engine)
TEXT ·jitcall(SB),NOSPLIT|NOFRAME,$0-16
        MOVD codeSegment+0(FP),R1  // Load the address of native code.
        MOVD engine+8(FP),R0       // Load the address of engine.
        JMP (R1)                   // Jump to native code.
