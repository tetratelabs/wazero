package wasm

import (
	"math"
	"math/bits"
)

// fixme: there seems to be virtually nop instructions

func i32eqz(vm *VirtualMachine) {
	vm.Operands.PushBool(int32(vm.Operands.Pop()) == 0)
	vm.ActiveFrame.PC++
}

func i32eq(vm *VirtualMachine) {
	vm.Operands.PushBool(int32(vm.Operands.Pop()) == int32(vm.Operands.Pop())) //nolint
	vm.ActiveFrame.PC++
}

func i32ne(vm *VirtualMachine) {
	vm.Operands.PushBool(int32(vm.Operands.Pop()) != int32(vm.Operands.Pop())) //nolint
	vm.ActiveFrame.PC++
}

func i32lts(vm *VirtualMachine) {
	v2 := vm.Operands.Pop()
	v1 := vm.Operands.Pop()
	vm.Operands.PushBool(int32(v1) < int32(v2))
	vm.ActiveFrame.PC++
}

func i32ltu(vm *VirtualMachine) {
	v2 := vm.Operands.Pop()
	v1 := vm.Operands.Pop()
	vm.Operands.PushBool(v1 < v2)
	vm.ActiveFrame.PC++
}

func i32gts(vm *VirtualMachine) {
	v2 := vm.Operands.Pop()
	v1 := vm.Operands.Pop()
	vm.Operands.PushBool(int32(v1) > int32(v2))
	vm.ActiveFrame.PC++
}

func i32gtu(vm *VirtualMachine) {
	v2 := vm.Operands.Pop()
	v1 := vm.Operands.Pop()
	vm.Operands.PushBool(v1 > v2)
	vm.ActiveFrame.PC++
}

func i32les(vm *VirtualMachine) {
	v2 := vm.Operands.Pop()
	v1 := vm.Operands.Pop()
	vm.Operands.PushBool(int32(v1) <= int32(v2))
	vm.ActiveFrame.PC++
}

func i32leu(vm *VirtualMachine) {
	v2 := vm.Operands.Pop()
	v1 := vm.Operands.Pop()
	vm.Operands.PushBool(v1 <= v2)
	vm.ActiveFrame.PC++
}

func i32ges(vm *VirtualMachine) {
	v2 := vm.Operands.Pop()
	v1 := vm.Operands.Pop()
	vm.Operands.PushBool(int32(v1) >= int32(v2))
	vm.ActiveFrame.PC++
}

func i32geu(vm *VirtualMachine) {
	v2 := vm.Operands.Pop()
	v1 := vm.Operands.Pop()
	vm.Operands.PushBool(v1 >= v2)
	vm.ActiveFrame.PC++
}

func i64eqz(vm *VirtualMachine) {
	vm.Operands.PushBool(vm.Operands.Pop() == 0)
	vm.ActiveFrame.PC++
}

func i64eq(vm *VirtualMachine) {
	vm.Operands.PushBool(vm.Operands.Pop() == vm.Operands.Pop()) //nolint
	vm.ActiveFrame.PC++
}

func i64ne(vm *VirtualMachine) {
	vm.Operands.PushBool(vm.Operands.Pop() != vm.Operands.Pop()) //nolint
	vm.ActiveFrame.PC++
}

func i64lts(vm *VirtualMachine) {
	v2 := vm.Operands.Pop()
	v1 := vm.Operands.Pop()
	vm.Operands.PushBool(int64(v1) < int64(v2))
	vm.ActiveFrame.PC++
}

func i64ltu(vm *VirtualMachine) {
	v2 := vm.Operands.Pop()
	v1 := vm.Operands.Pop()
	vm.Operands.PushBool(v1 < v2)
	vm.ActiveFrame.PC++
}

func i64gts(vm *VirtualMachine) {
	v2 := vm.Operands.Pop()
	v1 := vm.Operands.Pop()
	vm.Operands.PushBool(int64(v1) > int64(v2))
	vm.ActiveFrame.PC++
}

func i64gtu(vm *VirtualMachine) {
	v2 := vm.Operands.Pop()
	v1 := vm.Operands.Pop()
	vm.Operands.PushBool(v1 > v2)
	vm.ActiveFrame.PC++
}

