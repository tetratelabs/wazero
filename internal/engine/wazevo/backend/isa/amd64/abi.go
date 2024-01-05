package amd64

import (
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

// For the details of the ABI, see:
// https://github.com/golang/go/blob/49d42128fd8594c172162961ead19ac95e247d24/src/cmd/compile/abi-internal.md#amd64-architecture

var (
	intArgResultRegs   = []regalloc.RealReg{rax, rcx, rbx, rsi, rdi, r8, r9, r10, r11}
	floatArgResultRegs = []regalloc.RealReg{xmm0, xmm1, xmm2, xmm3, xmm4, xmm5, xmm6, xmm7}
)

var regInfo = &regalloc.RegisterInfo{
	AllocatableRegisters: [regalloc.NumRegType][]regalloc.RealReg{
		regalloc.RegTypeInt: {
			rax, rcx, rdx, rbx, rsi, rdi, r8, r9, r10, r11, r12, r13, r14, r15,
		},
		regalloc.RegTypeFloat: {
			xmm0, xmm1, xmm2, xmm3, xmm4, xmm5, xmm6, xmm7, xmm8, xmm9, xmm10, xmm11, xmm12, xmm13, xmm14, xmm15,
		},
	},
	CalleeSavedRegisters: regalloc.NewRegSet(
		rdx, r12, r13, r14, r15,
		xmm8, xmm9, xmm10, xmm11, xmm12, xmm13, xmm14, xmm15,
	),
	CallerSavedRegisters: regalloc.NewRegSet(
		rax, rcx, rbx, rsi, rdi, r8, r9, r10, r11,
		xmm0, xmm1, xmm2, xmm3, xmm4, xmm5, xmm6, xmm7,
	),
	RealRegToVReg: []regalloc.VReg{
		rax: raxVReg, rcx: rcxVReg, rdx: rdxVReg, rbx: rbxVReg, rsp: rspVReg, rbp: rbpVReg, rsi: rsiVReg, rdi: rdiVReg,
		r8: r8VReg, r9: r9VReg, r10: r10VReg, r11: r11VReg, r12: r12VReg, r13: r13VReg, r14: r14VReg, r15: r15VReg,
		xmm0: xmm0VReg, xmm1: xmm1VReg, xmm2: xmm2VReg, xmm3: xmm3VReg, xmm4: xmm4VReg, xmm5: xmm5VReg, xmm6: xmm6VReg,
		xmm7: xmm7VReg, xmm8: xmm8VReg, xmm9: xmm9VReg, xmm10: xmm10VReg, xmm11: xmm11VReg, xmm12: xmm12VReg,
		xmm13: xmm13VReg, xmm14: xmm14VReg, xmm15: xmm15VReg,
	},
	RealRegName: func(r regalloc.RealReg) string { return regNames[r] },
	RealRegType: func(r regalloc.RealReg) regalloc.RegType {
		if r < xmm0 {
			return regalloc.RegTypeInt
		}
		return regalloc.RegTypeFloat
	},
}

// functionABIRegInfo implements backend.FunctionABIRegInfo.
type functionABIRegInfo struct{}

// ArgsResultsRegs implements backend.FunctionABIRegInfo.
func (f functionABIRegInfo) ArgsResultsRegs() (argInts, argFloats, resultInt, resultFloats []regalloc.RealReg) {
	return intArgResultRegs, floatArgResultRegs, intArgResultRegs, floatArgResultRegs
}

type abiImpl = backend.FunctionABI[functionABIRegInfo]

func (m *machine) getOrCreateFunctionABI(sig *ssa.Signature) *abiImpl {
	if int(sig.ID) >= len(m.abis) {
		m.abis = append(m.abis, make([]abiImpl, int(sig.ID)+1)...)
	}

	abi := &m.abis[sig.ID]
	if abi.Initialized {
		return abi
	}

	abi.Init(sig)
	return abi
}

// LowerParams implements backend.Machine.
func (m *machine) LowerParams(params []ssa.Value) {
	// TODO implement me
	panic("implement me")
}

// LowerReturns implements backend.Machine.
func (m *machine) LowerReturns(returns []ssa.Value) {
	// TODO implement me
	panic("implement me")
}
