package wasm

import (
	"math"
	"math/bits"
)

// fixme: there seems to be virtually nop instructions

func i32eqz(vm *VirtualMachine) {
	vm.OperandStack.PushBool(vm.OperandStack.Pop() == 0)
}

func i32eq(vm *VirtualMachine) {
	vm.OperandStack.PushBool(vm.OperandStack.Pop() == vm.OperandStack.Pop())
}

func i32ne(vm *VirtualMachine) {
	vm.OperandStack.PushBool(vm.OperandStack.Pop() != vm.OperandStack.Pop())
}

func i32lts(vm *VirtualMachine) {
	v2 := vm.OperandStack.Pop()
	v1 := vm.OperandStack.Pop()
	vm.OperandStack.PushBool(int32(v1) < int32(v2))
}

func i32ltu(vm *VirtualMachine) {
	v2 := vm.OperandStack.Pop()
	v1 := vm.OperandStack.Pop()
	vm.OperandStack.PushBool(uint32(v1) < uint32(v2))
}

func i32gts(vm *VirtualMachine) {
	v2 := vm.OperandStack.Pop()
	v1 := vm.OperandStack.Pop()
	vm.OperandStack.PushBool(int32(v1) > int32(v2))
}

func i32gtu(vm *VirtualMachine) {
	v2 := vm.OperandStack.Pop()
	v1 := vm.OperandStack.Pop()
	vm.OperandStack.PushBool(uint32(v1) > uint32(v2))
}

func i32les(vm *VirtualMachine) {
	v2 := vm.OperandStack.Pop()
	v1 := vm.OperandStack.Pop()
	vm.OperandStack.PushBool(int32(v1) <= int32(v2))
}

func i32leu(vm *VirtualMachine) {
	v2 := vm.OperandStack.Pop()
	v1 := vm.OperandStack.Pop()
	vm.OperandStack.PushBool(uint32(v1) <= uint32(v2))
}

func i32ges(vm *VirtualMachine) {
	v2 := vm.OperandStack.Pop()
	v1 := vm.OperandStack.Pop()
	vm.OperandStack.PushBool(int32(v1) >= int32(v2))
}

func i32geu(vm *VirtualMachine) {
	v2 := vm.OperandStack.Pop()
	v1 := vm.OperandStack.Pop()
	vm.OperandStack.PushBool(uint32(v1) >= uint32(v2))
}

func i64eqz(vm *VirtualMachine) {
	vm.OperandStack.PushBool(vm.OperandStack.Pop() == 0)
}

func i64eq(vm *VirtualMachine) {
	vm.OperandStack.PushBool(vm.OperandStack.Pop() == vm.OperandStack.Pop())
}

func i64ne(vm *VirtualMachine) {
	vm.OperandStack.PushBool(vm.OperandStack.Pop() != vm.OperandStack.Pop())
}

func i64lts(vm *VirtualMachine) {
	v2 := vm.OperandStack.Pop()
	v1 := vm.OperandStack.Pop()
	vm.OperandStack.PushBool(int64(v1) < int64(v2))
}

func i64ltu(vm *VirtualMachine) {
	v2 := vm.OperandStack.Pop()
	v1 := vm.OperandStack.Pop()
	vm.OperandStack.PushBool(v1 < v2)
}

func i64gts(vm *VirtualMachine) {
	v2 := vm.OperandStack.Pop()
	v1 := vm.OperandStack.Pop()
	vm.OperandStack.PushBool(int64(v1) > int64(v2))
}

func i64gtu(vm *VirtualMachine) {
	v2 := vm.OperandStack.Pop()
	v1 := vm.OperandStack.Pop()
	vm.OperandStack.PushBool(v1 < v2)
}

func i64les(vm *VirtualMachine) {
	v2 := vm.OperandStack.Pop()
	v1 := vm.OperandStack.Pop()
	vm.OperandStack.PushBool(int64(v1) <= int64(v2))
}

func i64leu(vm *VirtualMachine) {
	v2 := vm.OperandStack.Pop()
	v1 := vm.OperandStack.Pop()
	vm.OperandStack.PushBool(v1 <= v2)
}

