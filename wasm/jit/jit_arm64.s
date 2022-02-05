//go:build arm64
// +build arm64

#include "funcdata.h"
#include "textflag.h"

// jitcall(codeSegment, engine)
TEXT ·jitcall(SB),NOSPLIT|NOFRAME,$0-16
        MOVD codeSegment+0(FP),R1  // Load the address of native code.
        MOVD engine+8(FP),R0       // Load the address of engine.
        // In arm64, return address is stored in R30 after jumping into the code.
        // Note that the const 136 drifts e 
	MOVD R30,136(R0)          
        JMP (R1)                   // Jump to native code.
