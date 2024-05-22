package arm64

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"testing"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/backend/regalloc"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_dummy(t *testing.T) {
	require.Equal(t, dummyInstruction, encodeUnconditionalBranch(false, 0))
}

func TestInstruction_encode(t *testing.T) {
	m := NewBackend().(*machine)
	dummyLabel := label(1)
	for _, tc := range []struct {
		setup func(*instruction)
		want  string
	}{
		{want: "3f441bd5", setup: func(i *instruction) { i.asMovToFPSR(xzrVReg) }},
		{want: "21441bd5", setup: func(i *instruction) { i.asMovToFPSR(x1VReg) }},
		{want: "21443bd5", setup: func(i *instruction) { i.asMovFromFPSR(x1VReg) }},
		{want: "2f08417a", setup: func(i *instruction) { i.asCCmpImm(operandNR(x1VReg), 1, eq, 0b1111, false) }},
		{want: "201841fa", setup: func(i *instruction) { i.asCCmpImm(operandNR(x1VReg), 1, ne, 0, true) }},
		{want: "410c010e", setup: func(i *instruction) { i.asVecDup(v1VReg, operandNR(v2VReg), vecArrangement8B) }},
		{want: "410c014e", setup: func(i *instruction) { i.asVecDup(v1VReg, operandNR(v2VReg), vecArrangement16B) }},
		{want: "410c020e", setup: func(i *instruction) { i.asVecDup(v1VReg, operandNR(v2VReg), vecArrangement4H) }},
		{want: "410c024e", setup: func(i *instruction) { i.asVecDup(v1VReg, operandNR(v2VReg), vecArrangement8H) }},
		{want: "410c040e", setup: func(i *instruction) { i.asVecDup(v1VReg, operandNR(v2VReg), vecArrangement2S) }},
		{want: "410c044e", setup: func(i *instruction) { i.asVecDup(v1VReg, operandNR(v2VReg), vecArrangement4S) }},
		{want: "410c084e", setup: func(i *instruction) { i.asVecDup(v1VReg, operandNR(v2VReg), vecArrangement2D) }},
		{want: "4104034e", setup: func(i *instruction) { i.asVecDupElement(v1VReg, operandNR(v2VReg), vecArrangementB, 1) }},
		{want: "4104064e", setup: func(i *instruction) { i.asVecDupElement(v1VReg, operandNR(v2VReg), vecArrangementH, 1) }},
		{want: "41040c4e", setup: func(i *instruction) { i.asVecDupElement(v1VReg, operandNR(v2VReg), vecArrangementS, 1) }},
		{want: "4104184e", setup: func(i *instruction) { i.asVecDupElement(v1VReg, operandNR(v2VReg), vecArrangementD, 1) }},
		{want: "4138032e", setup: func(i *instruction) {
			i.asVecExtract(v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B, 7)
		}},
		{want: "4138036e", setup: func(i *instruction) {
			i.asVecExtract(v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B, 7)
		}},
		{want: "410c036e", setup: func(i *instruction) { i.asVecMovElement(v1VReg, operandNR(v2VReg), vecArrangementB, 1, 1) }},
		{want: "4114066e", setup: func(i *instruction) { i.asVecMovElement(v1VReg, operandNR(v2VReg), vecArrangementH, 1, 1) }},
		{want: "41240c6e", setup: func(i *instruction) { i.asVecMovElement(v1VReg, operandNR(v2VReg), vecArrangementS, 1, 1) }},
		{want: "4144186e", setup: func(i *instruction) { i.asVecMovElement(v1VReg, operandNR(v2VReg), vecArrangementD, 1, 1) }},
		{want: "4104090f", setup: func(i *instruction) {
			i.asVecShiftImm(vecOpSshr, v1VReg, operandNR(v2VReg), operandShiftImm(7), vecArrangement8B)
		}},
		{want: "4104094f", setup: func(i *instruction) {
			i.asVecShiftImm(vecOpSshr, v1VReg, operandNR(v2VReg), operandShiftImm(7), vecArrangement16B)
		}},
		{want: "4104190f", setup: func(i *instruction) {
			i.asVecShiftImm(vecOpSshr, v1VReg, operandNR(v2VReg), operandShiftImm(7), vecArrangement4H)
		}},
		{want: "4104194f", setup: func(i *instruction) {
			i.asVecShiftImm(vecOpSshr, v1VReg, operandNR(v2VReg), operandShiftImm(7), vecArrangement8H)
		}},
		{want: "4104390f", setup: func(i *instruction) {
			i.asVecShiftImm(vecOpSshr, v1VReg, operandNR(v2VReg), operandShiftImm(7), vecArrangement2S)
		}},
		{want: "4104394f", setup: func(i *instruction) {
			i.asVecShiftImm(vecOpSshr, v1VReg, operandNR(v2VReg), operandShiftImm(7), vecArrangement4S)
		}},
		{want: "4104794f", setup: func(i *instruction) {
			i.asVecShiftImm(vecOpSshr, v1VReg, operandNR(v2VReg), operandShiftImm(7), vecArrangement2D)
		}},

		{want: "41a40d0f", setup: func(i *instruction) {
			i.asVecShiftImm(vecOpSshll, v1VReg, operandNR(v2VReg), operandShiftImm(3), vecArrangement8B)
		}},
		{want: "41a40d4f", setup: func(i *instruction) { // sshll2
			i.asVecShiftImm(vecOpSshll, v1VReg, operandNR(v2VReg), operandShiftImm(3), vecArrangement16B)
		}},
		{want: "41a41d0f", setup: func(i *instruction) {
			i.asVecShiftImm(vecOpSshll, v1VReg, operandNR(v2VReg), operandShiftImm(3), vecArrangement4H)
		}},
		{want: "41a41d4f", setup: func(i *instruction) { // sshll2
			i.asVecShiftImm(vecOpSshll, v1VReg, operandNR(v2VReg), operandShiftImm(3), vecArrangement8H)
		}},
		{want: "41a43d0f", setup: func(i *instruction) {
			i.asVecShiftImm(vecOpSshll, v1VReg, operandNR(v2VReg), operandShiftImm(3), vecArrangement2S)
		}},
		{want: "41a43d4f", setup: func(i *instruction) { // sshll2
			i.asVecShiftImm(vecOpSshll, v1VReg, operandNR(v2VReg), operandShiftImm(3), vecArrangement4S)
		}},
		{want: "41a40d2f", setup: func(i *instruction) {
			i.asVecShiftImm(vecOpUshll, v1VReg, operandNR(v2VReg), operandShiftImm(3), vecArrangement8B)
		}},
		{want: "41a40d6f", setup: func(i *instruction) { // ushll2
			i.asVecShiftImm(vecOpUshll, v1VReg, operandNR(v2VReg), operandShiftImm(3), vecArrangement16B)
		}},
		{want: "41a41d2f", setup: func(i *instruction) {
			i.asVecShiftImm(vecOpUshll, v1VReg, operandNR(v2VReg), operandShiftImm(3), vecArrangement4H)
		}},
		{want: "41a41d6f", setup: func(i *instruction) { // ushll2
			i.asVecShiftImm(vecOpUshll, v1VReg, operandNR(v2VReg), operandShiftImm(3), vecArrangement8H)
		}},
		{want: "41a43d2f", setup: func(i *instruction) {
			i.asVecShiftImm(vecOpUshll, v1VReg, operandNR(v2VReg), operandShiftImm(3), vecArrangement2S)
		}},
		{want: "41a43d6f", setup: func(i *instruction) { // ushll2
			i.asVecShiftImm(vecOpUshll, v1VReg, operandNR(v2VReg), operandShiftImm(3), vecArrangement4S)
		}},
		{want: "4100030e", setup: func(i *instruction) {
			i.asVecTbl(1, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "4100034e", setup: func(i *instruction) {
			i.asVecTbl(1, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "4120040e", setup: func(i *instruction) {
			i.asVecTbl(2, v1VReg, operandNR(v2VReg), operandNR(v4VReg), vecArrangement8B)
		}},
		{want: "4120044e", setup: func(i *instruction) {
			i.asVecTbl(2, v1VReg, operandNR(v2VReg), operandNR(v4VReg), vecArrangement16B)
		}},
		{want: "4138030e", setup: func(i *instruction) {
			i.asVecPermute(vecOpZip1, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "4138034e", setup: func(i *instruction) {
			i.asVecPermute(vecOpZip1, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "4138430e", setup: func(i *instruction) {
			i.asVecPermute(vecOpZip1, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "4138434e", setup: func(i *instruction) {
			i.asVecPermute(vecOpZip1, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "4138830e", setup: func(i *instruction) {
			i.asVecPermute(vecOpZip1, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "4138834e", setup: func(i *instruction) {
			i.asVecPermute(vecOpZip1, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "4138c34e", setup: func(i *instruction) {
			i.asVecPermute(vecOpZip1, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "411ca32e", setup: func(i *instruction) {
			i.asVecRRRRewrite(vecOpBit, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "411ca36e", setup: func(i *instruction) {
			i.asVecRRRRewrite(vecOpBit, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "411c236e", setup: func(i *instruction) {
			i.asVecRRR(vecOpEOR, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "411c232e", setup: func(i *instruction) {
			i.asVecRRR(vecOpEOR, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "4184234e", setup: func(i *instruction) {
			i.asVecRRR(vecOpAdd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "4184a34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpAdd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "4184e34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpAdd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "410c230e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSqadd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "410c234e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSqadd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "410c630e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSqadd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "410c634e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSqadd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "410ca30e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSqadd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "410ca34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSqadd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "410ce34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSqadd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "410c232e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUqadd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "410c236e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUqadd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "410c632e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUqadd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "410c636e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUqadd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "410ca32e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUqadd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "410ca36e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUqadd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "410ce36e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUqadd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "412c230e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSqsub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "412c234e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSqsub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "412c630e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSqsub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "412c634e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSqsub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "412ca30e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSqsub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "412ca34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSqsub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "412ce34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSqsub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "412c232e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUqsub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "412c236e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUqsub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "412c632e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUqsub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "412c636e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUqsub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "412ca32e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUqsub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "412ca36e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUqsub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "412ce36e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUqsub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "4184232e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "4184236e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "4184632e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "4184636e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "4184a32e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "4184a36e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "4184e36e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "41bc230e", setup: func(i *instruction) {
			i.asVecRRR(vecOpAddp, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "41bc234e", setup: func(i *instruction) {
			i.asVecRRR(vecOpAddp, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "41bc630e", setup: func(i *instruction) {
			i.asVecRRR(vecOpAddp, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "41bc634e", setup: func(i *instruction) {
			i.asVecRRR(vecOpAddp, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "41bca30e", setup: func(i *instruction) {
			i.asVecRRR(vecOpAddp, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "41bca34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpAddp, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "41bce34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpAddp, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "41bc230e", setup: func(i *instruction) {
			i.asVecRRR(vecOpAddp, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "41b8314e", setup: func(i *instruction) {
			i.asVecLanes(vecOpAddv, v1VReg, operandNR(v2VReg), vecArrangement16B)
		}},
		{want: "41b8710e", setup: func(i *instruction) {
			i.asVecLanes(vecOpAddv, v1VReg, operandNR(v2VReg), vecArrangement4H)
		}},
		{want: "41b8714e", setup: func(i *instruction) {
			i.asVecLanes(vecOpAddv, v1VReg, operandNR(v2VReg), vecArrangement8H)
		}},
		{want: "41b8b14e", setup: func(i *instruction) {
			i.asVecLanes(vecOpAddv, v1VReg, operandNR(v2VReg), vecArrangement4S)
		}},
		{want: "416c230e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSmin, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "416c234e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSmin, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "416c630e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSmin, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "416c634e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSmin, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "416ca30e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSmin, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "416ca34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSmin, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "416c232e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUmin, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "416c236e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUmin, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "416c632e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUmin, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "416c636e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUmin, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "416ca32e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUmin, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "416ca36e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUmin, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "4164230e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSmax, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "4164234e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSmax, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "4164630e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSmax, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "4164634e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSmax, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "4164a30e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSmax, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "4164a34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSmax, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "4164232e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUmax, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "4164236e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUmax, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "4164632e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUmax, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "4164636e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUmax, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "4164a32e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUmax, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "4164a36e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUmax, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "41a4232e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUmaxp, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "41a4236e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUmaxp, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "41a4632e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUmaxp, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "41a4636e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUmaxp, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "41a4a32e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUmaxp, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "41a8312e", setup: func(i *instruction) {
			i.asVecLanes(vecOpUminv, v1VReg, operandNR(v2VReg), vecArrangement8B)
		}},
		{want: "41a8316e", setup: func(i *instruction) {
			i.asVecLanes(vecOpUminv, v1VReg, operandNR(v2VReg), vecArrangement16B)
		}},
		{want: "41a8712e", setup: func(i *instruction) {
			i.asVecLanes(vecOpUminv, v1VReg, operandNR(v2VReg), vecArrangement4H)
		}},
		{want: "41a8716e", setup: func(i *instruction) {
			i.asVecLanes(vecOpUminv, v1VReg, operandNR(v2VReg), vecArrangement8H)
		}},
		{want: "41a8b16e", setup: func(i *instruction) {
			i.asVecLanes(vecOpUminv, v1VReg, operandNR(v2VReg), vecArrangement4S)
		}},
		{want: "4114232e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUrhadd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "4114236e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUrhadd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "4114632e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUrhadd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "4114636e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUrhadd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "4114a32e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUrhadd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "4114a36e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUrhadd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "419c230e", setup: func(i *instruction) {
			i.asVecRRR(vecOpMul, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "419c234e", setup: func(i *instruction) {
			i.asVecRRR(vecOpMul, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "419c630e", setup: func(i *instruction) {
			i.asVecRRR(vecOpMul, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "419c634e", setup: func(i *instruction) {
			i.asVecRRR(vecOpMul, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "419ca30e", setup: func(i *instruction) {
			i.asVecRRR(vecOpMul, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "419ca34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpMul, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "4198200e", setup: func(i *instruction) {
			i.asVecMisc(vecOpCmeq0, v1VReg, operandNR(v2VReg), vecArrangement8B)
		}},
		{want: "4198204e", setup: func(i *instruction) {
			i.asVecMisc(vecOpCmeq0, v1VReg, operandNR(v2VReg), vecArrangement16B)
		}},
		{want: "4198600e", setup: func(i *instruction) {
			i.asVecMisc(vecOpCmeq0, v1VReg, operandNR(v2VReg), vecArrangement4H)
		}},
		{want: "4198604e", setup: func(i *instruction) {
			i.asVecMisc(vecOpCmeq0, v1VReg, operandNR(v2VReg), vecArrangement8H)
		}},
		{want: "4198a00e", setup: func(i *instruction) {
			i.asVecMisc(vecOpCmeq0, v1VReg, operandNR(v2VReg), vecArrangement2S)
		}},
		{want: "4198a04e", setup: func(i *instruction) {
			i.asVecMisc(vecOpCmeq0, v1VReg, operandNR(v2VReg), vecArrangement4S)
		}},
		{want: "4198e04e", setup: func(i *instruction) {
			i.asVecMisc(vecOpCmeq0, v1VReg, operandNR(v2VReg), vecArrangement2D)
		}},
		{want: "418c232e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmeq, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "418c236e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmeq, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "418c632e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmeq, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "418c636e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmeq, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "418ca32e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmeq, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "418ca36e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmeq, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "418ce36e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmeq, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "4134230e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmgt, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "4134234e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmgt, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "4134630e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmgt, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "4134634e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmgt, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "4134a30e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmgt, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "4134a34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmgt, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "4134e34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmgt, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "4134232e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmhi, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "4134236e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmhi, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "4134632e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmhi, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "4134636e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmhi, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "4134a32e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmhi, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "4134a36e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmhi, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "4134e36e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmhi, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "413c230e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmge, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "413c234e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmge, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "413c630e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmge, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "413c634e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmge, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "413ca30e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmge, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "413ca34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmge, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "413ce34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmge, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "4134230e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmgt, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "4134234e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmgt, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "4134630e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmgt, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "4134634e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmgt, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "4134a30e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmgt, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "4134a34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmgt, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "4134e34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmgt, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "4134232e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmhi, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "4134236e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmhi, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "4134632e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmhi, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "4134636e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmhi, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "4134a32e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmhi, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "4134a36e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmhi, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "4134e36e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmhi, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "413c232e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmhs, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "413c236e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmhs, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "413c632e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmhs, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "413c636e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmhs, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "413ca32e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmhs, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "413ca36e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmhs, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "413ce36e", setup: func(i *instruction) {
			i.asVecRRR(vecOpCmhs, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "41f4230e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFmax, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "41f4234e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFmax, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "41f4634e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFmax, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "41f4a30e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFmin, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "41f4a34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFmin, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "41f4e34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFmin, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "41d4230e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFadd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "41d4234e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFadd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "41d4634e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFadd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "41d4a30e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFsub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "41d4a34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFsub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "41d4e34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFsub, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "41dc232e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFmul, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "41dc236e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFmul, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "41dc636e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFmul, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "41b4636e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSqrdmulh, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "41b4632e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSqrdmulh, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "41b4a32e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSqrdmulh, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "41b4a36e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSqrdmulh, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "41fc232e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFdiv, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "41fc236e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFdiv, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "41fc636e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFdiv, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "41e4230e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFcmeq, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "41e4234e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFcmeq, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "41e4634e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFcmeq, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "41e4a32e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFcmgt, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "41e4a36e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFcmgt, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "41e4e36e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFcmgt, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "41e4232e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFcmge, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "41e4236e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFcmge, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "41e4636e", setup: func(i *instruction) {
			i.asVecRRR(vecOpFcmge, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "4198210e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFrintm, v1VReg, operandNR(v2VReg), vecArrangement2S)
		}},
		{want: "4198214e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFrintm, v1VReg, operandNR(v2VReg), vecArrangement4S)
		}},
		{want: "4198614e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFrintm, v1VReg, operandNR(v2VReg), vecArrangement2D)
		}},
		{want: "4188210e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFrintn, v1VReg, operandNR(v2VReg), vecArrangement2S)
		}},
		{want: "4188214e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFrintn, v1VReg, operandNR(v2VReg), vecArrangement4S)
		}},
		{want: "4188614e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFrintn, v1VReg, operandNR(v2VReg), vecArrangement2D)
		}},
		{want: "4188a10e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFrintp, v1VReg, operandNR(v2VReg), vecArrangement2S)
		}},
		{want: "4188a14e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFrintp, v1VReg, operandNR(v2VReg), vecArrangement4S)
		}},
		{want: "4188e14e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFrintp, v1VReg, operandNR(v2VReg), vecArrangement2D)
		}},
		{want: "4198a10e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFrintz, v1VReg, operandNR(v2VReg), vecArrangement2S)
		}},
		{want: "4198a14e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFrintz, v1VReg, operandNR(v2VReg), vecArrangement4S)
		}},
		{want: "4198e14e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFrintz, v1VReg, operandNR(v2VReg), vecArrangement2D)
		}},
		{want: "4178610e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFcvtl, v1VReg, operandNR(v2VReg), vecArrangement2S)
		}},
		{want: "4178210e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFcvtl, v1VReg, operandNR(v2VReg), vecArrangement4H)
		}},
		{want: "4168610e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFcvtn, v1VReg, operandNR(v2VReg), vecArrangement2S)
		}},
		{want: "4168210e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFcvtn, v1VReg, operandNR(v2VReg), vecArrangement4H)
		}},
		{want: "41b8a10e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFcvtzs, v1VReg, operandNR(v2VReg), vecArrangement2S)
		}},
		{want: "41b8a14e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFcvtzs, v1VReg, operandNR(v2VReg), vecArrangement4S)
		}},
		{want: "41b8e14e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFcvtzs, v1VReg, operandNR(v2VReg), vecArrangement2D)
		}},
		{want: "41b8a12e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFcvtzu, v1VReg, operandNR(v2VReg), vecArrangement2S)
		}},
		{want: "41b8a16e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFcvtzu, v1VReg, operandNR(v2VReg), vecArrangement4S)
		}},
		{want: "41b8e16e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFcvtzu, v1VReg, operandNR(v2VReg), vecArrangement2D)
		}},
		{want: "41d8210e", setup: func(i *instruction) {
			i.asVecMisc(vecOpScvtf, v1VReg, operandNR(v2VReg), vecArrangement2S)
		}},
		{want: "41d8214e", setup: func(i *instruction) {
			i.asVecMisc(vecOpScvtf, v1VReg, operandNR(v2VReg), vecArrangement4S)
		}},
		{want: "41d8614e", setup: func(i *instruction) {
			i.asVecMisc(vecOpScvtf, v1VReg, operandNR(v2VReg), vecArrangement2D)
		}},
		{want: "41d8212e", setup: func(i *instruction) {
			i.asVecMisc(vecOpUcvtf, v1VReg, operandNR(v2VReg), vecArrangement2S)
		}},
		{want: "41d8216e", setup: func(i *instruction) {
			i.asVecMisc(vecOpUcvtf, v1VReg, operandNR(v2VReg), vecArrangement4S)
		}},
		{want: "41d8616e", setup: func(i *instruction) {
			i.asVecMisc(vecOpUcvtf, v1VReg, operandNR(v2VReg), vecArrangement2D)
		}},
		{want: "4148210e", setup: func(i *instruction) {
			i.asVecMisc(vecOpSqxtn, v1VReg, operandNR(v2VReg), vecArrangement8B)
		}},
		{want: "4148214e", setup: func(i *instruction) { // sqxtn2
			i.asVecMisc(vecOpSqxtn, v1VReg, operandNR(v2VReg), vecArrangement16B)
		}},
		{want: "4148610e", setup: func(i *instruction) {
			i.asVecMisc(vecOpSqxtn, v1VReg, operandNR(v2VReg), vecArrangement4H)
		}},
		{want: "4148614e", setup: func(i *instruction) { // sqxtn2
			i.asVecMisc(vecOpSqxtn, v1VReg, operandNR(v2VReg), vecArrangement8H)
		}},
		{want: "4148a10e", setup: func(i *instruction) {
			i.asVecMisc(vecOpSqxtn, v1VReg, operandNR(v2VReg), vecArrangement2S)
		}},
		{want: "4148a14e", setup: func(i *instruction) { // sqxtun2
			i.asVecMisc(vecOpSqxtn, v1VReg, operandNR(v2VReg), vecArrangement4S)
		}},
		{want: "4128212e", setup: func(i *instruction) {
			i.asVecMisc(vecOpSqxtun, v1VReg, operandNR(v2VReg), vecArrangement8B)
		}},
		{want: "4128216e", setup: func(i *instruction) { // uqxtun2
			i.asVecMisc(vecOpSqxtun, v1VReg, operandNR(v2VReg), vecArrangement16B)
		}},
		{want: "4128612e", setup: func(i *instruction) {
			i.asVecMisc(vecOpSqxtun, v1VReg, operandNR(v2VReg), vecArrangement4H)
		}},
		{want: "4128616e", setup: func(i *instruction) { // sqxtun2
			i.asVecMisc(vecOpSqxtun, v1VReg, operandNR(v2VReg), vecArrangement8H)
		}},
		{want: "4128a12e", setup: func(i *instruction) {
			i.asVecMisc(vecOpSqxtun, v1VReg, operandNR(v2VReg), vecArrangement2S)
		}},
		{want: "4128a16e", setup: func(i *instruction) { // sqxtun2
			i.asVecMisc(vecOpSqxtun, v1VReg, operandNR(v2VReg), vecArrangement4S)
		}},
		{want: "4148212e", setup: func(i *instruction) {
			i.asVecMisc(vecOpUqxtn, v1VReg, operandNR(v2VReg), vecArrangement8B)
		}},
		{want: "4148216e", setup: func(i *instruction) { // uqxtn2
			i.asVecMisc(vecOpUqxtn, v1VReg, operandNR(v2VReg), vecArrangement16B)
		}},
		{want: "4148612e", setup: func(i *instruction) {
			i.asVecMisc(vecOpUqxtn, v1VReg, operandNR(v2VReg), vecArrangement4H)
		}},
		{want: "4148616e", setup: func(i *instruction) { // sqxtn2
			i.asVecMisc(vecOpUqxtn, v1VReg, operandNR(v2VReg), vecArrangement8H)
		}},
		{want: "4148a12e", setup: func(i *instruction) {
			i.asVecMisc(vecOpUqxtn, v1VReg, operandNR(v2VReg), vecArrangement2S)
		}},
		{want: "4148a16e", setup: func(i *instruction) { // sqxtn2
			i.asVecMisc(vecOpUqxtn, v1VReg, operandNR(v2VReg), vecArrangement4S)
		}},
		{want: "41b8200e", setup: func(i *instruction) {
			i.asVecMisc(vecOpAbs, v1VReg, operandNR(v2VReg), vecArrangement8B)
		}},
		{want: "41b8204e", setup: func(i *instruction) {
			i.asVecMisc(vecOpAbs, v1VReg, operandNR(v2VReg), vecArrangement16B)
		}},
		{want: "41b8600e", setup: func(i *instruction) {
			i.asVecMisc(vecOpAbs, v1VReg, operandNR(v2VReg), vecArrangement4H)
		}},
		{want: "41b8604e", setup: func(i *instruction) {
			i.asVecMisc(vecOpAbs, v1VReg, operandNR(v2VReg), vecArrangement8H)
		}},
		{want: "41b8a00e", setup: func(i *instruction) {
			i.asVecMisc(vecOpAbs, v1VReg, operandNR(v2VReg), vecArrangement2S)
		}},
		{want: "41b8a04e", setup: func(i *instruction) {
			i.asVecMisc(vecOpAbs, v1VReg, operandNR(v2VReg), vecArrangement4S)
		}},
		{want: "41b8e04e", setup: func(i *instruction) {
			i.asVecMisc(vecOpAbs, v1VReg, operandNR(v2VReg), vecArrangement2D)
		}},
		{want: "41f8a00e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFabs, v1VReg, operandNR(v2VReg), vecArrangement2S)
		}},
		{want: "41f8a04e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFabs, v1VReg, operandNR(v2VReg), vecArrangement4S)
		}},
		{want: "41f8e04e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFabs, v1VReg, operandNR(v2VReg), vecArrangement2D)
		}},
		{want: "41b8202e", setup: func(i *instruction) {
			i.asVecMisc(vecOpNeg, v1VReg, operandNR(v2VReg), vecArrangement8B)
		}},
		{want: "41b8206e", setup: func(i *instruction) {
			i.asVecMisc(vecOpNeg, v1VReg, operandNR(v2VReg), vecArrangement16B)
		}},
		{want: "41b8602e", setup: func(i *instruction) {
			i.asVecMisc(vecOpNeg, v1VReg, operandNR(v2VReg), vecArrangement4H)
		}},
		{want: "41b8606e", setup: func(i *instruction) {
			i.asVecMisc(vecOpNeg, v1VReg, operandNR(v2VReg), vecArrangement8H)
		}},
		{want: "41b8a02e", setup: func(i *instruction) {
			i.asVecMisc(vecOpNeg, v1VReg, operandNR(v2VReg), vecArrangement2S)
		}},
		{want: "41b8a06e", setup: func(i *instruction) {
			i.asVecMisc(vecOpNeg, v1VReg, operandNR(v2VReg), vecArrangement4S)
		}},
		{want: "41b8e06e", setup: func(i *instruction) {
			i.asVecMisc(vecOpNeg, v1VReg, operandNR(v2VReg), vecArrangement2D)
		}},
		{want: "4128a10e", setup: func(i *instruction) {
			i.asVecMisc(vecOpXtn, v1VReg, operandNR(v2VReg), vecArrangement2S)
		}},
		{want: "4128a10e", setup: func(i *instruction) {
			i.asVecMisc(vecOpXtn, v1VReg, operandNR(v2VReg), vecArrangement2S)
		}},
		{want: "41f8a02e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFneg, v1VReg, operandNR(v2VReg), vecArrangement2S)
		}},
		{want: "41f8a06e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFneg, v1VReg, operandNR(v2VReg), vecArrangement4S)
		}},
		{want: "41f8e06e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFneg, v1VReg, operandNR(v2VReg), vecArrangement2D)
		}},
		{want: "41f8a12e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFsqrt, v1VReg, operandNR(v2VReg), vecArrangement2S)
		}},
		{want: "41f8a16e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFsqrt, v1VReg, operandNR(v2VReg), vecArrangement4S)
		}},
		{want: "41f8e16e", setup: func(i *instruction) {
			i.asVecMisc(vecOpFsqrt, v1VReg, operandNR(v2VReg), vecArrangement2D)
		}},
		{want: "4100839a", setup: func(i *instruction) { i.asCSel(x1VReg, operandNR(x2VReg), operandNR(x3VReg), eq, true) }},
		{want: "4110839a", setup: func(i *instruction) { i.asCSel(x1VReg, operandNR(x2VReg), operandNR(x3VReg), ne, true) }},
		{want: "4100831a", setup: func(i *instruction) { i.asCSel(x1VReg, operandNR(x2VReg), operandNR(x3VReg), eq, false) }},
		{want: "4110831a", setup: func(i *instruction) { i.asCSel(x1VReg, operandNR(x2VReg), operandNR(x3VReg), ne, false) }},
		{want: "41cc631e", setup: func(i *instruction) { i.asFpuCSel(v1VReg, operandNR(v2VReg), operandNR(v3VReg), gt, true) }},
		{want: "41bc631e", setup: func(i *instruction) { i.asFpuCSel(v1VReg, operandNR(v2VReg), operandNR(v3VReg), lt, true) }},
		{want: "41cc231e", setup: func(i *instruction) { i.asFpuCSel(v1VReg, operandNR(v2VReg), operandNR(v3VReg), gt, false) }},
		{want: "41bc231e", setup: func(i *instruction) { i.asFpuCSel(v1VReg, operandNR(v2VReg), operandNR(v3VReg), lt, false) }},
		{want: "411c014e", setup: func(i *instruction) { i.asMovToVec(v1VReg, operandNR(x2VReg), vecArrangementB, 0) }},
		{want: "411c024e", setup: func(i *instruction) { i.asMovToVec(v1VReg, operandNR(x2VReg), vecArrangementH, 0) }},
		{want: "411c044e", setup: func(i *instruction) { i.asMovToVec(v1VReg, operandNR(x2VReg), vecArrangementS, 0) }},
		{want: "411c084e", setup: func(i *instruction) { i.asMovToVec(v1VReg, operandNR(x2VReg), vecArrangementD, 0) }},
		{want: "413c010e", setup: func(i *instruction) { i.asMovFromVec(x1VReg, operandNR(v2VReg), vecArrangementB, 0, false) }},
		{want: "413c020e", setup: func(i *instruction) { i.asMovFromVec(x1VReg, operandNR(v2VReg), vecArrangementH, 0, false) }},
		{want: "413c040e", setup: func(i *instruction) { i.asMovFromVec(x1VReg, operandNR(v2VReg), vecArrangementS, 0, false) }},
		{want: "413c084e", setup: func(i *instruction) { i.asMovFromVec(x1VReg, operandNR(v2VReg), vecArrangementD, 0, false) }},
		{want: "412c030e", setup: func(i *instruction) { i.asMovFromVec(x1VReg, operandNR(v2VReg), vecArrangementB, 1, true) }},
		{want: "412c060e", setup: func(i *instruction) { i.asMovFromVec(x1VReg, operandNR(v2VReg), vecArrangementH, 1, true) }},
		{want: "412c0c4e", setup: func(i *instruction) { i.asMovFromVec(x1VReg, operandNR(v2VReg), vecArrangementS, 1, true) }},
		{want: "410c084e", setup: func(i *instruction) { i.asVecDup(x1VReg, operandNR(v2VReg), vecArrangement2D) }},
		{want: "4140036e", setup: func(i *instruction) { // 4140036e
			i.asVecExtract(x1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B, 8)
		}},
		{want: "4138034e", setup: func(i *instruction) {
			i.asVecPermute(vecOpZip1, x1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "4104214f", setup: func(i *instruction) {
			i.asVecShiftImm(vecOpSshr, x1VReg, operandNR(x2VReg), operandShiftImm(31), vecArrangement4S)
		}},
		{want: "5b28030b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSL), false)
		}},
		{want: "5b28038b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSL), true)
		}},
		{want: "5b28032b", setup: func(i *instruction) {
			i.asALU(aluOpAddS, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSL), false)
		}},
		{want: "5b2803ab", setup: func(i *instruction) {
			i.asALU(aluOpAddS, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSL), true)
		}},
		{want: "5b28430b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSR), false)
		}},
		{want: "5b28438b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSR), true)
		}},
		{want: "5b28432b", setup: func(i *instruction) {
			i.asALU(aluOpAddS, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSR), false)
		}},
		{want: "5b2843ab", setup: func(i *instruction) {
			i.asALU(aluOpAddS, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSR), true)
		}},
		{want: "5b28830b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpASR), false)
		}},
		{want: "5b28838b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpASR), true)
		}},
		{want: "5b28832b", setup: func(i *instruction) {
			i.asALU(aluOpAddS, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpASR), false)
		}},
		{want: "5b2883ab", setup: func(i *instruction) {
			i.asALU(aluOpAddS, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpASR), true)
		}},
		{want: "5b28034b", setup: func(i *instruction) {
			i.asALU(aluOpSub, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSL), false)
		}},
		{want: "5b2803cb", setup: func(i *instruction) {
			i.asALU(aluOpSub, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSL), true)
		}},
		{want: "5b28036b", setup: func(i *instruction) {
			i.asALU(aluOpSubS, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSL), false)
		}},
		{want: "5b2803eb", setup: func(i *instruction) {
			i.asALU(aluOpSubS, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSL), true)
		}},
		{want: "5b28434b", setup: func(i *instruction) {
			i.asALU(aluOpSub, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSR), false)
		}},
		{want: "5b2843cb", setup: func(i *instruction) {
			i.asALU(aluOpSub, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSR), true)
		}},
		{want: "5b28436b", setup: func(i *instruction) {
			i.asALU(aluOpSubS, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSR), false)
		}},
		{want: "5b2843eb", setup: func(i *instruction) {
			i.asALU(aluOpSubS, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpLSR), true)
		}},
		{want: "5b28834b", setup: func(i *instruction) {
			i.asALU(aluOpSub, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpASR), false)
		}},
		{want: "5b2883cb", setup: func(i *instruction) {
			i.asALU(aluOpSub, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpASR), true)
		}},
		{want: "5b28836b", setup: func(i *instruction) {
			i.asALU(aluOpSubS, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpASR), false)
		}},
		{want: "5b2883eb", setup: func(i *instruction) {
			i.asALU(aluOpSubS, tmpRegVReg, operandNR(x2VReg), operandSR(x3VReg, 10, shiftOpASR), true)
		}},
		{want: "60033fd6", setup: func(i *instruction) {
			i.asCallIndirect(tmpRegVReg, nil)
		}},
		{want: "fb633bcb", setup: func(i *instruction) {
			i.asALU(aluOpSub, tmpRegVReg, operandNR(spVReg), operandNR(tmpRegVReg), true)
		}},
		{want: "fb633b8b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, tmpRegVReg, operandNR(spVReg), operandNR(tmpRegVReg), true)
		}},
		{want: "2000020a", setup: func(i *instruction) {
			i.asALU(aluOpAnd, x0VReg, operandNR(x1VReg), operandNR(x2VReg), false)
		}},
		{want: "2000028a", setup: func(i *instruction) {
			i.asALU(aluOpAnd, x0VReg, operandNR(x1VReg), operandNR(x2VReg), true)
		}},
		{want: "2010028a", setup: func(i *instruction) {
			i.asALU(aluOpAnd, x0VReg, operandNR(x1VReg), operandSR(x2VReg, 4, shiftOpLSL), true)
		}},
		{want: "2030428a", setup: func(i *instruction) {
			i.asALU(aluOpAnd, x0VReg, operandNR(x1VReg), operandSR(x2VReg, 12, shiftOpLSR), true)
		}},
		{want: "2000026a", setup: func(i *instruction) {
			i.asALU(aluOpAnds, x0VReg, operandNR(x1VReg), operandNR(x2VReg), false)
		}},
		{want: "200002ea", setup: func(i *instruction) {
			i.asALU(aluOpAnds, x0VReg, operandNR(x1VReg), operandNR(x2VReg), true)
		}},
		{want: "201002ea", setup: func(i *instruction) {
			i.asALU(aluOpAnds, x0VReg, operandNR(x1VReg), operandSR(x2VReg, 4, shiftOpLSL), true)
		}},
		{want: "203042ea", setup: func(i *instruction) {
			i.asALU(aluOpAnds, x0VReg, operandNR(x1VReg), operandSR(x2VReg, 12, shiftOpLSR), true)
		}},
		{want: "2000022a", setup: func(i *instruction) {
			i.asALU(aluOpOrr, x0VReg, operandNR(x1VReg), operandNR(x2VReg), false)
		}},
		{want: "200002aa", setup: func(i *instruction) {
			i.asALU(aluOpOrr, x0VReg, operandNR(x1VReg), operandNR(x2VReg), true)
		}},
		{want: "201002aa", setup: func(i *instruction) {
			i.asALU(aluOpOrr, x0VReg, operandNR(x1VReg), operandSR(x2VReg, 4, shiftOpLSL), true)
		}},
		{want: "201082aa", setup: func(i *instruction) {
			i.asALU(aluOpOrr, x0VReg, operandNR(x1VReg), operandSR(x2VReg, 4, shiftOpASR), true)
		}},
		{want: "2000024a", setup: func(i *instruction) {
			i.asALU(aluOpEor, x0VReg, operandNR(x1VReg), operandNR(x2VReg), false)
		}},
		{want: "200002ca", setup: func(i *instruction) {
			i.asALU(aluOpEor, x0VReg, operandNR(x1VReg), operandNR(x2VReg), true)
		}},
		{want: "201002ca", setup: func(i *instruction) {
			i.asALU(aluOpEor, x0VReg, operandNR(x1VReg), operandSR(x2VReg, 4, shiftOpLSL), true)
		}},
		{want: "202cc21a", setup: func(i *instruction) {
			i.asALU(aluOpRotR, x0VReg, operandNR(x1VReg), operandNR(x2VReg), false)
		}},
		{want: "202cc29a", setup: func(i *instruction) {
			i.asALU(aluOpRotR, x0VReg, operandNR(x1VReg), operandNR(x2VReg), true)
		}},
		{want: "2000222a", setup: func(i *instruction) {
			i.asALU(aluOpOrn, x0VReg, operandNR(x1VReg), operandNR(x2VReg), false)
		}},
		{want: "200022aa", setup: func(i *instruction) {
			i.asALU(aluOpOrn, x0VReg, operandNR(x1VReg), operandNR(x2VReg), true)
		}},
		{want: "30000010", setup: func(i *instruction) { i.asAdr(v16VReg, 4) }},
		{want: "50050030", setup: func(i *instruction) { i.asAdr(v16VReg, 169) }},
		{want: "101e302e", setup: func(i *instruction) { i.asLoadFpuConst32(v16VReg, uint64(math.Float32bits(0))) }},
		{want: "5000001c020000140000803f", setup: func(i *instruction) {
			i.asLoadFpuConst32(v16VReg, uint64(math.Float32bits(1.0)))
		}},
		{want: "101e302e", setup: func(i *instruction) { i.asLoadFpuConst64(v16VReg, uint64(math.Float32bits(0))) }},
		{want: "5000005c03000014000000000000f03f", setup: func(i *instruction) {
			i.asLoadFpuConst64(v16VReg, math.Float64bits(1.0))
		}},
		{want: "101e306e", setup: func(i *instruction) { i.asLoadFpuConst128(v16VReg, 0, 0) }},
		{want: "5000009c05000014ffffffffffffffffaaaaaaaaaaaaaaaa", setup: func(i *instruction) { i.asLoadFpuConst128(v16VReg, 0xffffffff_ffffffff, 0xaaaaaaaa_aaaaaaaa) }},
		{want: "8220061b", setup: func(i *instruction) {
			i.asALURRRR(aluOpMAdd, x2VReg, operandNR(x4VReg), operandNR(x6VReg), x8VReg, false)
		}},
		{want: "8220069b", setup: func(i *instruction) {
			i.asALURRRR(aluOpMAdd, x2VReg, operandNR(x4VReg), operandNR(x6VReg), x8VReg, true)
		}},
		{want: "82a0061b", setup: func(i *instruction) {
			i.asALURRRR(aluOpMSub, x2VReg, operandNR(x4VReg), operandNR(x6VReg), x8VReg, false)
		}},
		{want: "82a0069b", setup: func(i *instruction) {
			i.asALURRRR(aluOpMSub, x2VReg, operandNR(x4VReg), operandNR(x6VReg), x8VReg, true)
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
		{want: "f2079f9a", setup: func(i *instruction) { i.asCSet(x18VReg, false, ne) }},
		{want: "f2179f9a", setup: func(i *instruction) { i.asCSet(x18VReg, false, eq) }},
		{want: "e0039fda", setup: func(i *instruction) { i.asCSet(x0VReg, true, ne) }},
		{want: "f2139fda", setup: func(i *instruction) { i.asCSet(x18VReg, true, eq) }},
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
		{want: "4f020012", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x15VReg, x18VReg, 0x1, false) }},
		{want: "4f0a0012", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x15VReg, x18VReg, 0x7, false) }},
		{want: "4f0e0012", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x15VReg, x18VReg, 0xf, false) }},
		{want: "4f120012", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x15VReg, x18VReg, 0x1f, false) }},
		{want: "4f160012", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x15VReg, x18VReg, 0x3f, false) }},
		{want: "4f021112", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x15VReg, x18VReg, 0x8000, false) }},
		{want: "4f721f12", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x15VReg, x18VReg, 0x3ffffffe, false) }},
		{want: "4f7a1f12", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x15VReg, x18VReg, 0xfffffffe, false) }},
		{want: "4f024092", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x15VReg, x18VReg, 0x1, true) }},
		{want: "4f0a4092", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x15VReg, x18VReg, 0x7, true) }},
		{want: "4f0e4092", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x15VReg, x18VReg, 0xf, true) }},
		{want: "4f124092", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x15VReg, x18VReg, 0x1f, true) }},
		{want: "4f164092", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x15VReg, x18VReg, 0x3f, true) }},
		{want: "4f4e4092", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x15VReg, x18VReg, 0xfffff, true) }},
		{want: "4f7e7092", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x15VReg, x18VReg, 0xffffffff0000, true) }},
		{want: "4f7a4092", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x15VReg, x18VReg, 0x7fffffff, true) }},
		{want: "4f767f92", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x15VReg, x18VReg, 0x7ffffffe, true) }},
		{want: "4fba7f92", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpAnd, x15VReg, x18VReg, 0xfffffffffffe, true) }},
		{want: "4f020032", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x15VReg, x18VReg, 0x1, false) }},
		{want: "4f0a0032", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x15VReg, x18VReg, 0x7, false) }},
		{want: "4f0e0032", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x15VReg, x18VReg, 0xf, false) }},
		{want: "4f120032", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x15VReg, x18VReg, 0x1f, false) }},
		{want: "4f160032", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x15VReg, x18VReg, 0x3f, false) }},
		{want: "4f021132", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x15VReg, x18VReg, 0x8000, false) }},
		{want: "4f721f32", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x15VReg, x18VReg, 0x3ffffffe, false) }},
		{want: "4f7a1f32", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x15VReg, x18VReg, 0xfffffffe, false) }},
		{want: "4f0240b2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x15VReg, x18VReg, 0x1, true) }},
		{want: "4f0a40b2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x15VReg, x18VReg, 0x7, true) }},
		{want: "4f0e40b2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x15VReg, x18VReg, 0xf, true) }},
		{want: "4f1240b2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x15VReg, x18VReg, 0x1f, true) }},
		{want: "4f1640b2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x15VReg, x18VReg, 0x3f, true) }},
		{want: "4f4e40b2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x15VReg, x18VReg, 0xfffff, true) }},
		{want: "4f7e70b2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x15VReg, x18VReg, 0xffffffff0000, true) }},
		{want: "4f7a40b2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x15VReg, x18VReg, 0x7fffffff, true) }},
		{want: "4f767fb2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x15VReg, x18VReg, 0x7ffffffe, true) }},
		{want: "4fba7fb2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x15VReg, x18VReg, 0xfffffffffffe, true) }},
		{want: "4f020052", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x15VReg, x18VReg, 0x1, false) }},
		{want: "4f0a0052", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x15VReg, x18VReg, 0x7, false) }},
		{want: "4f0e0052", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x15VReg, x18VReg, 0xf, false) }},
		{want: "4f120052", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x15VReg, x18VReg, 0x1f, false) }},
		{want: "4f160052", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x15VReg, x18VReg, 0x3f, false) }},
		{want: "4f021152", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x15VReg, x18VReg, 0x8000, false) }},
		{want: "4f721f52", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x15VReg, x18VReg, 0x3ffffffe, false) }},
		{want: "4f7a1f52", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x15VReg, x18VReg, 0xfffffffe, false) }},
		{want: "4f0240d2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x15VReg, x18VReg, 0x1, true) }},
		{want: "4f0a40d2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x15VReg, x18VReg, 0x7, true) }},
		{want: "4f0e40d2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x15VReg, x18VReg, 0xf, true) }},
		{want: "4f1240d2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x15VReg, x18VReg, 0x1f, true) }},
		{want: "4f1640d2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x15VReg, x18VReg, 0x3f, true) }},
		{want: "4f4e40d2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x15VReg, x18VReg, 0xfffff, true) }},
		{want: "4f7e70d2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x15VReg, x18VReg, 0xffffffff0000, true) }},
		{want: "4f7a40d2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x15VReg, x18VReg, 0x7fffffff, true) }},
		{want: "4f767fd2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x15VReg, x18VReg, 0x7ffffffe, true) }},
		{want: "4fba7fd2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpEor, x15VReg, x18VReg, 0xfffffffffffe, true) }},
		{want: "f20300b2", setup: func(i *instruction) { i.asALUBitmaskImm(aluOpOrr, x18VReg, xzrVReg, 0x100000001, true) }},
		{want: "f21fbf0e", setup: func(i *instruction) { i.asFpuMov64(v18VReg, v31VReg) }},
		{want: "f21fbf4e", setup: func(i *instruction) { i.asFpuMov128(v18VReg, v31VReg) }},
		{want: "40a034ab", setup: func(i *instruction) {
			i.asALU(aluOpAddS, x0VReg, operandNR(x2VReg), operandER(x20VReg, extendOpSXTH, 64), false)
		}},
		{want: "4080348b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, x0VReg, operandNR(x2VReg), operandER(x20VReg, extendOpSXTB, 64), false)
		}},
		{want: "40a0348b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, x0VReg, operandNR(x2VReg), operandER(x20VReg, extendOpSXTH, 64), false)
		}},
		{want: "40c0348b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, x0VReg, operandNR(x2VReg), operandER(x20VReg, extendOpSXTW, 64), false)
		}},
		{want: "4080340b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, x0VReg, operandNR(x2VReg), operandER(x20VReg, extendOpSXTB, 32), false)
		}},
		{want: "40a0340b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, x0VReg, operandNR(x2VReg), operandER(x20VReg, extendOpSXTH, 32), false)
		}},
		{want: "40c0340b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, x0VReg, operandNR(x2VReg), operandER(x20VReg, extendOpSXTW, 32), false)
		}},
		{want: "400034eb", setup: func(i *instruction) {
			i.asALU(aluOpSubS, x0VReg, operandNR(x2VReg), operandER(x20VReg, extendOpUXTB, 64), false)
		}},
		{want: "400034cb", setup: func(i *instruction) {
			i.asALU(aluOpSub, x0VReg, operandNR(x2VReg), operandER(x20VReg, extendOpUXTB, 64), false)
		}},
		{want: "402034cb", setup: func(i *instruction) {
			i.asALU(aluOpSub, x0VReg, operandNR(x2VReg), operandER(x20VReg, extendOpUXTH, 64), false)
		}},
		{want: "404034cb", setup: func(i *instruction) {
			i.asALU(aluOpSub, x0VReg, operandNR(x2VReg), operandER(x20VReg, extendOpUXTW, 64), false)
		}},
		{want: "4000344b", setup: func(i *instruction) {
			i.asALU(aluOpSub, x0VReg, operandNR(x2VReg), operandER(x20VReg, extendOpUXTB, 32), false)
		}},
		{want: "4020344b", setup: func(i *instruction) {
			i.asALU(aluOpSub, x0VReg, operandNR(x2VReg), operandER(x20VReg, extendOpUXTH, 32), false)
		}},
		{want: "4040344b", setup: func(i *instruction) {
			i.asALU(aluOpSub, x0VReg, operandNR(x2VReg), operandER(x20VReg, extendOpUXTW, 32), false)
		}},
		{want: "4000140b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, x0VReg, operandNR(x2VReg), operandNR(x20VReg), false)
		}},
		{want: "4000148b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, x0VReg, operandNR(x2VReg), operandNR(x20VReg), true)
		}},
		{want: "40001f8b", setup: func(i *instruction) {
			i.asALU(aluOpAdd, x0VReg, operandNR(x2VReg), operandNR(xzrVReg), true)
		}},
		{want: "4000142b", setup: func(i *instruction) {
			i.asALU(aluOpAddS, x0VReg, operandNR(x2VReg), operandNR(x20VReg), false)
		}},
		{want: "400014ab", setup: func(i *instruction) {
			i.asALU(aluOpAddS, x0VReg, operandNR(x2VReg), operandNR(x20VReg), true)
		}},
		{want: "4000144b", setup: func(i *instruction) {
			i.asALU(aluOpSub, x0VReg, operandNR(x2VReg), operandNR(x20VReg), false)
		}},
		{want: "400014cb", setup: func(i *instruction) {
			i.asALU(aluOpSub, x0VReg, operandNR(x2VReg), operandNR(x20VReg), true)
		}},
		{want: "40001fcb", setup: func(i *instruction) {
			i.asALU(aluOpSub, x0VReg, operandNR(x2VReg), operandNR(xzrVReg), true)
		}},
		{want: "400014eb", setup: func(i *instruction) {
			i.asALU(aluOpSubS, x0VReg, operandNR(x2VReg), operandNR(x20VReg), true)
		}},
		{want: "40001feb", setup: func(i *instruction) {
			i.asALU(aluOpSubS, x0VReg, operandNR(x2VReg), operandNR(xzrVReg), true)
		}},
		{want: "c0035fd6", setup: func(i *instruction) { i.asRet() }},
		{want: "e303042a", setup: func(i *instruction) { i.asMove32(x3VReg, x4VReg) }},
		{want: "fe03002a", setup: func(i *instruction) { i.asMove32(x30VReg, x0VReg) }},
		{want: "e30304aa", setup: func(i *instruction) { i.asMove64(x3VReg, x4VReg) }},
		{want: "fe0300aa", setup: func(i *instruction) { i.asMove64(x30VReg, x0VReg) }},
		{want: "9f000091", setup: func(i *instruction) { i.asMove64(spVReg, x4VReg) }},
		{want: "e0030091", setup: func(i *instruction) { i.asMove64(x0VReg, spVReg) }},
		{want: "e17bc1a8", setup: func(i *instruction) {
			i.asLoadPair64(x1VReg, x30VReg, addressModePreOrPostIndex(m, spVReg, 16, false))
		}},
		{want: "e17bc1a9", setup: func(i *instruction) {
			i.asLoadPair64(x1VReg, x30VReg, addressModePreOrPostIndex(m, spVReg, 16, true))
		}},
		{want: "e17b81a8", setup: func(i *instruction) {
			i.asStorePair64(x1VReg, x30VReg, addressModePreOrPostIndex(m, spVReg, 16, false))
		}},
		{want: "e17b81a9", setup: func(i *instruction) {
			i.asStorePair64(x1VReg, x30VReg, addressModePreOrPostIndex(m, spVReg, 16, true))
		}},
		{want: "e17f81a9", setup: func(i *instruction) {
			i.asStorePair64(x1VReg, xzrVReg, addressModePreOrPostIndex(m, spVReg, 16, true))
		}},
		{want: "ff7f81a9", setup: func(i *instruction) {
			i.asStorePair64(xzrVReg, xzrVReg, addressModePreOrPostIndex(m, spVReg, 16, true))
		}},
		{want: "20000014", setup: func(i *instruction) {
			i.asBr(dummyLabel)
			i.brOffsetResolve(0x80)
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
			i.asFpuRRR(fpuBinOpAdd, v3VReg, operandNR(v4VReg), operandNR(v18VReg), false)
		}},
		{want: "8328721e", setup: func(i *instruction) {
			i.asFpuRRR(fpuBinOpAdd, v3VReg, operandNR(v4VReg), operandNR(v18VReg), true)
		}},
		{want: "8338321e", setup: func(i *instruction) {
			i.asFpuRRR(fpuBinOpSub, v3VReg, operandNR(v4VReg), operandNR(v18VReg), false)
		}},
		{want: "8338721e", setup: func(i *instruction) {
			i.asFpuRRR(fpuBinOpSub, v3VReg, operandNR(v4VReg), operandNR(v18VReg), true)
		}},
		{want: "8308321e", setup: func(i *instruction) {
			i.asFpuRRR(fpuBinOpMul, v3VReg, operandNR(v4VReg), operandNR(v18VReg), false)
		}},
		{want: "8308721e", setup: func(i *instruction) {
			i.asFpuRRR(fpuBinOpMul, v3VReg, operandNR(v4VReg), operandNR(v18VReg), true)
		}},
		{want: "8318321e", setup: func(i *instruction) {
			i.asFpuRRR(fpuBinOpDiv, v3VReg, operandNR(v4VReg), operandNR(v18VReg), false)
		}},
		{want: "8318721e", setup: func(i *instruction) {
			i.asFpuRRR(fpuBinOpDiv, v3VReg, operandNR(v4VReg), operandNR(v18VReg), true)
		}},
		{want: "8348321e", setup: func(i *instruction) {
			i.asFpuRRR(fpuBinOpMax, v3VReg, operandNR(v4VReg), operandNR(v18VReg), false)
		}},
		{want: "8348721e", setup: func(i *instruction) {
			i.asFpuRRR(fpuBinOpMax, v3VReg, operandNR(v4VReg), operandNR(v18VReg), true)
		}},
		{want: "8358321e", setup: func(i *instruction) {
			i.asFpuRRR(fpuBinOpMin, v3VReg, operandNR(v4VReg), operandNR(v18VReg), false)
		}},
		{want: "8358721e", setup: func(i *instruction) {
			i.asFpuRRR(fpuBinOpMin, v3VReg, operandNR(v4VReg), operandNR(v18VReg), true)
		}},
		{want: "49fd7f11", setup: func(i *instruction) {
			i.asALU(aluOpAdd, x9VReg, operandNR(x10VReg), operandImm12(0b111111111111, 0b1), false)
		}},
		{want: "e9ff7f91", setup: func(i *instruction) {
			i.asALU(aluOpAdd, x9VReg, operandNR(spVReg), operandImm12(0b111111111111, 0b1), true)
		}},
		{want: "49fd3f11", setup: func(i *instruction) {
			i.asALU(aluOpAdd, x9VReg, operandNR(x10VReg), operandImm12(0b111111111111, 0b0), false)
		}},
		{want: "5ffd3f91", setup: func(i *instruction) {
			i.asALU(aluOpAdd, spVReg, operandNR(x10VReg), operandImm12(0b111111111111, 0b0), true)
		}},
		{want: "49fd7f31", setup: func(i *instruction) {
			i.asALU(aluOpAddS, x9VReg, operandNR(x10VReg), operandImm12(0b111111111111, 0b1), false)
		}},
		{want: "49fd7fb1", setup: func(i *instruction) {
			i.asALU(aluOpAddS, x9VReg, operandNR(x10VReg), operandImm12(0b111111111111, 0b1), true)
		}},
		{want: "49fd3f31", setup: func(i *instruction) {
			i.asALU(aluOpAddS, x9VReg, operandNR(x10VReg), operandImm12(0b111111111111, 0b0), false)
		}},
		{want: "49fd3fb1", setup: func(i *instruction) {
			i.asALU(aluOpAddS, x9VReg, operandNR(x10VReg), operandImm12(0b111111111111, 0b0), true)
		}},
		{want: "49fd7f51", setup: func(i *instruction) {
			i.asALU(aluOpSub, x9VReg, operandNR(x10VReg), operandImm12(0b111111111111, 0b1), false)
		}},
		{want: "e9ff7fd1", setup: func(i *instruction) {
			i.asALU(aluOpSub, x9VReg, operandNR(spVReg), operandImm12(0b111111111111, 0b1), true)
		}},
		{want: "49fd3f51", setup: func(i *instruction) {
			i.asALU(aluOpSub, x9VReg, operandNR(x10VReg), operandImm12(0b111111111111, 0b0), false)
		}},
		{want: "5ffd3fd1", setup: func(i *instruction) {
			i.asALU(aluOpSub, spVReg, operandNR(x10VReg), operandImm12(0b111111111111, 0b0), true)
		}},
		{want: "49fd7f71", setup: func(i *instruction) {
			i.asALU(aluOpSubS, x9VReg, operandNR(x10VReg), operandImm12(0b111111111111, 0b1), false)
		}},
		{want: "49fd7ff1", setup: func(i *instruction) {
			i.asALU(aluOpSubS, x9VReg, operandNR(x10VReg), operandImm12(0b111111111111, 0b1), true)
		}},
		{want: "49fd3f71", setup: func(i *instruction) {
			i.asALU(aluOpSubS, x9VReg, operandNR(x10VReg), operandImm12(0b111111111111, 0b0), false)
		}},
		{want: "49fd3ff1", setup: func(i *instruction) {
			i.asALU(aluOpSubS, x9VReg, operandNR(x10VReg), operandImm12(0b111111111111, 0b0), true)
		}},
		{want: "4020d41a", setup: func(i *instruction) {
			i.asALU(aluOpLsl, x0VReg, operandNR(x2VReg), operandNR(x20VReg), false)
		}},
		{want: "4020d49a", setup: func(i *instruction) {
			i.asALU(aluOpLsl, x0VReg, operandNR(x2VReg), operandNR(x20VReg), true)
		}},
		{want: "4024d41a", setup: func(i *instruction) {
			i.asALU(aluOpLsr, x0VReg, operandNR(x2VReg), operandNR(x20VReg), false)
		}},
		{want: "4024d49a", setup: func(i *instruction) {
			i.asALU(aluOpLsr, x0VReg, operandNR(x2VReg), operandNR(x20VReg), true)
		}},
		{want: "4028d41a", setup: func(i *instruction) {
			i.asALU(aluOpAsr, x0VReg, operandNR(x2VReg), operandNR(x20VReg), false)
		}},
		{want: "4028d49a", setup: func(i *instruction) {
			i.asALU(aluOpAsr, x0VReg, operandNR(x2VReg), operandNR(x20VReg), true)
		}},
		{want: "400cd49a", setup: func(i *instruction) {
			i.asALU(aluOpSDiv, x0VReg, operandNR(x2VReg), operandNR(x20VReg), true)
		}},
		{want: "400cd41a", setup: func(i *instruction) {
			i.asALU(aluOpSDiv, x0VReg, operandNR(x2VReg), operandNR(x20VReg), false)
		}},
		{want: "4008d49a", setup: func(i *instruction) {
			i.asALU(aluOpUDiv, x0VReg, operandNR(x2VReg), operandNR(x20VReg), true)
		}},
		{want: "4008d41a", setup: func(i *instruction) {
			i.asALU(aluOpUDiv, x0VReg, operandNR(x2VReg), operandNR(x20VReg), false)
		}},
		{want: "407c0013", setup: func(i *instruction) {
			i.asALUShift(aluOpAsr, x0VReg, operandNR(x2VReg), operandShiftImm(0), false)
		}},
		{want: "40fc4093", setup: func(i *instruction) {
			i.asALUShift(aluOpAsr, x0VReg, operandNR(x2VReg), operandShiftImm(0), true)
		}},
		{want: "407c0113", setup: func(i *instruction) {
			i.asALUShift(aluOpAsr, x0VReg, operandNR(x2VReg), operandShiftImm(1), false)
		}},
		{want: "407c1f13", setup: func(i *instruction) {
			i.asALUShift(aluOpAsr, x0VReg, operandNR(x2VReg), operandShiftImm(31), false)
		}},
		{want: "40fc4193", setup: func(i *instruction) {
			i.asALUShift(aluOpAsr, x0VReg, operandNR(x2VReg), operandShiftImm(1), true)
		}},
		{want: "40fc5f93", setup: func(i *instruction) {
			i.asALUShift(aluOpAsr, x0VReg, operandNR(x2VReg), operandShiftImm(31), true)
		}},
		{want: "40fc7f93", setup: func(i *instruction) {
			i.asALUShift(aluOpAsr, x0VReg, operandNR(x2VReg), operandShiftImm(63), true)
		}},
		{want: "407c0153", setup: func(i *instruction) {
			i.asALUShift(aluOpLsr, x0VReg, operandNR(x2VReg), operandShiftImm(1), false)
		}},
		{want: "407c1f53", setup: func(i *instruction) {
			i.asALUShift(aluOpLsr, x0VReg, operandNR(x2VReg), operandShiftImm(31), false)
		}},
		{want: "40fc41d3", setup: func(i *instruction) {
			i.asALUShift(aluOpLsr, x0VReg, operandNR(x2VReg), operandShiftImm(1), true)
		}},
		{want: "40fc5fd3", setup: func(i *instruction) {
			i.asALUShift(aluOpLsr, x0VReg, operandNR(x2VReg), operandShiftImm(31), true)
		}},
		{want: "40fc7fd3", setup: func(i *instruction) {
			i.asALUShift(aluOpLsr, x0VReg, operandNR(x2VReg), operandShiftImm(63), true)
		}},
		{want: "407c0053", setup: func(i *instruction) {
			i.asALUShift(aluOpLsl, x0VReg, operandNR(x2VReg), operandShiftImm(0), false)
		}},
		{want: "40fc40d3", setup: func(i *instruction) {
			i.asALUShift(aluOpLsl, x0VReg, operandNR(x2VReg), operandShiftImm(0), true)
		}},
		{want: "40781f53", setup: func(i *instruction) {
			i.asALUShift(aluOpLsl, x0VReg, operandNR(x2VReg), operandShiftImm(1), false)
		}},
		{want: "40000153", setup: func(i *instruction) {
			i.asALUShift(aluOpLsl, x0VReg, operandNR(x2VReg), operandShiftImm(31), false)
		}},
		{want: "40f87fd3", setup: func(i *instruction) {
			i.asALUShift(aluOpLsl, x0VReg, operandNR(x2VReg), operandShiftImm(1), true)
		}},
		{want: "408061d3", setup: func(i *instruction) {
			i.asALUShift(aluOpLsl, x0VReg, operandNR(x2VReg), operandShiftImm(31), true)
		}},
		{want: "400041d3", setup: func(i *instruction) {
			i.asALUShift(aluOpLsl, x0VReg, operandNR(x2VReg), operandShiftImm(63), true)
		}},
		{want: "4000c05a", setup: func(i *instruction) { i.asBitRR(bitOpRbit, x0VReg, x2VReg, false) }},
		{want: "4000c0da", setup: func(i *instruction) { i.asBitRR(bitOpRbit, x0VReg, x2VReg, true) }},
		{want: "4010c05a", setup: func(i *instruction) { i.asBitRR(bitOpClz, x0VReg, x2VReg, false) }},
		{want: "4010c0da", setup: func(i *instruction) { i.asBitRR(bitOpClz, x0VReg, x2VReg, true) }},
		{want: "4138302e", setup: func(i *instruction) {
			i.asVecLanes(vecOpUaddlv, v1VReg, operandNR(v2VReg), vecArrangement8B)
		}},
		{want: "4138306e", setup: func(i *instruction) {
			i.asVecLanes(vecOpUaddlv, v1VReg, operandNR(v2VReg), vecArrangement16B)
		}},
		{want: "4138702e", setup: func(i *instruction) {
			i.asVecLanes(vecOpUaddlv, v1VReg, operandNR(v2VReg), vecArrangement4H)
		}},
		{want: "4138706e", setup: func(i *instruction) {
			i.asVecLanes(vecOpUaddlv, v1VReg, operandNR(v2VReg), vecArrangement8H)
		}},
		{want: "4138b06e", setup: func(i *instruction) {
			i.asVecLanes(vecOpUaddlv, v1VReg, operandNR(v2VReg), vecArrangement4S)
		}},
		{want: "41c0230e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSmull, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "41c0630e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSmull, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "41c0a30e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSmull, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "41c0234e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSmull2, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "41c0634e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSmull2, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "41c0a34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSmull2, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4S)
		}},
		{want: "411c630e", setup: func(i *instruction) {
			i.asVecRRR(vecOpBic, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "411c634e", setup: func(i *instruction) {
			i.asVecRRR(vecOpBic, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "411c632e", setup: func(i *instruction) {
			i.asVecRRRRewrite(vecOpBsl, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "411c636e", setup: func(i *instruction) {
			i.asVecRRRRewrite(vecOpBsl, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "4158202e", setup: func(i *instruction) {
			i.asVecMisc(vecOpNot, v1VReg, operandNR(v2VReg), vecArrangement8B)
		}},
		{want: "4158206e", setup: func(i *instruction) {
			i.asVecMisc(vecOpNot, v1VReg, operandNR(v2VReg), vecArrangement16B)
		}},
		{want: "411c230e", setup: func(i *instruction) {
			i.asVecRRR(vecOpAnd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "411c234e", setup: func(i *instruction) {
			i.asVecRRR(vecOpAnd, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "411ca30e", setup: func(i *instruction) {
			i.asVecRRR(vecOpOrr, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "411ca34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpOrr, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "4144230e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSshl, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "4144234e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSshl, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "4144630e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSshl, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "4144634e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSshl, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "4144a30e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSshl, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "4144e34e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSshl, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "4144a30e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSshl, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "4144232e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUshl, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8B)
		}},
		{want: "4144236e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUshl, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement16B)
		}},
		{want: "4144632e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUshl, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement4H)
		}},
		{want: "4144636e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUshl, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement8H)
		}},
		{want: "4144a32e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUshl, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "4144e36e", setup: func(i *instruction) {
			i.asVecRRR(vecOpUshl, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2D)
		}},
		{want: "4144a30e", setup: func(i *instruction) {
			i.asVecRRR(vecOpSshl, v1VReg, operandNR(v2VReg), operandNR(v3VReg), vecArrangement2S)
		}},
		{want: "4158200e", setup: func(i *instruction) { i.asVecMisc(vecOpCnt, v1VReg, operandNR(v2VReg), vecArrangement8B) }},
		{want: "4158204e", setup: func(i *instruction) { i.asVecMisc(vecOpCnt, v1VReg, operandNR(v2VReg), vecArrangement16B) }},
		{want: "41c0221e", setup: func(i *instruction) {
			i.asFpuRR(fpuUniOpCvt32To64, v1VReg, operandNR(v2VReg), true)
		}},
		{want: "4140621e", setup: func(i *instruction) {
			i.asFpuRR(fpuUniOpCvt64To32, v1VReg, operandNR(v2VReg), true)
		}},
		{want: "4140211e", setup: func(i *instruction) {
			i.asFpuRR(fpuUniOpNeg, v1VReg, operandNR(v2VReg), false)
		}},

		{want: "41c0211e", setup: func(i *instruction) {
			i.asFpuRR(fpuUniOpSqrt, v1VReg, operandNR(v2VReg), false)
		}},
		{want: "41c0611e", setup: func(i *instruction) {
			i.asFpuRR(fpuUniOpSqrt, v1VReg, operandNR(v2VReg), true)
		}},
		{want: "41c0241e", setup: func(i *instruction) {
			i.asFpuRR(fpuUniOpRoundPlus, v1VReg, operandNR(v2VReg), false)
		}},
		{want: "41c0641e", setup: func(i *instruction) {
			i.asFpuRR(fpuUniOpRoundPlus, v1VReg, operandNR(v2VReg), true)
		}},
		{want: "4140251e", setup: func(i *instruction) {
			i.asFpuRR(fpuUniOpRoundMinus, v1VReg, operandNR(v2VReg), false)
		}},
		{want: "4140651e", setup: func(i *instruction) {
			i.asFpuRR(fpuUniOpRoundMinus, v1VReg, operandNR(v2VReg), true)
		}},
		{want: "41c0251e", setup: func(i *instruction) {
			i.asFpuRR(fpuUniOpRoundZero, v1VReg, operandNR(v2VReg), false)
		}},
		{want: "41c0651e", setup: func(i *instruction) {
			i.asFpuRR(fpuUniOpRoundZero, v1VReg, operandNR(v2VReg), true)
		}},
		{want: "4140241e", setup: func(i *instruction) {
			i.asFpuRR(fpuUniOpRoundNearest, v1VReg, operandNR(v2VReg), false)
		}},
		{want: "4140641e", setup: func(i *instruction) {
			i.asFpuRR(fpuUniOpRoundNearest, v1VReg, operandNR(v2VReg), true)
		}},
		{want: "4140611e", setup: func(i *instruction) { i.asFpuRR(fpuUniOpNeg, v1VReg, operandNR(v2VReg), true) }},
		{want: "41c0404d", setup: func(i *instruction) { i.asVecLoad1R(v1VReg, operandNR(x2VReg), vecArrangement16B) }},
		{want: "41c4404d", setup: func(i *instruction) { i.asVecLoad1R(v1VReg, operandNR(x2VReg), vecArrangement8H) }},
		{want: "41c8404d", setup: func(i *instruction) { i.asVecLoad1R(v1VReg, operandNR(x2VReg), vecArrangement4S) }},
		{want: "41cc404d", setup: func(i *instruction) { i.asVecLoad1R(v1VReg, operandNR(x2VReg), vecArrangement2D) }},
		{want: "0200e1b8", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpAdd, x0VReg, x1VReg, x2VReg, 4)
		}},
		{want: "0200e178", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpAdd, x0VReg, x1VReg, x2VReg, 2)
		}},
		{want: "0200e138", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpAdd, x0VReg, x1VReg, x2VReg, 1)
		}},
		{want: "0200e1f8", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpAdd, x0VReg, x1VReg, x2VReg, 8)
		}},
		{want: "0200e1b8", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpAdd, x0VReg, x1VReg, x2VReg, 4)
		}},
		{want: "0200e178", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpAdd, x0VReg, x1VReg, x2VReg, 2)
		}},
		{want: "0200e138", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpAdd, x0VReg, x1VReg, x2VReg, 1)
		}},
		{want: "0210e1b8", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpClr, x0VReg, x1VReg, x2VReg, 4)
		}},
		{want: "0210e178", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpClr, x0VReg, x1VReg, x2VReg, 2)
		}},
		{want: "0210e138", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpClr, x0VReg, x1VReg, x2VReg, 1)
		}},
		{want: "0210e1f8", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpClr, x0VReg, x1VReg, x2VReg, 8)
		}},
		{want: "0210e1b8", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpClr, x0VReg, x1VReg, x2VReg, 4)
		}},
		{want: "0210e178", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpClr, x0VReg, x1VReg, x2VReg, 2)
		}},
		{want: "0210e138", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpClr, x0VReg, x1VReg, x2VReg, 1)
		}},
		{want: "0230e1b8", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpSet, x0VReg, x1VReg, x2VReg, 4)
		}},
		{want: "0230e178", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpSet, x0VReg, x1VReg, x2VReg, 2)
		}},
		{want: "0230e138", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpSet, x0VReg, x1VReg, x2VReg, 1)
		}},
		{want: "0230e1f8", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpSet, x0VReg, x1VReg, x2VReg, 8)
		}},
		{want: "0230e1b8", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpSet, x0VReg, x1VReg, x2VReg, 4)
		}},
		{want: "0230e178", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpSet, x0VReg, x1VReg, x2VReg, 2)
		}},
		{want: "0230e138", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpSet, x0VReg, x1VReg, x2VReg, 1)
		}},
		{want: "0220e1b8", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpEor, x0VReg, x1VReg, x2VReg, 4)
		}},
		{want: "0220e178", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpEor, x0VReg, x1VReg, x2VReg, 2)
		}},
		{want: "0220e138", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpEor, x0VReg, x1VReg, x2VReg, 1)
		}},
		{want: "0220e1f8", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpEor, x0VReg, x1VReg, x2VReg, 8)
		}},
		{want: "0220e1b8", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpEor, x0VReg, x1VReg, x2VReg, 4)
		}},
		{want: "0220e178", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpEor, x0VReg, x1VReg, x2VReg, 2)
		}},
		{want: "0220e138", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpEor, x0VReg, x1VReg, x2VReg, 1)
		}},
		{want: "0280e1b8", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpSwp, x0VReg, x1VReg, x2VReg, 4)
		}},
		{want: "0280e178", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpSwp, x0VReg, x1VReg, x2VReg, 2)
		}},
		{want: "0280e138", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpSwp, x0VReg, x1VReg, x2VReg, 1)
		}},
		{want: "0280e1f8", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpSwp, x0VReg, x1VReg, x2VReg, 8)
		}},
		{want: "0280e1b8", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpSwp, x0VReg, x1VReg, x2VReg, 4)
		}},
		{want: "0280e178", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpSwp, x0VReg, x1VReg, x2VReg, 2)
		}},
		{want: "0280e138", setup: func(i *instruction) {
			i.asAtomicRmw(atomicRmwOpSwp, x0VReg, x1VReg, x2VReg, 1)
		}},
		{want: "02fce188", setup: func(i *instruction) {
			i.asAtomicCas(x0VReg, x1VReg, x2VReg, 4)
		}},
		{want: "02fce148", setup: func(i *instruction) {
			i.asAtomicCas(x0VReg, x1VReg, x2VReg, 2)
		}},
		{want: "02fce108", setup: func(i *instruction) {
			i.asAtomicCas(x0VReg, x1VReg, x2VReg, 1)
		}},
		{want: "02fce1c8", setup: func(i *instruction) {
			i.asAtomicCas(x0VReg, x1VReg, x2VReg, 8)
		}},
		{want: "02fce188", setup: func(i *instruction) {
			i.asAtomicCas(x0VReg, x1VReg, x2VReg, 4)
		}},
		{want: "02fce148", setup: func(i *instruction) {
			i.asAtomicCas(x0VReg, x1VReg, x2VReg, 2)
		}},
		{want: "02fce108", setup: func(i *instruction) {
			i.asAtomicCas(x0VReg, x1VReg, x2VReg, 1)
		}},
		{want: "01fcdf88", setup: func(i *instruction) {
			i.asAtomicLoad(x0VReg, x1VReg, 4)
		}},
		{want: "01fcdf48", setup: func(i *instruction) {
			i.asAtomicLoad(x0VReg, x1VReg, 2)
		}},
		{want: "01fcdf08", setup: func(i *instruction) {
			i.asAtomicLoad(x0VReg, x1VReg, 1)
		}},
		{want: "01fcdfc8", setup: func(i *instruction) {
			i.asAtomicLoad(x0VReg, x1VReg, 8)
		}},
		{want: "01fcdf88", setup: func(i *instruction) {
			i.asAtomicLoad(x0VReg, x1VReg, 4)
		}},
		{want: "01fcdf48", setup: func(i *instruction) {
			i.asAtomicLoad(x0VReg, x1VReg, 2)
		}},
		{want: "01fcdf08", setup: func(i *instruction) {
			i.asAtomicLoad(x0VReg, x1VReg, 1)
		}},
		{want: "01fc9f88", setup: func(i *instruction) {
			i.asAtomicStore(operandNR(x0VReg), operandNR(x1VReg), 4)
		}},
		{want: "01fc9f48", setup: func(i *instruction) {
			i.asAtomicStore(operandNR(x0VReg), operandNR(x1VReg), 2)
		}},
		{want: "01fc9f08", setup: func(i *instruction) {
			i.asAtomicStore(operandNR(x0VReg), operandNR(x1VReg), 1)
		}},
		{want: "01fc9fc8", setup: func(i *instruction) {
			i.asAtomicStore(operandNR(x0VReg), operandNR(x1VReg), 8)
		}},
		{want: "01fc9f88", setup: func(i *instruction) {
			i.asAtomicStore(operandNR(x0VReg), operandNR(x1VReg), 4)
		}},
		{want: "01fc9f48", setup: func(i *instruction) {
			i.asAtomicStore(operandNR(x0VReg), operandNR(x1VReg), 2)
		}},
		{want: "01fc9f08", setup: func(i *instruction) {
			i.asAtomicStore(operandNR(x0VReg), operandNR(x1VReg), 1)
		}},
		{want: "bf3b03d5", setup: func(i *instruction) {
			i.asDMB()
		}},
		{want: "4201231e4201631e4201239e4201639e4201221e4201621e4201229e4201629e", setup: func(i *instruction) {
			i.asNop0()
			cur := i
			trueFalse := []bool{false, true}
			for _, rnSigned := range trueFalse {
				for _, src64bit := range trueFalse {
					for _, dst64bit := range trueFalse {
						i := &instruction{prev: cur}
						cur.next = i
						i.asIntToFpu(v2VReg, operandNR(x10VReg), rnSigned, src64bit, dst64bit)
						cur = i
					}
				}
			}
		}},
		{want: "4201391e4201399e4201791e4201799e4201381e4201389e4201781e4201789e", setup: func(i *instruction) {
			i.asNop0()
			cur := i
			trueFalse := []bool{false, true}
			for _, rnSigned := range trueFalse {
				for _, src64bit := range trueFalse {
					for _, dst64bit := range trueFalse {
						i := &instruction{prev: cur}
						cur.next = i
						i.asFpuToInt(v2VReg, operandNR(x10VReg), rnSigned, src64bit, dst64bit)
						cur = i
					}
				}
			}
		}},
	} {
		tc := tc
		t.Run(tc.want, func(t *testing.T) {
			i := &instruction{}
			tc.setup(i)

			mc := &mockCompiler{}
			m := &machine{compiler: mc}
			m.encode(i)
			// Note: for quick iteration we can use golang.org/x/arch package to verify the encoding.
			// 	but wazero doesn't add even a test dependency to it, so commented out.
			// inst, err := arm64asm.Decode(m.buf)
			// require.NoError(t, err, hex.EncodeToString(m.buf))
			// fmt.Println(inst.String())
			require.Equal(t, tc.want, hex.EncodeToString(mc.buf))

			var actualSize int
			for cur := i; cur != nil; cur = cur.next {
				actualSize += int(cur.size())
			}
			require.Equal(t, len(tc.want)/2, actualSize)
		})
	}
}