func i64ges(vm *VirtualMachine) {
	v2 := vm.OperandStack.Pop()
	v1 := vm.OperandStack.Pop()
	vm.OperandStack.PushBool(int64(v1) >= int64(v2))
}

func i64geu(vm *VirtualMachine) {
	v2 := vm.OperandStack.Pop()
	v1 := vm.OperandStack.Pop()
	vm.OperandStack.PushBool(v1 >= v2)
}

func f32eq(vm *VirtualMachine) {
	f2 := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	f1 := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	vm.OperandStack.PushBool(f1 == f2)
}
func f32ne(vm *VirtualMachine) {
	f2 := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	f1 := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	vm.OperandStack.PushBool(f1 != f2)
}

func f32lt(vm *VirtualMachine) {
	f2 := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	f1 := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	vm.OperandStack.PushBool(f1 < f2)
}

func f32gt(vm *VirtualMachine) {
	f1 := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	f2 := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	vm.OperandStack.PushBool(f1 > f2)
}

func f32le(vm *VirtualMachine) {
	f2 := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	f1 := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	vm.OperandStack.PushBool(f1 <= f2)
}

func f32ge(vm *VirtualMachine) {
	f2 := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	f1 := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	vm.OperandStack.PushBool(f1 >= f2)
}

func f64eq(vm *VirtualMachine) {
	f2 := math.Float64frombits(vm.OperandStack.Pop())
	f1 := math.Float64frombits(vm.OperandStack.Pop())
	vm.OperandStack.PushBool(f1 == f2)
}
func f64ne(vm *VirtualMachine) {
	f2 := math.Float64frombits(vm.OperandStack.Pop())
	f1 := math.Float64frombits(vm.OperandStack.Pop())
	vm.OperandStack.PushBool(f1 != f2)
}

func f64lt(vm *VirtualMachine) {
	f2 := math.Float64frombits(vm.OperandStack.Pop())
	f1 := math.Float64frombits(vm.OperandStack.Pop())
	vm.OperandStack.PushBool(f1 < f2)
}

func f64gt(vm *VirtualMachine) {
	f2 := math.Float64frombits(vm.OperandStack.Pop())
	f1 := math.Float64frombits(vm.OperandStack.Pop())
	vm.OperandStack.PushBool(f1 > f2)
}

func f64le(vm *VirtualMachine) {
	f2 := math.Float64frombits(vm.OperandStack.Pop())
	f1 := math.Float64frombits(vm.OperandStack.Pop())
	vm.OperandStack.PushBool(f1 <= f2)
}

func f64ge(vm *VirtualMachine) {
	f2 := math.Float64frombits(vm.OperandStack.Pop())
	f1 := math.Float64frombits(vm.OperandStack.Pop())
	vm.OperandStack.PushBool(f1 >= f2)
}

func i32clz(vm *VirtualMachine) {
	vm.OperandStack.Push(uint64(bits.LeadingZeros32(uint32(vm.OperandStack.Pop()))))
}

func i32ctz(vm *VirtualMachine) {
	vm.OperandStack.Push(uint64(bits.TrailingZeros32(uint32(vm.OperandStack.Pop()))))
}

func i32popcnt(vm *VirtualMachine) {
	vm.OperandStack.Push(uint64(bits.OnesCount32(uint32(vm.OperandStack.Pop()))))
}

func i32add(vm *VirtualMachine) {
	vm.OperandStack.Push(uint64(uint32(vm.OperandStack.Pop()) + uint32(vm.OperandStack.Pop())))
}

func i32sub(vm *VirtualMachine) {
	v2 := uint32(vm.OperandStack.Pop())
	v1 := uint32(vm.OperandStack.Pop())
	vm.OperandStack.Push(uint64(v1 - v2))
}

func i32mul(vm *VirtualMachine) {
	vm.OperandStack.Push(uint64(uint32(vm.OperandStack.Pop()) * uint32(vm.OperandStack.Pop())))
}

