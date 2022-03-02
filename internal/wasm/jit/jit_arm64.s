//go:build arm64
// +build arm64

#include "funcdata.h"
#include "textflag.h"

// jitcall(codeSegment, engine)
TEXT ·jitcall(SB),NOSPLIT|NOFRAME,$0-16
        // Load the address of native code.
        MOVD codeSegment+0(FP),R1
        // Load the address of engine.
        MOVD engine+8(FP),R0
        // In arm64, return address is stored in R30 after jumping into the code.
        // We save the return address value into archContext.jitReturnAddress in Engine.
        // Note that the const 216 drifts after editting Engine or archContext struct. See TestArchContextOffsetInEngine.
        MOVD R30,216(R0)
        // TODO
        MOVD R19,128(R0)
        MOVD R20,136(R0)
        MOVD R21,144(R0)
        MOVD R22,152(R0)
        MOVD R23,160(R0)
        MOVD R24,168(R0)
        MOVD R25,176(R0)
        MOVD R26,184(R0)
        MOVD R27,192(R0)
        MOVD R29,208(R0)
        // Jump to native code.
        JMP (R1)