func i64les(vm *VirtualMachine) {
	v2 := vm.Operands.Pop()
	v1 := vm.Operands.Pop()
	vm.Operands.PushBool(int64(v1) <= int64(v2))
	vm.ActiveFrame.PC++
}

func i64leu(vm *VirtualMachine) {
	v2 := vm.Operands.Pop()
	v1 := vm.Operands.Pop()
	vm.Operands.PushBool(v1 <= v2)
	vm.ActiveFrame.PC++
}

func i64ges(vm *VirtualMachine) {
	v2 := vm.Operands.Pop()
	v1 := vm.Operands.Pop()
	vm.Operands.PushBool(int64(v1) >= int64(v2))
	vm.ActiveFrame.PC++
}

func i64geu(vm *VirtualMachine) {
	v2 := vm.Operands.Pop()
	v1 := vm.Operands.Pop()
	vm.Operands.PushBool(v1 >= v2)
	vm.ActiveFrame.PC++
}

func f32eq(vm *VirtualMachine) {
	f2 := math.Float32frombits(uint32(vm.Operands.Pop()))
	f1 := math.Float32frombits(uint32(vm.Operands.Pop()))
	vm.Operands.PushBool(f1 == f2)
	vm.ActiveFrame.PC++
}

func f32ne(vm *VirtualMachine) {
	f2 := math.Float32frombits(uint32(vm.Operands.Pop()))
	f1 := math.Float32frombits(uint32(vm.Operands.Pop()))
	vm.Operands.PushBool(f1 != f2)
	vm.ActiveFrame.PC++
}

func f32lt(vm *VirtualMachine) {
	f2 := math.Float32frombits(uint32(vm.Operands.Pop()))
	f1 := math.Float32frombits(uint32(vm.Operands.Pop()))
	vm.Operands.PushBool(f1 < f2)
	vm.ActiveFrame.PC++
}

func f32gt(vm *VirtualMachine) {
	f2 := math.Float32frombits(uint32(vm.Operands.Pop()))
	f1 := math.Float32frombits(uint32(vm.Operands.Pop()))
	vm.Operands.PushBool(f1 > f2)
	vm.ActiveFrame.PC++
}

func f32le(vm *VirtualMachine) {
	f2 := math.Float32frombits(uint32(vm.Operands.Pop()))
	f1 := math.Float32frombits(uint32(vm.Operands.Pop()))
	vm.Operands.PushBool(f1 <= f2)
	vm.ActiveFrame.PC++
}

func f32ge(vm *VirtualMachine) {
	f2 := math.Float32frombits(uint32(vm.Operands.Pop()))
	f1 := math.Float32frombits(uint32(vm.Operands.Pop()))
	vm.Operands.PushBool(f1 >= f2)
	vm.ActiveFrame.PC++
}

func f64eq(vm *VirtualMachine) {
	f2 := math.Float64frombits(vm.Operands.Pop())
	f1 := math.Float64frombits(vm.Operands.Pop())
	vm.Operands.PushBool(f1 == f2)
	vm.ActiveFrame.PC++
}

func f64ne(vm *VirtualMachine) {
	f2 := math.Float64frombits(vm.Operands.Pop())
	f1 := math.Float64frombits(vm.Operands.Pop())
	vm.Operands.PushBool(f1 != f2)
	vm.ActiveFrame.PC++
}

func f64lt(vm *VirtualMachine) {
	f2 := math.Float64frombits(vm.Operands.Pop())
	f1 := math.Float64frombits(vm.Operands.Pop())
	vm.Operands.PushBool(f1 < f2)
	vm.ActiveFrame.PC++
}

func f64gt(vm *VirtualMachine) {
	f2 := math.Float64frombits(vm.Operands.Pop())
	f1 := math.Float64frombits(vm.Operands.Pop())
	vm.Operands.PushBool(f1 > f2)
	vm.ActiveFrame.PC++
}

func f64le(vm *VirtualMachine) {
	f2 := math.Float64frombits(vm.Operands.Pop())
	f1 := math.Float64frombits(vm.Operands.Pop())
	vm.Operands.PushBool(f1 <= f2)
	vm.ActiveFrame.PC++
}

