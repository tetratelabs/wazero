#include "textflag.h"

TEXT libc_lstat64_trampoline<>(SB), NOSPLIT, $0-0
	JMP libc_lstat64(SB)
GLOBL ·libc_lstat64_trampoline_addr(SB), RODATA, $8
DATA ·libc_lstat64_trampoline_addr(SB)/8, $libc_lstat64_trampoline<>(SB)

TEXT libc_openat_trampoline<>(SB), NOSPLIT, $0-0
	JMP libc_openat(SB)
GLOBL ·libc_openat_trampoline_addr(SB), RODATA, $8
DATA ·libc_openat_trampoline_addr(SB)/8, $libc_openat_trampoline<>(SB)

TEXT libc_readlinkat_trampoline<>(SB), NOSPLIT, $0-0
	JMP libc_readlinkat(SB)
GLOBL ·libc_readlinkat_trampoline_addr(SB), RODATA, $8
DATA ·libc_readlinkat_trampoline_addr(SB)/8, $libc_readlinkat_trampoline<>(SB)