func i32divs(vm *VirtualMachine) {
	v2 := int32(vm.OperandStack.Pop())
	v1 := int32(vm.OperandStack.Pop())
	if v2 == 0 || (v1 == math.MinInt32 && v2 == -1) {
		panic("undefined")
	}
	vm.OperandStack.Push(uint64(v1 / v2))
}

func i32divu(vm *VirtualMachine) {
	v2 := uint32(vm.OperandStack.Pop())
	v1 := uint32(vm.OperandStack.Pop())
	vm.OperandStack.Push(uint64(v1 / v2))
}

func i32rems(vm *VirtualMachine) {
	v2 := int32(vm.OperandStack.Pop())
	v1 := int32(vm.OperandStack.Pop())
	vm.OperandStack.Push(uint64(v1 % v2))
}

func i32remu(vm *VirtualMachine) {
	v2 := uint32(vm.OperandStack.Pop())
	v1 := uint32(vm.OperandStack.Pop())
	vm.OperandStack.Push(uint64(v1 % v2))
}

func i32and(vm *VirtualMachine) {
	vm.OperandStack.Push(uint64(uint32(vm.OperandStack.Pop()) & uint32(vm.OperandStack.Pop())))
}

func i32or(vm *VirtualMachine) {
	vm.OperandStack.Push(uint64(uint32(vm.OperandStack.Pop()) | uint32(vm.OperandStack.Pop())))
}

func i32xor(vm *VirtualMachine) {
	vm.OperandStack.Push(uint64(uint32(vm.OperandStack.Pop()) ^ uint32(vm.OperandStack.Pop())))
}

func i32shl(vm *VirtualMachine) {
	v2 := uint32(vm.OperandStack.Pop())
	v1 := uint32(vm.OperandStack.Pop())
	vm.OperandStack.Push(uint64(v1 << (v2 % 32)))
}

func i32shru(vm *VirtualMachine) {
	v2 := uint32(vm.OperandStack.Pop())
	v1 := uint32(vm.OperandStack.Pop())
	vm.OperandStack.Push(uint64(v1 >> (v2 % 32)))
}

func i32shrs(vm *VirtualMachine) {
	v2 := uint32(vm.OperandStack.Pop())
	v1 := int32(vm.OperandStack.Pop())
	vm.OperandStack.Push(uint64(v1 >> (v2 % 32)))
}

func i32rotl(vm *VirtualMachine) {
	v2 := int(vm.OperandStack.Pop())
	v1 := uint32(vm.OperandStack.Pop())
	vm.OperandStack.Push(uint64(bits.RotateLeft32(v1, v2)))
}

func i32rotr(vm *VirtualMachine) {
	v2 := int(vm.OperandStack.Pop())
	v1 := uint32(vm.OperandStack.Pop())
	vm.OperandStack.Push(uint64(bits.RotateLeft32(v1, -v2)))
}

// i64
func i64clz(vm *VirtualMachine) {
	vm.OperandStack.Push(uint64(bits.LeadingZeros64(vm.OperandStack.Pop())))
}

func i64ctz(vm *VirtualMachine) {
	vm.OperandStack.Push(uint64(bits.TrailingZeros64(vm.OperandStack.Pop())))
}

func i64popcnt(vm *VirtualMachine) {
	vm.OperandStack.Push(uint64(bits.OnesCount64(vm.OperandStack.Pop())))
}

func i64add(vm *VirtualMachine) {
	vm.OperandStack.Push(vm.OperandStack.Pop() + vm.OperandStack.Pop())
}

func i64sub(vm *VirtualMachine) {
	v2 := vm.OperandStack.Pop()
	v1 := vm.OperandStack.Pop()
	vm.OperandStack.Push(v1 - v2)
}

func i64mul(vm *VirtualMachine) {
	vm.OperandStack.Push(vm.OperandStack.Pop() * vm.OperandStack.Pop())
}

func i64divs(vm *VirtualMachine) {
	v2 := int64(vm.OperandStack.Pop())
	v1 := int64(vm.OperandStack.Pop())
	if v2 == 0 || (v1 == math.MinInt64 && v2 == -1) {
		panic("undefined")
	}
	vm.OperandStack.Push(uint64(v1 / v2))
}