func f64ge(vm *VirtualMachine) {
	f2 := math.Float64frombits(vm.Operands.Pop())
	f1 := math.Float64frombits(vm.Operands.Pop())
	vm.Operands.PushBool(f1 >= f2)
	vm.ActiveFrame.PC++
}

func i32clz(vm *VirtualMachine) {
	vm.Operands.Push(uint64(bits.LeadingZeros32(uint32(vm.Operands.Pop()))))
	vm.ActiveFrame.PC++
}

func i32ctz(vm *VirtualMachine) {
	vm.Operands.Push(uint64(bits.TrailingZeros32(uint32(vm.Operands.Pop()))))
	vm.ActiveFrame.PC++
}

func i32popcnt(vm *VirtualMachine) {
	vm.Operands.Push(uint64(bits.OnesCount32(uint32(vm.Operands.Pop()))))
	vm.ActiveFrame.PC++
}

func i32add(vm *VirtualMachine) {
	vm.Operands.Push(uint64(uint32(vm.Operands.Pop()) + uint32(vm.Operands.Pop())))
	vm.ActiveFrame.PC++
}

func i32sub(vm *VirtualMachine) {
	v2 := uint32(vm.Operands.Pop())
	v1 := uint32(vm.Operands.Pop())
	vm.Operands.Push(uint64(v1 - v2))
	vm.ActiveFrame.PC++
}

func i32mul(vm *VirtualMachine) {
	vm.Operands.Push(uint64(uint32(vm.Operands.Pop()) * uint32(vm.Operands.Pop())))
	vm.ActiveFrame.PC++
}

func i32divs(vm *VirtualMachine) {
	v2 := int32(vm.Operands.Pop())
	v1 := int32(vm.Operands.Pop())
	if v2 == 0 || (v1 == math.MinInt32 && v2 == -1) {
		panic("undefined")
	}
	vm.Operands.Push(uint64(uint32(v1 / v2)))
	vm.ActiveFrame.PC++
}

func i32divu(vm *VirtualMachine) {
	v2 := uint32(vm.Operands.Pop())
	v1 := uint32(vm.Operands.Pop())
	vm.Operands.Push(uint64(v1 / v2))
	vm.ActiveFrame.PC++
}

func i32rems(vm *VirtualMachine) {
	v2 := int32(vm.Operands.Pop())
	v1 := int32(vm.Operands.Pop())
	vm.Operands.Push(uint64(uint32(v1 % v2)))
	vm.ActiveFrame.PC++
}

func i32remu(vm *VirtualMachine) {
	v2 := uint32(vm.Operands.Pop())
	v1 := uint32(vm.Operands.Pop())
	vm.Operands.Push(uint64(v1 % v2))
	vm.ActiveFrame.PC++
}

func i32and(vm *VirtualMachine) {
	vm.Operands.Push(uint64(uint32(vm.Operands.Pop()) & uint32(vm.Operands.Pop()))) //nolint
	vm.ActiveFrame.PC++
}

func i32or(vm *VirtualMachine) {
	vm.Operands.Push(uint64(uint32(vm.Operands.Pop()) | uint32(vm.Operands.Pop()))) //nolint
	vm.ActiveFrame.PC++
}

func i32xor(vm *VirtualMachine) {
	vm.Operands.Push(uint64(uint32(vm.Operands.Pop()) ^ uint32(vm.Operands.Pop()))) //nolint
	vm.ActiveFrame.PC++
}

func i32shl(vm *VirtualMachine) {
	v2 := uint32(vm.Operands.Pop())
	v1 := uint32(vm.Operands.Pop())
	vm.Operands.Push(uint64(v1 << (v2 % 32)))
	vm.ActiveFrame.PC++
}

func i32shru(vm *VirtualMachine) {
	v2 := uint32(vm.Operands.Pop())
	v1 := uint32(vm.Operands.Pop())
	vm.Operands.Push(uint64(v1 >> (v2 % 32)))
	vm.ActiveFrame.PC++
}

func i32shrs(vm *VirtualMachine) {
	v2 := uint32(vm.Operands.Pop())
	v1 := int32(vm.Operands.Pop())
	vm.Operands.Push(uint64(v1 >> (v2 % 32)))
	vm.ActiveFrame.PC++
}

