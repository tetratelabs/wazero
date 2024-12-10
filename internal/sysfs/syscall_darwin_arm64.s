#include "textflag.h"

TEXT libc_openat_trampoline<>(SB), NOSPLIT, $0-0
	JMP libc_openat(SB)

GLOBL ·libc_openat_trampoline_addr(SB), RODATA, $8
DATA ·libc_openat_trampoline_addr(SB)/8, $libc_openat_trampoline<>(SB)
