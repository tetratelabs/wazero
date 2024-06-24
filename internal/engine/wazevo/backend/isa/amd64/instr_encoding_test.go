package amd64

import (
	"encoding/hex"
	"runtime"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestInstruction_format_encode(t *testing.T) {
	_, _, m := newSetupWithMockContext()
	defer func() { runtime.KeepAlive(m) }()
	newAmodeImmReg := m.newAmodeImmReg
	newAmodeRegRegShift := m.newAmodeRegRegShift
	allocateExitSeq := m.allocateExitSeq

	for _, tc := range []struct {
		setup      func(*instruction)
		want       string
		wantFormat string
	}{
		{
			setup:      func(i *instruction) { i.asRet() },
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
			setup:      func(i *instruction) { i.asSignExtendData(false) },
			want:       "99",
			wantFormat: "cdq",
		},
		{
			setup:      func(i *instruction) { i.asSignExtendData(true) },
			want:       "4899",
			wantFormat: "cqo",
		},
		{
			setup:      func(i *instruction) { i.asDiv(newOperandReg(raxVReg), true, true) },
			want:       "48f7f8",
			wantFormat: "idivq %rax",
		},
		{
			setup:      func(i *instruction) { i.asDiv(newOperandReg(raxVReg), false, true) },
			want:       "48f7f0",
			wantFormat: "divq %rax",
		},
		{
			setup:      func(i *instruction) { i.asDiv(newOperandReg(raxVReg), true, false) },
			want:       "f7f8",
			wantFormat: "idivl %eax",
		},
		{
			setup:      func(i *instruction) { i.asDiv(newOperandReg(raxVReg), false, false) },
			want:       "f7f0",
			wantFormat: "divl %eax",
		},
		{
			setup:      func(i *instruction) { i.asDiv(newOperandMem(newAmodeImmReg(123, raxVReg)), true, true) },
			want:       "48f7787b",
			wantFormat: "idivq 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asDiv(newOperandMem(newAmodeImmReg(123, raxVReg)), false, true) },
			want:       "48f7707b",
			wantFormat: "divq 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asDiv(newOperandMem(newAmodeImmReg(123, raxVReg)), true, false) },
			want:       "f7787b",
			wantFormat: "idivl 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asDiv(newOperandMem(newAmodeImmReg(123, raxVReg)), false, false) },
			want:       "f7707b",
			wantFormat: "divl 123(%rax)",
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
			setup:      func(i *instruction) { i.asMulHi(newOperandMem(newAmodeImmReg(123, raxVReg)), true, false) },
			want:       "f7687b",
			wantFormat: "imull 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asMulHi(newOperandMem(newAmodeImmReg(123, raxVReg)), false, false) },
			want:       "f7607b",
			wantFormat: "mull 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asMulHi(newOperandMem(newAmodeImmReg(123, raxVReg)), true, true) },
			want:       "48f7687b",
			wantFormat: "imulq 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asMulHi(newOperandMem(newAmodeImmReg(123, raxVReg)), false, true) },
			want:       "48f7607b",
			wantFormat: "mulq 123(%rax)",
		},
		// bsr
		{
			setup:      func(i *instruction) { i.asUnaryRmR(unaryRmROpcodeBsr, newOperandReg(raxVReg), rdiVReg, false) },
			want:       "0fbdf8",
			wantFormat: "bsrl %eax, %edi",
		},
		{
			setup: func(i *instruction) {
				i.asUnaryRmR(unaryRmROpcodeBsr, newOperandMem(newAmodeImmReg(123, raxVReg)), rdiVReg, false)
			},
			want:       "0fbd787b",
			wantFormat: "bsrl 123(%rax), %edi",
		},
		// bsf
		{
			setup:      func(i *instruction) { i.asUnaryRmR(unaryRmROpcodeBsf, newOperandReg(raxVReg), rdiVReg, false) },
			want:       "0fbcf8",
			wantFormat: "bsfl %eax, %edi",
		},
		{
			setup: func(i *instruction) {
				i.asUnaryRmR(unaryRmROpcodeBsf, newOperandMem(newAmodeImmReg(123, raxVReg)), rdiVReg, false)
			},
			want:       "0fbc787b",
			wantFormat: "bsfl 123(%rax), %edi",
		},
		// tzcnt
		{
			setup:      func(i *instruction) { i.asUnaryRmR(unaryRmROpcodeTzcnt, newOperandReg(raxVReg), rdiVReg, false) },
			want:       "f30fbcf8",
			wantFormat: "tzcntl %eax, %edi",
		},
		{
			setup: func(i *instruction) {
				i.asUnaryRmR(unaryRmROpcodeTzcnt, newOperandMem(newAmodeImmReg(123, raxVReg)), rdiVReg, false)
			},
			want:       "f30fbc787b",
			wantFormat: "tzcntl 123(%rax), %edi",
		},
		// lzcnt
		{
			setup:      func(i *instruction) { i.asUnaryRmR(unaryRmROpcodeLzcnt, newOperandReg(raxVReg), rdiVReg, false) },
			want:       "f30fbdf8",
			wantFormat: "lzcntl %eax, %edi",
		},
		{
			setup: func(i *instruction) {
				i.asUnaryRmR(unaryRmROpcodeLzcnt, newOperandMem(newAmodeImmReg(123, raxVReg)), rdiVReg, false)
			},
			want:       "f30fbd787b",
			wantFormat: "lzcntl 123(%rax), %edi",
		},
		// popcnt
		{
			setup:      func(i *instruction) { i.asUnaryRmR(unaryRmROpcodePopcnt, newOperandReg(raxVReg), rdiVReg, false) },
			want:       "f30fb8f8",
			wantFormat: "popcntl %eax, %edi",
		},
		{
			setup: func(i *instruction) {
				i.asUnaryRmR(unaryRmROpcodePopcnt, newOperandMem(newAmodeImmReg(123, raxVReg)), rdiVReg, false)
			},
			want:       "f30fb8787b",
			wantFormat: "popcntl 123(%rax), %edi",
		},
		// bsr
		{
			setup:      func(i *instruction) { i.asUnaryRmR(unaryRmROpcodeBsr, newOperandReg(raxVReg), rdiVReg, true) },
			want:       "480fbdf8",
			wantFormat: "bsrq %rax, %rdi",
		},
		{
			setup: func(i *instruction) {
				i.asUnaryRmR(unaryRmROpcodeBsr, newOperandMem(newAmodeImmReg(123, raxVReg)), rdiVReg, true)
			},
			want:       "480fbd787b",
			wantFormat: "bsrq 123(%rax), %rdi",
		},
		// bsf
		{
			setup:      func(i *instruction) { i.asUnaryRmR(unaryRmROpcodeBsf, newOperandReg(raxVReg), rdiVReg, true) },
			want:       "480fbcf8",
			wantFormat: "bsfq %rax, %rdi",
		},
		{
			setup: func(i *instruction) {
				i.asUnaryRmR(unaryRmROpcodeBsf, newOperandMem(newAmodeImmReg(123, raxVReg)), rdiVReg, true)
			},
			want:       "480fbc787b",
			wantFormat: "bsfq 123(%rax), %rdi",
		},
		// tzcnt
		{
			setup:      func(i *instruction) { i.asUnaryRmR(unaryRmROpcodeTzcnt, newOperandReg(raxVReg), rdiVReg, true) },
			want:       "f3480fbcf8",
			wantFormat: "tzcntq %rax, %rdi",
		},
		{
			setup: func(i *instruction) {
				i.asUnaryRmR(unaryRmROpcodeTzcnt, newOperandMem(newAmodeImmReg(123, raxVReg)), rdiVReg, true)
			},
			want:       "f3480fbc787b",
			wantFormat: "tzcntq 123(%rax), %rdi",
		},
		// lzcnt
		{
			setup:      func(i *instruction) { i.asUnaryRmR(unaryRmROpcodeLzcnt, newOperandReg(raxVReg), rdiVReg, true) },
			want:       "f3480fbdf8",
			wantFormat: "lzcntq %rax, %rdi",
		},
		{
			setup: func(i *instruction) {
				i.asUnaryRmR(unaryRmROpcodeLzcnt, newOperandMem(newAmodeImmReg(123, raxVReg)), rdiVReg, true)
			},
			want:       "f3480fbd787b",
			wantFormat: "lzcntq 123(%rax), %rdi",
		},
		// popcnt
		{
			setup:      func(i *instruction) { i.asUnaryRmR(unaryRmROpcodePopcnt, newOperandReg(raxVReg), rdiVReg, true) },
			want:       "f3480fb8f8",
			wantFormat: "popcntq %rax, %rdi",
		},
		{
			setup: func(i *instruction) {
				i.asUnaryRmR(unaryRmROpcodePopcnt, newOperandMem(newAmodeImmReg(123, raxVReg)), rdiVReg, true)
			},
			want:       "f3480fb8787b",
			wantFormat: "popcntq 123(%rax), %rdi",
		},
		// addss
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAddss, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f30f58c1",
			wantFormat: "addss %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAddss, newOperandReg(xmm13VReg), xmm11VReg) },
			want:       "f3450f58dd",
			wantFormat: "addss %xmm13, %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmR(sseOpcodeAddss, newOperandMem(newAmodeImmReg(0, r13VReg)), xmm11VReg)
			},
			want:       "f3450f585d00",
			wantFormat: "addss (%r13), %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmR(sseOpcodeAddss, newOperandMem(newAmodeImmReg(123, r13VReg)), xmm11VReg)
			},
			want:       "f3450f585d7b",
			wantFormat: "addss 123(%r13), %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmR(sseOpcodeAddss, newOperandMem(newAmodeImmReg(1<<25, r13VReg)), xmm11VReg)
			},
			want:       "f3450f589d00000002",
			wantFormat: "addss 33554432(%r13), %xmm11",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAddsd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f20f58c1",
			wantFormat: "addsd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAddsd, newOperandReg(xmm13VReg), xmm11VReg) },
			want:       "f2450f58dd",
			wantFormat: "addsd %xmm13, %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmR(sseOpcodeAddsd, newOperandMem(newAmodeImmReg(0, rbpVReg)), xmm11VReg)
			},
			want:       "f2440f585d00",
			wantFormat: "addsd (%rbp), %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmR(sseOpcodeAddsd, newOperandMem(newAmodeImmReg(123, rbpVReg)), xmm11VReg)
			},
			want:       "f2440f585d7b",
			wantFormat: "addsd 123(%rbp), %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmR(sseOpcodeAddsd, newOperandMem(newAmodeImmReg(1<<25, rbpVReg)), xmm11VReg)
			},
			want:       "f2440f589d00000002",
			wantFormat: "addsd 33554432(%rbp), %xmm11",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAddps, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "0f58c1",
			wantFormat: "addps %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAddpd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f58c1",
			wantFormat: "addpd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAddss, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f30f58c1",
			wantFormat: "addss %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAddsd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f20f58c1",
			wantFormat: "addsd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAndps, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "0f54c1",
			wantFormat: "andps %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAndpd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f54c1",
			wantFormat: "andpd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAndnps, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "0f55c1",
			wantFormat: "andnps %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeAndnpd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f55c1",
			wantFormat: "andnpd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeBlendvpd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3815c1",
			wantFormat: "blendvpd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeBlendvps, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3814c1",
			wantFormat: "blendvps %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeCvttps2dq, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f30f5bc1",
			wantFormat: "cvttps2dq %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeCvtdq2ps, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "0f5bc1",
			wantFormat: "cvtdq2ps %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeCvtdq2pd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f30fe6c1",
			wantFormat: "cvtdq2pd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeDivps, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "0f5ec1",
			wantFormat: "divps %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeDivpd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f5ec1",
			wantFormat: "divpd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeDivss, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f30f5ec1",
			wantFormat: "divss %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeDivsd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f20f5ec1",
			wantFormat: "divsd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMaxps, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "0f5fc1",
			wantFormat: "maxps %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMaxpd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f5fc1",
			wantFormat: "maxpd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMaxss, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f30f5fc1",
			wantFormat: "maxss %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMaxsd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f20f5fc1",
			wantFormat: "maxsd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMinps, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "0f5dc1",
			wantFormat: "minps %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePaddsw, newOperandReg(xmm7VReg), xmm6VReg) },
			want:       "660fedf7",
			wantFormat: "paddsw %xmm7, %xmm6",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePaddusb, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660fdcc1",
			wantFormat: "paddusb %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePaddusw, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660fddc1",
			wantFormat: "paddusw %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePand, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660fdbc1",
			wantFormat: "pand %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePandn, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660fdfc1",
			wantFormat: "pandn %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePavgb, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660fe0c1",
			wantFormat: "pavgb %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePavgw, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660fe3c1",
			wantFormat: "pavgw %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePcmpeqb, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f74c1",
			wantFormat: "pcmpeqb %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePcmpeqw, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f75c1",
			wantFormat: "pcmpeqw %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePcmpeqd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f76c1",
			wantFormat: "pcmpeqd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePcmpeqq, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3829c1",
			wantFormat: "pcmpeqq %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePcmpgtb, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f64c1",
			wantFormat: "pcmpgtb %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePcmpgtw, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f65c1",
			wantFormat: "pcmpgtw %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePcmpgtd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f66c1",
			wantFormat: "pcmpgtd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePcmpgtq, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3837c1",
			wantFormat: "pcmpgtq %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePmaddwd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660ff5c1",
			wantFormat: "pmaddwd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePmaxsb, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f383cc1",
			wantFormat: "pmaxsb %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePmaxsw, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660feec1",
			wantFormat: "pmaxsw %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePmaxsd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f383dc1",
			wantFormat: "pmaxsd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePmaxub, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660fdec1",
			wantFormat: "pmaxub %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePmaxuw, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f383ec1",
			wantFormat: "pmaxuw %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePmaxud, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f383fc1",
			wantFormat: "pmaxud %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePminsb, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3838c1",
			wantFormat: "pminsb %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePminsw, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660feac1",
			wantFormat: "pminsw %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePminsd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3839c1",
			wantFormat: "pminsd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePminub, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660fdac1",
			wantFormat: "pminub %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePminuw, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f383ac1",
			wantFormat: "pminuw %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePminud, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f383bc1",
			wantFormat: "pminud %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePmulld, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3840c1",
			wantFormat: "pmulld %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePmullw, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660fd5c1",
			wantFormat: "pmullw %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePmuludq, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660ff4c1",
			wantFormat: "pmuludq %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePor, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660febc1",
			wantFormat: "por %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePshufb, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3800c1",
			wantFormat: "pshufb %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePsubb, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660ff8c1",
			wantFormat: "psubb %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePsubd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660ffac1",
			wantFormat: "psubd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePsubq, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660ffbc1",
			wantFormat: "psubq %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePsubw, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660ff9c1",
			wantFormat: "psubw %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePsubsb, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660fe8c1",
			wantFormat: "psubsb %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePsubsw, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660fe9c1",
			wantFormat: "psubsw %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePsubusb, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660fd8c1",
			wantFormat: "psubusb %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePsubusw, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660fd9c1",
			wantFormat: "psubusw %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePunpckhbw, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f68c1",
			wantFormat: "punpckhbw %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePunpcklbw, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f60c1",
			wantFormat: "punpcklbw %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePxor, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660fefc1",
			wantFormat: "pxor %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeSubps, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "0f5cc1",
			wantFormat: "subps %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeSubpd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f5cc1",
			wantFormat: "subpd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeSubss, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f30f5cc1",
			wantFormat: "subss %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeSubsd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f20f5cc1",
			wantFormat: "subsd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeXorps, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "0f57c1",
			wantFormat: "xorps %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeXorps, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "0f57c1",
			wantFormat: "xorps %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeXorpd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f57c1",
			wantFormat: "xorpd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeXorpd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f57c1",
			wantFormat: "xorpd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMinpd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f5dc1",
			wantFormat: "minpd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMinss, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f30f5dc1",
			wantFormat: "minss %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMinsd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f20f5dc1",
			wantFormat: "minsd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMovlhps, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "0f16c1",
			wantFormat: "movlhps %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMovsd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f20f10c1",
			wantFormat: "movsd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMulps, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "0f59c1",
			wantFormat: "mulps %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMulpd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f59c1",
			wantFormat: "mulpd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMulss, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f30f59c1",
			wantFormat: "mulss %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeMulsd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f20f59c1",
			wantFormat: "mulsd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeOrpd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f56c1",
			wantFormat: "orpd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeOrps, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "0f56c1",
			wantFormat: "orps %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePackssdw, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f6bc1",
			wantFormat: "packssdw %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePacksswb, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f63c1",
			wantFormat: "packsswb %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePackusdw, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f382bc1",
			wantFormat: "packusdw %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePackuswb, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f67c1",
			wantFormat: "packuswb %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePaddb, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660ffcc1",
			wantFormat: "paddb %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePaddd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660ffec1",
			wantFormat: "paddd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePaddq, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660fd4c1",
			wantFormat: "paddq %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePaddw, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660ffdc1",
			wantFormat: "paddw %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePaddsb, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660fecc1",
			wantFormat: "paddsb %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeCvtss2sd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f30f5ac1",
			wantFormat: "cvtss2sd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeCvtsd2ss, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f20f5ac1",
			wantFormat: "cvtsd2ss %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeMovaps, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "0f28c1",
			wantFormat: "movaps %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeMovapd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f28c1",
			wantFormat: "movapd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeMovdqa, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f6fc1",
			wantFormat: "movdqa %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeMovdqu, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f30f6fc1",
			wantFormat: "movdqu %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeMovsd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f20f10c1",
			wantFormat: "movsd %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmR(sseOpcodeMovsd, newOperandMem(newAmodeImmReg(16, r12VReg)), xmm0VReg)
			},
			want:       "f2410f10442410",
			wantFormat: "movsd 16(%r12), %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeMovss, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f30f10c1",
			wantFormat: "movss %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmR(sseOpcodeMovss, newOperandMem(newAmodeImmReg(160, r12VReg)), xmm15VReg)
			},
			want:       "f3450f10bc24a0000000",
			wantFormat: "movss 160(%r12), %xmm15",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeMovups, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "0f10c1",
			wantFormat: "movups %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeMovupd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f10c1",
			wantFormat: "movupd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePabsb, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f381cc1",
			wantFormat: "pabsb %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePabsw, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f381dc1",
			wantFormat: "pabsw %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePabsd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f381ec1",
			wantFormat: "pabsd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovsxbd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3821c1",
			wantFormat: "pmovsxbd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovsxbw, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3820c1",
			wantFormat: "pmovsxbw %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovsxbq, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3822c1",
			wantFormat: "pmovsxbq %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovsxwd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3823c1",
			wantFormat: "pmovsxwd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovsxwq, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3824c1",
			wantFormat: "pmovsxwq %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovsxdq, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3825c1",
			wantFormat: "pmovsxdq %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovzxbd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3831c1",
			wantFormat: "pmovzxbd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovzxbw, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3830c1",
			wantFormat: "pmovzxbw %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovzxbq, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3832c1",
			wantFormat: "pmovzxbq %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovzxwd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3833c1",
			wantFormat: "pmovzxwd %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmR(sseOpcodePmovzxwd, newOperandMem(newAmodeImmReg(0, rbpVReg)), xmm11VReg)
			},
			want:       "66440f38335d00",
			wantFormat: "pmovzxwd (%rbp), %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmR(sseOpcodePmovzxwd, newOperandMem(newAmodeImmReg(123, rbpVReg)), xmm11VReg)
			},
			want:       "66440f38335d7b",
			wantFormat: "pmovzxwd 123(%rbp), %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmR(sseOpcodePmovzxwd, newOperandMem(newAmodeImmReg(1<<25, rbpVReg)), xmm11VReg)
			},
			want:       "66440f38339d00000002",
			wantFormat: "pmovzxwd 33554432(%rbp), %xmm11",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovzxwq, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3834c1",
			wantFormat: "pmovzxwq %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodePmovzxdq, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3835c1",
			wantFormat: "pmovzxdq %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeSqrtps, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "0f51c1",
			wantFormat: "sqrtps %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeSqrtpd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f51c1",
			wantFormat: "sqrtpd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeSqrtss, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f30f51c1",
			wantFormat: "sqrtss %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeSqrtsd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f20f51c1",
			wantFormat: "sqrtsd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeSqrtsd, newOperandReg(xmm15VReg), xmm0VReg) },
			want:       "f2410f51c7",
			wantFormat: "sqrtsd %xmm15, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeSqrtsd, newOperandReg(xmm1VReg), xmm15VReg) },
			want:       "f2440f51f9",
			wantFormat: "sqrtsd %xmm1, %xmm15",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeSqrtsd, newOperandReg(xmm11VReg), xmm15VReg) },
			want:       "f2450f51fb",
			wantFormat: "sqrtsd %xmm11, %xmm15",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmRImm(sseOpcodeRoundss, uint8(roundingModeNearest), newOperandReg(xmm11VReg), xmm15VReg)
			},
			want:       "66450f3a0afb00",
			wantFormat: "roundss $0, %xmm11, %xmm15",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmRImm(sseOpcodeRoundss, uint8(roundingModeDown), newOperandReg(xmm11VReg), xmm15VReg)
			},
			want:       "66450f3a0afb01",
			wantFormat: "roundss $1, %xmm11, %xmm15",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmRImm(sseOpcodeRoundss, uint8(roundingModeUp), newOperandReg(xmm11VReg), xmm15VReg)
			},
			want:       "66450f3a0afb02",
			wantFormat: "roundss $2, %xmm11, %xmm15",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmRImm(sseOpcodeRoundss, uint8(roundingModeZero), newOperandReg(xmm11VReg), xmm15VReg)
			},
			want:       "66450f3a0afb03",
			wantFormat: "roundss $3, %xmm11, %xmm15",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmRImm(sseOpcodeRoundss, uint8(roundingModeNearest), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm15VReg)
			},
			want:       "66440f3a0a787b00",
			wantFormat: "roundss $0, 123(%rax), %xmm15",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmRImm(sseOpcodeRoundss, uint8(roundingModeDown), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm15VReg)
			},
			want:       "66440f3a0a787b01",
			wantFormat: "roundss $1, 123(%rax), %xmm15",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmRImm(sseOpcodeRoundss, uint8(roundingModeUp), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm15VReg)
			},
			want:       "66440f3a0a787b02",
			wantFormat: "roundss $2, 123(%rax), %xmm15",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmRImm(sseOpcodeRoundss, uint8(roundingModeZero), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm15VReg)
			},
			want:       "66440f3a0a787b03",
			wantFormat: "roundss $3, 123(%rax), %xmm15",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmRImm(sseOpcodeRoundsd, uint8(roundingModeNearest), newOperandReg(xmm11VReg), xmm15VReg)
			},
			want:       "66450f3a0bfb00",
			wantFormat: "roundsd $0, %xmm11, %xmm15",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmRImm(sseOpcodeRoundsd, uint8(roundingModeDown), newOperandReg(xmm11VReg), xmm15VReg)
			},
			want:       "66450f3a0bfb01",
			wantFormat: "roundsd $1, %xmm11, %xmm15",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmRImm(sseOpcodeRoundsd, uint8(roundingModeUp), newOperandReg(xmm11VReg), xmm15VReg)
			},
			want:       "66450f3a0bfb02",
			wantFormat: "roundsd $2, %xmm11, %xmm15",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmRImm(sseOpcodeRoundsd, uint8(roundingModeZero), newOperandReg(xmm11VReg), xmm15VReg)
			},
			want:       "66450f3a0bfb03",
			wantFormat: "roundsd $3, %xmm11, %xmm15",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmRImm(sseOpcodeRoundsd, uint8(roundingModeNearest), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm15VReg)
			},
			want:       "66440f3a0b787b00",
			wantFormat: "roundsd $0, 123(%rax), %xmm15",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmRImm(sseOpcodeRoundsd, uint8(roundingModeDown), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm15VReg)
			},
			want:       "66440f3a0b787b01",
			wantFormat: "roundsd $1, 123(%rax), %xmm15",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmRImm(sseOpcodeRoundsd, uint8(roundingModeUp), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm15VReg)
			},
			want:       "66440f3a0b787b02",
			wantFormat: "roundsd $2, 123(%rax), %xmm15",
		},
		{
			setup: func(i *instruction) {
				i.asXmmUnaryRmRImm(sseOpcodeRoundsd, uint8(roundingModeZero), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm15VReg)
			},
			want:       "66440f3a0b787b03",
			wantFormat: "roundsd $3, 123(%rax), %xmm15",
		},
		{
			setup:      func(i *instruction) { i.asXmmCmpRmR(sseOpcodePtest, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3817c1",
			wantFormat: "ptest %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmCmpRmR(sseOpcodeUcomisd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f2ec1",
			wantFormat: "ucomisd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmCmpRmR(sseOpcodeUcomiss, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "0f2ec1",
			wantFormat: "ucomiss %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmCmpRmR(sseOpcodePtest, newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660f3817407b",
			wantFormat: "ptest 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmCmpRmR(sseOpcodeUcomisd, newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660f2e407b",
			wantFormat: "ucomisd 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmCmpRmR(sseOpcodeUcomiss, newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0f2e407b",
			wantFormat: "ucomiss 123(%rax), %xmm0",
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
			setup:      func(i *instruction) { i.asGprToXmm(sseOpcodeCvtsi2ss, newOperandReg(rdiVReg), xmm0VReg, false) },
			want:       "f30f2ac7",
			wantFormat: "cvtsi2ss %edi, %xmm0",
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
				i.asGprToXmm(sseOpcodeCvtsi2ss, newOperandMem(newAmodeImmReg(123, rspVReg)), xmm15VReg, false)
			},
			want:       "f3440f2a7c247b",
			wantFormat: "cvtsi2ss 123(%rsp), %xmm15",
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
			// This is actually equivalent to movq, because of _64=true.
			setup:      func(i *instruction) { i.asXmmToGpr(sseOpcodeMovd, xmm0VReg, rdiVReg, true) },
			want:       "66480f7ec7",
			wantFormat: "movd %xmm0, %rdi",
		},
		{
			setup:      func(i *instruction) { i.asXmmToGpr(sseOpcodeMovd, xmm0VReg, rdiVReg, false) },
			want:       "660f7ec7",
			wantFormat: "movd %xmm0, %edi",
		},
		{
			setup:      func(i *instruction) { i.asXmmToGpr(sseOpcodeMovq, xmm0VReg, rdiVReg, true) },
			want:       "66480f7ec7",
			wantFormat: "movq %xmm0, %rdi",
		},
		// This is actually equivalent to movq, because of _64=false.
		{
			setup:      func(i *instruction) { i.asXmmToGpr(sseOpcodeMovq, xmm0VReg, rdiVReg, false) },
			want:       "660f7ec7",
			wantFormat: "movq %xmm0, %edi",
		},
		{
			setup:      func(i *instruction) { i.asXmmToGpr(sseOpcodeCvttss2si, xmm0VReg, rdiVReg, true) },
			want:       "f3480f2cf8",
			wantFormat: "cvttss2si %xmm0, %rdi",
		},
		{
			setup:      func(i *instruction) { i.asXmmToGpr(sseOpcodeCvttss2si, xmm0VReg, rdiVReg, false) },
			want:       "f30f2cf8",
			wantFormat: "cvttss2si %xmm0, %edi",
		},
		{
			setup:      func(i *instruction) { i.asXmmToGpr(sseOpcodeCvttsd2si, xmm0VReg, rdiVReg, true) },
			want:       "f2480f2cf8",
			wantFormat: "cvttsd2si %xmm0, %rdi",
		},
		{
			setup:      func(i *instruction) { i.asXmmToGpr(sseOpcodeCvttsd2si, xmm0VReg, rdiVReg, false) },
			want:       "f20f2cf8",
			wantFormat: "cvttsd2si %xmm0, %edi",
		},
		{
			setup:      func(i *instruction) { i.asXmmToGpr(sseOpcodeMovmskps, xmm1VReg, xmm0VReg, true) },
			want:       "480f50c1",
			wantFormat: "movmskps %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmToGpr(sseOpcodeMovmskpd, xmm1VReg, xmm0VReg, true) },
			want:       "66480f50c1",
			wantFormat: "movmskpd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmToGpr(sseOpcodePmovmskb, xmm1VReg, xmm0VReg, true) },
			want:       "66480fd7c1",
			wantFormat: "pmovmskb %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmToGpr(sseOpcodeMovmskps, xmm1VReg, xmm0VReg, false) },
			want:       "0f50c1",
			wantFormat: "movmskps %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmToGpr(sseOpcodeMovmskpd, xmm1VReg, xmm0VReg, false) },
			want:       "660f50c1",
			wantFormat: "movmskpd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmToGpr(sseOpcodePmovmskb, xmm1VReg, xmm0VReg, false) },
			want:       "660fd7c1",
			wantFormat: "pmovmskb %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredEQ_OQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c100",
			wantFormat: "cmppd $0, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredLT_OS), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c101",
			wantFormat: "cmppd $1, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredLE_OS), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c102",
			wantFormat: "cmppd $2, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredUNORD_Q), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c103",
			wantFormat: "cmppd $3, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNEQ_UQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c104",
			wantFormat: "cmppd $4, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNLT_US), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c105",
			wantFormat: "cmppd $5, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNLE_US), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c106",
			wantFormat: "cmppd $6, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredORD_Q), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c107",
			wantFormat: "cmppd $7, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredEQ_UQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c108",
			wantFormat: "cmppd $8, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNGE_US), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c109",
			wantFormat: "cmppd $9, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNGT_US), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c10a",
			wantFormat: "cmppd $10, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredFALSE_OQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c10b",
			wantFormat: "cmppd $11, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNEQ_OQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c10c",
			wantFormat: "cmppd $12, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredGE_OS), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c10d",
			wantFormat: "cmppd $13, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredGT_OS), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c10e",
			wantFormat: "cmppd $14, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredTRUE_UQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c10f",
			wantFormat: "cmppd $15, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredEQ_OS), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c110",
			wantFormat: "cmppd $16, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredLT_OQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c111",
			wantFormat: "cmppd $17, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredLE_OQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c112",
			wantFormat: "cmppd $18, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredUNORD_S), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c113",
			wantFormat: "cmppd $19, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNEQ_US), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c114",
			wantFormat: "cmppd $20, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNLT_UQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c115",
			wantFormat: "cmppd $21, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNLE_UQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c116",
			wantFormat: "cmppd $22, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredORD_S), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c117",
			wantFormat: "cmppd $23, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredEQ_US), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c118",
			wantFormat: "cmppd $24, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNGE_UQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c119",
			wantFormat: "cmppd $25, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNGT_UQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c11a",
			wantFormat: "cmppd $26, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredFALSE_OS), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c11b",
			wantFormat: "cmppd $27, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNEQ_OS), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c11c",
			wantFormat: "cmppd $28, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredGE_OQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c11d",
			wantFormat: "cmppd $29, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredGT_OQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c11e",
			wantFormat: "cmppd $30, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredTRUE_US), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "660fc2c11f",
			wantFormat: "cmppd $31, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredEQ_OQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b00",
			wantFormat: "cmppd $0, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredLT_OS), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b01",
			wantFormat: "cmppd $1, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredLE_OS), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b02",
			wantFormat: "cmppd $2, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredUNORD_Q), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b03",
			wantFormat: "cmppd $3, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNEQ_UQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b04",
			wantFormat: "cmppd $4, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNLT_US), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b05",
			wantFormat: "cmppd $5, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNLE_US), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b06",
			wantFormat: "cmppd $6, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredORD_Q), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b07",
			wantFormat: "cmppd $7, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredEQ_UQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b08",
			wantFormat: "cmppd $8, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNGE_US), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b09",
			wantFormat: "cmppd $9, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNGT_US), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b0a",
			wantFormat: "cmppd $10, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredFALSE_OQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b0b",
			wantFormat: "cmppd $11, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNEQ_OQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b0c",
			wantFormat: "cmppd $12, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredGE_OS), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b0d",
			wantFormat: "cmppd $13, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredGT_OS), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b0e",
			wantFormat: "cmppd $14, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredTRUE_UQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b0f",
			wantFormat: "cmppd $15, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredEQ_OS), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b10",
			wantFormat: "cmppd $16, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredLT_OQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b11",
			wantFormat: "cmppd $17, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredLE_OQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b12",
			wantFormat: "cmppd $18, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredUNORD_S), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b13",
			wantFormat: "cmppd $19, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNEQ_US), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b14",
			wantFormat: "cmppd $20, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNLT_UQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b15",
			wantFormat: "cmppd $21, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNLE_UQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b16",
			wantFormat: "cmppd $22, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredORD_S), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b17",
			wantFormat: "cmppd $23, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredEQ_US), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b18",
			wantFormat: "cmppd $24, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNGE_UQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b19",
			wantFormat: "cmppd $25, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNGT_UQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b1a",
			wantFormat: "cmppd $26, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredFALSE_OS), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b1b",
			wantFormat: "cmppd $27, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredNEQ_OS), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b1c",
			wantFormat: "cmppd $28, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredGE_OQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b1d",
			wantFormat: "cmppd $29, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredGT_OQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b1e",
			wantFormat: "cmppd $30, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmppd, uint8(cmpPredTRUE_US), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "660fc2407b1f",
			wantFormat: "cmppd $31, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredEQ_OQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c100",
			wantFormat: "cmpps $0, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredLT_OS), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c101",
			wantFormat: "cmpps $1, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredLE_OS), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c102",
			wantFormat: "cmpps $2, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredUNORD_Q), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c103",
			wantFormat: "cmpps $3, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNEQ_UQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c104",
			wantFormat: "cmpps $4, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNLT_US), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c105",
			wantFormat: "cmpps $5, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNLE_US), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c106",
			wantFormat: "cmpps $6, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredORD_Q), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c107",
			wantFormat: "cmpps $7, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredEQ_UQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c108",
			wantFormat: "cmpps $8, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNGE_US), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c109",
			wantFormat: "cmpps $9, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNGT_US), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c10a",
			wantFormat: "cmpps $10, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredFALSE_OQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c10b",
			wantFormat: "cmpps $11, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNEQ_OQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c10c",
			wantFormat: "cmpps $12, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredGE_OS), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c10d",
			wantFormat: "cmpps $13, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredGT_OS), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c10e",
			wantFormat: "cmpps $14, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredTRUE_UQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c10f",
			wantFormat: "cmpps $15, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredEQ_OS), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c110",
			wantFormat: "cmpps $16, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredLT_OQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c111",
			wantFormat: "cmpps $17, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredLE_OQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c112",
			wantFormat: "cmpps $18, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredUNORD_S), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c113",
			wantFormat: "cmpps $19, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNEQ_US), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c114",
			wantFormat: "cmpps $20, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNLT_UQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c115",
			wantFormat: "cmpps $21, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNLE_UQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c116",
			wantFormat: "cmpps $22, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredORD_S), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c117",
			wantFormat: "cmpps $23, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredEQ_US), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c118",
			wantFormat: "cmpps $24, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNGE_UQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c119",
			wantFormat: "cmpps $25, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNGT_UQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c11a",
			wantFormat: "cmpps $26, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredFALSE_OS), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c11b",
			wantFormat: "cmpps $27, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNEQ_OS), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c11c",
			wantFormat: "cmpps $28, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredGE_OQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c11d",
			wantFormat: "cmpps $29, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredGT_OQ), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c11e",
			wantFormat: "cmpps $30, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredTRUE_US), newOperandReg(xmm1VReg), xmm0VReg)
			},
			want:       "0fc2c11f",
			wantFormat: "cmpps $31, %xmm1, %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredEQ_OQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b00",
			wantFormat: "cmpps $0, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredLT_OS), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b01",
			wantFormat: "cmpps $1, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredLE_OS), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b02",
			wantFormat: "cmpps $2, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredUNORD_Q), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b03",
			wantFormat: "cmpps $3, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNEQ_UQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b04",
			wantFormat: "cmpps $4, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNLT_US), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b05",
			wantFormat: "cmpps $5, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNLE_US), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b06",
			wantFormat: "cmpps $6, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredORD_Q), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b07",
			wantFormat: "cmpps $7, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredEQ_UQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b08",
			wantFormat: "cmpps $8, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNGE_US), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b09",
			wantFormat: "cmpps $9, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNGT_US), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b0a",
			wantFormat: "cmpps $10, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredFALSE_OQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b0b",
			wantFormat: "cmpps $11, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNEQ_OQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b0c",
			wantFormat: "cmpps $12, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredGE_OS), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b0d",
			wantFormat: "cmpps $13, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredGT_OS), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b0e",
			wantFormat: "cmpps $14, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredTRUE_UQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b0f",
			wantFormat: "cmpps $15, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredEQ_OS), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b10",
			wantFormat: "cmpps $16, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredLT_OQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b11",
			wantFormat: "cmpps $17, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredLE_OQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b12",
			wantFormat: "cmpps $18, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredUNORD_S), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b13",
			wantFormat: "cmpps $19, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNEQ_US), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b14",
			wantFormat: "cmpps $20, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNLT_UQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b15",
			wantFormat: "cmpps $21, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNLE_UQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b16",
			wantFormat: "cmpps $22, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredORD_S), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b17",
			wantFormat: "cmpps $23, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredEQ_US), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b18",
			wantFormat: "cmpps $24, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNGE_UQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b19",
			wantFormat: "cmpps $25, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNGT_UQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b1a",
			wantFormat: "cmpps $26, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredFALSE_OS), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b1b",
			wantFormat: "cmpps $27, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredNEQ_OS), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b1c",
			wantFormat: "cmpps $28, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredGE_OQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b1d",
			wantFormat: "cmpps $29, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredGT_OQ), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b1e",
			wantFormat: "cmpps $30, 123(%rax), %xmm0",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeCmpps, uint8(cmpPredTRUE_US), newOperandMem(newAmodeImmReg(123, raxVReg)), xmm0VReg)
			},
			want:       "0fc2407b1f",
			wantFormat: "cmpps $31, 123(%rax), %xmm0",
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
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShift(0, rbpVReg, r13VReg, 2))) },
			want:       "42ff74ad00",
			wantFormat: "pushq (%rbp,%r13,4)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShift(1, rspVReg, r13VReg, 2))) },
			want:       "42ff74ac01",
			wantFormat: "pushq 1(%rsp,%r13,4)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShift(1<<14, rspVReg, r13VReg, 3))) },
			want:       "42ffb4ec00400000",
			wantFormat: "pushq 16384(%rsp,%r13,8)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShift(1<<14, rspVReg, rcxVReg, 3))) },
			want:       "ffb4cc00400000",
			wantFormat: "pushq 16384(%rsp,%rcx,8)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShift(1<<14, rspVReg, rbpVReg, 3))) },
			want:       "ffb4ec00400000",
			wantFormat: "pushq 16384(%rsp,%rbp,8)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShift(1<<14, rsiVReg, rcxVReg, 3))) },
			want:       "ffb4ce00400000",
			wantFormat: "pushq 16384(%rsi,%rcx,8)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShift(321, rsiVReg, rcxVReg, 3))) },
			want:       "ffb4ce41010000",
			wantFormat: "pushq 321(%rsi,%rcx,8)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShift(0, rsiVReg, rcxVReg, 3))) },
			want:       "ff34ce",
			wantFormat: "pushq (%rsi,%rcx,8)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShift(321, r9VReg, rbxVReg, 2))) },
			want:       "41ffb49941010000",
			wantFormat: "pushq 321(%r9,%rbx,4)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShift(321, r9VReg, rbpVReg, 2))) },
			want:       "41ffb4a941010000",
			wantFormat: "pushq 321(%r9,%rbp,4)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShift(321, r9VReg, r13VReg, 2))) },
			want:       "43ffb4a941010000",
			wantFormat: "pushq 321(%r9,%r13,4)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShift(321, rbpVReg, r13VReg, 2))) },
			want:       "42ffb4ad41010000",
			wantFormat: "pushq 321(%rbp,%r13,4)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShift(0, r9VReg, r13VReg, 2))) },
			want:       "43ff34a9",
			wantFormat: "pushq (%r9,%r13,4)",
		},
		{
			setup:      func(i *instruction) { i.asPush64(newOperandMem(newAmodeRegRegShift(1<<20, r9VReg, r13VReg, 2))) },
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
			setup:      func(i *instruction) { i.asLEA(newOperandMem(newAmodeImmReg(0, rdiVReg)), rdxVReg) },
			want:       "488d17",
			wantFormat: "lea (%rdi), %rdx",
		},
		{
			setup:      func(i *instruction) { i.asLEA(newOperandMem(newAmodeImmReg(0xffff, rdiVReg)), rdxVReg) },
			want:       "488d97ffff0000",
			wantFormat: "lea 65535(%rdi), %rdx",
		},
		{
			setup: func(i *instruction) {
				i.asLEA(newOperandMem(newAmodeRegRegShift(0xffff, rspVReg, r13VReg, 3)), rdxVReg)
			},
			want:       "4a8d94ecffff0000",
			wantFormat: "lea 65535(%rsp,%r13,8), %rdx",
		},
		{
			setup: func(i *instruction) {
				i.asLEA(newOperandLabel(label(1234)), r11VReg)
			},
			want:       "4c8d1dffffffff",
			wantFormat: "lea L1234, %r11",
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
			wantFormat: "call f0",
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
			want:       "4088b800001000",
			wantFormat: "mov.b %rdi, 1048576(%rax)",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeRegRegShift(1, rcxVReg, rdxVReg, 0))
				i.asMovRM(rsiVReg, a, 1)
			},
			want:       "4088741101",
			wantFormat: "mov.b %rsi, 1(%rcx,%rdx,1)",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeRegRegShift(1, rcxVReg, rdxVReg, 1))
				i.asMovRM(rdiVReg, a, 1)
			},
			want:       "40887c5101",
			wantFormat: "mov.b %rdi, 1(%rcx,%rdx,2)",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeRegRegShift(1, rcxVReg, rdxVReg, 1))
				i.asMovRM(rdiVReg, a, 2)
			},
			want:       "66897c5101",
			wantFormat: "mov.w %rdi, 1(%rcx,%rdx,2)",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeRegRegShift(1<<20, raxVReg, rcxVReg, 3))
				i.asMovRM(rdiVReg, a, 2)
			},
			want:       "6689bcc800001000",
			wantFormat: "mov.w %rdi, 1048576(%rax,%rcx,8)",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeRegRegShift(1<<20, rcxVReg, rdxVReg, 3))
				i.asMovRM(rdiVReg, a, 2)
			},
			want:       "6689bcd100001000",
			wantFormat: "mov.w %rdi, 1048576(%rcx,%rdx,8)",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeRegRegShift(123, raxVReg, rcxVReg, 0))
				i.asMovRM(rdiVReg, a, 4)
			},
			want:       "897c087b",
			wantFormat: "mov.l %rdi, 123(%rax,%rcx,1)",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeRegRegShift(123, raxVReg, rcxVReg, 0))
				i.asMovRM(rdiVReg, a, 8)
			},
			want:       "48897c087b",
			wantFormat: "mov.q %rdi, 123(%rax,%rcx,1)",
		},
		{
			setup: func(i *instruction) {
				a := newOperandMem(newAmodeRegRegShift(123, raxVReg, rcxVReg, 0))
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
		{
			setup: func(i *instruction) {
				i.asShiftR(shiftROpRotateLeft, newOperandReg(rcxVReg), rdiVReg, false)
			},
			want:       "d3c7",
			wantFormat: "roll %ecx, %edi",
		},
		{
			setup: func(i *instruction) {
				i.asShiftR(shiftROpRotateLeft, newOperandImm32(128), rdiVReg, false)
			},
			want:       "c1c780",
			wantFormat: "roll $128, %edi",
		},
		{
			setup: func(i *instruction) {
				i.asShiftR(shiftROpRotateLeft, newOperandReg(rcxVReg), rdiVReg, true)
			},
			want:       "48d3c7",
			wantFormat: "rolq %ecx, %rdi",
		},
		{
			setup: func(i *instruction) {
				i.asShiftR(shiftROpRotateLeft, newOperandImm32(128), rdiVReg, true)
			},
			want:       "48c1c780",
			wantFormat: "rolq $128, %rdi",
		},
		{
			setup: func(i *instruction) {
				i.asShiftR(shiftROpRotateRight, newOperandReg(rcxVReg), rdiVReg, false)
			},
			want:       "d3cf",
			wantFormat: "rorl %ecx, %edi",
		},
		{
			setup: func(i *instruction) {
				i.asShiftR(shiftROpRotateRight, newOperandImm32(128), rdiVReg, false)
			},
			want:       "c1cf80",
			wantFormat: "rorl $128, %edi",
		},
		{
			setup: func(i *instruction) {
				i.asShiftR(shiftROpRotateRight, newOperandReg(rcxVReg), rdiVReg, true)
			},
			want:       "48d3cf",
			wantFormat: "rorq %ecx, %rdi",
		},
		{
			setup: func(i *instruction) {
				i.asShiftR(shiftROpRotateRight, newOperandImm32(128), rdiVReg, true)
			},
			want:       "48c1cf80",
			wantFormat: "rorq $128, %rdi",
		},
		{
			setup: func(i *instruction) {
				i.asShiftR(shiftROpShiftLeft, newOperandReg(rcxVReg), rdiVReg, false)
			},
			want:       "d3e7",
			wantFormat: "shll %ecx, %edi",
		},
		{
			setup: func(i *instruction) {
				i.asShiftR(shiftROpShiftLeft, newOperandImm32(128), rdiVReg, false)
			},
			want:       "c1e780",
			wantFormat: "shll $128, %edi",
		},
		{
			setup: func(i *instruction) {
				i.asShiftR(shiftROpShiftLeft, newOperandReg(rcxVReg), rdiVReg, true)
			},
			want:       "48d3e7",
			wantFormat: "shlq %ecx, %rdi",
		},
		{
			setup: func(i *instruction) {
				i.asShiftR(shiftROpShiftLeft, newOperandImm32(128), rdiVReg, true)
			},
			want:       "48c1e780",
			wantFormat: "shlq $128, %rdi",
		},
		{
			setup: func(i *instruction) {
				i.asShiftR(shiftROpShiftRightLogical, newOperandReg(rcxVReg), rdiVReg, false)
			},
			want:       "d3ef",
			wantFormat: "shrl %ecx, %edi",
		},
		{
			setup: func(i *instruction) {
				i.asShiftR(shiftROpShiftRightLogical, newOperandImm32(128), rdiVReg, false)
			},
			want:       "c1ef80",
			wantFormat: "shrl $128, %edi",
		},
		{
			setup: func(i *instruction) {
				i.asShiftR(shiftROpShiftRightLogical, newOperandReg(rcxVReg), rdiVReg, true)
			},
			want:       "48d3ef",
			wantFormat: "shrq %ecx, %rdi",
		},
		{
			setup: func(i *instruction) {
				i.asShiftR(shiftROpShiftRightLogical, newOperandImm32(128), rdiVReg, true)
			},
			want:       "48c1ef80",
			wantFormat: "shrq $128, %rdi",
		},
		{
			setup: func(i *instruction) {
				i.asShiftR(shiftROpShiftRightArithmetic, newOperandReg(rcxVReg), rdiVReg, false)
			},
			want:       "d3ff",
			wantFormat: "sarl %ecx, %edi",
		},
		{
			setup: func(i *instruction) {
				i.asShiftR(shiftROpShiftRightArithmetic, newOperandImm32(128), rdiVReg, false)
			},
			want:       "c1ff80",
			wantFormat: "sarl $128, %edi",
		},
		{
			setup: func(i *instruction) {
				i.asShiftR(shiftROpShiftRightArithmetic, newOperandReg(rcxVReg), rdiVReg, true)
			},
			want:       "48d3ff",
			wantFormat: "sarq %ecx, %rdi",
		},
		{
			setup: func(i *instruction) {
				i.asShiftR(shiftROpShiftRightArithmetic, newOperandImm32(128), rdiVReg, true)
			},
			want:       "48c1ff80",
			wantFormat: "sarq $128, %rdi",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePsllw, newOperandImm32(128), xmm11VReg)
			},
			want:       "66410f71f380",
			wantFormat: "psllw $128, %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePslld, newOperandImm32(128), xmm11VReg)
			},
			want:       "66410f72f380",
			wantFormat: "pslld $128, %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePsllq, newOperandImm32(128), xmm11VReg)
			},
			want:       "66410f73f380",
			wantFormat: "psllq $128, %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePsraw, newOperandImm32(128), xmm11VReg)
			},
			want:       "66410f71e380",
			wantFormat: "psraw $128, %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePsrad, newOperandImm32(128), xmm11VReg)
			},
			want:       "66410f72e380",
			wantFormat: "psrad $128, %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePsrlw, newOperandImm32(128), xmm11VReg)
			},
			want:       "66410f71d380",
			wantFormat: "psrlw $128, %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePsrld, newOperandImm32(128), xmm11VReg)
			},
			want:       "66410f72d380",
			wantFormat: "psrld $128, %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePsrlq, newOperandImm32(128), xmm11VReg)
			},
			want:       "66410f73d380",
			wantFormat: "psrlq $128, %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePsllw, newOperandReg(xmm13VReg), xmm11VReg)
			},
			want:       "66450ff1dd",
			wantFormat: "psllw %xmm13, %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePslld, newOperandReg(xmm13VReg), xmm11VReg)
			},
			want:       "66450ff2dd",
			wantFormat: "pslld %xmm13, %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePsllq, newOperandReg(xmm13VReg), xmm11VReg)
			},
			want:       "66450ff3dd",
			wantFormat: "psllq %xmm13, %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePsraw, newOperandReg(xmm13VReg), xmm11VReg)
			},
			want:       "66450fe1dd",
			wantFormat: "psraw %xmm13, %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePsrad, newOperandReg(xmm13VReg), xmm11VReg)
			},
			want:       "66450fe2dd",
			wantFormat: "psrad %xmm13, %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePsrlw, newOperandReg(xmm13VReg), xmm11VReg)
			},
			want:       "66450fd1dd",
			wantFormat: "psrlw %xmm13, %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePsrld, newOperandReg(xmm13VReg), xmm11VReg)
			},
			want:       "66450fd2dd",
			wantFormat: "psrld %xmm13, %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePsrlq, newOperandReg(xmm13VReg), xmm11VReg)
			},
			want:       "66450fd3dd",
			wantFormat: "psrlq %xmm13, %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePsllw, newOperandMem(newAmodeImmReg(128, r13VReg)), xmm11VReg)
			},
			want:       "66450ff19d80000000",
			wantFormat: "psllw 128(%r13), %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePslld, newOperandMem(newAmodeImmReg(128, r13VReg)), xmm11VReg)
			},
			want:       "66450ff29d80000000",
			wantFormat: "pslld 128(%r13), %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePsllq, newOperandMem(newAmodeImmReg(128, r13VReg)), xmm11VReg)
			},
			want:       "66450ff39d80000000",
			wantFormat: "psllq 128(%r13), %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePsraw, newOperandMem(newAmodeImmReg(128, r13VReg)), xmm11VReg)
			},
			want:       "66450fe19d80000000",
			wantFormat: "psraw 128(%r13), %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePsrad, newOperandMem(newAmodeImmReg(128, r13VReg)), xmm11VReg)
			},
			want:       "66450fe29d80000000",
			wantFormat: "psrad 128(%r13), %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePsrlw, newOperandMem(newAmodeImmReg(128, r13VReg)), xmm11VReg)
			},
			want:       "66450fd19d80000000",
			wantFormat: "psrlw 128(%r13), %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePsrld, newOperandMem(newAmodeImmReg(128, r13VReg)), xmm11VReg)
			},
			want:       "66450fd29d80000000",
			wantFormat: "psrld 128(%r13), %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmiReg(sseOpcodePsrlq, newOperandMem(newAmodeImmReg(128, r13VReg)), xmm11VReg)
			},
			want:       "66450fd39d80000000",
			wantFormat: "psrlq 128(%r13), %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asCmpRmiR(true, newOperandReg(r13VReg), rdiVReg, true)
			},
			want:       "4c39ef",
			wantFormat: "cmpq %r13, %rdi",
		},
		{
			setup: func(i *instruction) {
				i.asCmpRmiR(false, newOperandReg(r13VReg), rdiVReg, true)
			},
			want:       "4c85ef",
			wantFormat: "testq %r13, %rdi",
		},
		{
			setup: func(i *instruction) {
				i.asCmpRmiR(true, newOperandReg(r13VReg), rdiVReg, false)
			},
			want:       "4439ef",
			wantFormat: "cmpl %r13d, %edi",
		},
		{
			setup: func(i *instruction) {
				i.asCmpRmiR(false, newOperandReg(r13VReg), rdiVReg, false)
			},
			want:       "4485ef",
			wantFormat: "testl %r13d, %edi",
		},
		{
			setup: func(i *instruction) {
				i.asCmpRmiR(true, newOperandMem(newAmodeImmReg(128, r13VReg)), rdiVReg, true)
			},
			want:       "493bbd80000000",
			wantFormat: "cmpq 128(%r13), %rdi",
		},
		{
			setup: func(i *instruction) {
				i.asCmpRmiR(false, newOperandMem(newAmodeImmReg(128, r13VReg)), rdiVReg, true)
			},
			want:       "4985bd80000000",
			wantFormat: "testq %rdi, 128(%r13)",
		},
		{
			setup: func(i *instruction) {
				i.asCmpRmiR(true, newOperandMem(newAmodeImmReg(128, r13VReg)), rdiVReg, false)
			},
			want:       "413bbd80000000",
			wantFormat: "cmpl 128(%r13), %edi",
		},
		{
			setup: func(i *instruction) {
				i.asCmpRmiR(false, newOperandMem(newAmodeImmReg(128, r13VReg)), rdiVReg, false)
			},
			want:       "4185bd80000000",
			wantFormat: "testl %edi, 128(%r13)",
		},
		{
			setup: func(i *instruction) {
				i.asCmpRmiR(true, newOperandImm32(128), rdiVReg, true)
			},
			want:       "4881ff80000000",
			wantFormat: "cmpq $128, %rdi",
		},
		{
			setup: func(i *instruction) {
				i.asCmpRmiR(false, newOperandImm32(128), rdiVReg, true)
			},
			want:       "48f7c780000000",
			wantFormat: "testq $128, %rdi",
		},
		{
			setup: func(i *instruction) {
				i.asCmpRmiR(true, newOperandImm32(128), rdiVReg, false)
			},
			want:       "81ff80000000",
			wantFormat: "cmpl $128, %edi",
		},
		{
			setup: func(i *instruction) {
				i.asCmpRmiR(false, newOperandImm32(128), rdiVReg, false)
			},
			want:       "f7c780000000",
			wantFormat: "testl $128, %edi",
		},
		{
			setup:      func(i *instruction) { i.asSetcc(condO, rsiVReg) },
			want:       "400f90c6",
			wantFormat: "seto %rsi",
		},
		{
			setup:      func(i *instruction) { i.asSetcc(condNO, rsiVReg) },
			want:       "400f91c6",
			wantFormat: "setno %rsi",
		},
		{
			setup:      func(i *instruction) { i.asSetcc(condB, rsiVReg) },
			want:       "400f92c6",
			wantFormat: "setb %rsi",
		},
		{
			setup:      func(i *instruction) { i.asSetcc(condNB, rsiVReg) },
			want:       "400f93c6",
			wantFormat: "setnb %rsi",
		},
		{
			setup:      func(i *instruction) { i.asSetcc(condZ, rsiVReg) },
			want:       "400f94c6",
			wantFormat: "setz %rsi",
		},
		{
			setup:      func(i *instruction) { i.asSetcc(condNZ, rsiVReg) },
			want:       "400f95c6",
			wantFormat: "setnz %rsi",
		},
		{
			setup:      func(i *instruction) { i.asSetcc(condBE, rsiVReg) },
			want:       "400f96c6",
			wantFormat: "setbe %rsi",
		},
		{
			setup:      func(i *instruction) { i.asSetcc(condNBE, rsiVReg) },
			want:       "400f97c6",
			wantFormat: "setnbe %rsi",
		},
		{
			setup:      func(i *instruction) { i.asSetcc(condS, rsiVReg) },
			want:       "400f98c6",
			wantFormat: "sets %rsi",
		},
		{
			setup:      func(i *instruction) { i.asSetcc(condNS, rsiVReg) },
			want:       "400f99c6",
			wantFormat: "setns %rsi",
		},
		{
			setup:      func(i *instruction) { i.asSetcc(condP, rsiVReg) },
			want:       "400f9ac6",
			wantFormat: "setp %rsi",
		},
		{
			setup:      func(i *instruction) { i.asSetcc(condNP, rsiVReg) },
			want:       "400f9bc6",
			wantFormat: "setnp %rsi",
		},
		{
			setup:      func(i *instruction) { i.asSetcc(condL, rsiVReg) },
			want:       "400f9cc6",
			wantFormat: "setl %rsi",
		},
		{
			setup:      func(i *instruction) { i.asSetcc(condNL, rsiVReg) },
			want:       "400f9dc6",
			wantFormat: "setnl %rsi",
		},
		{
			setup:      func(i *instruction) { i.asSetcc(condLE, rsiVReg) },
			want:       "400f9ec6",
			wantFormat: "setle %rsi",
		},
		{
			setup:      func(i *instruction) { i.asSetcc(condNLE, rsiVReg) },
			want:       "400f9fc6",
			wantFormat: "setnle %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condO, newOperandReg(rdiVReg), rsiVReg, true) },
			want:       "480f40f7",
			wantFormat: "cmovoq %rdi, %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNO, newOperandReg(rdiVReg), rsiVReg, true) },
			want:       "480f41f7",
			wantFormat: "cmovnoq %rdi, %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condB, newOperandReg(rdiVReg), rsiVReg, true) },
			want:       "480f42f7",
			wantFormat: "cmovbq %rdi, %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNB, newOperandReg(rdiVReg), rsiVReg, true) },
			want:       "480f43f7",
			wantFormat: "cmovnbq %rdi, %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condZ, newOperandReg(rdiVReg), rsiVReg, true) },
			want:       "480f44f7",
			wantFormat: "cmovzq %rdi, %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNZ, newOperandReg(rdiVReg), rsiVReg, true) },
			want:       "480f45f7",
			wantFormat: "cmovnzq %rdi, %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condBE, newOperandReg(rdiVReg), rsiVReg, true) },
			want:       "480f46f7",
			wantFormat: "cmovbeq %rdi, %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNBE, newOperandReg(rdiVReg), rsiVReg, true) },
			want:       "480f47f7",
			wantFormat: "cmovnbeq %rdi, %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condS, newOperandReg(rdiVReg), rsiVReg, true) },
			want:       "480f48f7",
			wantFormat: "cmovsq %rdi, %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNS, newOperandReg(rdiVReg), rsiVReg, true) },
			want:       "480f49f7",
			wantFormat: "cmovnsq %rdi, %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condP, newOperandReg(rdiVReg), rsiVReg, true) },
			want:       "480f4af7",
			wantFormat: "cmovpq %rdi, %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNP, newOperandReg(rdiVReg), rsiVReg, true) },
			want:       "480f4bf7",
			wantFormat: "cmovnpq %rdi, %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condL, newOperandReg(rdiVReg), rsiVReg, true) },
			want:       "480f4cf7",
			wantFormat: "cmovlq %rdi, %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNL, newOperandReg(rdiVReg), rsiVReg, true) },
			want:       "480f4df7",
			wantFormat: "cmovnlq %rdi, %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condLE, newOperandReg(rdiVReg), rsiVReg, true) },
			want:       "480f4ef7",
			wantFormat: "cmovleq %rdi, %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNLE, newOperandReg(rdiVReg), rsiVReg, true) },
			want:       "480f4ff7",
			wantFormat: "cmovnleq %rdi, %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condO, newOperandReg(rdiVReg), rsiVReg, false) },
			want:       "0f40f7",
			wantFormat: "cmovol %edi, %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNO, newOperandReg(rdiVReg), rsiVReg, false) },
			want:       "0f41f7",
			wantFormat: "cmovnol %edi, %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condB, newOperandReg(rdiVReg), rsiVReg, false) },
			want:       "0f42f7",
			wantFormat: "cmovbl %edi, %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNB, newOperandReg(rdiVReg), rsiVReg, false) },
			want:       "0f43f7",
			wantFormat: "cmovnbl %edi, %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condZ, newOperandReg(rdiVReg), rsiVReg, false) },
			want:       "0f44f7",
			wantFormat: "cmovzl %edi, %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNZ, newOperandReg(rdiVReg), rsiVReg, false) },
			want:       "0f45f7",
			wantFormat: "cmovnzl %edi, %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condBE, newOperandReg(rdiVReg), rsiVReg, false) },
			want:       "0f46f7",
			wantFormat: "cmovbel %edi, %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNBE, newOperandReg(rdiVReg), rsiVReg, false) },
			want:       "0f47f7",
			wantFormat: "cmovnbel %edi, %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condS, newOperandReg(rdiVReg), rsiVReg, false) },
			want:       "0f48f7",
			wantFormat: "cmovsl %edi, %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNS, newOperandReg(rdiVReg), rsiVReg, false) },
			want:       "0f49f7",
			wantFormat: "cmovnsl %edi, %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condP, newOperandReg(rdiVReg), rsiVReg, false) },
			want:       "0f4af7",
			wantFormat: "cmovpl %edi, %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNP, newOperandReg(rdiVReg), rsiVReg, false) },
			want:       "0f4bf7",
			wantFormat: "cmovnpl %edi, %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condL, newOperandReg(rdiVReg), rsiVReg, false) },
			want:       "0f4cf7",
			wantFormat: "cmovll %edi, %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNL, newOperandReg(rdiVReg), rsiVReg, false) },
			want:       "0f4df7",
			wantFormat: "cmovnll %edi, %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condLE, newOperandReg(rdiVReg), rsiVReg, false) },
			want:       "0f4ef7",
			wantFormat: "cmovlel %edi, %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNLE, newOperandReg(rdiVReg), rsiVReg, false) },
			want:       "0f4ff7",
			wantFormat: "cmovnlel %edi, %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condO, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, true) },
			want:       "480f40707b",
			wantFormat: "cmovoq 123(%rax), %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNO, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, true) },
			want:       "480f41707b",
			wantFormat: "cmovnoq 123(%rax), %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condB, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, true) },
			want:       "480f42707b",
			wantFormat: "cmovbq 123(%rax), %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNB, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, true) },
			want:       "480f43707b",
			wantFormat: "cmovnbq 123(%rax), %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condZ, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, true) },
			want:       "480f44707b",
			wantFormat: "cmovzq 123(%rax), %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNZ, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, true) },
			want:       "480f45707b",
			wantFormat: "cmovnzq 123(%rax), %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condBE, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, true) },
			want:       "480f46707b",
			wantFormat: "cmovbeq 123(%rax), %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNBE, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, true) },
			want:       "480f47707b",
			wantFormat: "cmovnbeq 123(%rax), %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condS, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, true) },
			want:       "480f48707b",
			wantFormat: "cmovsq 123(%rax), %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNS, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, true) },
			want:       "480f49707b",
			wantFormat: "cmovnsq 123(%rax), %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condP, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, true) },
			want:       "480f4a707b",
			wantFormat: "cmovpq 123(%rax), %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNP, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, true) },
			want:       "480f4b707b",
			wantFormat: "cmovnpq 123(%rax), %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condL, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, true) },
			want:       "480f4c707b",
			wantFormat: "cmovlq 123(%rax), %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNL, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, true) },
			want:       "480f4d707b",
			wantFormat: "cmovnlq 123(%rax), %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condLE, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, true) },
			want:       "480f4e707b",
			wantFormat: "cmovleq 123(%rax), %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNLE, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, true) },
			want:       "480f4f707b",
			wantFormat: "cmovnleq 123(%rax), %rsi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condO, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, false) },
			want:       "0f40707b",
			wantFormat: "cmovol 123(%rax), %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNO, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, false) },
			want:       "0f41707b",
			wantFormat: "cmovnol 123(%rax), %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condB, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, false) },
			want:       "0f42707b",
			wantFormat: "cmovbl 123(%rax), %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNB, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, false) },
			want:       "0f43707b",
			wantFormat: "cmovnbl 123(%rax), %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condZ, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, false) },
			want:       "0f44707b",
			wantFormat: "cmovzl 123(%rax), %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNZ, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, false) },
			want:       "0f45707b",
			wantFormat: "cmovnzl 123(%rax), %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condBE, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, false) },
			want:       "0f46707b",
			wantFormat: "cmovbel 123(%rax), %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNBE, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, false) },
			want:       "0f47707b",
			wantFormat: "cmovnbel 123(%rax), %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condS, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, false) },
			want:       "0f48707b",
			wantFormat: "cmovsl 123(%rax), %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNS, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, false) },
			want:       "0f49707b",
			wantFormat: "cmovnsl 123(%rax), %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condP, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, false) },
			want:       "0f4a707b",
			wantFormat: "cmovpl 123(%rax), %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNP, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, false) },
			want:       "0f4b707b",
			wantFormat: "cmovnpl 123(%rax), %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condL, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, false) },
			want:       "0f4c707b",
			wantFormat: "cmovll 123(%rax), %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNL, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, false) },
			want:       "0f4d707b",
			wantFormat: "cmovnll 123(%rax), %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condLE, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, false) },
			want:       "0f4e707b",
			wantFormat: "cmovlel 123(%rax), %esi",
		},
		{
			setup:      func(i *instruction) { i.asCmove(condNLE, newOperandMem(newAmodeImmReg(123, raxVReg)), rsiVReg, false) },
			want:       "0f4f707b",
			wantFormat: "cmovnlel 123(%rax), %esi",
		},
		{
			setup: func(i *instruction) { *i = *allocateExitSeq(r15VReg) },
			// movq 0x10(%r15), %rbp
			// movq 0x18(%r15), %rsp
			// retq
			want:       "498b6f10498b6718c3",
			wantFormat: "exit_sequence %r15",
		},
		{
			setup:      func(i *instruction) { i.asXCHG(r11VReg, newOperandReg(r14VReg), 8) },
			want:       "4d87de",
			wantFormat: "xchg.q %r11, %r14",
		},
		{
			setup:      func(i *instruction) { i.asXCHG(r11VReg, newOperandMem(newAmodeImmReg(123, raxVReg)), 8) },
			want:       "4c87587b",
			wantFormat: "xchg.q %r11, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asXCHG(r15VReg, newOperandReg(raxVReg), 8) },
			want:       "4c87f8",
			wantFormat: "xchg.q %r15, %rax",
		},
		{
			setup:      func(i *instruction) { i.asXCHG(rbxVReg, newOperandReg(rsiVReg), 8) },
			want:       "4887de",
			wantFormat: "xchg.q %rbx, %rsi",
		},
		{
			setup:      func(i *instruction) { i.asXCHG(r11VReg, newOperandReg(r14VReg), 4) },
			want:       "4587de",
			wantFormat: "xchg.l %r11, %r14",
		},
		{
			setup:      func(i *instruction) { i.asXCHG(r15VReg, newOperandReg(raxVReg), 4) },
			want:       "4487f8",
			wantFormat: "xchg.l %r15, %rax",
		},
		{
			setup:      func(i *instruction) { i.asXCHG(rbxVReg, newOperandReg(rsiVReg), 4) },
			want:       "87de",
			wantFormat: "xchg.l %rbx, %rsi",
		},
		{
			setup:      func(i *instruction) { i.asXCHG(r11VReg, newOperandMem(newAmodeImmReg(123, raxVReg)), 4) },
			want:       "4487587b",
			wantFormat: "xchg.l %r11, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asXCHG(r11VReg, newOperandReg(r14VReg), 2) },
			want:       "664587de",
			wantFormat: "xchg.w %r11, %r14",
		},
		{
			setup:      func(i *instruction) { i.asXCHG(r15VReg, newOperandReg(raxVReg), 2) },
			want:       "664487f8",
			wantFormat: "xchg.w %r15, %rax",
		},
		{
			setup:      func(i *instruction) { i.asXCHG(rbxVReg, newOperandReg(rsiVReg), 2) },
			want:       "6687de",
			wantFormat: "xchg.w %rbx, %rsi",
		},
		{
			setup:      func(i *instruction) { i.asXCHG(r11VReg, newOperandMem(newAmodeImmReg(123, raxVReg)), 2) },
			want:       "664487587b",
			wantFormat: "xchg.w %r11, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asXCHG(r11VReg, newOperandMem(newAmodeImmReg(123, raxVReg)), 1) },
			want:       "4486587b",
			wantFormat: "xchg.b %r11, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asXCHG(r11VReg, newOperandMem(newAmodeImmReg(123, rdiVReg)), 1) },
			want:       "44865f7b",
			wantFormat: "xchg.b %r11, 123(%rdi)",
		},
		{
			setup:      func(i *instruction) { i.asXCHG(rsiVReg, newOperandMem(newAmodeImmReg(123, raxVReg)), 1) },
			want:       "4086707b",
			wantFormat: "xchg.b %rsi, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asXCHG(rdiVReg, newOperandMem(newAmodeImmReg(123, raxVReg)), 1) },
			want:       "4086787b",
			wantFormat: "xchg.b %rdi, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asXCHG(rdiVReg, newOperandMem(newAmodeImmReg(123, rsiVReg)), 1) },
			want:       "40867e7b",
			wantFormat: "xchg.b %rdi, 123(%rsi)",
		},
		{
			setup:      func(i *instruction) { i.asZeros(rbxVReg) },
			want:       "4831db",
			wantFormat: "xor %rbx, %rbx",
		},
		{
			setup:      func(i *instruction) { i.asZeros(r14VReg) },
			want:       "4d31f6",
			wantFormat: "xor %r14, %r14",
		},
		{
			setup:      func(i *instruction) { i.asZeros(xmm1VReg) },
			want:       "660fefc9",
			wantFormat: "xor %xmm1, %xmm1",
		},
		{
			setup:      func(i *instruction) { i.asZeros(xmm12VReg) },
			want:       "66450fefe4",
			wantFormat: "xor %xmm12, %xmm12",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmRImm(sseOpcodeCmpss, uint8(25), newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f30fc2c119",
			wantFormat: "cmpss $25, %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmRImm(sseOpcodeCmpsd, uint8(25), newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "f20fc2c119",
			wantFormat: "cmpsd $25, %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmRImm(sseOpcodeInsertps, uint8(25), newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3a21c119",
			wantFormat: "insertps $25, %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmRImm(sseOpcodePalignr, uint8(25), newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3a0fc119",
			wantFormat: "palignr $25, %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmRImm(sseOpcodePinsrb, uint8(25), newOperandReg(r14VReg), xmm1VReg) },
			want:       "66410f3a20ce19",
			wantFormat: "pinsrb $25, %r14d, %xmm1",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmRImm(sseOpcodePinsrw, uint8(25), newOperandReg(r14VReg), xmm1VReg) },
			want:       "66410fc4ce19",
			wantFormat: "pinsrw $25, %r14d, %xmm1",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmRImm(sseOpcodePinsrd, uint8(25), newOperandReg(r14VReg), xmm1VReg) },
			want:       "66410f3a22ce19",
			wantFormat: "pinsrd $25, %r14d, %xmm1",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmRImm(sseOpcodePinsrq, uint8(25), newOperandReg(r14VReg), xmm1VReg) },
			want:       "66490f3a22ce19",
			wantFormat: "pinsrq $25, %r14, %xmm1",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmRImm(sseOpcodePextrb, uint8(25), newOperandReg(xmm1VReg), r14VReg) },
			want:       "66410f3a14ce19",
			wantFormat: "pextrb $25, %xmm1, %r14d",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmRImm(sseOpcodePextrw, uint8(25), newOperandReg(xmm1VReg), r14VReg) },
			want:       "66440fc5f119",
			wantFormat: "pextrw $25, %xmm1, %r14d",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmRImm(sseOpcodePextrd, uint8(25), newOperandReg(xmm1VReg), rbxVReg) },
			want:       "660f3a16cb19",
			wantFormat: "pextrd $25, %xmm1, %ebx",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmRImm(sseOpcodePextrq, uint8(25), newOperandReg(xmm1VReg), rdxVReg) },
			want:       "66480f3a16ca19",
			wantFormat: "pextrq $25, %xmm1, %rdx",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmRImm(sseOpcodePshufd, uint8(25), newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f70c119",
			wantFormat: "pshufd $25, %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmRImm(sseOpcodeRoundps, uint8(25), newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3a08c119",
			wantFormat: "roundps $25, %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmRImm(sseOpcodeRoundpd, uint8(25), newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3a09c119",
			wantFormat: "roundpd $25, %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePmulhrsw, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f380bc1",
			wantFormat: "pmulhrsw %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodeUnpcklps, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "0f14c1",
			wantFormat: "unpcklps %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeCvtps2pd, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "0f5ac1",
			wantFormat: "cvtps2pd %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeCvtpd2ps, newOperandReg(xmm15VReg), xmm0VReg) },
			want:       "66410f5ac7",
			wantFormat: "cvtpd2ps %xmm15, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asXmmUnaryRmR(sseOpcodeCvttpd2dq, newOperandReg(xmm15VReg), xmm11VReg) },
			want:       "66450fe6df",
			wantFormat: "cvttpd2dq %xmm15, %xmm11",
		},
		{
			setup: func(i *instruction) {
				i.asXmmRmRImm(sseOpcodeShufps, 0b00_00_10_00, newOperandReg(xmm15VReg), xmm11VReg)
			},
			want:       "450fc6df08",
			wantFormat: "shufps $8, %xmm15, %xmm11",
		},
		{
			setup:      func(i *instruction) { i.asXmmRmR(sseOpcodePmaddubsw, newOperandReg(xmm1VReg), xmm0VReg) },
			want:       "660f3804c1",
			wantFormat: "pmaddubsw %xmm1, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asBlendvpd(newOperandReg(xmm1VReg), xmm15VReg) },
			want:       "66440f3815f9",
			wantFormat: "blendvpd %xmm1, %xmm15, %xmm0",
		},
		{
			setup:      func(i *instruction) { i.asMFence() },
			want:       "0faef0",
			wantFormat: "mfence",
		},
		{
			setup:      func(i *instruction) { i.asLockCmpXCHG(rsiVReg, newAmodeImmReg(123, raxVReg), 8) },
			want:       "f0480fb1707b",
			wantFormat: "lock cmpxchg.q %rsi, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asLockCmpXCHG(r11VReg, newAmodeImmReg(123, raxVReg), 8) },
			want:       "f04c0fb1587b",
			wantFormat: "lock cmpxchg.q %r11, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asLockCmpXCHG(rsiVReg, newAmodeImmReg(123, raxVReg), 4) },
			want:       "f00fb1707b",
			wantFormat: "lock cmpxchg.l %rsi, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asLockCmpXCHG(r11VReg, newAmodeImmReg(123, raxVReg), 4) },
			want:       "f0440fb1587b",
			wantFormat: "lock cmpxchg.l %r11, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asLockCmpXCHG(r11VReg, newAmodeImmReg(123, raxVReg), 2) },
			want:       "66f0440fb1587b",
			wantFormat: "lock cmpxchg.w %r11, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asLockCmpXCHG(rsiVReg, newAmodeImmReg(123, raxVReg), 2) },
			want:       "66f00fb1707b",
			wantFormat: "lock cmpxchg.w %rsi, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asLockCmpXCHG(r11VReg, newAmodeImmReg(123, raxVReg), 1) },
			want:       "f0440fb0587b",
			wantFormat: "lock cmpxchg.b %r11, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asLockCmpXCHG(r11VReg, newAmodeImmReg(123, rdiVReg), 1) },
			want:       "f0440fb05f7b",
			wantFormat: "lock cmpxchg.b %r11, 123(%rdi)",
		},
		{
			setup:      func(i *instruction) { i.asLockCmpXCHG(rsiVReg, newAmodeImmReg(123, raxVReg), 1) },
			want:       "f0400fb0707b",
			wantFormat: "lock cmpxchg.b %rsi, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asLockCmpXCHG(rdiVReg, newAmodeImmReg(123, raxVReg), 1) },
			want:       "f0400fb0787b",
			wantFormat: "lock cmpxchg.b %rdi, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asLockCmpXCHG(rdiVReg, newAmodeImmReg(123, rsiVReg), 1) },
			want:       "f0400fb07e7b",
			wantFormat: "lock cmpxchg.b %rdi, 123(%rsi)",
		},
		{
			setup:      func(i *instruction) { i.asLockXAdd(rsiVReg, newAmodeImmReg(123, raxVReg), 8) },
			want:       "f0480fc1707b",
			wantFormat: "lock xadd.q %rsi, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asLockXAdd(r11VReg, newAmodeImmReg(123, raxVReg), 8) },
			want:       "f04c0fc1587b",
			wantFormat: "lock xadd.q %r11, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asLockXAdd(rsiVReg, newAmodeImmReg(123, raxVReg), 4) },
			want:       "f00fc1707b",
			wantFormat: "lock xadd.l %rsi, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asLockXAdd(r11VReg, newAmodeImmReg(123, raxVReg), 4) },
			want:       "f0440fc1587b",
			wantFormat: "lock xadd.l %r11, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asLockXAdd(r11VReg, newAmodeImmReg(123, raxVReg), 2) },
			want:       "66f0440fc1587b",
			wantFormat: "lock xadd.w %r11, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asLockXAdd(rsiVReg, newAmodeImmReg(123, raxVReg), 2) },
			want:       "66f00fc1707b",
			wantFormat: "lock xadd.w %rsi, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asLockXAdd(r11VReg, newAmodeImmReg(123, raxVReg), 1) },
			want:       "f0440fc0587b",
			wantFormat: "lock xadd.b %r11, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asLockXAdd(r11VReg, newAmodeImmReg(123, rdiVReg), 1) },
			want:       "f0440fc05f7b",
			wantFormat: "lock xadd.b %r11, 123(%rdi)",
		},
		{
			setup:      func(i *instruction) { i.asLockXAdd(rsiVReg, newAmodeImmReg(123, raxVReg), 1) },
			want:       "f0400fc0707b",
			wantFormat: "lock xadd.b %rsi, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asLockXAdd(rdiVReg, newAmodeImmReg(123, raxVReg), 1) },
			want:       "f0400fc0787b",
			wantFormat: "lock xadd.b %rdi, 123(%rax)",
		},
		{
			setup:      func(i *instruction) { i.asLockXAdd(rdiVReg, newAmodeImmReg(123, rsiVReg), 1) },
			want:       "f0400fc07e7b",
			wantFormat: "lock xadd.b %rdi, 123(%rsi)",
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
		})
	}
}