func i32rotl(vm *VirtualMachine) {
	v2 := int(vm.Operands.Pop())
	v1 := uint32(vm.Operands.Pop())
	vm.Operands.Push(uint64(bits.RotateLeft32(v1, v2)))
	vm.ActiveFrame.PC++
}

func i32rotr(vm *VirtualMachine) {
	v2 := int(vm.Operands.Pop())
	v1 := uint32(vm.Operands.Pop())
	vm.Operands.Push(uint64(bits.RotateLeft32(v1, -v2)))
	vm.ActiveFrame.PC++
}

// i64
func i64clz(vm *VirtualMachine) {
	vm.Operands.Push(uint64(bits.LeadingZeros64(vm.Operands.Pop())))
	vm.ActiveFrame.PC++
}

func i64ctz(vm *VirtualMachine) {
	vm.Operands.Push(uint64(bits.TrailingZeros64(vm.Operands.Pop())))
	vm.ActiveFrame.PC++
}

func i64popcnt(vm *VirtualMachine) {
	vm.Operands.Push(uint64(bits.OnesCount64(vm.Operands.Pop())))
	vm.ActiveFrame.PC++
}

func i64add(vm *VirtualMachine) {
	vm.Operands.Push(vm.Operands.Pop() + vm.Operands.Pop())
	vm.ActiveFrame.PC++
}

func i64sub(vm *VirtualMachine) {
	v2 := vm.Operands.Pop()
	v1 := vm.Operands.Pop()
	vm.Operands.Push(v1 - v2)
	vm.ActiveFrame.PC++
}

func i64mul(vm *VirtualMachine) {
	vm.Operands.Push(vm.Operands.Pop() * vm.Operands.Pop())
	vm.ActiveFrame.PC++
}

func i64divs(vm *VirtualMachine) {
	v2 := int64(vm.Operands.Pop())
	v1 := int64(vm.Operands.Pop())
	if v2 == 0 || (v1 == math.MinInt64 && v2 == -1) {
		panic("undefined")
	}
	vm.Operands.Push(uint64(v1 / v2))
	vm.ActiveFrame.PC++
}

func i64divu(vm *VirtualMachine) {
	v2 := vm.Operands.Pop()
	v1 := vm.Operands.Pop()
	vm.Operands.Push(v1 / v2)
	vm.ActiveFrame.PC++
}

func i64rems(vm *VirtualMachine) {
	v2 := int64(vm.Operands.Pop())
	v1 := int64(vm.Operands.Pop())
	vm.Operands.Push(uint64(v1 % v2))
	vm.ActiveFrame.PC++
}

func i64remu(vm *VirtualMachine) {
	v2 := vm.Operands.Pop()
	v1 := vm.Operands.Pop()
	vm.Operands.Push(v1 % v2)
	vm.ActiveFrame.PC++
}

func i64and(vm *VirtualMachine) {
	vm.Operands.Push(vm.Operands.Pop() & vm.Operands.Pop()) //nolint
	vm.ActiveFrame.PC++
}

func i64or(vm *VirtualMachine) {
	vm.Operands.Push(vm.Operands.Pop() | vm.Operands.Pop()) //nolint
	vm.ActiveFrame.PC++
}

func i64xor(vm *VirtualMachine) {
	vm.Operands.Push(vm.Operands.Pop() ^ vm.Operands.Pop()) //nolint
	vm.ActiveFrame.PC++
}

func i64shl(vm *VirtualMachine) {
	v2 := vm.Operands.Pop()
	v1 := vm.Operands.Pop()
	vm.Operands.Push(v1 << (v2 % 64))
	vm.ActiveFrame.PC++
}

func i64shru(vm *VirtualMachine) {
	v2 := vm.Operands.Pop()
	v1 := vm.Operands.Pop()
	vm.Operands.Push(v1 >> (v2 % 64))
	vm.ActiveFrame.PC++
}

func i64shrs(vm *VirtualMachine) {
	v2 := vm.Operands.Pop()
	v1 := int64(vm.Operands.Pop())
	vm.Operands.Push(uint64(v1 >> (v2 % 64)))
	vm.ActiveFrame.PC++
}