func TestInstruction_encode_call(t *testing.T) {
	_, _, m := newSetupWithMockContext()
	mock := m.compiler.(*mockCompiler)
	mock.buf = make([]byte, 128)
	i := &instruction{}
	i.asCall(ssa.FuncRef(555), nil)
	i.encode(m)
	buf := mock.buf[128:]
	require.Equal(t, "00000094", hex.EncodeToString(buf))
	require.Equal(t, 1, len(mock.relocs))
	require.Equal(t, ssa.FuncRef(555), mock.relocs[0].FuncRef)
	require.Equal(t, int64(128), mock.relocs[0].Offset)
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
		_, _, m := newSetupWithMockContext()
		i.encode(m)
		// Note: for quick iteration we can use golang.org/x/arch package to verify the encoding.
		// 	but wazero doesn't add even a test dependency to it, so commented out.
		// inst, err := arm64asm.Decode(m.buf)
		// require.NoError(t, err)
		// fmt.Println(inst.String())
		require.Equal(t, tc.want, hex.EncodeToString(m.compiler.Buf()))
	}
}

func TestInstruction_encoding_store_encoding(t *testing.T) {
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
		{k: store64, amode: amodePreIndex2, rn: xzrVReg, want: "ff0f10f8"},
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
				i = &instruction{kind: tc.k, rn: operandNR(tc.rn)}
			case uLoad8, uLoad16, uLoad32, uLoad64, sLoad8, sLoad16, sLoad32, fpuLoad32, fpuLoad64, fpuLoad128:
				i = &instruction{kind: tc.k, rd: tc.rn}
			default:
				t.Fatalf("unknown kind: %v", tc.k)
			}
			i.setAmode(&tc.amode)
			_, _, m := newSetupWithMockContext()
			i.encode(m)
			// Note: for quick iteration we can use golang.org/x/arch package to verify the encoding.
			// 	but wazero doesn't add even a test dependency to it, so commented out.
			// inst, err := arm64asm.Decode(m.buf)
			// require.NoError(t, err)
			// fmt.Println(inst.String())
			require.Equal(t, tc.want, hex.EncodeToString(m.compiler.Buf()))
		})
	}
}

