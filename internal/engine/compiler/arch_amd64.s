#include "funcdata.h"
#include "textflag.h"

// nativecall(codeSegment, ce, moduleInstanceAddress)
TEXT ·nativecall(SB),$1048576-24
        NO_LOCAL_POINTERS
        MOVQ ce+8(FP),R13                     // Load the address of *callEngine. into amd64ReservedRegisterForCallEngine.
        // We have to save the current stack pointer (stored in SP register) at ArchContext
        // so that native codes can use it freely.
        MOVQ SP,136(R13)
        MOVQ moduleInstanceAddress+16(FP),R12 // Load the address of *wasm.ModuleInstance into amd64CallingConventionModuleInstanceAddressRegister.
        MOVQ codeSegment+0(FP),AX             // Load the address of native code.
        JMP AX                                // Jump to native code.