func i64rotl(vm *VirtualMachine) {
	v2 := int(vm.Operands.Pop())
	v1 := vm.Operands.Pop()
	vm.Operands.Push(bits.RotateLeft64(v1, v2))
	vm.ActiveFrame.PC++
}

func i64rotr(vm *VirtualMachine) {
	v2 := int(vm.Operands.Pop())
	v1 := vm.Operands.Pop()
	vm.Operands.Push(bits.RotateLeft64(v1, -v2))
	vm.ActiveFrame.PC++
}

func f32abs(vm *VirtualMachine) {
	const mask uint32 = 1 << 31
	v := uint32(vm.Operands.Pop()) &^ mask
	vm.Operands.Push(uint64(v))
	vm.ActiveFrame.PC++
}

func f32neg(vm *VirtualMachine) {
	v := -math.Float32frombits(uint32(vm.Operands.Pop()))
	vm.Operands.Push(uint64(math.Float32bits(v)))
	vm.ActiveFrame.PC++
}

func f32ceil(vm *VirtualMachine) {
	v := math.Float32frombits(uint32(vm.Operands.Pop()))
	vm.Operands.Push(uint64(math.Float32bits(float32(math.Ceil(float64(v))))))
	vm.ActiveFrame.PC++
}

func f32floor(vm *VirtualMachine) {
	v := math.Float32frombits(uint32(vm.Operands.Pop()))
	vm.Operands.Push(uint64(math.Float32bits(float32(math.Floor(float64(v))))))
	vm.ActiveFrame.PC++
}

func f32trunc(vm *VirtualMachine) {
	v := math.Float32frombits(uint32(vm.Operands.Pop()))
	vm.Operands.Push(uint64(math.Float32bits(float32(math.Trunc(float64(v))))))
	vm.ActiveFrame.PC++
}

func f32nearest(vm *VirtualMachine) {
	// Borrowed from https://github.com/wasmerio/wasmer/blob/703bb4ee2ffb17b2929a194fc045a7e351b696e2/lib/vm/src/libcalls.rs#L77
	f := math.Float32frombits(uint32(vm.Operands.Pop()))
	f64 := float64(f)
	if f != -0 && f != 0 {
		u := float32(math.Ceil(f64))
		d := float32(math.Floor(f64))
		um := math.Abs(float64(f - u))
		dm := math.Abs(float64(f - d))
		h := u / 2.0
		if um < dm || float32(math.Floor(float64(h))) == h {
			f = u
		} else {
			f = d
		}
	}
	vm.Operands.Push(uint64(math.Float32bits(f)))
	vm.ActiveFrame.PC++
}

func f32sqrt(vm *VirtualMachine) {
	v := math.Float32frombits(uint32(vm.Operands.Pop()))
	vm.Operands.Push(uint64(math.Float32bits(float32(math.Sqrt(float64(v))))))
	vm.ActiveFrame.PC++
}

func f32add(vm *VirtualMachine) {
	v := math.Float32frombits(uint32(vm.Operands.Pop())) + math.Float32frombits(uint32(vm.Operands.Pop()))
	vm.Operands.Push(uint64(math.Float32bits(v)))
	vm.ActiveFrame.PC++
}

func f32sub(vm *VirtualMachine) {
	v2 := math.Float32frombits(uint32(vm.Operands.Pop()))
	v1 := math.Float32frombits(uint32(vm.Operands.Pop()))
	vm.Operands.Push(uint64(math.Float32bits(v1 - v2)))
	vm.ActiveFrame.PC++
}

func f32mul(vm *VirtualMachine) {
	v := math.Float32frombits(uint32(vm.Operands.Pop())) * math.Float32frombits(uint32(vm.Operands.Pop()))
	vm.Operands.Push(uint64(math.Float32bits(v)))
	vm.ActiveFrame.PC++
}

func f32div(vm *VirtualMachine) {
	v2 := math.Float32frombits(uint32(vm.Operands.Pop()))
	v1 := math.Float32frombits(uint32(vm.Operands.Pop()))
	vm.Operands.Push(uint64(math.Float32bits(v1 / v2)))
	vm.ActiveFrame.PC++
}

