#include "funcdata.h"
#include "textflag.h"

// jitcall(codeSegment, ce, moduleInstanceAddress)
TEXT ·jitcall(SB),NOSPLIT|NOFRAME,$0-24
        MOVQ ce+8(FP),R13                     // Load the address of *callEngine. into amd64ReservedRegisterForCallEngine.
        MOVQ moduleInstanceAddress+16(FP),R12 // Load the address of *wasm.ModuleInstance into amd64CallingConvensionModuleInstanceAddressRegister.
        MOVQ codeSegment+0(FP),AX             // Load the address of native code.
        JMP AX                                // Jump to native code.