func i64divu(vm *VirtualMachine) {
	v2 := vm.OperandStack.Pop()
	v1 := vm.OperandStack.Pop()
	vm.OperandStack.Push(v1 / v2)
}

func i64rems(vm *VirtualMachine) {
	v2 := int64(vm.OperandStack.Pop())
	v1 := int64(vm.OperandStack.Pop())
	vm.OperandStack.Push(uint64(v1 % v2))
}

func i64remu(vm *VirtualMachine) {
	v2 := vm.OperandStack.Pop()
	v1 := vm.OperandStack.Pop()
	vm.OperandStack.Push(v1 % v2)
}

func i64and(vm *VirtualMachine) {
	vm.OperandStack.Push(vm.OperandStack.Pop() & vm.OperandStack.Pop())
}

func i64or(vm *VirtualMachine) {
	vm.OperandStack.Push(vm.OperandStack.Pop() | vm.OperandStack.Pop())
}

func i64xor(vm *VirtualMachine) {
	vm.OperandStack.Push(vm.OperandStack.Pop() ^ vm.OperandStack.Pop())
}

func i64shl(vm *VirtualMachine) {
	v2 := vm.OperandStack.Pop()
	v1 := vm.OperandStack.Pop()
	vm.OperandStack.Push(v1 << (v2 % 64))
}

func i64shru(vm *VirtualMachine) {
	v2 := vm.OperandStack.Pop()
	v1 := vm.OperandStack.Pop()
	vm.OperandStack.Push(v1 >> (v2 % 64))
}

func i64shrs(vm *VirtualMachine) {
	v2 := vm.OperandStack.Pop()
	v1 := int64(vm.OperandStack.Pop())
	vm.OperandStack.Push(uint64(v1 >> (v2 % 64)))
}

func i64rotl(vm *VirtualMachine) {
	v2 := int(vm.OperandStack.Pop())
	v1 := vm.OperandStack.Pop()
	vm.OperandStack.Push(bits.RotateLeft64(v1, v2))
}

func i64rotr(vm *VirtualMachine) {
	v2 := int(vm.OperandStack.Pop())
	v1 := vm.OperandStack.Pop()
	vm.OperandStack.Push(bits.RotateLeft64(v1, -v2))
}

func f32abs(vm *VirtualMachine) {
	const mask uint32 = 1 << 31
	v := uint32(vm.OperandStack.Pop()) &^ mask
	vm.OperandStack.Push(uint64(v))
}

func f32neg(vm *VirtualMachine) {
	v := -math.Float32frombits(uint32(vm.OperandStack.Pop()))
	vm.OperandStack.Push(uint64(math.Float32bits(v)))
}

func f32ceil(vm *VirtualMachine) {
	v := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	vm.OperandStack.Push(uint64(math.Float32bits(float32(math.Ceil(float64(v))))))
}

func f32floor(vm *VirtualMachine) {
	v := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	vm.OperandStack.Push(uint64(math.Float32bits(float32(math.Floor(float64(v))))))
}

func f32trunc(vm *VirtualMachine) {
	v := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	vm.OperandStack.Push(uint64(math.Float32bits(float32(math.Trunc(float64(v))))))
}

func f32nearest(vm *VirtualMachine) {
	raw := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	v := math.Float64bits(float64(int32(raw + float32(math.Copysign(0.5, float64(raw))))))
	vm.OperandStack.Push(v)
}

func f32sqrt(vm *VirtualMachine) {
	v := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	vm.OperandStack.Push(uint64(math.Float32bits(float32(math.Sqrt(float64(v))))))
}

func f32add(vm *VirtualMachine) {
	v := math.Float32frombits(uint32(vm.OperandStack.Pop())) + math.Float32frombits(uint32(vm.OperandStack.Pop()))
	vm.OperandStack.Push(uint64(math.Float32bits(v)))
}

func f32sub(vm *VirtualMachine) {
	v2 := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	v1 := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	vm.OperandStack.Push(uint64(math.Float32bits(v1 - v2)))
}