func f32min(vm *VirtualMachine) {
	v2 := math.Float32frombits(uint32(vm.Operands.Pop()))
	v1 := math.Float32frombits(uint32(vm.Operands.Pop()))
	vm.Operands.Push(uint64(math.Float32bits(float32(min(float64(v1), float64(v2))))))
	vm.ActiveFrame.PC++
}

func f32max(vm *VirtualMachine) {
	v2 := math.Float32frombits(uint32(vm.Operands.Pop()))
	v1 := math.Float32frombits(uint32(vm.Operands.Pop()))
	vm.Operands.Push(uint64(math.Float32bits(float32(max(float64(v1), float64(v2))))))
	vm.ActiveFrame.PC++
}

func f32copysign(vm *VirtualMachine) {
	v2 := math.Float32frombits(uint32(vm.Operands.Pop()))
	v1 := math.Float32frombits(uint32(vm.Operands.Pop()))
	vm.Operands.Push(uint64(math.Float32bits(float32(math.Copysign(float64(v1), float64(v2))))))
	vm.ActiveFrame.PC++
}

func f64abs(vm *VirtualMachine) {
	const mask = 1 << 63
	v := vm.Operands.Pop() &^ mask
	vm.Operands.Push(v)
	vm.ActiveFrame.PC++
}

func f64neg(vm *VirtualMachine) {
	v := -math.Float64frombits(vm.Operands.Pop())
	vm.Operands.Push(math.Float64bits(v))
	vm.ActiveFrame.PC++
}

func f64ceil(vm *VirtualMachine) {
	v := math.Float64frombits(vm.Operands.Pop())
	vm.Operands.Push(math.Float64bits(math.Ceil(v)))
	vm.ActiveFrame.PC++
}

func f64floor(vm *VirtualMachine) {
	v := math.Float64frombits(vm.Operands.Pop())
	vm.Operands.Push(math.Float64bits(math.Floor(v)))
	vm.ActiveFrame.PC++
}

func f64trunc(vm *VirtualMachine) {
	v := math.Float64frombits(vm.Operands.Pop())
	vm.Operands.Push(math.Float64bits(math.Trunc(v)))
	vm.ActiveFrame.PC++
}

func f64nearest(vm *VirtualMachine) {
	// Borrowed from https://github.com/wasmerio/wasmer/blob/703bb4ee2ffb17b2929a194fc045a7e351b696e2/lib/vm/src/libcalls.rs#L77
	f := math.Float64frombits(vm.Operands.Pop())
	f64 := float64(f)
	if f != -0 && f != 0 {
		u := math.Ceil(f64)
		d := math.Floor(f64)
		um := math.Abs(f - u)
		dm := math.Abs(f - d)
		h := u / 2.0
		if um < dm || math.Floor(float64(h)) == h {
			f = u
		} else {
			f = d
		}
	}
	vm.Operands.Push(math.Float64bits(f))
	vm.ActiveFrame.PC++
}

func f64sqrt(vm *VirtualMachine) {
	v := math.Float64frombits(vm.Operands.Pop())
	vm.Operands.Push(math.Float64bits(math.Sqrt(v)))
	vm.ActiveFrame.PC++
}

func f64add(vm *VirtualMachine) {
	v := math.Float64frombits(vm.Operands.Pop()) + math.Float64frombits(vm.Operands.Pop())
	vm.Operands.Push(math.Float64bits(v))
	vm.ActiveFrame.PC++
}

func f64sub(vm *VirtualMachine) {
	v2 := math.Float64frombits(vm.Operands.Pop())
	v1 := math.Float64frombits(vm.Operands.Pop())
	vm.Operands.Push(math.Float64bits(v1 - v2))
	vm.ActiveFrame.PC++
}

func f64mul(vm *VirtualMachine) {
	v := math.Float64frombits(vm.Operands.Pop()) * math.Float64frombits(vm.Operands.Pop())
	vm.Operands.Push(math.Float64bits(v))
	vm.ActiveFrame.PC++
}

func f64div(vm *VirtualMachine) {
	v2 := math.Float64frombits(vm.Operands.Pop())
	v1 := math.Float64frombits(vm.Operands.Pop())
	vm.Operands.Push(math.Float64bits(v1 / v2))
	vm.ActiveFrame.PC++
}

