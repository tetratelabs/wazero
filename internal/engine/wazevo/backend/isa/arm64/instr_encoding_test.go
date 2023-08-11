package arm64

import (
	"encoding/hex"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestInstruction_encode(t *testing.T) {
	dummyLabel := label(1)
	for _, tc := range []struct {
		setup func(*instruction)
		want  string
	}{
		{want: "4100839a", setup: func(i *instruction) { i.asCSel(operandNR(x1VReg), operandNR(x2VReg), operandNR(x3VReg), eq, true) }},
		{want: "4110839a", setup: func(i *instruction) { i.asCSel(operandNR(x1VReg), operandNR(x2VReg), operandNR(x3VReg), ne, true) }},
		{want: "4100831a", setup: func(i *instruction) { i.asCSel(operandNR(x1VReg), operandNR(x2VReg), operandNR(x3VReg), eq, false) }},
		{want: "4110831a", setup: func(i *instruction) { i.asCSel(operandNR(x1VReg), operandNR(x2VReg), operandNR(x3VReg), ne, false) }},
		{want: "41cc631e", setup: func(i *instruction) { i.asFpuCSel(operandNR(v1VReg), operandNR(v2VReg), operandNR(v3VReg), gt, true) }},
		{want: "41bc631e", setup: func(i *instruction) { i.asFpuCSel(operandNR(v1VReg), operandNR(v2VReg), operandNR(v3VReg), lt, true) }},
		{want: "41cc231e", setup: func(i *instruction) { i.asFpuCSel(operandNR(v1VReg), operandNR(v2VReg), operandNR(v3VReg), gt, false) }},
		{want: "41bc231e", setup: func(i *instruction) { i.asFpuCSel(operandNR(v1VReg), operandNR(v2VReg), operandNR(v3VReg), lt, false) }},
		{want: "5b28030b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSL), false)
		}},
		{want: "5b28038b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSL), true)
		}},
		{want: "5b28032b", setup: func(i *instruction) {
			i.asALU(aluOpAddS, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSL), false)
		}},
		{want: "5b2803ab", setup: func(i *instruction) {
			i.asALU(aluOpAddS, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSL), true)
		}},
		{want: "5b28430b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSR), false)
		}},
		{want: "5b28438b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSR), true)
		}},
		{want: "5b28432b", setup: func(i *instruction) {
			i.asALU(aluOpAddS, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSR), false)
		}},
		{want: "5b2843ab", setup: func(i *instruction) {
			i.asALU(aluOpAddS, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSR), true)
		}},
		{want: "5b28830b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpASR), false)
		}},
		{want: "5b28838b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpASR), true)
		}},
		{want: "5b28832b", setup: func(i *instruction) {
			i.asALU(aluOpAddS, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpASR), false)
		}},
		{want: "5b2883ab", setup: func(i *instruction) {
			i.asALU(aluOpAddS, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpASR), true)
		}},
		{want: "5b28034b", setup: func(i *instruction) {
			i.asALU(aluOpSub, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSL), false)
		}},
		{want: "5b2803cb", setup: func(i *instruction) {
			i.asALU(aluOpSub, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSL), true)
		}},
		{want: "5b28036b", setup: func(i *instruction) {
			i.asALU(aluOpSubS, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSL), false)
		}},
		{want: "5b2803eb", setup: func(i *instruction) {
			i.asALU(aluOpSubS, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSL), true)
		}},
		{want: "5b28434b", setup: func(i *instruction) {
			i.asALU(aluOpSub, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSR), false)
		}},
		{want: "5b2843cb", setup: func(i *instruction) {
			i.asALU(aluOpSub, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSR), true)
		}},
		{want: "5b28436b", setup: func(i *instruction) {
			i.asALU(aluOpSubS, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSR), false)
		}},
		{want: "5b2843eb", setup: func(i *instruction) {
			i.asALU(aluOpSubS, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSR), true)
		}},
		{want: "5b28834b", setup: func(i *instruction) {
			i.asALU(aluOpSub, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpASR), false)
		}},
		{want: "5b2883cb", setup: func(i *instruction) {
			i.asALU(aluOpSub, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpASR), true)
		}},
		{want: "5b28836b", setup: func(i *instruction) {
			i.asALU(aluOpSubS, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpASR), false)
		}},
		{want: "5b2883eb", setup: func(i *instruction) {
			i.asALU(aluOpSubS, operandNR(tmpRegVReg), operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpASR), true)
		}},
		{want: "60033fd6", setup: func(i *instruction) {
			i.asCallIndirect(tmpRegVReg, nil)
		}},
		{want: "fb633bcb", setup: func(i *instruction) {
			i.asALU(aluOpSub, operandNR(tmpRegVReg), operandNR(spVReg), operandNR(tmpRegVReg), true)
		}},
		{want: "fb633b8b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, operandNR(tmpRegVReg), operandNR(spVReg), operandNR(tmpRegVReg), true)
		}},
		{want: "30000010", setup: func(i *instruction) { i.asAdr(v16VReg, 4) }},
		{want: "50050030", setup: func(i *instruction) { i.asAdr(v16VReg, 169) }},
		{want: "5000001c020000140000803f", setup: func(i *instruction) {
			i.asLoadFpuConst32(v16VReg, uint64(math.Float32bits(1.0)))
		}},
		{want: "5000005c03000014000000000000f03f", setup: func(i *instruction) {
			i.asLoadFpuConst64(v16VReg, math.Float64bits(1.0))
		}},
		{want: "8220061b", setup: func(i *instruction) {
			i.asALURRRR(aluOpMAdd, operandNR(x2VReg), operandNR(x4VReg), operandNR(x6VReg), operandNR(x8VReg), false)
		}},
		{want: "8220069b", setup: func(i *instruction) {
			i.asALURRRR(aluOpMAdd, operandNR(x2VReg), operandNR(x4VReg), operandNR(x6VReg), operandNR(x8VReg), true)
		}},
		{want: "82a0061b", setup: func(i *instruction) {
			i.asALURRRR(aluOpMSub, operandNR(x2VReg), operandNR(x4VReg), operandNR(x6VReg), operandNR(x8VReg), false)
		}},
		{want: "82a0069b", setup: func(i *instruction) {
			i.asALURRRR(aluOpMSub, operandNR(x2VReg), operandNR(x4VReg), operandNR(x6VReg), operandNR(x8VReg), true)
		}},
		{want: "00213f1e", setup: func(i *instruction) { i.asFpuCmp(operandNR(v8VReg), operandNR(v31VReg), false) }},
		{want: "00217f1e", setup: func(i *instruction) { i.asFpuCmp(operandNR(v8VReg), operandNR(v31VReg), true) }},
		{want: "b21c0053", setup: func(i *instruction) { i.asExtend(x18VReg, x5VReg, 8, 32, false) }},
		{want: "b23c0053", setup: func(i *instruction) { i.asExtend(x18VReg, x5VReg, 16, 32, false) }},
		{want: "b21c0053", setup: func(i *instruction) { i.asExtend(x18VReg, x5VReg, 8, 64, false) }},
		{want: "b23c0053", setup: func(i *instruction) { i.asExtend(x18VReg, x5VReg, 16, 64, false) }},
		{want: "f203052a", setup: func(i *instruction) { i.asExtend(x18VReg, x5VReg, 32, 64, false) }},
		{want: "b21c0013", setup: func(i *instruction) { i.asExtend(x18VReg, x5VReg, 8, 32, true) }},
		{want: "b23c0013", setup: func(i *instruction) { i.asExtend(x18VReg, x5VReg, 16, 32, true) }},
		{want: "b21c4093", setup: func(i *instruction) { i.asExtend(x18VReg, x5VReg, 8, 64, true) }},
		{want: "b23c4093", setup: func(i *instruction) { i.asExtend(x18VReg, x5VReg, 16, 64, true) }},
		{want: "b27c4093", setup: func(i *instruction) { i.asExtend(x18VReg, x5VReg, 32, 64, true) }},
		{want: "f2079f9a", setup: func(i *instruction) { i.asCSet(x18VReg, ne) }},
		{want: "f2179f9a", setup: func(i *instruction) { i.asCSet(x18VReg, eq) }},
		{want: "32008012", setup: func(i *instruction) { i.asMOVN(x18VReg, 1, 0, false) }},
		{want: "52559512", setup: func(i *instruction) { i.asMOVN(x18VReg, 0xaaaa, 0, false) }},
		{want: "f2ff9f12", setup: func(i *instruction) { i.asMOVN(x18VReg, 0xffff, 0, false) }},
		{want: "3200a012", setup: func(i *instruction) { i.asMOVN(x18VReg, 1, 1, false) }},
		{want: "5255b512", setup: func(i *instruction) { i.asMOVN(x18VReg, 0xaaaa, 1, false) }},
		{want: "f2ffbf12", setup: func(i *instruction) { i.asMOVN(x18VReg, 0xffff, 1, false) }},
		{want: "32008092", setup: func(i *instruction) { i.asMOVN(x18VReg, 1, 0, true) }},
		{want: "52559592", setup: func(i *instruction) { i.asMOVN(x18VReg, 0xaaaa, 0, true) }},
		{want: "f2ff9f92", setup: func(i *instruction) { i.asMOVN(x18VReg, 0xffff, 0, true) }},
		{want: "3200a092", setup: func(i *instruction) { i.asMOVN(x18VReg, 1, 1, true) }},
		{want: "5255b592", setup: func(i *instruction) { i.asMOVN(x18VReg, 0xaaaa, 1, true) }},
		{want: "f2ffbf92", setup: func(i *instruction) { i.asMOVN(x18VReg, 0xffff, 1, true) }},
		{want: "3200c092", setup: func(i *instruction) { i.asMOVN(x18VReg, 1, 2, true) }},
		{want: "5255d592", setup: func(i *instruction) { i.asMOVN(x18VReg, 0xaaaa, 2, true) }},
		{want: "f2ffdf92", setup: func(i *instruction) { i.asMOVN(x18VReg, 0xffff, 2, true) }},
		{want: "3200e092", setup: func(i *instruction) { i.asMOVN(x18VReg, 1, 3, true) }},
		{want: "5255f592", setup: func(i *instruction) { i.asMOVN(x18VReg, 0xaaaa, 3, true) }},
		{want: "f2ffff92", setup: func(i *instruction) { i.asMOVN(x18VReg, 0xffff, 3, true) }},
		{want: "5255b572", setup: func(i *instruction) { i.asMOVK(x18VReg, 0xaaaa, 1, false) }},
		{want: "5255f5f2", setup: func(i *instruction) { i.asMOVK(x18VReg, 0xaaaa, 3, true) }},
		{want: "5255b552", setup: func(i *instruction) { i.asMOVZ(x18VReg, 0xaaaa, 1, false) }},
		{want: "5255f5d2", setup: func(i *instruction) { i.asMOVZ(x18VReg, 0xaaaa, 3, true) }},
		{want: "4f020012", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x18VReg, x15VReg, 0x1, false) }},
		{want: "4f0a0012", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x18VReg, x15VReg, 0x7, false) }},
		{want: "4f0e0012", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x18VReg, x15VReg, 0xf, false) }},
		{want: "4f120012", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x18VReg, x15VReg, 0x1f, false) }},
		{want: "4f160012", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x18VReg, x15VReg, 0x3f, false) }},
		{want: "4f021112", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x18VReg, x15VReg, 0x8000, false) }},
		{want: "4f721f12", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x18VReg, x15VReg, 0x3ffffffe, false) }},
		{want: "4f7a1f12", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x18VReg, x15VReg, 0xfffffffe, false) }},
		{want: "4f024092", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x18VReg, x15VReg, 0x1, true) }},
		{want: "4f0a4092", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x18VReg, x15VReg, 0x7, true) }},
		{want: "4f0e4092", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x18VReg, x15VReg, 0xf, true) }},
		{want: "4f124092", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x18VReg, x15VReg, 0x1f, true) }},
		{want: "4f164092", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x18VReg, x15VReg, 0x3f, true) }},
		{want: "4f4e4092", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x18VReg, x15VReg, 0xfffff, true) }},
		{want: "4f7e7092", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x18VReg, x15VReg, 0xffffffff0000, true) }},
		{want: "4f7a4092", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x18VReg, x15VReg, 0x7fffffff, true) }},
		{want: "4f767f92", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x18VReg, x15VReg, 0x7ffffffe, true) }},
		{want: "4fba7f92", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x18VReg, x15VReg, 0xfffffffffffe, true) }},
		{want: "4f020032", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x18VReg, x15VReg, 0x1, false) }},
		{want: "4f0a0032", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x18VReg, x15VReg, 0x7, false) }},
		{want: "4f0e0032", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x18VReg, x15VReg, 0xf, false) }},
		{want: "4f120032", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x18VReg, x15VReg, 0x1f, false) }},
		{want: "4f160032", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x18VReg, x15VReg, 0x3f, false) }},
		{want: "4f021132", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x18VReg, x15VReg, 0x8000, false) }},
		{want: "4f721f32", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x18VReg, x15VReg, 0x3ffffffe, false) }},
		{want: "4f7a1f32", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x18VReg, x15VReg, 0xfffffffe, false) }},
		{want: "4f0240b2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x18VReg, x15VReg, 0x1, true) }},
		{want: "4f0a40b2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x18VReg, x15VReg, 0x7, true) }},
		{want: "4f0e40b2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x18VReg, x15VReg, 0xf, true) }},
		{want: "4f1240b2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x18VReg, x15VReg, 0x1f, true) }},
		{want: "4f1640b2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x18VReg, x15VReg, 0x3f, true) }},
		{want: "4f4e40b2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x18VReg, x15VReg, 0xfffff, true) }},
		{want: "4f7e70b2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x18VReg, x15VReg, 0xffffffff0000, true) }},
		{want: "4f7a40b2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x18VReg, x15VReg, 0x7fffffff, true) }},
		{want: "4f767fb2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x18VReg, x15VReg, 0x7ffffffe, true) }},
		{want: "4fba7fb2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x18VReg, x15VReg, 0xfffffffffffe, true) }},
		{want: "4f020052", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x18VReg, x15VReg, 0x1, false) }},
		{want: "4f0a0052", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x18VReg, x15VReg, 0x7, false) }},
		{want: "4f0e0052", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x18VReg, x15VReg, 0xf, false) }},
		{want: "4f120052", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x18VReg, x15VReg, 0x1f, false) }},
		{want: "4f160052", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x18VReg, x15VReg, 0x3f, false) }},
		{want: "4f021152", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x18VReg, x15VReg, 0x8000, false) }},
		{want: "4f721f52", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x18VReg, x15VReg, 0x3ffffffe, false) }},
		{want: "4f7a1f52", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x18VReg, x15VReg, 0xfffffffe, false) }},
		{want: "4f0240d2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x18VReg, x15VReg, 0x1, true) }},
		{want: "4f0a40d2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x18VReg, x15VReg, 0x7, true) }},
		{want: "4f0e40d2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x18VReg, x15VReg, 0xf, true) }},
		{want: "4f1240d2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x18VReg, x15VReg, 0x1f, true) }},
		{want: "4f1640d2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x18VReg, x15VReg, 0x3f, true) }},
		{want: "4f4e40d2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x18VReg, x15VReg, 0xfffff, true) }},
		{want: "4f7e70d2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x18VReg, x15VReg, 0xffffffff0000, true) }},
		{want: "4f7a40d2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x18VReg, x15VReg, 0x7fffffff, true) }},
		{want: "4f767fd2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x18VReg, x15VReg, 0x7ffffffe, true) }},
		{want: "4fba7fd2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x18VReg, x15VReg, 0xfffffffffffe, true) }},
		{want: "f21fbf0e", setup: func(i *instruction) { i.asFpuMov64(v18VReg, v31VReg) }},
		{want: "f21fbf4e", setup: func(i *instruction) { i.asFpuMov128(v18VReg, v31VReg) }},
		{want: "4000140b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, operandNR(x0VReg), operandNR(x2VReg), operandNR(x20VReg), false)
		}},
		{want: "4000148b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, operandNR(x0VReg), operandNR(x2VReg), operandNR(x20VReg), true)
		}},
		{want: "40001f8b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, operandNR(x0VReg), operandNR(x2VReg), operandNR(xzrVReg), true)
		}},
		{want: "4000142b", setup: func(i *instruction) {
			i.asALU(aluOpAddS, operandNR(x0VReg), operandNR(x2VReg), operandNR(x20VReg), false)
		}},
		{want: "400014ab", setup: func(i *instruction) {
			i.asALU(aluOpAddS, operandNR(x0VReg), operandNR(x2VReg), operandNR(x20VReg), true)
		}},
		{want: "4000144b", setup: func(i *instruction) {
			i.asALU(aluOpSub, operandNR(x0VReg), operandNR(x2VReg), operandNR(x20VReg), false)
		}},
		{want: "400014cb", setup: func(i *instruction) {
			i.asALU(aluOpSub, operandNR(x0VReg), operandNR(x2VReg), operandNR(x20VReg), true)
		}},
		{want: "40001fcb", setup: func(i *instruction) {
			i.asALU(aluOpSub, operandNR(x0VReg), operandNR(x2VReg), operandNR(xzrVReg), true)
		}},
		{want: "400014eb", setup: func(i *instruction) {
			i.asALU(aluOpSubS, operandNR(x0VReg), operandNR(x2VReg), operandNR(x20VReg), true)
		}},
		{want: "40001feb", setup: func(i *instruction) {
			i.asALU(aluOpSubS, operandNR(x0VReg), operandNR(x2VReg), operandNR(xzrVReg), true)
		}},
		{want: "c0035fd6", setup: func(i *instruction) { i.asRet(nil) }},
		{want: "e303042a", setup: func(i *instruction) { i.asMove32(x3VReg, x4VReg) }},
		{want: "fe03002a", setup: func(i *instruction) { i.asMove32(x30VReg, x0VReg) }},
		{want: "e30304aa", setup: func(i *instruction) { i.asMove64(x3VReg, x4VReg) }},
		{want: "fe0300aa", setup: func(i *instruction) { i.asMove64(x30VReg, x0VReg) }},
		{want: "9f000091", setup: func(i *instruction) { i.asMove64(spVReg, x4VReg) }},
		{want: "e0030091", setup: func(i *instruction) { i.asMove64(x0VReg, spVReg) }},
		{want: "e17bc1a8", setup: func(i *instruction) {
			i.asLoadPair64(x1VReg, x30VReg, addressModePreOrPostIndex(spVReg, 16, false))
		}},
		{want: "e17bc1a9", setup: func(i *instruction) {
			i.asLoadPair64(x1VReg, x30VReg, addressModePreOrPostIndex(spVReg, 16, true))
		}},
		{want: "e17b81a8", setup: func(i *instruction) {
			i.asStorePair64(x1VReg, x30VReg, addressModePreOrPostIndex(spVReg, 16, false))
		}},
		{want: "e17b81a9", setup: func(i *instruction) {
			i.asStorePair64(x1VReg, x30VReg, addressModePreOrPostIndex(spVReg, 16, true))
		}},
		{want: "20000014", setup: func(i *instruction) {
			i.asBr(dummyLabel)
			i.brOffsetResolved(0x80)
		}},
		{want: "01040034", setup: func(i *instruction) {
			i.asCondBr(registerAsRegZeroCond(x1VReg), dummyLabel, false)
			i.condBrOffsetResolve(0x80)
		}},
		{want: "010400b4", setup: func(i *instruction) {
			i.asCondBr(registerAsRegZeroCond(x1VReg), dummyLabel, true)
			i.condBrOffsetResolve(0x80)
		}},
		{want: "01040035", setup: func(i *instruction) {
			i.asCondBr(registerAsRegNotZeroCond(x1VReg), dummyLabel, false)
			i.condBrOffsetResolve(0x80)
		}},
		{want: "010400b5", setup: func(i *instruction) {
			i.asCondBr(registerAsRegNotZeroCond(x1VReg), dummyLabel, true)
			i.condBrOffsetResolve(0x80)
		}},
		{want: "8328321e", setup: func(i *instruction) {
			i.asFpuRRR(fpuBinOpAdd, operandNR(v3VReg), operandNR(v4VReg), operandNR(v18VReg), false)
		}},
		{want: "8328721e", setup: func(i *instruction) {
			i.asFpuRRR(fpuBinOpAdd, operandNR(v3VReg), operandNR(v4VReg), operandNR(v18VReg), true)
		}},
		{want: "8338321e", setup: func(i *instruction) {
			i.asFpuRRR(fpuBinOpSub, operandNR(v3VReg), operandNR(v4VReg), operandNR(v18VReg), false)
		}},
		{want: "8338721e", setup: func(i *instruction) {
			i.asFpuRRR(fpuBinOpSub, operandNR(v3VReg), operandNR(v4VReg), operandNR(v18VReg), true)
		}},
		{want: "8308321e", setup: func(i *instruction) {
			i.asFpuRRR(fpuBinOpMul, operandNR(v3VReg), operandNR(v4VReg), operandNR(v18VReg), false)
		}},
		{want: "8308721e", setup: func(i *instruction) {
			i.asFpuRRR(fpuBinOpMul, operandNR(v3VReg), operandNR(v4VReg), operandNR(v18VReg), true)
		}},
		{want: "8318321e", setup: func(i *instruction) {
			i.asFpuRRR(fpuBinOpDiv, operandNR(v3VReg), operandNR(v4VReg), operandNR(v18VReg), false)
		}},
		{want: "8318721e", setup: func(i *instruction) {
			i.asFpuRRR(fpuBinOpDiv, operandNR(v3VReg), operandNR(v4VReg), operandNR(v18VReg), true)
		}},
		{want: "8348321e", setup: func(i *instruction) {
			i.asFpuRRR(fpuBinOpMax, operandNR(v3VReg), operandNR(v4VReg), operandNR(v18VReg), false)
		}},
		{want: "8348721e", setup: func(i *instruction) {
			i.asFpuRRR(fpuBinOpMax, operandNR(v3VReg), operandNR(v4VReg), operandNR(v18VReg), true)
		}},
		{want: "8358321e", setup: func(i *instruction) {
			i.asFpuRRR(fpuBinOpMin, operandNR(v3VReg), operandNR(v4VReg), operandNR(v18VReg), false)
		}},
		{want: "8358721e", setup: func(i *instruction) {
			i.asFpuRRR(fpuBinOpMin, operandNR(v3VReg), operandNR(v4VReg), operandNR(v18VReg), true)
		}},
		{want: "49fd7f11", setup: func(i *instruction) {
			i.asALU(aluOpAdd, operandNR(x9VReg), operandNR(x10VReg), operandImm12(0b111111111111, 0b1), false)
		}},
		{want: "e9ff7f91", setup: func(i *instruction) {
			i.asALU(aluOpAdd, operandNR(x9VReg), operandNR(spVReg), operandImm12(0b111111111111, 0b1), true)
		}},
		{want: "49fd3f11", setup: func(i *instruction) {
			i.asALU(aluOpAdd, operandNR(x9VReg), operandNR(x10VReg), operandImm12(0b111111111111, 0b0), false)
		}},
		{want: "5ffd3f91", setup: func(i *instruction) {
			i.asALU(aluOpAdd, operandNR(spVReg), operandNR(x10VReg), operandImm12(0b111111111111, 0b0), true)
		}},
		{want: "49fd7f31", setup: func(i *instruction) {
			i.asALU(aluOpAddS, operandNR(x9VReg), operandNR(x10VReg), operandImm12(0b111111111111, 0b1), false)
		}},
		{want: "49fd7fb1", setup: func(i *instruction) {
			i.asALU(aluOpAddS, operandNR(x9VReg), operandNR(x10VReg), operandImm12(0b111111111111, 0b1), true)
		}},
		{want: "49fd3f31", setup: func(i *instruction) {
			i.asALU(aluOpAddS, operandNR(x9VReg), operandNR(x10VReg), operandImm12(0b111111111111, 0b0), false)
		}},
		{want: "49fd3fb1", setup: func(i *instruction) {
			i.asALU(aluOpAddS, operandNR(x9VReg), operandNR(x10VReg), operandImm12(0b111111111111, 0b0), true)
		}},
		{want: "49fd7f51", setup: func(i *instruction) {
			i.asALU(aluOpSub, operandNR(x9VReg), operandNR(x10VReg), operandImm12(0b111111111111, 0b1), false)
		}},
		{want: "e9ff7fd1", setup: func(i *instruction) {
			i.asALU(aluOpSub, operandNR(x9VReg), operandNR(spVReg), operandImm12(0b111111111111, 0b1), true)
		}},
		{want: "49fd3f51", setup: func(i *instruction) {
			i.asALU(aluOpSub, operandNR(x9VReg), operandNR(x10VReg), operandImm12(0b111111111111, 0b0), false)
		}},
		{want: "5ffd3fd1", setup: func(i *instruction) {
			i.asALU(aluOpSub, operandNR(spVReg), operandNR(x10VReg), operandImm12(0b111111111111, 0b0), true)
		}},
		{want: "49fd7f71", setup: func(i *instruction) {
			i.asALU(aluOpSubS, operandNR(x9VReg), operandNR(x10VReg), operandImm12(0b111111111111, 0b1), false)
		}},
		{want: "49fd7ff1", setup: func(i *instruction) {
			i.asALU(aluOpSubS, operandNR(x9VReg), operandNR(x10VReg), operandImm12(0b111111111111, 0b1), true)
		}},
		{want: "49fd3f71", setup: func(i *instruction) {
			i.asALU(aluOpSubS, operandNR(x9VReg), operandNR(x10VReg), operandImm12(0b111111111111, 0b0), false)
		}},
		{want: "49fd3ff1", setup: func(i *instruction) {
			i.asALU(aluOpSubS, operandNR(x9VReg), operandNR(x10VReg), operandImm12(0b111111111111, 0b0), true)
		}},
		{want: "4020d41a", setup: func(i *instruction) {
			i.asALU(aluOpLsl, operandNR(x0VReg), operandNR(x2VReg), operandNR(x20VReg), false)
		}},
		{want: "4020d49a", setup: func(i *instruction) {
			i.asALU(aluOpLsl, operandNR(x0VReg), operandNR(x2VReg), operandNR(x20VReg), true)
		}},
		{want: "4024d41a", setup: func(i *instruction) {
			i.asALU(aluOpLsr, operandNR(x0VReg), operandNR(x2VReg), operandNR(x20VReg), false)
		}},
		{want: "4024d49a", setup: func(i *instruction) {
			i.asALU(aluOpLsr, operandNR(x0VReg), operandNR(x2VReg), operandNR(x20VReg), true)
		}},
		{want: "4028d41a", setup: func(i *instruction) {
			i.asALU(aluOpAsr, operandNR(x0VReg), operandNR(x2VReg), operandNR(x20VReg), false)
		}},
		{want: "4028d49a", setup: func(i *instruction) {
			i.asALU(aluOpAsr, operandNR(x0VReg), operandNR(x2VReg), operandNR(x20VReg), true)
		}},
		{want: "407c0113", setup: func(i *instruction) {
			i.asALUShift(aluOpAsr, operandNR(x0VReg), operandNR(x2VReg), operandShiftImm(1), false)
		}},
		{want: "407c1f13", setup: func(i *instruction) {
			i.asALUShift(aluOpAsr, operandNR(x0VReg), operandNR(x2VReg), operandShiftImm(31), false)
		}},
		{want: "40fc4193", setup: func(i *instruction) {
			i.asALUShift(aluOpAsr, operandNR(x0VReg), operandNR(x2VReg), operandShiftImm(1), true)
		}},
		{want: "40fc5f93", setup: func(i *instruction) {
			i.asALUShift(aluOpAsr, operandNR(x0VReg), operandNR(x2VReg), operandShiftImm(31), true)
		}},
		{want: "40fc7f93", setup: func(i *instruction) {
			i.asALUShift(aluOpAsr, operandNR(x0VReg), operandNR(x2VReg), operandShiftImm(63), true)
		}},
		{want: "407c0153", setup: func(i *instruction) {
			i.asALUShift(aluOpLsr, operandNR(x0VReg), operandNR(x2VReg), operandShiftImm(1), false)
		}},
		{want: "407c1f53", setup: func(i *instruction) {
			i.asALUShift(aluOpLsr, operandNR(x0VReg), operandNR(x2VReg), operandShiftImm(31), false)
		}},
		{want: "40fc41d3", setup: func(i *instruction) {
			i.asALUShift(aluOpLsr, operandNR(x0VReg), operandNR(x2VReg), operandShiftImm(1), true)
		}},
		{want: "40fc5fd3", setup: func(i *instruction) {
			i.asALUShift(aluOpLsr, operandNR(x0VReg), operandNR(x2VReg), operandShiftImm(31), true)
		}},
		{want: "40fc7fd3", setup: func(i *instruction) {
			i.asALUShift(aluOpLsr, operandNR(x0VReg), operandNR(x2VReg), operandShiftImm(63), true)
		}},
		{want: "40781f53", setup: func(i *instruction) {
			i.asALUShift(aluOpLsl, operandNR(x0VReg), operandNR(x2VReg), operandShiftImm(1), false)
		}},
		{want: "40000153", setup: func(i *instruction) {
			i.asALUShift(aluOpLsl, operandNR(x0VReg), operandNR(x2VReg), operandShiftImm(31), false)
		}},
		{want: "40f87fd3", setup: func(i *instruction) {
			i.asALUShift(aluOpLsl, operandNR(x0VReg), operandNR(x2VReg), operandShiftImm(1), true)
		}},
		{want: "408061d3", setup: func(i *instruction) {
			i.asALUShift(aluOpLsl, operandNR(x0VReg), operandNR(x2VReg), operandShiftImm(31), true)
		}},
		{want: "400041d3", setup: func(i *instruction) {
			i.asALUShift(aluOpLsl, operandNR(x0VReg), operandNR(x2VReg), operandShiftImm(63), true)
		}},
		{want: "4010c05a", setup: func(i *instruction) { i.asBitRR(bitOpClz, x0VReg, x2VReg, false) }},
		{want: "4010c0da", setup: func(i *instruction) { i.asBitRR(bitOpClz, x0VReg, x2VReg, true) }},
	} {
		tc := tc
		t.Run(tc.want, func(t *testing.T) {
			i := &instruction{}
			tc.setup(i)

			m := &mockCompiler{}
			i.encode(m)
			// Note: for quick iteration we can use golang.org/x/arch package to verify the encoding.
			// 	but wazero doesn't add even a test dependency to it, so commented out.
			// inst, err := arm64asm.Decode(m.buf)
			// require.NoError(t, err, hex.EncodeToString(m.buf))
			// fmt.Println(inst.String())
			require.Equal(t, tc.want, hex.EncodeToString(m.buf))
		})
	}
}

func TestInstruction_encode_call(t *testing.T) {
	m := &mockCompiler{buf: make([]byte, 128)}
	i := &instruction{}
	i.asCall(ssa.FuncRef(555), nil)
	i.encode(m)
	buf := m.buf[128:]
	require.Equal(t, "00000094", hex.EncodeToString(buf))
	require.Equal(t, 1, len(m.relocs))
	require.Equal(t, ssa.FuncRef(555), m.relocs[0].FuncRef)
	require.Equal(t, int64(128), m.relocs[0].Offset)
}

func TestInstruction_encode_br_condflag(t *testing.T) {
	for _, tc := range []struct {
		c    condFlag
		want string
	}{
		{c: eq, want: "80070054"},
		{c: ne, want: "81070054"},
		{c: hs, want: "82070054"},
		{c: lo, want: "83070054"},
		{c: mi, want: "84070054"},
		{c: pl, want: "85070054"},
		{c: vs, want: "86070054"},
		{c: vc, want: "87070054"},
		{c: hi, want: "88070054"},
		{c: ls, want: "89070054"},
		{c: ge, want: "8a070054"},
		{c: lt, want: "8b070054"},
		{c: gt, want: "8c070054"},
		{c: le, want: "8d070054"},
		{c: al, want: "8e070054"},
		{c: nv, want: "8f070054"},
	} {
		i := &instruction{}
		i.asCondBr(tc.c.asCond(), label(1), false)
		i.condBrOffsetResolve(0xf0)
		m := &mockCompiler{}
		i.encode(m)
		// Note: for quick iteration we can use golang.org/x/arch package to verify the encoding.
		// 	but wazero doesn't add even a test dependency to it, so commented out.
		// inst, err := arm64asm.Decode(m.buf)
		// require.NoError(t, err)
		// fmt.Println(inst.String())
		require.Equal(t, tc.want, hex.EncodeToString(m.buf))
	}
}

func TestInstruction_encoding_store(t *testing.T) {
	amodeRegScaledExtended1 := addressMode{kind: addressModeKindRegScaledExtended, rn: x30VReg, rm: x1VReg, extOp: extendOpUXTW}
	amodeRegScaledExtended2 := addressMode{kind: addressModeKindRegScaledExtended, rn: spVReg, rm: x1VReg, extOp: extendOpSXTW}
	amodeRegScaled1 := addressMode{kind: addressModeKindRegScaled, rn: x30VReg, rm: x1VReg}
	amodeRegScaled2 := addressMode{kind: addressModeKindRegScaled, rn: spVReg, rm: x1VReg}
	amodeRegExtended1 := addressMode{kind: addressModeKindRegExtended, rn: x30VReg, rm: x1VReg, extOp: extendOpUXTW}
	amodeRegExtended2 := addressMode{kind: addressModeKindRegExtended, rn: spVReg, rm: x1VReg, extOp: extendOpSXTW}
	amodeRegReg1 := addressMode{kind: addressModeKindRegReg, rn: x30VReg, rm: x1VReg}
	amodeRegReg2 := addressMode{kind: addressModeKindRegReg, rn: spVReg, rm: x1VReg}
	amodeRegSignedImm9_1 := addressMode{kind: addressModeKindRegSignedImm9, rn: x30VReg, imm: 10}
	amodeRegSignedImm9_2 := addressMode{kind: addressModeKindRegSignedImm9, rn: spVReg, imm: 0b111111111}
	amodePostIndex1 := addressMode{kind: addressModeKindPostIndex, rn: x30VReg, imm: 10}
	amodePostIndex2 := addressMode{kind: addressModeKindPostIndex, rn: spVReg, imm: 0b100000000}
	amodePreIndex1 := addressMode{kind: addressModeKindPreIndex, rn: x30VReg, imm: 10}
	amodePreIndex2 := addressMode{kind: addressModeKindPreIndex, rn: spVReg, imm: 0b100000000}
	amodeUnsignedImm12_1 := addressMode{kind: addressModeKindRegUnsignedImm12, rn: x30VReg}
	amodeUnsignedImm12_2 := addressMode{kind: addressModeKindRegUnsignedImm12, rn: spVReg}
	setImm := func(amode addressMode, imm int64) addressMode {
		amode.imm = imm
		return amode
	}
	for _, tc := range []struct {
		k     instructionKind
		amode addressMode
		rn    regalloc.VReg
		want  string
	}{
		// addressModeKindRegScaledExtended.
		{k: store8, amode: amodeRegScaledExtended1, rn: x5VReg, want: "c55b2138"},
		{k: store8, amode: amodeRegScaledExtended2, rn: x5VReg, want: "e5db2138"},
		{k: store16, amode: amodeRegScaledExtended1, rn: x5VReg, want: "c55b2178"},
		{k: store16, amode: amodeRegScaledExtended2, rn: x5VReg, want: "e5db2178"},
		{k: store32, amode: amodeRegScaledExtended1, rn: x5VReg, want: "c55b21b8"},
		{k: store32, amode: amodeRegScaledExtended2, rn: x5VReg, want: "e5db21b8"},
		{k: store64, amode: amodeRegScaledExtended1, rn: x5VReg, want: "c55b21f8"},
		{k: store64, amode: amodeRegScaledExtended2, rn: x5VReg, want: "e5db21f8"},
		{k: fpuStore32, amode: amodeRegScaledExtended1, rn: v5VReg, want: "c55b21bc"},
		{k: fpuStore32, amode: amodeRegScaledExtended2, rn: v5VReg, want: "e5db21bc"},
		{k: fpuStore64, amode: amodeRegScaledExtended1, rn: v5VReg, want: "c55b21fc"},
		{k: fpuStore64, amode: amodeRegScaledExtended2, rn: v5VReg, want: "e5db21fc"},
		{k: fpuStore128, amode: amodeRegScaledExtended1, rn: v5VReg, want: "c55ba13c"},
		{k: fpuStore128, amode: amodeRegScaledExtended2, rn: v5VReg, want: "e5dba13c"},
		{k: uLoad8, amode: amodeRegScaledExtended1, rn: x5VReg, want: "c55b6138"},
		{k: uLoad8, amode: amodeRegScaledExtended2, rn: x5VReg, want: "e5db6138"},
		{k: uLoad16, amode: amodeRegScaledExtended1, rn: x5VReg, want: "c55b6178"},
		{k: uLoad16, amode: amodeRegScaledExtended2, rn: x5VReg, want: "e5db6178"},
		{k: uLoad32, amode: amodeRegScaledExtended1, rn: x5VReg, want: "c55b61b8"},
		{k: uLoad32, amode: amodeRegScaledExtended2, rn: x5VReg, want: "e5db61b8"},
		{k: uLoad64, amode: amodeRegScaledExtended1, rn: x5VReg, want: "c55b61f8"},
		{k: uLoad64, amode: amodeRegScaledExtended2, rn: x5VReg, want: "e5db61f8"},
		{k: sLoad8, amode: amodeRegScaledExtended1, rn: x5VReg, want: "c55ba138"},
		{k: sLoad8, amode: amodeRegScaledExtended2, rn: x5VReg, want: "e5dba138"},
		{k: sLoad16, amode: amodeRegScaledExtended1, rn: x5VReg, want: "c55ba178"},
		{k: sLoad16, amode: amodeRegScaledExtended2, rn: x5VReg, want: "e5dba178"},
		{k: sLoad32, amode: amodeRegScaledExtended1, rn: x5VReg, want: "c55ba1b8"},
		{k: sLoad32, amode: amodeRegScaledExtended2, rn: x5VReg, want: "e5dba1b8"},
		{k: fpuLoad32, amode: amodeRegScaledExtended1, rn: v5VReg, want: "c55b61bc"},
		{k: fpuLoad32, amode: amodeRegScaledExtended2, rn: v5VReg, want: "e5db61bc"},
		{k: fpuLoad64, amode: amodeRegScaledExtended1, rn: v5VReg, want: "c55b61fc"},
		{k: fpuLoad64, amode: amodeRegScaledExtended2, rn: v5VReg, want: "e5db61fc"},
		{k: fpuLoad128, amode: amodeRegScaledExtended1, rn: v5VReg, want: "c55be13c"},
		{k: fpuLoad128, amode: amodeRegScaledExtended2, rn: v5VReg, want: "e5dbe13c"},
		// addressModeKindRegScaled.
		{k: store8, amode: amodeRegScaled1, rn: x5VReg, want: "c5fb2138"},
		{k: store8, amode: amodeRegScaled2, rn: x5VReg, want: "e5fb2138"},
		{k: store16, amode: amodeRegScaled1, rn: x5VReg, want: "c5fb2178"},
		{k: store16, amode: amodeRegScaled2, rn: x5VReg, want: "e5fb2178"},
		{k: store32, amode: amodeRegScaled1, rn: x5VReg, want: "c5fb21b8"},
		{k: store32, amode: amodeRegScaled2, rn: x5VReg, want: "e5fb21b8"},
		{k: store64, amode: amodeRegScaled1, rn: x5VReg, want: "c5fb21f8"},
		{k: store64, amode: amodeRegScaled2, rn: x5VReg, want: "e5fb21f8"},
		{k: fpuStore32, amode: amodeRegScaled1, rn: v5VReg, want: "c5fb21bc"},
		{k: fpuStore32, amode: amodeRegScaled2, rn: v5VReg, want: "e5fb21bc"},
		{k: fpuStore64, amode: amodeRegScaled1, rn: v5VReg, want: "c5fb21fc"},
		{k: fpuStore64, amode: amodeRegScaled2, rn: v5VReg, want: "e5fb21fc"},
		{k: fpuStore128, amode: amodeRegScaled1, rn: v5VReg, want: "c5fba13c"},
		{k: fpuStore128, amode: amodeRegScaled2, rn: v5VReg, want: "e5fba13c"},
		{k: uLoad8, amode: amodeRegScaled1, rn: x5VReg, want: "c5fb6138"},
		{k: uLoad8, amode: amodeRegScaled2, rn: x5VReg, want: "e5fb6138"},
		{k: uLoad16, amode: amodeRegScaled1, rn: x5VReg, want: "c5fb6178"},
		{k: uLoad16, amode: amodeRegScaled2, rn: x5VReg, want: "e5fb6178"},
		{k: uLoad32, amode: amodeRegScaled1, rn: x5VReg, want: "c5fb61b8"},
		{k: uLoad32, amode: amodeRegScaled2, rn: x5VReg, want: "e5fb61b8"},
		{k: uLoad64, amode: amodeRegScaled1, rn: x5VReg, want: "c5fb61f8"},
		{k: uLoad64, amode: amodeRegScaled2, rn: x5VReg, want: "e5fb61f8"},
		{k: sLoad8, amode: amodeRegScaled1, rn: x5VReg, want: "c5fba138"},
		{k: sLoad8, amode: amodeRegScaled2, rn: x5VReg, want: "e5fba138"},
		{k: sLoad16, amode: amodeRegScaled1, rn: x5VReg, want: "c5fba178"},
		{k: sLoad16, amode: amodeRegScaled2, rn: x5VReg, want: "e5fba178"},
		{k: sLoad32, amode: amodeRegScaled1, rn: x5VReg, want: "c5fba1b8"},
		{k: sLoad32, amode: amodeRegScaled2, rn: x5VReg, want: "e5fba1b8"},
		{k: fpuLoad32, amode: amodeRegScaled1, rn: v5VReg, want: "c5fb61bc"},
		{k: fpuLoad32, amode: amodeRegScaled2, rn: v5VReg, want: "e5fb61bc"},
		{k: fpuLoad64, amode: amodeRegScaled1, rn: v5VReg, want: "c5fb61fc"},
		{k: fpuLoad64, amode: amodeRegScaled2, rn: v5VReg, want: "e5fb61fc"},
		{k: fpuLoad128, amode: amodeRegScaled1, rn: v5VReg, want: "c5fbe13c"},
		{k: fpuLoad128, amode: amodeRegScaled2, rn: v5VReg, want: "e5fbe13c"},
		// addressModeKindRegExtended.
		{k: store8, amode: amodeRegExtended1, rn: x5VReg, want: "c54b2138"},
		{k: store8, amode: amodeRegExtended2, rn: x5VReg, want: "e5cb2138"},
		{k: store16, amode: amodeRegExtended1, rn: x5VReg, want: "c54b2178"},
		{k: store16, amode: amodeRegExtended2, rn: x5VReg, want: "e5cb2178"},
		{k: store32, amode: amodeRegExtended1, rn: x5VReg, want: "c54b21b8"},
		{k: store32, amode: amodeRegExtended2, rn: x5VReg, want: "e5cb21b8"},
		{k: store64, amode: amodeRegExtended1, rn: x5VReg, want: "c54b21f8"},
		{k: store64, amode: amodeRegExtended2, rn: x5VReg, want: "e5cb21f8"},
		{k: fpuStore32, amode: amodeRegExtended1, rn: v5VReg, want: "c54b21bc"},
		{k: fpuStore32, amode: amodeRegExtended2, rn: v5VReg, want: "e5cb21bc"},
		{k: fpuStore64, amode: amodeRegExtended1, rn: v5VReg, want: "c54b21fc"},
		{k: fpuStore64, amode: amodeRegExtended2, rn: v5VReg, want: "e5cb21fc"},
		{k: fpuStore128, amode: amodeRegExtended1, rn: v5VReg, want: "c54ba13c"},
		{k: fpuStore128, amode: amodeRegExtended2, rn: v5VReg, want: "e5cba13c"},
		{k: uLoad8, amode: amodeRegExtended1, rn: x5VReg, want: "c54b6138"},
		{k: uLoad8, amode: amodeRegExtended2, rn: x5VReg, want: "e5cb6138"},
		{k: uLoad16, amode: amodeRegExtended1, rn: x5VReg, want: "c54b6178"},
		{k: uLoad16, amode: amodeRegExtended2, rn: x5VReg, want: "e5cb6178"},
		{k: uLoad32, amode: amodeRegExtended1, rn: x5VReg, want: "c54b61b8"},
		{k: uLoad32, amode: amodeRegExtended2, rn: x5VReg, want: "e5cb61b8"},
		{k: uLoad64, amode: amodeRegExtended1, rn: x5VReg, want: "c54b61f8"},
		{k: uLoad64, amode: amodeRegExtended2, rn: x5VReg, want: "e5cb61f8"},
		{k: sLoad8, amode: amodeRegExtended1, rn: x5VReg, want: "c54ba138"},
		{k: sLoad8, amode: amodeRegExtended2, rn: x5VReg, want: "e5cba138"},
		{k: sLoad16, amode: amodeRegExtended1, rn: x5VReg, want: "c54ba178"},
		{k: sLoad16, amode: amodeRegExtended2, rn: x5VReg, want: "e5cba178"},
		{k: sLoad32, amode: amodeRegExtended1, rn: x5VReg, want: "c54ba1b8"},
		{k: sLoad32, amode: amodeRegExtended2, rn: x5VReg, want: "e5cba1b8"},
		{k: fpuLoad32, amode: amodeRegExtended1, rn: v5VReg, want: "c54b61bc"},
		{k: fpuLoad32, amode: amodeRegExtended2, rn: v5VReg, want: "e5cb61bc"},
		{k: fpuLoad64, amode: amodeRegExtended1, rn: v5VReg, want: "c54b61fc"},
		{k: fpuLoad64, amode: amodeRegExtended2, rn: v5VReg, want: "e5cb61fc"},
		{k: fpuLoad128, amode: amodeRegExtended1, rn: v5VReg, want: "c54be13c"},
		{k: fpuLoad128, amode: amodeRegExtended2, rn: v5VReg, want: "e5cbe13c"},
		// addressModeKindRegReg.
		{k: store8, amode: amodeRegReg1, rn: x5VReg, want: "c5eb2138"},
		{k: store8, amode: amodeRegReg2, rn: x5VReg, want: "e5eb2138"},
		{k: store16, amode: amodeRegReg1, rn: x5VReg, want: "c5eb2178"},
		{k: store16, amode: amodeRegReg2, rn: x5VReg, want: "e5eb2178"},
		{k: store32, amode: amodeRegReg1, rn: x5VReg, want: "c5eb21b8"},
		{k: store32, amode: amodeRegReg2, rn: x5VReg, want: "e5eb21b8"},
		{k: store64, amode: amodeRegReg1, rn: x5VReg, want: "c5eb21f8"},
		{k: store64, amode: amodeRegReg2, rn: x5VReg, want: "e5eb21f8"},
		{k: fpuStore32, amode: amodeRegReg1, rn: v5VReg, want: "c5eb21bc"},
		{k: fpuStore32, amode: amodeRegReg2, rn: v5VReg, want: "e5eb21bc"},
		{k: fpuStore64, amode: amodeRegReg1, rn: v5VReg, want: "c5eb21fc"},
		{k: fpuStore64, amode: amodeRegReg2, rn: v5VReg, want: "e5eb21fc"},
		{k: fpuStore128, amode: amodeRegReg1, rn: v5VReg, want: "c5eba13c"},
		{k: fpuStore128, amode: amodeRegReg2, rn: v5VReg, want: "e5eba13c"},
		{k: uLoad8, amode: amodeRegReg1, rn: x5VReg, want: "c5eb6138"},
		{k: uLoad8, amode: amodeRegReg2, rn: x5VReg, want: "e5eb6138"},
		{k: uLoad16, amode: amodeRegReg1, rn: x5VReg, want: "c5eb6178"},
		{k: uLoad16, amode: amodeRegReg2, rn: x5VReg, want: "e5eb6178"},
		{k: uLoad32, amode: amodeRegReg1, rn: x5VReg, want: "c5eb61b8"},
		{k: uLoad32, amode: amodeRegReg2, rn: x5VReg, want: "e5eb61b8"},
		{k: uLoad64, amode: amodeRegReg1, rn: x5VReg, want: "c5eb61f8"},
		{k: uLoad64, amode: amodeRegReg2, rn: x5VReg, want: "e5eb61f8"},
		{k: sLoad8, amode: amodeRegReg1, rn: x5VReg, want: "c5eba138"},
		{k: sLoad8, amode: amodeRegReg2, rn: x5VReg, want: "e5eba138"},
		{k: sLoad16, amode: amodeRegReg1, rn: x5VReg, want: "c5eba178"},
		{k: sLoad16, amode: amodeRegReg2, rn: x5VReg, want: "e5eba178"},
		{k: sLoad32, amode: amodeRegReg1, rn: x5VReg, want: "c5eba1b8"},
		{k: sLoad32, amode: amodeRegReg2, rn: x5VReg, want: "e5eba1b8"},
		{k: fpuLoad32, amode: amodeRegReg1, rn: v5VReg, want: "c5eb61bc"},
		{k: fpuLoad32, amode: amodeRegReg2, rn: v5VReg, want: "e5eb61bc"},
		{k: fpuLoad64, amode: amodeRegReg1, rn: v5VReg, want: "c5eb61fc"},
		{k: fpuLoad64, amode: amodeRegReg2, rn: v5VReg, want: "e5eb61fc"},
		{k: fpuLoad128, amode: amodeRegReg1, rn: v5VReg, want: "c5ebe13c"},
		{k: fpuLoad128, amode: amodeRegReg2, rn: v5VReg, want: "e5ebe13c"},
		// addressModeKindRegSignedImm9.
		{k: store8, amode: amodeRegSignedImm9_1, rn: x5VReg, want: "c5a30038"},
		{k: store8, amode: amodeRegSignedImm9_2, rn: x5VReg, want: "e5f31f38"},
		{k: store16, amode: amodeRegSignedImm9_1, rn: x5VReg, want: "c5a30078"},
		{k: store16, amode: amodeRegSignedImm9_2, rn: x5VReg, want: "e5f31f78"},
		{k: store32, amode: amodeRegSignedImm9_1, rn: x5VReg, want: "c5a300b8"},
		{k: store32, amode: amodeRegSignedImm9_2, rn: x5VReg, want: "e5f31fb8"},
		{k: store64, amode: amodeRegSignedImm9_1, rn: x5VReg, want: "c5a300f8"},
		{k: store64, amode: amodeRegSignedImm9_2, rn: x5VReg, want: "e5f31ff8"},
		{k: fpuStore32, amode: amodeRegSignedImm9_1, rn: v5VReg, want: "c5a300bc"},
		{k: fpuStore32, amode: amodeRegSignedImm9_2, rn: v5VReg, want: "e5f31fbc"},
		{k: fpuStore64, amode: amodeRegSignedImm9_1, rn: v5VReg, want: "c5a300fc"},
		{k: fpuStore64, amode: amodeRegSignedImm9_2, rn: v5VReg, want: "e5f31ffc"},
		{k: fpuStore128, amode: amodeRegSignedImm9_1, rn: v5VReg, want: "c5a3803c"},
		{k: fpuStore128, amode: amodeRegSignedImm9_2, rn: v5VReg, want: "e5f39f3c"},
		{k: uLoad8, amode: amodeRegSignedImm9_1, rn: x5VReg, want: "c5a34038"},
		{k: uLoad8, amode: amodeRegSignedImm9_2, rn: x5VReg, want: "e5f35f38"},
		{k: uLoad16, amode: amodeRegSignedImm9_1, rn: x5VReg, want: "c5a34078"},
		{k: uLoad16, amode: amodeRegSignedImm9_2, rn: x5VReg, want: "e5f35f78"},
		{k: uLoad32, amode: amodeRegSignedImm9_1, rn: x5VReg, want: "c5a340b8"},
		{k: uLoad32, amode: amodeRegSignedImm9_2, rn: x5VReg, want: "e5f35fb8"},
		{k: uLoad64, amode: amodeRegSignedImm9_1, rn: x5VReg, want: "c5a340f8"},
		{k: uLoad64, amode: amodeRegSignedImm9_2, rn: x5VReg, want: "e5f35ff8"},
		{k: sLoad8, amode: amodeRegSignedImm9_1, rn: x5VReg, want: "c5a38038"},
		{k: sLoad8, amode: amodeRegSignedImm9_2, rn: x5VReg, want: "e5f39f38"},
		{k: sLoad16, amode: amodeRegSignedImm9_1, rn: x5VReg, want: "c5a38078"},
		{k: sLoad16, amode: amodeRegSignedImm9_2, rn: x5VReg, want: "e5f39f78"},
		{k: sLoad32, amode: amodeRegSignedImm9_1, rn: x5VReg, want: "c5a380b8"},
		{k: sLoad32, amode: amodeRegSignedImm9_2, rn: x5VReg, want: "e5f39fb8"},
		{k: fpuLoad32, amode: amodeRegSignedImm9_1, rn: v5VReg, want: "c5a340bc"},
		{k: fpuLoad32, amode: amodeRegSignedImm9_2, rn: v5VReg, want: "e5f35fbc"},
		{k: fpuLoad64, amode: amodeRegSignedImm9_1, rn: v5VReg, want: "c5a340fc"},
		{k: fpuLoad64, amode: amodeRegSignedImm9_2, rn: v5VReg, want: "e5f35ffc"},
		{k: fpuLoad128, amode: amodeRegSignedImm9_1, rn: v5VReg, want: "c5a3c03c"},
		{k: fpuLoad128, amode: amodeRegSignedImm9_2, rn: v5VReg, want: "e5f3df3c"},
		// addressModeKindPostIndex.
		{k: store8, amode: amodePostIndex1, rn: x5VReg, want: "c5a70038"},
		{k: store8, amode: amodePostIndex2, rn: x5VReg, want: "e5071038"},
		{k: store16, amode: amodePostIndex1, rn: x5VReg, want: "c5a70078"},
		{k: store16, amode: amodePostIndex2, rn: x5VReg, want: "e5071078"},
		{k: store32, amode: amodePostIndex1, rn: x5VReg, want: "c5a700b8"},
		{k: store32, amode: amodePostIndex2, rn: x5VReg, want: "e50710b8"},
		{k: store64, amode: amodePostIndex1, rn: x5VReg, want: "c5a700f8"},
		{k: store64, amode: amodePostIndex2, rn: x5VReg, want: "e50710f8"},
		{k: fpuStore32, amode: amodePostIndex1, rn: v5VReg, want: "c5a700bc"},
		{k: fpuStore32, amode: amodePostIndex2, rn: v5VReg, want: "e50710bc"},
		{k: fpuStore64, amode: amodePostIndex1, rn: v5VReg, want: "c5a700fc"},
		{k: fpuStore64, amode: amodePostIndex2, rn: v5VReg, want: "e50710fc"},
		{k: fpuStore128, amode: amodePostIndex1, rn: v5VReg, want: "c5a7803c"},
		{k: fpuStore128, amode: amodePostIndex2, rn: v5VReg, want: "e507903c"},
		{k: uLoad8, amode: amodePostIndex1, rn: x5VReg, want: "c5a74038"},
		{k: uLoad8, amode: amodePostIndex2, rn: x5VReg, want: "e5075038"},
		{k: uLoad16, amode: amodePostIndex1, rn: x5VReg, want: "c5a74078"},
		{k: uLoad16, amode: amodePostIndex2, rn: x5VReg, want: "e5075078"},
		{k: uLoad32, amode: amodePostIndex1, rn: x5VReg, want: "c5a740b8"},
		{k: uLoad32, amode: amodePostIndex2, rn: x5VReg, want: "e50750b8"},
		{k: uLoad64, amode: amodePostIndex1, rn: x5VReg, want: "c5a740f8"},
		{k: uLoad64, amode: amodePostIndex2, rn: x5VReg, want: "e50750f8"},
		{k: sLoad8, amode: amodePostIndex1, rn: x5VReg, want: "c5a78038"},
		{k: sLoad8, amode: amodePostIndex2, rn: x5VReg, want: "e5079038"},
		{k: sLoad16, amode: amodePostIndex1, rn: x5VReg, want: "c5a78078"},
		{k: sLoad16, amode: amodePostIndex2, rn: x5VReg, want: "e5079078"},
		{k: sLoad32, amode: amodePostIndex1, rn: x5VReg, want: "c5a780b8"},
		{k: sLoad32, amode: amodePostIndex2, rn: x5VReg, want: "e50790b8"},
		{k: fpuLoad32, amode: amodePostIndex1, rn: v5VReg, want: "c5a740bc"},
		{k: fpuLoad32, amode: amodePostIndex2, rn: v5VReg, want: "e50750bc"},
		{k: fpuLoad64, amode: amodePostIndex1, rn: v5VReg, want: "c5a740fc"},
		{k: fpuLoad64, amode: amodePostIndex2, rn: v5VReg, want: "e50750fc"},
		{k: fpuLoad128, amode: amodePostIndex1, rn: v5VReg, want: "c5a7c03c"},
		{k: fpuLoad128, amode: amodePostIndex2, rn: v5VReg, want: "e507d03c"},
		// addressModeKindPreIndex.
		{k: store8, amode: amodePreIndex1, rn: x5VReg, want: "c5af0038"},
		{k: store8, amode: amodePreIndex2, rn: x5VReg, want: "e50f1038"},
		{k: store16, amode: amodePreIndex1, rn: x5VReg, want: "c5af0078"},
		{k: store16, amode: amodePreIndex2, rn: x5VReg, want: "e50f1078"},
		{k: store32, amode: amodePreIndex1, rn: x5VReg, want: "c5af00b8"},
		{k: store32, amode: amodePreIndex2, rn: x5VReg, want: "e50f10b8"},
		{k: store64, amode: amodePreIndex1, rn: x5VReg, want: "c5af00f8"},
		{k: store64, amode: amodePreIndex2, rn: x5VReg, want: "e50f10f8"},
		{k: fpuStore32, amode: amodePreIndex1, rn: v5VReg, want: "c5af00bc"},
		{k: fpuStore32, amode: amodePreIndex2, rn: v5VReg, want: "e50f10bc"},
		{k: fpuStore64, amode: amodePreIndex1, rn: v5VReg, want: "c5af00fc"},
		{k: fpuStore64, amode: amodePreIndex2, rn: v5VReg, want: "e50f10fc"},
		{k: fpuStore128, amode: amodePreIndex1, rn: v5VReg, want: "c5af803c"},
		{k: fpuStore128, amode: amodePreIndex2, rn: v5VReg, want: "e50f903c"},
		{k: uLoad8, amode: amodePreIndex1, rn: x5VReg, want: "c5af4038"},
		{k: uLoad8, amode: amodePreIndex2, rn: x5VReg, want: "e50f5038"},
		{k: uLoad16, amode: amodePreIndex1, rn: x5VReg, want: "c5af4078"},
		{k: uLoad16, amode: amodePreIndex2, rn: x5VReg, want: "e50f5078"},
		{k: uLoad32, amode: amodePreIndex1, rn: x5VReg, want: "c5af40b8"},
		{k: uLoad32, amode: amodePreIndex2, rn: x5VReg, want: "e50f50b8"},
		{k: uLoad64, amode: amodePreIndex1, rn: x5VReg, want: "c5af40f8"},
		{k: uLoad64, amode: amodePreIndex2, rn: x5VReg, want: "e50f50f8"},
		{k: sLoad8, amode: amodePreIndex1, rn: x5VReg, want: "c5af8038"},
		{k: sLoad8, amode: amodePreIndex2, rn: x5VReg, want: "e50f9038"},
		{k: sLoad16, amode: amodePreIndex1, rn: x5VReg, want: "c5af8078"},
		{k: sLoad16, amode: amodePreIndex2, rn: x5VReg, want: "e50f9078"},
		{k: sLoad32, amode: amodePreIndex1, rn: x5VReg, want: "c5af80b8"},
		{k: sLoad32, amode: amodePreIndex2, rn: x5VReg, want: "e50f90b8"},
		{k: fpuLoad32, amode: amodePreIndex1, rn: v5VReg, want: "c5af40bc"},
		{k: fpuLoad32, amode: amodePreIndex2, rn: v5VReg, want: "e50f50bc"},
		{k: fpuLoad64, amode: amodePreIndex1, rn: v5VReg, want: "c5af40fc"},
		{k: fpuLoad64, amode: amodePreIndex2, rn: v5VReg, want: "e50f50fc"},
		{k: fpuLoad128, amode: amodePreIndex1, rn: v5VReg, want: "c5afc03c"},
		{k: fpuLoad128, amode: amodePreIndex2, rn: v5VReg, want: "e50fd03c"},
		// addressModeKindRegUnsignedImm12.
		{k: store8, amode: setImm(amodeUnsignedImm12_1, 10), rn: x5VReg, want: "c52b0039"},
		{k: store8, amode: setImm(amodeUnsignedImm12_2, 4095), rn: x5VReg, want: "e5ff3f39"},
		{k: store16, amode: setImm(amodeUnsignedImm12_1, 10), rn: x5VReg, want: "c5170079"},
		{k: store16, amode: setImm(amodeUnsignedImm12_2, 4095*2), rn: x5VReg, want: "e5ff3f79"},
		{k: store32, amode: setImm(amodeUnsignedImm12_1, 16), rn: x5VReg, want: "c51300b9"},
		{k: store32, amode: setImm(amodeUnsignedImm12_2, 4095*4), rn: x5VReg, want: "e5ff3fb9"},
		{k: store64, amode: setImm(amodeUnsignedImm12_1, 16), rn: x5VReg, want: "c50b00f9"},
		{k: store64, amode: setImm(amodeUnsignedImm12_2, 4095*8), rn: x5VReg, want: "e5ff3ff9"},
		{k: fpuStore32, amode: setImm(amodeUnsignedImm12_1, 256), rn: v5VReg, want: "c50301bd"},
		{k: fpuStore32, amode: setImm(amodeUnsignedImm12_2, 4095*4), rn: v5VReg, want: "e5ff3fbd"},
		{k: fpuStore64, amode: setImm(amodeUnsignedImm12_1, 512), rn: v5VReg, want: "c50301fd"},
		{k: fpuStore64, amode: setImm(amodeUnsignedImm12_2, 4095*8), rn: v5VReg, want: "e5ff3ffd"},
		{k: fpuStore128, amode: setImm(amodeUnsignedImm12_1, 16), rn: v5VReg, want: "c507803d"},
		{k: fpuStore128, amode: setImm(amodeUnsignedImm12_2, 4095*16), rn: v5VReg, want: "e5ffbf3d"},
		{k: uLoad8, amode: setImm(amodeUnsignedImm12_1, 10), rn: x5VReg, want: "c52b4039"},
		{k: uLoad8, amode: setImm(amodeUnsignedImm12_2, 4095), rn: x5VReg, want: "e5ff7f39"},
		{k: uLoad16, amode: setImm(amodeUnsignedImm12_1, 10), rn: x5VReg, want: "c5174079"},
		{k: uLoad16, amode: setImm(amodeUnsignedImm12_2, 4095*2), rn: x5VReg, want: "e5ff7f79"},
		{k: uLoad32, amode: setImm(amodeUnsignedImm12_1, 16), rn: x5VReg, want: "c51340b9"},
		{k: uLoad32, amode: setImm(amodeUnsignedImm12_2, 4095*4), rn: x5VReg, want: "e5ff7fb9"},
		{k: uLoad64, amode: setImm(amodeUnsignedImm12_1, 16), rn: x5VReg, want: "c50b40f9"},
		{k: uLoad64, amode: setImm(amodeUnsignedImm12_2, 4095*8), rn: x5VReg, want: "e5ff7ff9"},
		{k: sLoad8, amode: setImm(amodeUnsignedImm12_1, 10), rn: x5VReg, want: "c52b8039"},
		{k: sLoad8, amode: setImm(amodeUnsignedImm12_2, 4095), rn: x5VReg, want: "e5ffbf39"},
		{k: sLoad16, amode: setImm(amodeUnsignedImm12_1, 10), rn: x5VReg, want: "c5178079"},
		{k: sLoad16, amode: setImm(amodeUnsignedImm12_2, 4095*2), rn: x5VReg, want: "e5ffbf79"},
		{k: sLoad32, amode: setImm(amodeUnsignedImm12_1, 16), rn: x5VReg, want: "c51380b9"},
		{k: sLoad32, amode: setImm(amodeUnsignedImm12_2, 4095*4), rn: x5VReg, want: "e5ffbfb9"},
		{k: fpuLoad32, amode: setImm(amodeUnsignedImm12_1, 256), rn: v5VReg, want: "c50341bd"},
		{k: fpuLoad32, amode: setImm(amodeUnsignedImm12_2, 4095*4), rn: v5VReg, want: "e5ff7fbd"},
		{k: fpuLoad64, amode: setImm(amodeUnsignedImm12_1, 512), rn: v5VReg, want: "c50341fd"},
		{k: fpuLoad64, amode: setImm(amodeUnsignedImm12_2, 4095*8), rn: v5VReg, want: "e5ff7ffd"},
		{k: fpuLoad128, amode: setImm(amodeUnsignedImm12_1, 16), rn: v5VReg, want: "c507c03d"},
		{k: fpuLoad128, amode: setImm(amodeUnsignedImm12_2, 4095*16), rn: v5VReg, want: "e5ffff3d"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			var i *instruction
			switch tc.k {
			case store8, store16, store32, store64, fpuStore32, fpuStore64, fpuStore128:
				i = &instruction{kind: tc.k, amode: tc.amode, rn: operandNR(tc.rn)}
			case uLoad8, uLoad16, uLoad32, uLoad64, sLoad8, sLoad16, sLoad32, fpuLoad32, fpuLoad64, fpuLoad128:
				i = &instruction{kind: tc.k, amode: tc.amode, rd: operandNR(tc.rn)}
			default:
				t.Fatalf("unknown kind: %v", tc.k)
			}
			m := &mockCompiler{}
			i.encode(m)
			// Note: for quick iteration we can use golang.org/x/arch package to verify the encoding.
			// 	but wazero doesn't add even a test dependency to it, so commented out.
			// inst, err := arm64asm.Decode(m.buf)
			// require.NoError(t, err)
			// fmt.Println(inst.String())
			require.Equal(t, tc.want, hex.EncodeToString(m.buf))
		})
	}
}

func Test_encodeExitSequence(t *testing.T) {
	m := &mockCompiler{}
	encodeExitSequence(m, x22VReg)
	// ldr x29, [x22, #0x10]
	// ldr x27, [x22, #0x18]
	// mov sp, x27
	// ldr x30, [x22, #0x20]
	// ret
	require.Equal(t, "dd0a40f9db0e40f97f030091de1240f9c0035fd6", hex.EncodeToString(m.buf))
	require.Equal(t, len(m.buf), exitSequenceSize)
}

func Test_lowerExitWithCodeEncodingSize(t *testing.T) {
	compiler, _, m := newSetupWithMockContext()
	m.lowerExitWithCode(x10VReg, wazevoapi.ExitCodeGrowStack)
	m.FlushPendingInstructions()
	require.NotNil(t, m.perBlockHead)
	m.encode(m.perBlockHead)
	require.Equal(t, exitWithCodeEncodingSize, len(compiler.Buf()))
}