func Test_encodeExitSequence(t *testing.T) {
	t.Run("no overlap", func(t *testing.T) {
		m := &mockCompiler{}
		encodeExitSequence(m, x22VReg)
		// ldr x29, [x22, #0x10]
		// ldr x30, [x22, #0x20]
		// ldr x27, [x22, #0x18]
		// mov sp, x27
		// ret
		// b   #0x14 ;; dummy
		require.Equal(t, "dd0a40f9de1240f9db0e40f97f030091c0035fd600000014", hex.EncodeToString(m.buf))
		require.Equal(t, len(m.buf), exitSequenceSize)
	})
	t.Run("fp", func(t *testing.T) {
		m := &mockCompiler{}
		encodeExitSequence(m, fpVReg)
		// mov x27, x29
		// ldr x29, [x27, #0x10]
		// ldr x30, [x27, #0x20]
		// ldr x27, [x27, #0x18]
		// mov sp, x27
		// ret
		require.Equal(t, "fb031daa7d0b40f97e1340f97b0f40f97f030091c0035fd6", hex.EncodeToString(m.buf))
		require.Equal(t, len(m.buf), exitSequenceSize)
	})
	t.Run("lr", func(t *testing.T) {
		m := &mockCompiler{}
		encodeExitSequence(m, lrVReg)
		// mov x27, x30
		// ldr x29, [x27, #0x10]
		// ldr x30, [x27, #0x20]
		// ldr x27, [x27, #0x18]
		// mov sp, x27
		// ret
		require.Equal(t, "fb031eaa7d0b40f97e1340f97b0f40f97f030091c0035fd6", hex.EncodeToString(m.buf))
		require.Equal(t, len(m.buf), exitSequenceSize)
	})
}