// math.Min doen't comply with the Wasm spec, so we borrow from the original
// with a change that either one of NaN results in NaN even if another is -Inf.
// https://github.com/golang/go/blob/1d20a362d0ca4898d77865e314ef6f73582daef0/src/math/dim.go#L74-L91
func min(x, y float64) float64 {
	switch {
	case math.IsNaN(x) || math.IsNaN(y):
		return math.NaN()
	case math.IsInf(x, -1) || math.IsInf(y, -1):
		return math.Inf(-1)
	case x == 0 && x == y:
		if math.Signbit(x) {
			return x
		}
		return y
	}
	if x < y {
		return x
	}
	return y
}

func f64min(vm *VirtualMachine) {
	v2 := math.Float64frombits(vm.Operands.Pop())
	v1 := math.Float64frombits(vm.Operands.Pop())
	vm.Operands.Push(math.Float64bits(min(v1, v2)))
	vm.ActiveFrame.PC++
}

// math.Max doen't comply with the Wasm spec, so we borrow from the original
// with a change that either one of NaN results in NaN even if another is Inf.
// https://github.com/golang/go/blob/1d20a362d0ca4898d77865e314ef6f73582daef0/src/math/dim.go#L42-L59
func max(x, y float64) float64 {
	switch {
	case math.IsNaN(x) || math.IsNaN(y):
		return math.NaN()
	case math.IsInf(x, 1) || math.IsInf(y, 1):
		return math.Inf(1)

	case x == 0 && x == y:
		if math.Signbit(x) {
			return y
		}
		return x
	}
	if x > y {
		return x
	}
	return y
}

func f64max(vm *VirtualMachine) {
	v2 := math.Float64frombits(vm.Operands.Pop())
	v1 := math.Float64frombits(vm.Operands.Pop())
	vm.Operands.Push(math.Float64bits(max(v1, v2)))
	vm.ActiveFrame.PC++
}

func f64copysign(vm *VirtualMachine) {
	v2 := math.Float64frombits(vm.Operands.Pop())
	v1 := math.Float64frombits(vm.Operands.Pop())
	vm.Operands.Push(math.Float64bits(math.Copysign(v1, v2)))
	vm.ActiveFrame.PC++
}

func i32wrapi64(vm *VirtualMachine) {
	vm.Operands.Push(uint64(uint32(vm.Operands.Pop())))
	vm.ActiveFrame.PC++
}

func i32truncf32s(vm *VirtualMachine) {
	v := math.Trunc(float64(math.Float32frombits(uint32(vm.Operands.Pop()))))
	if math.IsNaN(v) {
		panic("invalid conversion")
	} else if v < math.MinInt32 || v > math.MaxInt32 {
		panic("integer overflow")
	}
	vm.Operands.Push(uint64(int32(v)))
	vm.ActiveFrame.PC++
}

func i32truncf32u(vm *VirtualMachine) {
	v := math.Trunc(float64(math.Float32frombits(uint32(vm.Operands.Pop()))))
	if math.IsNaN(v) {
		panic("invalid conversion")
	} else if v < 0 || v > math.MaxUint32 {
		panic("integer overflow")
	}
	vm.Operands.Push(uint64(uint32(v)))
	vm.ActiveFrame.PC++
}

func i32truncf64s(vm *VirtualMachine) {
	v := math.Trunc(math.Float64frombits(vm.Operands.Pop()))
	if math.IsNaN(v) {
		panic("invalid conversion")
	} else if v < math.MinInt32 || v > math.MaxInt32 {
		panic("integer overflow")
	}
	vm.Operands.Push(uint64(int32(v)))
	vm.ActiveFrame.PC++
}

func i32truncf64u(vm *VirtualMachine) {
	v := math.Trunc(math.Float64frombits(vm.Operands.Pop()))
	if math.IsNaN(v) {
		panic("invalid conversion")
	} else if v < 0 || v > math.MaxUint32 {
		panic("integer overflow")
	}
	vm.Operands.Push(uint64(uint32(v)))
	vm.ActiveFrame.PC++
}

