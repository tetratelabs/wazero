#include "funcdata.h"
#include "textflag.h"

// nativecall(codeSegment, ce, moduleInstanceAddress)
TEXT ·nativecall(SB),$1048576-24
        // NO_LOCAL_POINTERS tells the GC that there are no pointers to heap
        // inside the function frame, therein no need to scan the stack.
        NO_LOCAL_POINTERS
        // Load the address of *callEngine into arm64ReservedRegisterForCallEngine.
        MOVD ce+8(FP),R0
        // In arm64, return address is stored in R30 after jumping into the code.
        // However, when a function has a function frame like this function, it is stored at the top of the stack:
        // https://github.com/golang/go/blob/38cfb3be9d486833456276777155980d1ec0823e/src/cmd/compile/abi-internal.md#stack-layout-1
        // Thereofore, we save the stack pointer (RSP register) value into ArchContext here,
        // so that the generated JIT code can use RSP freely.
        MOVD RSP,R27
        MOVD R27,136(R0)
        // Load the address of *wasm.ModuleInstance into arm64CallingConventionModuleInstanceAddressRegister.
        MOVD moduleInstanceAddress+16(FP),R29
        // Load the address of native code.
        MOVD codeSegment+0(FP),R1
        // Jump to native code.
        JMP (R1)
