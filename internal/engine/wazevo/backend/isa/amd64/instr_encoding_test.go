package amd64

import (
	"encoding/hex"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestInstruction_format_encode(t *testing.T) {
	for _, tc := range []struct {
		setup      func(*instruction)
		want       string
		wantFormat string
	}{
		{
			setup:      func(i *instruction) { i.asRet(nil) },
			wantFormat: "ret",
			want:       "c3",
		},
		{
			setup:      func(i *instruction) { i.asImm(r14VReg, 1234567, false) },
			wantFormat: "movl $1234567, %r14d",
			want:       "41be87d61200",
		},
		{
			setup: func(i *instruction) {
				v := -126
				i.asImm(r14VReg, uint64(int64(v)), false)
			},
			wantFormat: "movl $-126, %r14d",
			want:       "41be82ffffff",
		},
		{
			setup:      func(i *instruction) { i.asImm(r14VReg, 0x200000000, true) },
			want:       "49be0000000002000000",
			wantFormat: "movabsq $8589934592, %r14",
		},
		{
			setup: func(i *instruction) {
				v := -126
				i.asImm(r14VReg, uint64(int64(v)), true)
			},
			want:       "49c7c682ffffff",
			wantFormat: "movabsq $-126, %r14",
		},
		{
			setup:      func(i *instruction) { i.asImm(rcxVReg, 1234567, false) },
			wantFormat: "movl $1234567, %ecx",
			want:       "b987d61200",
		},
		{
			setup: func(i *instruction) {
				v := -126
				i.asImm(rcxVReg, uint64(int64(v)), false)
			},
			wantFormat: "movl $-126, %ecx",
			want:       "b982ffffff",
		},
		{
			setup:      func(i *instruction) { i.asImm(rcxVReg, 0x200000000, true) },
			want:       "48b90000000002000000",
			wantFormat: "movabsq $8589934592, %rcx",
		},
		{
			setup: func(i *instruction) {
				v := -126
				i.asImm(rcxVReg, uint64(int64(v)), true)
			},
			want:       "48c7c182ffffff",
			wantFormat: "movabsq $-126, %rcx",
		},
		{
			setup:      func(i *instruction) { i.asMovRR(raxVReg, rdiVReg, false) },
			want:       "89c7",
			wantFormat: "movl %eax, %edi",
		},
		{
			setup:      func(i *instruction) { i.asMovRR(raxVReg, r15VReg, false) },
			want:       "4189c7",
			wantFormat: "movl %eax, %r15d",
		},
		{
			setup:      func(i *instruction) { i.asMovRR(r14VReg, r15VReg, false) },
			want:       "4589f7",
			wantFormat: "movl %r14d, %r15d",
		},
		{
			setup:      func(i *instruction) { i.asMovRR(raxVReg, rcxVReg, true) },
			want:       "4889c1",
			wantFormat: "movq %rax, %rcx",
		},
		{
			setup:      func(i *instruction) { i.asMovRR(raxVReg, r15VReg, true) },
			want:       "4989c7",
			wantFormat: "movq %rax, %r15",
		},
		{
			setup:      func(i *instruction) { i.asMovRR(r11VReg, r12VReg, true) },
			want:       "4d89dc",
			wantFormat: "movq %r11, %r12",
		},
		{
			setup:      func(i *instruction) { i.asNot(newOperandReg(raxVReg), false) },
			want:       "f7d0",
			wantFormat: "notl %eax",
		},
		{
			setup:      func(i *instruction) { i.asNot(newOperandReg(raxVReg), true) },
			want:       "48f7d0",
			wantFormat: "notq %rax",
		},
		{
			setup:      func(i *instruction) { i.asNeg(newOperandReg(raxVReg), false) },
			want:       "f7d8",
			wantFormat: "negl %eax",
		},
		{
			setup:      func(i *instruction) { i.asNeg(newOperandReg(raxVReg), true) },
			want:       "48f7d8",
			wantFormat: "negq %rax",
		},
		{
			setup:      func(i *instruction) { i.asMulHi(newOperandReg(rsiVReg), true, false) },
			want:       "f7ee",
			wantFormat: "imull %esi",
		},
		{
			setup:      func(i *instruction) { i.asMulHi(newOperandReg(r14VReg), false, false) },
			want:       "41f7e6",
			wantFormat: "mull %r14d",
		},
		{
			setup:      func(i *instruction) { i.asMulHi(newOperandReg(r15VReg), true, true) },
			want:       "49f7ef",
			wantFormat: "imulq %r15",
		},
		{
			setup:      func(i *instruction) { i.asMulHi(newOperandReg(rdiVReg), false, true) },
			want:       "48f7e7",
			wantFormat: "mulq %rdi",
		},
		{
			setup:      func(i *instruction) { i.asMulHi(newOperandMem(newAmodeImmReg(123, rdiVReg)), true, false) },
			want:       "f76f7b",
			wantFormat: "imull 123(%edi)",
		},
		{
			setup:      func(i *instruction) { i.asMulHi(newOperandMem(newAmodeImmReg(123, rdiVReg)), false, false) },
			want:       "f7677b",
			wantFormat: "mull 123(%edi)",
		},
		{
			setup:      func(i *instruction) { i.asMulHi(newOperandMem(newAmodeImmReg(123, rdiVReg)), true, true) },
			want:       "48f76f7b",
			wantFormat: "imulq 123(%rdi)",
		},
		{
			setup:      func(i *instruction) { i.asMulHi(newOperandMem(newAmodeImmReg(123, rdiVReg)), false, true) },
			want:       "48f7677b",
			wantFormat: "mulq 123(%rdi)",
		},
		// bsr
		{
			setup:      func(i *instruction) { i.asUnaryRmR(unaryRmROpcodeBsr, newOperandReg(raxVReg), rdiVReg, false) },
			want:       "0fbdf8",
			wantFormat: "bsr %eax, %edi",
		},
		// bsf
		{
			setup:      func(i *instruction) { i.asUnaryRmR(unaryRmROpcodeBsf, newOperandReg(raxVReg), rdiVReg, false) },
			want:       "0fbcf8",
			wantFormat: "bsf %eax, %edi",
		},
		// tzcnt
		{
			setup:      func(i *instruction) { i.asUnaryRmR(unaryRmROpcodeTzcnt, newOperandReg(raxVReg), rdiVReg, false) },
			want:       "f30fbcf8",
			wantFormat: "tzcnt %eax, %edi",
		},
		// lzcnt
		{
			setup:      func(i *instruction) { i.asUnaryRmR(unaryRmROpcodeLzcnt, newOperandReg(raxVReg), rdiVReg, false) },
			want:       "f30fbdf8",
			wantFormat: "lzcnt %eax, %edi",
		},
		// popcnt
		{
			setup:      func(i *instruction) { i.asUnaryRmR(unaryRmROpcodePopcnt, newOperandReg(raxVReg), rdiVReg, false) },
			want:       "f30fb8f8",
			wantFormat: "popcnt %eax, %edi",
		},
		// bsr
		{
			setup:      func(i *instruction) { i.asUnaryRmR(unaryRmROpcodeBsr, newOperandReg(raxVReg), rdiVReg, true) },
			want:       "480fbdf8",
			wantFormat: "bsr %rax, %rdi",
		},
		// bsf
		{
			setup:      func(i *instruction) { i.asUnaryRmR(unaryRmROpcodeBsf, newOperandReg(raxVReg), rdiVReg, true) },
			want:       "480fbcf8",
			wantFormat: "bsf %rax, %rdi",
		},
		// tzcnt
		{
			setup:      func(i *instruction) { i.asUnaryRmR(unaryRmROpcodeTzcnt, newOperandReg(raxVReg), rdiVReg, true) },
			want:       "f3480fbcf8",
			wantFormat: "tzcnt %rax, %rdi",
		},
		// lzcnt
		{
			setup:      func(i *instruction) { i.asUnaryRmR(unaryRmROpcodeLzcnt, newOperandReg(raxVReg), rdiVReg, true) },
			want:       "f3480fbdf8",
			wantFormat: "lzcnt %rax, %rdi",
		},
		// popcnt
		{
			setup:      func(i *instruction) { i.asUnaryRmR(unaryRmROpcodePopcnt, newOperandReg(raxVReg), rdiVReg, true) },
			want:       "f3480fb8f8",
			wantFormat: "popcnt %rax, %rdi",
		},
		// addss
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAddss, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "f30f58c1",
			wantFormat: "addss %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAddss, newOperandReg(xmm13VReg), xmm11VReg, false) },
			want:       "f3450f58dd",
			wantFormat: "addss %xmm13, %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmR(sseOpcodeAddss, newOperandMem(newAmodeImmReg(0, r13VReg)), xmm11VReg, true)
			},
			want:       "f3450f585d00",
			wantFormat: "addss (%r13), %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmR(sseOpcodeAddss, newOperandMem(newAmodeImmReg(123, r13VReg)), xmm11VReg, true)
			},
			want:       "f3450f585d7b",
			wantFormat: "addss 123(%r13), %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmR(sseOpcodeAddss, newOperandMem(newAmodeImmReg(1<<25, r13VReg)), xmm11VReg, true)
			},
			want:       "f3450f589d00000002",
			wantFormat: "addss 33554432(%r13), %xmm11",
		},
		// addsd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAddsd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "f20f58c1",
			wantFormat: "addsd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAddsd, newOperandReg(xmm13VReg), xmm11VReg, false) },
			want:       "f2450f58dd",
			wantFormat: "addsd %xmm13, %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmR(sseOpcodeAddsd, newOperandMem(newAmodeImmReg(0, rbpVReg)), xmm11VReg, true)
			},
			want:       "f2440f585d00",
			wantFormat: "addsd (%rbp), %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmR(sseOpcodeAddsd, newOperandMem(newAmodeImmReg(123, rbpVReg)), xmm11VReg, true)
			},
			want:       "f2440f585d7b",
			wantFormat: "addsd 123(%rbp), %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmR(sseOpcodeAddsd, newOperandMem(newAmodeImmReg(1<<25, rbpVReg)), xmm11VReg, true)
			},
			want:       "f2440f589d00000002",
			wantFormat: "addsd 33554432(%rbp), %xmm11",
		},
		// addps
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAddps, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "0f58c1",
			wantFormat: "addps %xmm1, %xmm0",
		},
		// addpd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAddpd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f58c1",
			wantFormat: "addpd %xmm1, %xmm0",
		},
		// addss
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAddss, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "f30f58c1",
			wantFormat: "addss %xmm1, %xmm0",
		},
		// addsd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAddsd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "f20f58c1",
			wantFormat: "addsd %xmm1, %xmm0",
		},
		// andps
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAndps, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "0f54c1",
			wantFormat: "andps %xmm1, %xmm0",
		},
		// andpd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAndpd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f54c1",
			wantFormat: "andpd %xmm1, %xmm0",
		},
		// andnps
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAndnps, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "0f55c1",
			wantFormat: "andnps %xmm1, %xmm0",
		},
		// andnpd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAndnpd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f55c1",
			wantFormat: "andnpd %xmm1, %xmm0",
		},
		// cvttps2dq
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeCvttps2dq, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "f30f5bc1",
			wantFormat: "cvttps2dq %xmm1, %xmm0",
		},
		// cvtdq2ps
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeCvtdq2ps, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "0f5bc1",
			wantFormat: "cvtdq2ps %xmm1, %xmm0",
		},
		// divps
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeDivps, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "0f5ec1",
			wantFormat: "divps %xmm1, %xmm0",
		},
		// divpd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeDivpd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f5ec1",
			wantFormat: "divpd %xmm1, %xmm0",
		},
		// divss
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeDivss, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "f30f5ec1",
			wantFormat: "divss %xmm1, %xmm0",
		},
		// divsd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeDivsd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "f20f5ec1",
			wantFormat: "divsd %xmm1, %xmm0",
		},
		// maxps
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMaxps, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "0f5fc1",
			wantFormat: "maxps %xmm1, %xmm0",
		},
		// maxpd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMaxpd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f5fc1",
			wantFormat: "maxpd %xmm1, %xmm0",
		},
		// maxss
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMaxss, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "f30f5fc1",
			wantFormat: "maxss %xmm1, %xmm0",
		},
		// maxsd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMaxsd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "f20f5fc1",
			wantFormat: "maxsd %xmm1, %xmm0",
		},
		// minps
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMinps, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "0f5dc1",
			wantFormat: "minps %xmm1, %xmm0",
		},
		// paddsw
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePaddsw, newOperandReg(xmm7VReg), xmm6VReg, true) },
			want:       "660fedf7",
			wantFormat: "paddsw %xmm7, %xmm6",
		},
		// paddusb
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePaddusb, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660fdcc1",
			wantFormat: "paddusb %xmm1, %xmm0",
		},
		// paddusw
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePaddusw, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660fddc1",
			wantFormat: "paddusw %xmm1, %xmm0",
		},
		// pand
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePand, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660fdbc1",
			wantFormat: "pand %xmm1, %xmm0",
		},
		// pandn
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePandn, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660fdfc1",
			wantFormat: "pandn %xmm1, %xmm0",
		},
		// pavgb
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePavgb, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660fe0c1",
			wantFormat: "pavgb %xmm1, %xmm0",
		},
		// pavgw
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePavgw, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660fe3c1",
			wantFormat: "pavgw %xmm1, %xmm0",
		},
		// pcmpeqb
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePcmpeqb, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f74c1",
			wantFormat: "pcmpeqb %xmm1, %xmm0",
		},
		// pcmpeqw
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePcmpeqw, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f75c1",
			wantFormat: "pcmpeqw %xmm1, %xmm0",
		},
		// pcmpeqd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePcmpeqd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f76c1",
			wantFormat: "pcmpeqd %xmm1, %xmm0",
		},
		// pcmpeqq
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePcmpeqq, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f3829c1",
			wantFormat: "pcmpeqq %xmm1, %xmm0",
		},
		// pcmpgtb
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePcmpgtb, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f64c1",
			wantFormat: "pcmpgtb %xmm1, %xmm0",
		},
		// pcmpgtw
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePcmpgtw, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f65c1",
			wantFormat: "pcmpgtw %xmm1, %xmm0",
		},
		// pcmpgtd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePcmpgtd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f66c1",
			wantFormat: "pcmpgtd %xmm1, %xmm0",
		},
		// pcmpgtq
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePcmpgtq, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f3837c1",
			wantFormat: "pcmpgtq %xmm1, %xmm0",
		},
		// pmaddwd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePmaddwd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660ff5c1",
			wantFormat: "pmaddwd %xmm1, %xmm0",
		},
		// pmaxsb
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePmaxsb, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f383cc1",
			wantFormat: "pmaxsb %xmm1, %xmm0",
		},
		// pmaxsw
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePmaxsw, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660feec1",
			wantFormat: "pmaxsw %xmm1, %xmm0",
		},
		// pmaxsd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePmaxsd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f383dc1",
			wantFormat: "pmaxsd %xmm1, %xmm0",
		},
		// pmaxub
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePmaxub, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660fdec1",
			wantFormat: "pmaxub %xmm1, %xmm0",
		},
		// pmaxuw
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePmaxuw, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f383ec1",
			wantFormat: "pmaxuw %xmm1, %xmm0",
		},
		// pmaxud
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePmaxud, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f383fc1",
			wantFormat: "pmaxud %xmm1, %xmm0",
		},
		// pminsb
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePminsb, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f3838c1",
			wantFormat: "pminsb %xmm1, %xmm0",
		},
		// pminsw
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePminsw, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660feac1",
			wantFormat: "pminsw %xmm1, %xmm0",
		},
		// pminsd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePminsd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f3839c1",
			wantFormat: "pminsd %xmm1, %xmm0",
		},
		// pminub
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePminub, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660fdac1",
			wantFormat: "pminub %xmm1, %xmm0",
		},
		// pminuw
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePminuw, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f383ac1",
			wantFormat: "pminuw %xmm1, %xmm0",
		},
		// pminud
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePminud, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f383bc1",
			wantFormat: "pminud %xmm1, %xmm0",
		},
		// pmulld
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePmulld, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f3840c1",
			wantFormat: "pmulld %xmm1, %xmm0",
		},
		// pmullw
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePmullw, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660fd5c1",
			wantFormat: "pmullw %xmm1, %xmm0",
		},
		// pmuludq
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePmuludq, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660ff4c1",
			wantFormat: "pmuludq %xmm1, %xmm0",
		},
		// por
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePor, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660febc1",
			wantFormat: "por %xmm1, %xmm0",
		},
		// pshufb
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePshufb, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f3800c1",
			wantFormat: "pshufb %xmm1, %xmm0",
		},
		// psubb
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePsubb, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660ff8c1",
			wantFormat: "psubb %xmm1, %xmm0",
		},
		// psubd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePsubd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660ffac1",
			wantFormat: "psubd %xmm1, %xmm0",
		},
		// psubq
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePsubq, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660ffbc1",
			wantFormat: "psubq %xmm1, %xmm0",
		},
		// psubw
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePsubw, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660ff9c1",
			wantFormat: "psubw %xmm1, %xmm0",
		},
		// psubsb
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePsubsb, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660fe8c1",
			wantFormat: "psubsb %xmm1, %xmm0",
		},
		// psubsw
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePsubsw, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660fe9c1",
			wantFormat: "psubsw %xmm1, %xmm0",
		},
		// psubusb
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePsubusb, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660fd8c1",
			wantFormat: "psubusb %xmm1, %xmm0",
		},
		// psubusw
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePsubusw, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660fd9c1",
			wantFormat: "psubusw %xmm1, %xmm0",
		},
		// punpckhbw
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePunpckhbw, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f68c1",
			wantFormat: "punpckhbw %xmm1, %xmm0",
		},
		// punpcklbw
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePunpcklbw, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f60c1",
			wantFormat: "punpcklbw %xmm1, %xmm0",
		},
		// pxor
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePxor, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660fefc1",
			wantFormat: "pxor %xmm1, %xmm0",
		},
		// subps
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeSubps, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "0f5cc1",
			wantFormat: "subps %xmm1, %xmm0",
		},
		// subpd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeSubpd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f5cc1",
			wantFormat: "subpd %xmm1, %xmm0",
		},
		// subss
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeSubss, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "f30f5cc1",
			wantFormat: "subss %xmm1, %xmm0",
		},
		// subsd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeSubsd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "f20f5cc1",
			wantFormat: "subsd %xmm1, %xmm0",
		},
		// xorps
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeXorps, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "0f57c1",
			wantFormat: "xorps %xmm1, %xmm0",
		},
		// xorpd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeXorpd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f57c1",
			wantFormat: "xorpd %xmm1, %xmm0",
		},
		// minpd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMinpd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f5dc1",
			wantFormat: "minpd %xmm1, %xmm0",
		},
		// minss
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMinss, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "f30f5dc1",
			wantFormat: "minss %xmm1, %xmm0",
		},
		// minsd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMinsd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "f20f5dc1",
			wantFormat: "minsd %xmm1, %xmm0",
		},
		// movlhps
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMovlhps, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "0f16c1",
			wantFormat: "movlhps %xmm1, %xmm0",
		},
		// movsd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMovsd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "f20f10c1",
			wantFormat: "movsd %xmm1, %xmm0",
		},
		// mulps
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMulps, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "0f59c1",
			wantFormat: "mulps %xmm1, %xmm0",
		},
		// mulpd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMulpd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f59c1",
			wantFormat: "mulpd %xmm1, %xmm0",
		},
		// mulss
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMulss, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "f30f59c1",
			wantFormat: "mulss %xmm1, %xmm0",
		},
		// mulsd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMulsd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "f20f59c1",
			wantFormat: "mulsd %xmm1, %xmm0",
		},
		// orpd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeOrpd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f56c1",
			wantFormat: "orpd %xmm1, %xmm0",
		},
		// orps
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeOrps, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "0f56c1",
			wantFormat: "orps %xmm1, %xmm0",
		},
		// packssdw
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePackssdw, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f6bc1",
			wantFormat: "packssdw %xmm1, %xmm0",
		},
		// packsswb
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePacksswb, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f63c1",
			wantFormat: "packsswb %xmm1, %xmm0",
		},
		// packusdw
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePackusdw, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f382bc1",
			wantFormat: "packusdw %xmm1, %xmm0",
		},
		// packuswb
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePackuswb, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f67c1",
			wantFormat: "packuswb %xmm1, %xmm0",
		},
		// paddb
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePaddb, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660ffcc1",
			wantFormat: "paddb %xmm1, %xmm0",
		},
		// paddd
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePaddd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660ffec1",
			wantFormat: "paddd %xmm1, %xmm0",
		},
		// paddq
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePaddq, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660fd4c1",
			wantFormat: "paddq %xmm1, %xmm0",
		},
		// paddw
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePaddw, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660ffdc1",
			wantFormat: "paddw %xmm1, %xmm0",
		},
		// paddsb
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePaddsb, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660fecc1",
			wantFormat: "paddsb %xmm1, %xmm0",
		},
		// cvtss2sd
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeCvtss2sd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "f30f5ac1",
			wantFormat: "cvtss2sd %xmm1, %xmm0",
		},
		// cvtsd2ss
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeCvtsd2ss, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "f20f5ac1",
			wantFormat: "cvtsd2ss %xmm1, %xmm0",
		},
		// movaps
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeMovaps, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "0f28c1",
			wantFormat: "movaps %xmm1, %xmm0",
		},
		// movapd
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeMovapd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f28c1",
			wantFormat: "movapd %xmm1, %xmm0",
		},
		// movdqa
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeMovdqa, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f6fc1",
			wantFormat: "movdqa %xmm1, %xmm0",
		},
		// movdqu
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeMovdqu, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "f30f6fc1",
			wantFormat: "movdqu %xmm1, %xmm0",
		},
		// movsd
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeMovsd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "f20f10c1",
			wantFormat: "movsd %xmm1, %xmm0",
		},
		// movss
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeMovss, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "f30f10c1",
			wantFormat: "movss %xmm1, %xmm0",
		},
		// movups
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeMovups, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "0f10c1",
			wantFormat: "movups %xmm1, %xmm0",
		},
		// movupd
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeMovupd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f10c1",
			wantFormat: "movupd %xmm1, %xmm0",
		},
		// pabsb
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePabsb, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f381cc1",
			wantFormat: "pabsb %xmm1, %xmm0",
		},
		// pabsw
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePabsw, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f381dc1",
			wantFormat: "pabsw %xmm1, %xmm0",
		},
		// pabsd
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePabsd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f381ec1",
			wantFormat: "pabsd %xmm1, %xmm0",
		},
		// pmovsxbd
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovsxbd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f3821c1",
			wantFormat: "pmovsxbd %xmm1, %xmm0",
		},
		// pmovsxbw
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovsxbw, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f3820c1",
			wantFormat: "pmovsxbw %xmm1, %xmm0",
		},
		// pmovsxbq
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovsxbq, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f3822c1",
			wantFormat: "pmovsxbq %xmm1, %xmm0",
		},
		// pmovsxwd
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovsxwd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f3823c1",
			wantFormat: "pmovsxwd %xmm1, %xmm0",
		},
		// pmovsxwq
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovsxwq, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f3824c1",
			wantFormat: "pmovsxwq %xmm1, %xmm0",
		},
		// pmovsxdq
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovsxdq, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f3825c1",
			wantFormat: "pmovsxdq %xmm1, %xmm0",
		},
		// pmovzxbd
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovzxbd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f3831c1",
			wantFormat: "pmovzxbd %xmm1, %xmm0",
		},
		// pmovzxbw
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovzxbw, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f3830c1",
			wantFormat: "pmovzxbw %xmm1, %xmm0",
		},
		// pmovzxbq
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovzxbq, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f3832c1",
			wantFormat: "pmovzxbq %xmm1, %xmm0",
		},
		// pmovzxwd
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovzxwd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f3833c1",
			wantFormat: "pmovzxwd %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmR(sseOpcodePmovzxwd, newOperandMem(newAmodeImmReg(0, rbpVReg)), xmm11VReg, true)
			},
			want:       "66440f38335d00",
			wantFormat: "pmovzxwd (%rbp), %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmR(sseOpcodePmovzxwd, newOperandMem(newAmodeImmReg(123, rbpVReg)), xmm11VReg, true)
			},
			want:       "66440f38335d7b",
			wantFormat: "pmovzxwd 123(%rbp), %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmR(sseOpcodePmovzxwd, newOperandMem(newAmodeImmReg(1<<25, rbpVReg)), xmm11VReg, true)
			},
			want:       "66440f38339d00000002",
			wantFormat: "pmovzxwd 33554432(%rbp), %xmm11",
		},
		// pmovzxwq
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovzxwq, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f3834c1",
			wantFormat: "pmovzxwq %xmm1, %xmm0",
		},
		// pmovzxdq
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovzxdq, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f3835c1",
			wantFormat: "pmovzxdq %xmm1, %xmm0",
		},
		// sqrtps
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeSqrtps, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "0f51c1",
			wantFormat: "sqrtps %xmm1, %xmm0",
		},
		// sqrtpd
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeSqrtpd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "660f51c1",
			wantFormat: "sqrtpd %xmm1, %xmm0",
		},
		// sqrtss
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeSqrtss, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "f30f51c1",
			wantFormat: "sqrtss %xmm1, %xmm0",
		},
		// sqrtsd
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeSqrtsd, newOperandReg(xmm1VReg), xmm0VReg, true) },
			want:       "f20f51c1",
			wantFormat: "sqrtsd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeSqrtsd, newOperandReg(xmm15VReg), xmm0VReg, true) },
			want:       "f2410f51c7",
			wantFormat: "sqrtsd %xmm15, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeSqrtsd, newOperandReg(xmm1VReg), xmm15VReg, true) },
			want:       "f2440f51f9",
			wantFormat: "sqrtsd %xmm1, %xmm15",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeSqrtsd, newOperandReg(xmm11VReg), xmm15VReg, true) },
			want:       "f2450f51fb",
			wantFormat: "sqrtsd %xmm11, %xmm15",
		},
		// movd
		{
			setup:      func(i *instruction) { i.asGprToXmm(sseOpcodeMovd, newOperandReg(rdiVReg), xmm0VReg, false) },
			want:       "660f6ec7",
			wantFormat: "movd %edi, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asGprToXmm(sseOpcodeMovd, newOperandMem(newAmodeImmReg(0, rspVReg)), xmm0VReg, true)
			},
			want:       "66480f6e0424",
			wantFormat: "movd (%rsp), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asGprToXmm(sseOpcodeMovd, newOperandMem(newAmodeImmReg(123, rspVReg)), xmm0VReg, true)
			},
			want:       "66480f6e44247b",
			wantFormat: "movd 123(%rsp), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asGprToXmm(sseOpcodeMovd, newOperandMem(newAmodeImmReg(1<<25, rspVReg)), xmm0VReg, true)
			},
			want:       "66480f6e842400000002",
			wantFormat: "movd 33554432(%rsp), %xmm0",
		},
		// movq
		{
			setup:      func(i *instruction) { i.asGprToXmm(sseOpcodeMovq, newOperandReg(rcxVReg), xmm0VReg, true) },
			want:       "66480f6ec1",
			wantFormat: "movq %rcx, %xmm0",
		},
		// cvtsi2ss
		{
			setup:      func(i *instruction) { i.asGprToXmm(sseOpcodeCvtsi2ss, newOperandReg(rdiVReg), xmm0VReg, true) },
			want:       "f3480f2ac7",
			wantFormat: "cvtsi2ss %rdi, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asGprToXmm(sseOpcodeCvtsi2ss, newOperandMem(newAmodeImmReg(0, rspVReg)), xmm0VReg, true)
			},
			want:       "f3480f2a0424",
			wantFormat: "cvtsi2ss (%rsp), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asGprToXmm(sseOpcodeCvtsi2ss, newOperandMem(newAmodeImmReg(123, rspVReg)), xmm0VReg, true)
			},
			want:       "f3480f2a44247b",
			wantFormat: "cvtsi2ss 123(%rsp), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asGprToXmm(sseOpcodeCvtsi2ss, newOperandMem(newAmodeImmReg(1<<25, rspVReg)), xmm0VReg, true)
			},
			want:       "f3480f2a842400000002",
			wantFormat: "cvtsi2ss 33554432(%rsp), %xmm0",
		},
		// cvtsi2sd
		{
			setup:      func(i *instruction) { i.asGprToXmm(sseOpcodeCvtsi2sd, newOperandReg(rdiVReg), xmm0VReg, true) },
			want:       "f2480f2ac7",
			wantFormat: "cvtsi2sd %rdi, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asGprToXmm(sseOpcodeCvtsi2sd, newOperandReg(rdiVReg), xmm0VReg, false) },
			want:       "f20f2ac7",
			wantFormat: "cvtsi2sd %edi, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asPop64(raxVReg) },
			want:       "58",
			wantFormat: "popq %rax",
		},
		{
			setup:      func(i *instruction) { i.asPop64(rdiVReg) },
			want:       "5f",
			wantFormat: "popq %rdi",
		},
		{
			setup:      func(i *instruction) { i.asPop64(r8VReg) },
			want:       "4158",
			wantFormat: "popq %r8",
		},
		{
			setup:      func(i *instruction) { i.asPop64(r15VReg) },
			want:       "415f",
			wantFormat: "popq %r15",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShit(0, rbpVReg, r13VReg, 2))) },
			want:       "42ff74ad00",
			wantFormat: "pushq (%rbp,%r13,4)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShit(1, rspVReg, r13VReg, 2))) },
			want:       "42ff74ac01",
			wantFormat: "pushq 1(%rsp,%r13,4)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShit(1<<14, rspVReg, r13VReg, 3))) },
			want:       "42ffb4ec00400000",
			wantFormat: "pushq 16384(%rsp,%r13,8)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShit(1<<14, rspVReg, rcxVReg, 3))) },
			want:       "ffb4cc00400000",
			wantFormat: "pushq 16384(%rsp,%rcx,8)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShit(1<<14, rspVReg, rbpVReg, 3))) },
			want:       "ffb4ec00400000",
			wantFormat: "pushq 16384(%rsp,%rbp,8)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShit(1<<14, rsiVReg, rcxVReg, 3))) },
			want:       "ffb4ce00400000",
			wantFormat: "pushq 16384(%rsi,%rcx,8)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShit(321, rsiVReg, rcxVReg, 3))) },
			want:       "ffb4ce41010000",
			wantFormat: "pushq 321(%rsi,%rcx,8)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShit(0, rsiVReg, rcxVReg, 3))) },
			want:       "ff34ce",
			wantFormat: "pushq (%rsi,%rcx,8)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShit(321, r9VReg, rbxVReg, 2))) },
			want:       "41ffb49941010000",
			wantFormat: "pushq 321(%r9,%rbx,4)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShit(321, r9VReg, rbpVReg, 2))) },
			want:       "41ffb4a941010000",
			wantFormat: "pushq 321(%r9,%rbp,4)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShit(321, r9VReg, r13VReg, 2))) },
			want:       "43ffb4a941010000",
			wantFormat: "pushq 321(%r9,%r13,4)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShit(321, rbpVReg, r13VReg, 2))) },
			want:       "42ffb4ad41010000",
			wantFormat: "pushq 321(%rbp,%r13,4)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShit(0, r9VReg, r13VReg, 2))) },
			want:       "43ff34a9",
			wantFormat: "pushq (%r9,%r13,4)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShit(1<<20, r9VReg, r13VReg, 2))) },
			want:       "43ffb4a900001000",
			wantFormat: "pushq 1048576(%r9,%r13,4)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandReg(rdiVReg)) },
			want:       "57",
			wantFormat: "pushq %rdi",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandReg(r8VReg)) },
			want:       "4150",
			wantFormat: "pushq %r8",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandImm32(128)) },
			want:       "6880000000",
			wantFormat: "pushq $128",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandImm32(0x31415927)) },
			want:       "6827594131",
			wantFormat: "pushq $826366247",
		},
		{
			setup: func(i *instruction) {
				v := int32(-128)
				i.asPush64(newOperandImm32(uint32(v)))
			},
			want:       "6880ffffff",
			wantFormat: "pushq $-128",
		},
		{
			setup: func(i *instruction) {
				v := int32(-129)
				i.asPush64(newOperandImm32(uint32(v)))
			},
			want:       "687fffffff",
			wantFormat: "pushq $-129",
		},
		{
			setup: func(i *instruction) {
				v := int32(-0x75c4e8a1)
				i.asPush64(newOperandImm32(uint32(v)))
			},
			want:       "685f173b8a",
			wantFormat: "pushq $-1975838881",
		},
		{
			setup: func(i *instruction) {
				i.asAluRmiR(aluRmiROpcodeMul, newOperandMem(newAmodeImmReg(0, rdiVReg)), rdxVReg, true)
			},
			want:       "480faf17",
			wantFormat: "imul (%rdi), %rdx",
		},
		{
			setup: func(i *instruction) {
				i.asAluRmiR(aluRmiROpcodeMul, newOperandMem(newAmodeImmReg(99, rdiVReg)), rdxVReg, true)
			},
			want:       "480faf5763",
			wantFormat: "imul 99(%rdi), %rdx",
		},
		{
			setup: func(i *instruction) {
				i.asAluRmiR(aluRmiROpcodeMul, newOperandMem(newAmodeImmReg(1<<21, rdiVReg)), rdxVReg, true)
			},
			want:       "480faf9700002000",
			wantFormat: "imul 2097152(%rdi), %rdx",
		},
		{
			setup: func(i *instruction) {
				i.asAluRmiR(aluRmiROpcodeMul, newOperandReg(r15VReg), rdxVReg, true)
			},
			want:       "490fafd7",
			wantFormat: "imul %r15, %rdx",
		},
		{
			setup: func(i *instruction) {
				i.asAluRmiR(aluRmiROpcodeMul, newOperandReg(rcxVReg), r8VReg, false)
			},
			want:       "440fafc1",
			wantFormat: "imul %ecx, %r8d",
		},
		{
			setup: func(i *instruction) {
				i.asAluRmiR(aluRmiROpcodeMul, newOperandReg(rcxVReg), rsiVReg, false)
			},
			want:       "0faff1",
			wantFormat: "imul %ecx, %esi",
		},
		{
			setup: func(i *instruction) {
				i.asAluRmiR(aluRmiROpcodeMul, newOperandImm32(76543210), rdxVReg, true)
			},
			want:       "4869d2eaf48f04",
			wantFormat: "imul $76543210, %rdx",
		},
		{
			setup: func(i *instruction) {
				minusOne := int32(-1)
				i.asAluRmiR(aluRmiROpcodeMul, newOperandImm32(uint32(minusOne)), rdxVReg, true)
			},
			want:       "486bd2ff",
			wantFormat: "imul $-1, %rdx",
		},
		{
			setup: func(i *instruction) {
				i.asAluRmiR(aluRmiROpcodeMul, newOperandImm32(76543210), rdxVReg, false)
			},
			want:       "69d2eaf48f04",
			wantFormat: "imul $76543210, %edx",
		},
		{
			setup: func(i *instruction) {
				minusOne := int32(-1)
				i.asAluRmiR(aluRmiROpcodeMul, newOperandImm32(uint32(minusOne)), rdxVReg, false)
			},
			want:       "6bd2ff",
			wantFormat: "imul $-1, %edx",
		},
		{
			setup: func(i *instruction) {
				i.asAluRmiR(aluRmiROpcodeAdd, newOperandMem(newAmodeImmReg(0, r12VReg)), rdxVReg, true)
			},
			want:       "49031424",
			wantFormat: "add (%r12), %rdx",
		},
		{
			setup: func(i *instruction) {
				i.asAluRmiR(aluRmiROpcodeAdd, newOperandMem(newAmodeImmReg(123, r12VReg)), rdxVReg, true)
			},
			want:       "490354247b",
			wantFormat: "add 123(%r12), %rdx",
		},
		{
			setup: func(i *instruction) {
				i.asAluRmiR(aluRmiROpcodeAdd, newOperandMem(newAmodeImmReg(1<<25, r12VReg)), rdxVReg, true)
			},
			want:       "4903942400000002",
			wantFormat: "add 33554432(%r12), %rdx",
		},
		{
			setup: func(i *instruction) {
				i.asAluRmiR(aluRmiROpcodeAdd, newOperandImm32(76543210), r15VReg, true)
			},
			want:       "4981c7eaf48f04",
			wantFormat: "add $76543210, %r15",
		},
		{
			setup: func(i *instruction) {
				minusOne := int32(-1)
				i.asAluRmiR(aluRmiROpcodeAdd, newOperandImm32(uint32(minusOne)), r15VReg, true)
			},
			want:       "4983c7ff",
			wantFormat: "add $-1, %r15",
		},
		{
			setup: func(i *instruction) {
				i.asAluRmiR(aluRmiROpcodeAdd, newOperandImm32(76543210), rsiVReg, true)
			},
			want:       "4881c6eaf48f04",
			wantFormat: "add $76543210, %rsi",
		},
		{
			setup: func(i *instruction) {
				minusOne := int32(-1)
				i.asAluRmiR(aluRmiROpcodeAdd, newOperandImm32(uint32(minusOne)), rsiVReg, true)
			},
			want:       "4883c6ff",
			wantFormat: "add $-1, %rsi",
		},
		{
			setup: func(i *instruction) {
				i.asAluRmiR(aluRmiROpcodeAdd, newOperandImm32(76543210), r15VReg, false)
			},
			want:       "4181c7eaf48f04",
			wantFormat: "add $76543210, %r15d",
		},
		{
			setup: func(i *instruction) {
				minusOne := int32(-1)
				i.asAluRmiR(aluRmiROpcodeAdd, newOperandImm32(uint32(minusOne)), r15VReg, false)
			},
			want:       "4183c7ff",
			wantFormat: "add $-1, %r15d",
		},
		{
			setup: func(i *instruction) {
				i.asAluRmiR(aluRmiROpcodeAdd, newOperandImm32(76543210), rsiVReg, false)
			},
			want:       "81c6eaf48f04",
			wantFormat: "add $76543210, %esi",
		},
		{
			setup: func(i *instruction) {
				minusOne := int32(-1)
				i.asAluRmiR(aluRmiROpcodeAdd, newOperandImm32(uint32(minusOne)), rsiVReg, false)
			},
			want:       "83c6ff",
			wantFormat: "add $-1, %esi",
		},
		{
			setup:      func(i *instruction) { i.asAluRmiR(aluRmiROpcodeAdd, newOperandReg(raxVReg), rdxVReg, true) },
			want:       "4801c2",
			wantFormat: "add %rax, %rdx",
		},
		{
			setup:      func(i *instruction) { i.asAluRmiR(aluRmiROpcodeAdd, newOperandReg(raxVReg), r15VReg, true) },
			want:       "4901c7",
			wantFormat: "add %rax, %r15",
		},
		{
			setup:      func(i *instruction) { i.asAluRmiR(aluRmiROpcodeAdd, newOperandReg(r11VReg), r15VReg, true) },
			want:       "4d01df",
			wantFormat: "add %r11, %r15",
		},
		{
			setup:      func(i *instruction) { i.asAluRmiR(aluRmiROpcodeAdd, newOperandReg(raxVReg), rdxVReg, false) },
			want:       "01c2",
			wantFormat: "add %eax, %edx",
		},
		{
			setup:      func(i *instruction) { i.asAluRmiR(aluRmiROpcodeAdd, newOperandReg(raxVReg), r15VReg, false) },
			want:       "4101c7",
			wantFormat: "add %eax, %r15d",
		},
		{
			setup:      func(i *instruction) { i.asAluRmiR(aluRmiROpcodeAdd, newOperandReg(r11VReg), r15VReg, false) },
			want:       "4501df",
			wantFormat: "add %r11d, %r15d",
		},
		{
			setup:      func(i *instruction) { i.asAluRmiR(aluRmiROpcodeSub, newOperandReg(r11VReg), r15VReg, false) },
			want:       "4529df",
			wantFormat: "sub %r11d, %r15d",
		},
		{
			setup: func(i *instruction) {
				i.asAluRmiR(aluRmiROpcodeSub, newOperandMem(newAmodeImmReg(0, r13VReg)), rdxVReg, true)
			},
			want:       "492b5500",
			wantFormat: "sub (%r13), %rdx",
		},
		{
			setup: func(i *instruction) {
				i.asAluRmiR(aluRmiROpcodeSub, newOperandMem(newAmodeImmReg(123, r13VReg)), rdxVReg, true)
			},
			want:       "492b557b",
			wantFormat: "sub 123(%r13), %rdx",
		},
		{
			setup: func(i *instruction) {
				i.asAluRmiR(aluRmiROpcodeSub, newOperandMem(newAmodeImmReg(1<<25, r13VReg)), rdxVReg, true)
			},
			want:       "492b9500000002",
			wantFormat: "sub 33554432(%r13), %rdx",
		},
		{
			setup:      func(i *instruction) { i.asAluRmiR(aluRmiROpcodeAnd, newOperandReg(r11VReg), r15VReg, false) },
			want:       "4521df",
			wantFormat: "and %r11d, %r15d",
		},
		{
			setup:      func(i *instruction) { i.asAluRmiR(aluRmiROpcodeOr, newOperandReg(r11VReg), r15VReg, false) },
			want:       "4509df",
			wantFormat: "or %r11d, %r15d",
		},
		{
			setup:      func(i *instruction) { i.asAluRmiR(aluRmiROpcodeXor, newOperandReg(r11VReg), r15VReg, false) },
			want:       "4531df",
			wantFormat: "xor %r11d, %r15d",
		},
		{
			setup:      func(i *instruction) { i.asLEA(newAmodeImmReg(0, rdiVReg), rdxVReg) },
			want:       "488d17",
			wantFormat: "lea (%rdi), %rdx",
		},
		{
			setup:      func(i *instruction) { i.asLEA(newAmodeImmReg(0xffff, rdiVReg), rdxVReg) },
			want:       "488d97ffff0000",
			wantFormat: "lea 65535(%rdi), %rdx",
		},
		{
			setup:      func(i *instruction) { i.asLEA(newAmodeRegRegShit(0xffff, rspVReg, r13VReg, 3), rdxVReg) },
			want:       "4a8d94ecffff0000",
			wantFormat: "lea 65535(%rsp,%r13,8), %rdx",
		},
		{
			setup: func(i *instruction) {
				a := newAmodeRipRelative(1)
				a.resolveRipRelative(1234)
				i.asLEA(a, r11VReg)
			},
			want:       "4c8d1dd2040000",
			wantFormat: "lea 1234(%rip), %r11",
		},
		{
			setup: func(i *instruction) {
				a := newAmodeRipRelative(1)
				a.resolveRipRelative(8)
				i.asLEA(a, r11VReg)
			},
			want:       "4c8d1d08000000",
			wantFormat: "lea 8(%rip), %r11",
		},
		{
			setup:      func(i *instruction) { i.kind = ud2 },
			want:       "0f0b",
			wantFormat: "ud2",
		},
		{
			setup: func(i *instruction) {
				i.asCall(0, nil)
				i.u2 = 0xff
			},
			want:       "e8ff000000",
			wantFormat: "call $255",
		},
		{
			setup: func(i *instruction) {
				i.asCallIndirect(newOperandReg(r12VReg), nil)
			},
			want:       "41ffd4",
			wantFormat: "callq *%r12",
		},
		{
			setup: func(i *instruction) {
				i.asCallIndirect(newOperandMem(newAmodeImmReg(0, raxVReg)), nil)
			},
			want:       "ff10",
			wantFormat: "callq *(%rax)",
		},
		{
			setup: func(i *instruction) {
				i.asCallIndirect(newOperandMem(newAmodeImmReg(0xffff_0000, raxVReg)), nil)
			},
			want:       "ff900000ffff",
			wantFormat: "callq *-65536(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asJmp(newOperandReg(r12VReg)) },
			want:       "41ffe4",
			wantFormat: "jmp %r12",
		},
		{
			setup:      func(i *instruction) { i.asJmp(newOperandMem(newAmodeImmReg(0, raxVReg))) },
			want:       "ff20",
			wantFormat: "jmp (%rax)",
		},
		{
			setup:      func(i *instruction) { i.asJmp(newOperandImm32(12345)) },
			want:       "e939300000",
			wantFormat: "jmp $12345",
		},
		{
			setup:      func(i *instruction) { i.asJmpIf(condO, newOperandImm32(4)) },
			want:       "0f8004000000",
			wantFormat: "jo $4",
		},
		{
			setup:      func(i *instruction) { i.asJmpIf(condNO, newOperandImm32(4)) },
			want:       "0f8104000000",
			wantFormat: "jno $4",
		},
		{
			setup:      func(i *instruction) { i.asJmpIf(condB, newOperandImm32(4)) },
			want:       "0f8204000000",
			wantFormat: "jb $4",
		},
		{
			setup:      func(i *instruction) { i.asJmpIf(condNB, newOperandImm32(4)) },
			want:       "0f8304000000",
			wantFormat: "jnb $4",
		},
		{
			setup:      func(i *instruction) { i.asJmpIf(condZ, newOperandImm32(4)) },
			want:       "0f8404000000",
			wantFormat: "jz $4",
		},
		{
			setup:      func(i *instruction) { i.asJmpIf(condNZ, newOperandImm32(4)) },
			want:       "0f8504000000",
			wantFormat: "jnz $4",
		},
		{
			setup:      func(i *instruction) { i.asJmpIf(condBE, newOperandImm32(4)) },
			want:       "0f8604000000",
			wantFormat: "jbe $4",
		},
		{
			setup:      func(i *instruction) { i.asJmpIf(condNBE, newOperandImm32(4)) },
			want:       "0f8704000000",
			wantFormat: "jnbe $4",
		},
		{
			setup:      func(i *instruction) { i.asJmpIf(condS, newOperandImm32(4)) },
			want:       "0f8804000000",
			wantFormat: "js $4",
		},
		{
			setup:      func(i *instruction) { i.asJmpIf(condNS, newOperandImm32(4)) },
			want:       "0f8904000000",
			wantFormat: "jns $4",
		},
		{
			setup:      func(i *instruction) { i.asJmpIf(condL, newOperandImm32(4)) },
			want:       "0f8c04000000",
			wantFormat: "jl $4",
		},
		{
			setup:      func(i *instruction) { i.asJmpIf(condNL, newOperandImm32(4)) },
			want:       "0f8d04000000",
			wantFormat: "jnl $4",
		},
		{
			setup:      func(i *instruction) { i.asJmpIf(condLE, newOperandImm32(4)) },
			want:       "0f8e04000000",
			wantFormat: "jle $4",
		},
		{
			setup:      func(i *instruction) { i.asJmpIf(condNLE, newOperandImm32(4)) },
			want:       "0f8f04000000",
			wantFormat: "jnle $4",
		},
		{
			setup:      func(i *instruction) { i.asJmpIf(condP, newOperandImm32(4)) },
			want:       "0f8a04000000",
			wantFormat: "jp $4",
		},
		{
			setup:      func(i *instruction) { i.asJmpIf(condNP, newOperandImm32(4)) },
			want:       "0f8b04000000",
			wantFormat: "jnp $4",
		},
		{
			setup: func(i *instruction) {
				i.asMov64MR(newOperandMem(
					newAmodeImmReg(0, raxVReg),
				), r15VReg)
			},
			want:       "4c8b38",
			wantFormat: "movq (%rax), %r15",
		},
		{
			setup: func(i *instruction) {
				i.asMov64MR(newOperandMem(
					newAmodeImmReg(0, r12VReg),
				), r15VReg)
			},
			want:       "4d8b3c24",
			wantFormat: "movq (%r12), %r15",
		},
		{
			setup: func(i *instruction) {
				i.asMov64MR(newOperandMem(
					newAmodeImmReg(1, r12VReg),
				), r15VReg)
			},
			want:       "4d8b7c2401",
			wantFormat: "movq 1(%r12), %r15",
		},
		{
			setup: func(i *instruction) {
				i.asMov64MR(newOperandMem(
					newAmodeImmReg(1<<20, r12VReg),
				), r15VReg)
			},
			want:       "4d8bbc2400001000",
			wantFormat: "movq 1048576(%r12), %r15",
		},
		{
			setup: func(i *instruction) {
				i.asMov64MR(newOperandMem(
					newAmodeImmReg(1<<20, raxVReg),
				), r15VReg)
			},
			want:       "4c8bb800001000",
			wantFormat: "movq 1048576(%rax), %r15",
		},
		//
		{
			setup:      func(i *instruction) { i.asMovsxRmR(extModeBL, newOperandReg(raxVReg), rdiVReg) },
			want:       "0fbef8",
			wantFormat: "movsx.bl %rax, %rdi",
		},
		{
			setup:      func(i *instruction) { i.asMovsxRmR(extModeBL, newOperandReg(rbxVReg), rdiVReg) },
			want:       "0fbefb",
			wantFormat: "movsx.bl %rbx, %rdi",
		},
		{
			setup:      func(i *instruction) { i.asMovsxRmR(extModeBQ, newOperandReg(raxVReg), rdiVReg) },
			want:       "480fbef8",
			wantFormat: "movsx.bq %rax, %rdi",
		},
		{
			setup:      func(i *instruction) { i.asMovsxRmR(extModeBQ, newOperandReg(r15VReg), rdiVReg) },
			want:       "490fbeff",
			wantFormat: "movsx.bq %r15, %rdi",
		},
		{
			setup:      func(i *instruction) { i.asMovsxRmR(extModeWL, newOperandReg(raxVReg), rdiVReg) },
			want:       "0fbff8",
			wantFormat: "movsx.wl %rax, %rdi",
		},
		{
			setup:      func(i *instruction) { i.asMovsxRmR(extModeWQ, newOperandReg(raxVReg), rdiVReg) },
			want:       "480fbff8",
			wantFormat: "movsx.wq %rax, %rdi",
		},
		{
			setup:      func(i *instruction) { i.asMovsxRmR(extModeLQ, newOperandReg(raxVReg), rdiVReg) },
			want:       "4863f8",
			wantFormat: "movsx.lq %rax, %rdi",
		},
		{
			setup:      func(i *instruction) { i.asMovzxRmR(extModeBL, newOperandReg(raxVReg), rdiVReg) },
			want:       "0fb6f8",
			wantFormat: "movzx.bl %rax, %rdi",
		},
		{
			setup:      func(i *instruction) { i.asMovzxRmR(extModeBL, newOperandReg(rbxVReg), rdiVReg) },
			want:       "0fb6fb",
			wantFormat: "movzx.bl %rbx, %rdi",
		},
		{
			setup:      func(i *instruction) { i.asMovzxRmR(extModeBQ, newOperandReg(raxVReg), rdiVReg) },
			want:       "480fb6f8",
			wantFormat: "movzx.bq %rax, %rdi",
		},
		{
			setup:      func(i *instruction) { i.asMovzxRmR(extModeBQ, newOperandReg(r15VReg), rdiVReg) },
			want:       "490fb6ff",
			wantFormat: "movzx.bq %r15, %rdi",
		},
		{
			setup:      func(i *instruction) { i.asMovzxRmR(extModeWL, newOperandReg(raxVReg), rdiVReg) },
			want:       "0fb7f8",
			wantFormat: "movzx.wl %rax, %rdi",
		},
		{
			setup:      func(i *instruction) { i.asMovzxRmR(extModeWQ, newOperandReg(raxVReg), rdiVReg) },
			want:       "480fb7f8",
			wantFormat: "movzx.wq %rax, %rdi",
		},
		{
			setup:      func(i *instruction) { i.asMovzxRmR(extModeLQ, newOperandReg(raxVReg), rdiVReg) },
			want:       "8bf8",
			wantFormat: "movzx.lq %rax, %rdi",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeImmReg(1<<20, raxVReg))
				i.asMovsxRmR(extModeBQ, a, rdiVReg)
			},
			want:       "480fbeb800001000",
			wantFormat: "movsx.bq 1048576(%rax), %rdi",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeImmReg(1<<20, raxVReg))
				i.asMovsxRmR(extModeBL, a, rdiVReg)
			},
			want:       "0fbeb800001000",
			wantFormat: "movsx.bl 1048576(%rax), %rdi",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeImmReg(1<<20, raxVReg))
				i.asMovsxRmR(extModeWL, a, rdiVReg)
			},
			want:       "0fbfb800001000",
			wantFormat: "movsx.wl 1048576(%rax), %rdi",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeImmReg(1<<20, raxVReg))
				i.asMovsxRmR(extModeWQ, a, rdiVReg)
			},
			want:       "480fbfb800001000",
			wantFormat: "movsx.wq 1048576(%rax), %rdi",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeImmReg(1<<20, raxVReg))
				i.asMovsxRmR(extModeLQ, a, rdiVReg)
			},
			want:       "4863b800001000",
			wantFormat: "movsx.lq 1048576(%rax), %rdi",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeImmReg(1<<20, raxVReg))
				i.asMovzxRmR(extModeLQ, a, rdiVReg)
			},
			want:       "8bb800001000",
			wantFormat: "movzx.lq 1048576(%rax), %rdi",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeImmReg(1<<20, raxVReg))
				i.asMovRM(rcxVReg, a, 1)
			},
			want:       "888800001000",
			wantFormat: "mov.b %rcx, 1048576(%rax)",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeImmReg(1<<20, raxVReg))
				i.asMovRM(rdiVReg, a, 1)
			},
			want:       "88b800001000",
			wantFormat: "mov.b %rdi, 1048576(%rax)",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeRegRegShit(1<<20, raxVReg, rcxVReg, 3))
				i.asMovRM(rdiVReg, a, 2)
			},
			want:       "6689bcc800001000",
			wantFormat: "mov.w %rdi, 1048576(%rax,%rcx,8)",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeRegRegShit(123, raxVReg, rcxVReg, 0))
				i.asMovRM(rdiVReg, a, 4)
			},
			want:       "897c087b",
			wantFormat: "mov.l %rdi, 123(%rax,%rcx,1)",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeRegRegShit(123, raxVReg, rcxVReg, 0))
				i.asMovRM(rdiVReg, a, 8)
			},
			want:       "48897c087b",
			wantFormat: "mov.q %rdi, 123(%rax,%rcx,1)",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeRegRegShit(123, raxVReg, rcxVReg, 0))
				i.asXmmMovRM(sseOpcodeMovaps, xmm1VReg, a)
			},
			want:       "0f294c087b",
			wantFormat: "movaps %xmm1, 123(%rax,%rcx,1)",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeImmReg(123, rcxVReg))
				i.asXmmMovRM(sseOpcodeMovaps, xmm15VReg, a)
			},
			want:       "440f29797b",
			wantFormat: "movaps %xmm15, 123(%rcx)",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeImmReg(123, rcxVReg))
				i.asXmmMovRM(sseOpcodeMovapd, xmm15VReg, a)
			},
			want:       "66440f29797b",
			wantFormat: "movapd %xmm15, 123(%rcx)",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeImmReg(123, rcxVReg))
				i.asXmmMovRM(sseOpcodeMovdqa, xmm15VReg, a)
			},
			want:       "66440f7f797b",
			wantFormat: "movdqa %xmm15, 123(%rcx)",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeImmReg(123, rcxVReg))
				i.asXmmMovRM(sseOpcodeMovdqu, xmm15VReg, a)
			},
			want:       "f3440f7f797b",
			wantFormat: "movdqu %xmm15, 123(%rcx)",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeImmReg(123, rcxVReg))
				i.asXmmMovRM(sseOpcodeMovss, xmm15VReg, a)
			},
			want:       "f3440f11797b",
			wantFormat: "movss %xmm15, 123(%rcx)",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeImmReg(123, rcxVReg))
				i.asXmmMovRM(sseOpcodeMovsd, xmm15VReg, a)
			},
			want:       "f2440f11797b",
			wantFormat: "movsd %xmm15, 123(%rcx)",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeImmReg(123, rcxVReg))
				i.asXmmMovRM(sseOpcodeMovups, xmm15VReg, a)
			},
			want:       "440f11797b",
			wantFormat: "movups %xmm15, 123(%rcx)",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeImmReg(123, rcxVReg))
				i.asXmmMovRM(sseOpcodeMovupd, xmm15VReg, a)
			},
			want:       "66440f11797b",
			wantFormat: "movupd %xmm15, 123(%rcx)",
		},
	} {
		tc := tc
		t.Run(tc.wantFormat, func(t *testing.T) {
			i := &instruction{}
			tc.setup(i)

			require.Equal(t, tc.wantFormat, i.String())

			mc := &mockCompiler{}
			m := &machine{c: mc}
			i.encode(m.c)
			require.Equal(t, tc.want, hex.EncodeToString(mc.buf))

			// TODO: verify the size of the encoded instructions.
			//var actualSize int
			//for cur := i; cur != nil; cur = cur.next {
			//	actualSize += int(cur.size())
			//}
			//require.Equal(t, len(tc.want)/2, actualSize)
		})
	}
}