func i64extendi32s(vm *VirtualMachine) {
	v := int64(int32(vm.Operands.Pop()))
	vm.Operands.Push(uint64(v))
	vm.ActiveFrame.PC++
}

func i64extendi32u(vm *VirtualMachine) {
	v := uint64(uint32(vm.Operands.Pop()))
	vm.Operands.Push(v)
	vm.ActiveFrame.PC++
}

func i64truncf32s(vm *VirtualMachine) {
	v := math.Trunc(float64(math.Float32frombits(uint32(vm.Operands.Pop()))))
	res := int64(v)
	if math.IsNaN(v) {
		panic("invalid conversion")
	} else if v < math.MinInt64 || v > 0 && res < 0 {
		panic("integer overflow")
	}
	vm.Operands.Push(uint64(res))
	vm.ActiveFrame.PC++
}

func i64truncf32u(vm *VirtualMachine) {
	v := math.Trunc(float64(math.Float32frombits(uint32(vm.Operands.Pop()))))
	res := uint64(v)
	if math.IsNaN(v) {
		panic("invalid conversion")
	} else if v < 0 || v > float64(res) {
		panic("integer overflow")
	}
	vm.Operands.Push(res)
	vm.ActiveFrame.PC++
}

func i64truncf64s(vm *VirtualMachine) {
	v := math.Trunc(math.Float64frombits(vm.Operands.Pop()))
	res := int64(v)
	if math.IsNaN(v) {
		panic("invalid conversion")
	} else if v < math.MinInt64 || v > 0 && res < 0 {
		panic("integer overflow")
	}
	vm.Operands.Push(uint64(res))
	vm.ActiveFrame.PC++
}

func i64truncf64u(vm *VirtualMachine) {
	v := math.Trunc(math.Float64frombits(vm.Operands.Pop()))
	res := uint64(v)
	if math.IsNaN(v) {
		panic("invalid conversion")
	} else if v < 0 || v > float64(res) {
		panic("integer overflow")
	}
	vm.Operands.Push(res)
	vm.ActiveFrame.PC++
}

func f32converti32s(vm *VirtualMachine) {
	v := float32(int32(vm.Operands.Pop()))
	vm.Operands.Push(uint64(math.Float32bits(v)))
	vm.ActiveFrame.PC++
}

func f32converti32u(vm *VirtualMachine) {
	v := float32(uint32(vm.Operands.Pop()))
	vm.Operands.Push(uint64(math.Float32bits(v)))
	vm.ActiveFrame.PC++
}

func f32converti64s(vm *VirtualMachine) {
	v := float32(int64(vm.Operands.Pop()))
	vm.Operands.Push(uint64(math.Float32bits(v)))
	vm.ActiveFrame.PC++
}

func f32converti64u(vm *VirtualMachine) {
	v := float32(vm.Operands.Pop())
	vm.Operands.Push(uint64(math.Float32bits(v)))
	vm.ActiveFrame.PC++
}

func f32demotef64(vm *VirtualMachine) {
	v := float32(math.Float64frombits(vm.Operands.Pop()))
	vm.Operands.Push(uint64(math.Float32bits(v)))
	vm.ActiveFrame.PC++
}

func f64converti32s(vm *VirtualMachine) {
	v := float64(int32(vm.Operands.Pop()))
	vm.Operands.Push(math.Float64bits(v))
	vm.ActiveFrame.PC++
}

func f64converti32u(vm *VirtualMachine) {
	v := float64(uint32(vm.Operands.Pop()))
	vm.Operands.Push(math.Float64bits(v))
	vm.ActiveFrame.PC++
}

func f64converti64s(vm *VirtualMachine) {
	v := float64(int64(vm.Operands.Pop()))
	vm.Operands.Push(math.Float64bits(v))
	vm.ActiveFrame.PC++
}

func f64converti64u(vm *VirtualMachine) {
	v := float64(vm.Operands.Pop())
	vm.Operands.Push(math.Float64bits(v))
	vm.ActiveFrame.PC++
}

func f64promotef32(vm *VirtualMachine) {
	v := float64(math.Float32frombits(uint32(vm.Operands.Pop())))
	vm.Operands.Push(math.Float64bits(v))
	vm.ActiveFrame.PC++
}