func f32mul(vm *VirtualMachine) {
	v := math.Float32frombits(uint32(vm.OperandStack.Pop())) * math.Float32frombits(uint32(vm.OperandStack.Pop()))
	vm.OperandStack.Push(uint64(math.Float32bits(v)))
}

func f32div(vm *VirtualMachine) {
	v2 := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	v1 := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	vm.OperandStack.Push(uint64(math.Float32bits(v1 / v2)))
}

func f32min(vm *VirtualMachine) {
	v2 := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	v1 := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	vm.OperandStack.Push(uint64(math.Float32bits(float32(math.Min(float64(v1), float64(v2))))))
}

func f32max(vm *VirtualMachine) {
	v2 := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	v1 := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	vm.OperandStack.Push(uint64(math.Float32bits(float32(math.Min(float64(v1), float64(v2))))))
}

func f32copysign(vm *VirtualMachine) {
	v2 := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	v1 := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	vm.OperandStack.Push(uint64(math.Float32bits(float32(math.Copysign(float64(v1), float64(v2))))))
}

func f64abs(vm *VirtualMachine) {
	const mask = 1 << 63
	v := vm.OperandStack.Pop() &^ mask
	vm.OperandStack.Push(v)
}

func f64neg(vm *VirtualMachine) {
	v := -math.Float64frombits(vm.OperandStack.Pop())
	vm.OperandStack.Push(math.Float64bits(v))
}

func f64ceil(vm *VirtualMachine) {
	v := math.Float64frombits(vm.OperandStack.Pop())
	vm.OperandStack.Push(math.Float64bits(math.Ceil(v)))
}

func f64floor(vm *VirtualMachine) {
	v := math.Float64frombits(vm.OperandStack.Pop())
	vm.OperandStack.Push(math.Float64bits(math.Floor(v)))
}

func f64trunc(vm *VirtualMachine) {
	v := math.Float64frombits(vm.OperandStack.Pop())
	vm.OperandStack.Push(math.Float64bits(math.Trunc(v)))
}

func f64nearest(vm *VirtualMachine) {
	raw := math.Float64frombits(vm.OperandStack.Pop())
	v := math.Float64bits(float64(int64(raw + math.Copysign(0.5, raw))))
	vm.OperandStack.Push(v)
}

func f64sqrt(vm *VirtualMachine) {
	v := math.Float64frombits(vm.OperandStack.Pop())
	vm.OperandStack.Push(math.Float64bits(math.Sqrt(v)))
}

func f64add(vm *VirtualMachine) {
	v := math.Float64frombits(vm.OperandStack.Pop()) + math.Float64frombits(vm.OperandStack.Pop())
	vm.OperandStack.Push(math.Float64bits(v))
}

func f64sub(vm *VirtualMachine) {
	v2 := math.Float64frombits(vm.OperandStack.Pop())
	v1 := math.Float64frombits(vm.OperandStack.Pop())
	vm.OperandStack.Push(math.Float64bits(v1 - v2))
}

func f64mul(vm *VirtualMachine) {
	v := math.Float64frombits(vm.OperandStack.Pop()) * math.Float64frombits(vm.OperandStack.Pop())
	vm.OperandStack.Push(math.Float64bits(v))
}

func f64div(vm *VirtualMachine) {
	v2 := math.Float64frombits(vm.OperandStack.Pop())
	v1 := math.Float64frombits(vm.OperandStack.Pop())
	vm.OperandStack.Push(math.Float64bits(v1 / v2))
}

func f64min(vm *VirtualMachine) {
	v2 := math.Float64frombits(vm.OperandStack.Pop())
	v1 := math.Float64frombits(vm.OperandStack.Pop())
	vm.OperandStack.Push(math.Float64bits(math.Min(v1, v2)))
}

func f64max(vm *VirtualMachine) {
	v2 := math.Float64frombits(vm.OperandStack.Pop())
	v1 := math.Float64frombits(vm.OperandStack.Pop())
	vm.OperandStack.Push(math.Float64bits(math.Min(v1, v2)))
}

func f64copysign(vm *VirtualMachine) {
	v2 := math.Float64frombits(vm.OperandStack.Pop())
	v1 := math.Float64frombits(vm.OperandStack.Pop())
	vm.OperandStack.Push(math.Float64bits(math.Copysign(v1, v2)))
}