func Test_encodeBrTableSequence(t *testing.T) {
	_, _, m := newSetupWithMockContext()
	i := &instruction{}
	const tableIndex, tableSize = 5, 10
	i.asBrTableSequence(x22VReg, tableIndex, tableSize)
	m.jmpTableTargets = [][]uint32{{}, {}, {}, {}, {}, {1, 2, 3, 4, 5, 6, 7, 8, 9, 10}}
	i.encode(m)
	encoded := m.compiler.Buf()
	require.Equal(t, i.size(), int64(len(encoded)))
	require.Equal(t, "9b000010765bb6b87b03168b60031fd6", hex.EncodeToString(encoded[:brTableSequenceOffsetTableBegin]))
	require.Equal(t, "0100000002000000030000000400000005000000060000000700000008000000090000000a000000", hex.EncodeToString(encoded[brTableSequenceOffsetTableBegin:]))
}

func Test_encodeUnconditionalBranch(t *testing.T) {
	buf := make([]byte, 4)

	actual := encodeUnconditionalBranch(true, 4)
	binary.LittleEndian.PutUint32(buf, actual)
	require.Equal(t, "0x01000094", fmt.Sprintf("%#x", buf))

	actual = encodeUnconditionalBranch(false, 4*1024)
	binary.LittleEndian.PutUint32(buf, actual)
	require.Equal(t, "0x00040014", fmt.Sprintf("%#x", buf))
}
