//go:build amd64
// +build amd64

#include "funcdata.h"
#include "textflag.h"

// jitcall(codeSegment, engine)
TEXT ·jitcall(SB),NOSPLIT|NOFRAME,$0-24
        MOVQ codeSegment+0(FP),AX  // Load the address of native code.
        MOVQ engine+8(FP),R13      // Load the address of engine.
        JMP AX                     // Jump to native code.