func i32wrapi64(vm *VirtualMachine) {
	vm.OperandStack.Push(uint64(uint32(vm.OperandStack.Pop())))
}

func i32truncf32s(vm *VirtualMachine) {
	v := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	vm.OperandStack.Push(uint64(int32(math.Trunc(float64(v)))))
}

func i32truncf32u(vm *VirtualMachine) {
	v := math.Float32frombits(uint32(vm.OperandStack.Pop()))
	vm.OperandStack.Push(uint64(uint32(math.Trunc(float64(v)))))
}

func i32truncf64s(vm *VirtualMachine) {
	v := math.Float64frombits(vm.OperandStack.Pop())
	vm.OperandStack.Push(uint64(int32(math.Trunc(v))))
}

func i32truncf64u(vm *VirtualMachine) {
	v := math.Float64frombits(vm.OperandStack.Pop())
	vm.OperandStack.Push(uint64(uint32(math.Trunc(v))))
}

func i64extendi32s(vm *VirtualMachine) {
	v := int64(int32(vm.OperandStack.Pop()))
	vm.OperandStack.Push(uint64(v))
}

func i64extendi32u(vm *VirtualMachine) {
	v := uint64(uint32(vm.OperandStack.Pop()))
	vm.OperandStack.Push(v)
}

func i64truncf32s(vm *VirtualMachine) {
	v := math.Trunc(float64(math.Float32frombits(uint32(vm.OperandStack.Pop()))))
	vm.OperandStack.Push(uint64(int64(v)))
}

func i64truncf32u(vm *VirtualMachine) {
	v := math.Trunc(float64(math.Float32frombits(uint32(vm.OperandStack.Pop()))))
	vm.OperandStack.Push(uint64(v))
}

func i64truncf64s(vm *VirtualMachine) {
	v := math.Trunc(math.Float64frombits(vm.OperandStack.Pop()))
	vm.OperandStack.Push(uint64(int64(v)))
}

func i64truncf64u(vm *VirtualMachine) {
	v := math.Trunc(math.Float64frombits(vm.OperandStack.Pop()))
	vm.OperandStack.Push(uint64(v))
}

func f32converti32s(vm *VirtualMachine) {
	v := float32(int32(vm.OperandStack.Pop()))
	vm.OperandStack.Push(uint64(math.Float32bits(v)))
}

func f32converti32u(vm *VirtualMachine) {
	v := float32(uint32(vm.OperandStack.Pop()))
	vm.OperandStack.Push(uint64(math.Float32bits(v)))
}

func f32converti64s(vm *VirtualMachine) {
	v := float32(int64(vm.OperandStack.Pop()))
	vm.OperandStack.Push(uint64(math.Float32bits(v)))
}

func f32converti64u(vm *VirtualMachine) {
	v := float32(vm.OperandStack.Pop())
	vm.OperandStack.Push(uint64(math.Float32bits(v)))
}

func f32demotef64(vm *VirtualMachine) {
	v := float32(math.Float64frombits(vm.OperandStack.Pop()))
	vm.OperandStack.Push(uint64(math.Float32bits(v)))
}

func f64converti32s(vm *VirtualMachine) {
	v := float64(int32(vm.OperandStack.Pop()))
	vm.OperandStack.Push(math.Float64bits(v))
}

func f64converti32u(vm *VirtualMachine) {
	v := float64(uint32(vm.OperandStack.Pop()))
	vm.OperandStack.Push(math.Float64bits(v))
}

func f64converti64s(vm *VirtualMachine) {
	v := float64(int64(vm.OperandStack.Pop()))
	vm.OperandStack.Push(math.Float64bits(v))
}

func f64converti64u(vm *VirtualMachine) {
	v := float64(vm.OperandStack.Pop())
	vm.OperandStack.Push(math.Float64bits(v))
}

func f64promotef32(vm *VirtualMachine) {
	v := float64(math.Float32frombits(uint32(vm.OperandStack.Pop())))
	vm.OperandStack.Push(math.Float64bits(v))
}
