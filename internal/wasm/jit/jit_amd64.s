//go:build amd64
// +build amd64

#include "funcdata.h"
#include "textflag.h"

// jitcall(codeSegment, callEngine)
TEXT ·jitcall(SB),NOSPLIT|NOFRAME,$0-16
        MOVQ codeSegment+0(FP),AX      // Load the address of native code.
        MOVQ callEngine+8(FP),R13  // Load the address of callEngine.
        JMP AX                         // Jump to native code.
