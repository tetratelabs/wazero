package naivevm

import (
	"math"
	"math/bits"
)

// fixme: there seems to be virtually nop instructions

func i32eqz(vm *naiveVirtualMachine) {
	vm.operands.pushBool(int32(vm.operands.pop()) == 0)
	vm.activeFrame.pc++
}

func i32eq(vm *naiveVirtualMachine) {
	vm.operands.pushBool(int32(vm.operands.pop()) == int32(vm.operands.pop())) //nolint
	vm.activeFrame.pc++
}

func i32ne(vm *naiveVirtualMachine) {
	vm.operands.pushBool(int32(vm.operands.pop()) != int32(vm.operands.pop())) //nolint
	vm.activeFrame.pc++
}

func i32lts(vm *naiveVirtualMachine) {
	v2 := vm.operands.pop()
	v1 := vm.operands.pop()
	vm.operands.pushBool(int32(v1) < int32(v2))
	vm.activeFrame.pc++
}

func i32ltu(vm *naiveVirtualMachine) {
	v2 := vm.operands.pop()
	v1 := vm.operands.pop()
	vm.operands.pushBool(v1 < v2)
	vm.activeFrame.pc++
}

func i32gts(vm *naiveVirtualMachine) {
	v2 := vm.operands.pop()
	v1 := vm.operands.pop()
	vm.operands.pushBool(int32(v1) > int32(v2))
	vm.activeFrame.pc++
}

func i32gtu(vm *naiveVirtualMachine) {
	v2 := vm.operands.pop()
	v1 := vm.operands.pop()
	vm.operands.pushBool(v1 > v2)
	vm.activeFrame.pc++
}

func i32les(vm *naiveVirtualMachine) {
	v2 := vm.operands.pop()
	v1 := vm.operands.pop()
	vm.operands.pushBool(int32(v1) <= int32(v2))
	vm.activeFrame.pc++
}

func i32leu(vm *naiveVirtualMachine) {
	v2 := vm.operands.pop()
	v1 := vm.operands.pop()
	vm.operands.pushBool(v1 <= v2)
	vm.activeFrame.pc++
}

func i32ges(vm *naiveVirtualMachine) {
	v2 := vm.operands.pop()
	v1 := vm.operands.pop()
	vm.operands.pushBool(int32(v1) >= int32(v2))
	vm.activeFrame.pc++
}

func i32geu(vm *naiveVirtualMachine) {
	v2 := vm.operands.pop()
	v1 := vm.operands.pop()
	vm.operands.pushBool(v1 >= v2)
	vm.activeFrame.pc++
}

func i64eqz(vm *naiveVirtualMachine) {
	vm.operands.pushBool(vm.operands.pop() == 0)
	vm.activeFrame.pc++
}

func i64eq(vm *naiveVirtualMachine) {
	vm.operands.pushBool(vm.operands.pop() == vm.operands.pop()) //nolint
	vm.activeFrame.pc++
}

func i64ne(vm *naiveVirtualMachine) {
	vm.operands.pushBool(vm.operands.pop() != vm.operands.pop()) //nolint
	vm.activeFrame.pc++
}

func i64lts(vm *naiveVirtualMachine) {
	v2 := vm.operands.pop()
	v1 := vm.operands.pop()
	vm.operands.pushBool(int64(v1) < int64(v2))
	vm.activeFrame.pc++
}

func i64ltu(vm *naiveVirtualMachine) {
	v2 := vm.operands.pop()
	v1 := vm.operands.pop()
	vm.operands.pushBool(v1 < v2)
	vm.activeFrame.pc++
}

func i64gts(vm *naiveVirtualMachine) {
	v2 := vm.operands.pop()
	v1 := vm.operands.pop()
	vm.operands.pushBool(int64(v1) > int64(v2))
	vm.activeFrame.pc++
}

func i64gtu(vm *naiveVirtualMachine) {
	v2 := vm.operands.pop()
	v1 := vm.operands.pop()
	vm.operands.pushBool(v1 > v2)
	vm.activeFrame.pc++
}

func i64les(vm *naiveVirtualMachine) {
	v2 := vm.operands.pop()
	v1 := vm.operands.pop()
	vm.operands.pushBool(int64(v1) <= int64(v2))
	vm.activeFrame.pc++
}

func i64leu(vm *naiveVirtualMachine) {
	v2 := vm.operands.pop()
	v1 := vm.operands.pop()
	vm.operands.pushBool(v1 <= v2)
	vm.activeFrame.pc++
}

func i64ges(vm *naiveVirtualMachine) {
	v2 := vm.operands.pop()
	v1 := vm.operands.pop()
	vm.operands.pushBool(int64(v1) >= int64(v2))
	vm.activeFrame.pc++
}

func i64geu(vm *naiveVirtualMachine) {
	v2 := vm.operands.pop()
	v1 := vm.operands.pop()
	vm.operands.pushBool(v1 >= v2)
	vm.activeFrame.pc++
}

func f32eq(vm *naiveVirtualMachine) {
	f2 := math.Float32frombits(uint32(vm.operands.pop()))
	f1 := math.Float32frombits(uint32(vm.operands.pop()))
	vm.operands.pushBool(f1 == f2)
	vm.activeFrame.pc++
}

func f32ne(vm *naiveVirtualMachine) {
	f2 := math.Float32frombits(uint32(vm.operands.pop()))
	f1 := math.Float32frombits(uint32(vm.operands.pop()))
	vm.operands.pushBool(f1 != f2)
	vm.activeFrame.pc++
}

func f32lt(vm *naiveVirtualMachine) {
	f2 := math.Float32frombits(uint32(vm.operands.pop()))
	f1 := math.Float32frombits(uint32(vm.operands.pop()))
	vm.operands.pushBool(f1 < f2)
	vm.activeFrame.pc++
}

func f32gt(vm *naiveVirtualMachine) {
	f2 := math.Float32frombits(uint32(vm.operands.pop()))
	f1 := math.Float32frombits(uint32(vm.operands.pop()))
	vm.operands.pushBool(f1 > f2)
	vm.activeFrame.pc++
}

func f32le(vm *naiveVirtualMachine) {
	f2 := math.Float32frombits(uint32(vm.operands.pop()))
	f1 := math.Float32frombits(uint32(vm.operands.pop()))
	vm.operands.pushBool(f1 <= f2)
	vm.activeFrame.pc++
}

func f32ge(vm *naiveVirtualMachine) {
	f2 := math.Float32frombits(uint32(vm.operands.pop()))
	f1 := math.Float32frombits(uint32(vm.operands.pop()))
	vm.operands.pushBool(f1 >= f2)
	vm.activeFrame.pc++
}

func f64eq(vm *naiveVirtualMachine) {
	f2 := math.Float64frombits(vm.operands.pop())
	f1 := math.Float64frombits(vm.operands.pop())
	vm.operands.pushBool(f1 == f2)
	vm.activeFrame.pc++
}

func f64ne(vm *naiveVirtualMachine) {
	f2 := math.Float64frombits(vm.operands.pop())
	f1 := math.Float64frombits(vm.operands.pop())
	vm.operands.pushBool(f1 != f2)
	vm.activeFrame.pc++
}

func f64lt(vm *naiveVirtualMachine) {
	f2 := math.Float64frombits(vm.operands.pop())
	f1 := math.Float64frombits(vm.operands.pop())
	vm.operands.pushBool(f1 < f2)
	vm.activeFrame.pc++
}

func f64gt(vm *naiveVirtualMachine) {
	f2 := math.Float64frombits(vm.operands.pop())
	f1 := math.Float64frombits(vm.operands.pop())
	vm.operands.pushBool(f1 > f2)
	vm.activeFrame.pc++
}

func f64le(vm *naiveVirtualMachine) {
	f2 := math.Float64frombits(vm.operands.pop())
	f1 := math.Float64frombits(vm.operands.pop())
	vm.operands.pushBool(f1 <= f2)
	vm.activeFrame.pc++
}

func f64ge(vm *naiveVirtualMachine) {
	f2 := math.Float64frombits(vm.operands.pop())
	f1 := math.Float64frombits(vm.operands.pop())
	vm.operands.pushBool(f1 >= f2)
	vm.activeFrame.pc++
}

func i32clz(vm *naiveVirtualMachine) {
	vm.operands.push(uint64(bits.LeadingZeros32(uint32(vm.operands.pop()))))
	vm.activeFrame.pc++
}

func i32ctz(vm *naiveVirtualMachine) {
	vm.operands.push(uint64(bits.TrailingZeros32(uint32(vm.operands.pop()))))
	vm.activeFrame.pc++
}

func i32popcnt(vm *naiveVirtualMachine) {
	vm.operands.push(uint64(bits.OnesCount32(uint32(vm.operands.pop()))))
	vm.activeFrame.pc++
}

func i32add(vm *naiveVirtualMachine) {
	vm.operands.push(uint64(uint32(vm.operands.pop()) + uint32(vm.operands.pop())))
	vm.activeFrame.pc++
}

func i32sub(vm *naiveVirtualMachine) {
	v2 := uint32(vm.operands.pop())
	v1 := uint32(vm.operands.pop())
	vm.operands.push(uint64(v1 - v2))
	vm.activeFrame.pc++
}

func i32mul(vm *naiveVirtualMachine) {
	vm.operands.push(uint64(uint32(vm.operands.pop()) * uint32(vm.operands.pop())))
	vm.activeFrame.pc++
}

func i32divs(vm *naiveVirtualMachine) {
	v2 := int32(vm.operands.pop())
	v1 := int32(vm.operands.pop())
	if v2 == 0 || (v1 == math.MinInt32 && v2 == -1) {
		panic("undefined")
	}
	vm.operands.push(uint64(uint32(v1 / v2)))
	vm.activeFrame.pc++
}

func i32divu(vm *naiveVirtualMachine) {
	v2 := uint32(vm.operands.pop())
	v1 := uint32(vm.operands.pop())
	vm.operands.push(uint64(v1 / v2))
	vm.activeFrame.pc++
}

func i32rems(vm *naiveVirtualMachine) {
	v2 := int32(vm.operands.pop())
	v1 := int32(vm.operands.pop())
	vm.operands.push(uint64(uint32(v1 % v2)))
	vm.activeFrame.pc++
}

func i32remu(vm *naiveVirtualMachine) {
	v2 := uint32(vm.operands.pop())
	v1 := uint32(vm.operands.pop())
	vm.operands.push(uint64(v1 % v2))
	vm.activeFrame.pc++
}

func i32and(vm *naiveVirtualMachine) {
	vm.operands.push(uint64(uint32(vm.operands.pop()) & uint32(vm.operands.pop()))) //nolint
	vm.activeFrame.pc++
}

func i32or(vm *naiveVirtualMachine) {
	vm.operands.push(uint64(uint32(vm.operands.pop()) | uint32(vm.operands.pop()))) //nolint
	vm.activeFrame.pc++
}

func i32xor(vm *naiveVirtualMachine) {
	vm.operands.push(uint64(uint32(vm.operands.pop()) ^ uint32(vm.operands.pop()))) //nolint
	vm.activeFrame.pc++
}

func i32shl(vm *naiveVirtualMachine) {
	v2 := uint32(vm.operands.pop())
	v1 := uint32(vm.operands.pop())
	vm.operands.push(uint64(v1 << (v2 % 32)))
	vm.activeFrame.pc++
}

func i32shru(vm *naiveVirtualMachine) {
	v2 := uint32(vm.operands.pop())
	v1 := uint32(vm.operands.pop())
	vm.operands.push(uint64(v1 >> (v2 % 32)))
	vm.activeFrame.pc++
}

func i32shrs(vm *naiveVirtualMachine) {
	v2 := uint32(vm.operands.pop())
	v1 := int32(vm.operands.pop())
	vm.operands.push(uint64(v1 >> (v2 % 32)))
	vm.activeFrame.pc++
}

func i32rotl(vm *naiveVirtualMachine) {
	v2 := int(vm.operands.pop())
	v1 := uint32(vm.operands.pop())
	vm.operands.push(uint64(bits.RotateLeft32(v1, v2)))
	vm.activeFrame.pc++
}

func i32rotr(vm *naiveVirtualMachine) {
	v2 := int(vm.operands.pop())
	v1 := uint32(vm.operands.pop())
	vm.operands.push(uint64(bits.RotateLeft32(v1, -v2)))
	vm.activeFrame.pc++
}

// i64
func i64clz(vm *naiveVirtualMachine) {
	vm.operands.push(uint64(bits.LeadingZeros64(vm.operands.pop())))
	vm.activeFrame.pc++
}

func i64ctz(vm *naiveVirtualMachine) {
	vm.operands.push(uint64(bits.TrailingZeros64(vm.operands.pop())))
	vm.activeFrame.pc++
}

func i64popcnt(vm *naiveVirtualMachine) {
	vm.operands.push(uint64(bits.OnesCount64(vm.operands.pop())))
	vm.activeFrame.pc++
}

func i64add(vm *naiveVirtualMachine) {
	vm.operands.push(vm.operands.pop() + vm.operands.pop())
	vm.activeFrame.pc++
}

func i64sub(vm *naiveVirtualMachine) {
	v2 := vm.operands.pop()
	v1 := vm.operands.pop()
	vm.operands.push(v1 - v2)
	vm.activeFrame.pc++
}

func i64mul(vm *naiveVirtualMachine) {
	vm.operands.push(vm.operands.pop() * vm.operands.pop())
	vm.activeFrame.pc++
}

func i64divs(vm *naiveVirtualMachine) {
	v2 := int64(vm.operands.pop())
	v1 := int64(vm.operands.pop())
	if v2 == 0 || (v1 == math.MinInt64 && v2 == -1) {
		panic("undefined")
	}
	vm.operands.push(uint64(v1 / v2))
	vm.activeFrame.pc++
}

func i64divu(vm *naiveVirtualMachine) {
	v2 := vm.operands.pop()
	v1 := vm.operands.pop()
	vm.operands.push(v1 / v2)
	vm.activeFrame.pc++
}

func i64rems(vm *naiveVirtualMachine) {
	v2 := int64(vm.operands.pop())
	v1 := int64(vm.operands.pop())
	vm.operands.push(uint64(v1 % v2))
	vm.activeFrame.pc++
}

func i64remu(vm *naiveVirtualMachine) {
	v2 := vm.operands.pop()
	v1 := vm.operands.pop()
	vm.operands.push(v1 % v2)
	vm.activeFrame.pc++
}

func i64and(vm *naiveVirtualMachine) {
	vm.operands.push(vm.operands.pop() & vm.operands.pop()) //nolint
	vm.activeFrame.pc++
}

func i64or(vm *naiveVirtualMachine) {
	vm.operands.push(vm.operands.pop() | vm.operands.pop()) //nolint
	vm.activeFrame.pc++
}

func i64xor(vm *naiveVirtualMachine) {
	vm.operands.push(vm.operands.pop() ^ vm.operands.pop()) //nolint
	vm.activeFrame.pc++
}

func i64shl(vm *naiveVirtualMachine) {
	v2 := vm.operands.pop()
	v1 := vm.operands.pop()
	vm.operands.push(v1 << (v2 % 64))
	vm.activeFrame.pc++
}

func i64shru(vm *naiveVirtualMachine) {
	v2 := vm.operands.pop()
	v1 := vm.operands.pop()
	vm.operands.push(v1 >> (v2 % 64))
	vm.activeFrame.pc++
}

func i64shrs(vm *naiveVirtualMachine) {
	v2 := vm.operands.pop()
	v1 := int64(vm.operands.pop())
	vm.operands.push(uint64(v1 >> (v2 % 64)))
	vm.activeFrame.pc++
}

func i64rotl(vm *naiveVirtualMachine) {
	v2 := int(vm.operands.pop())
	v1 := vm.operands.pop()
	vm.operands.push(bits.RotateLeft64(v1, v2))
	vm.activeFrame.pc++
}

func i64rotr(vm *naiveVirtualMachine) {
	v2 := int(vm.operands.pop())
	v1 := vm.operands.pop()
	vm.operands.push(bits.RotateLeft64(v1, -v2))
	vm.activeFrame.pc++
}

func f32abs(vm *naiveVirtualMachine) {
	const mask uint32 = 1 << 31
	v := uint32(vm.operands.pop()) &^ mask
	vm.operands.push(uint64(v))
	vm.activeFrame.pc++
}

func f32neg(vm *naiveVirtualMachine) {
	v := -math.Float32frombits(uint32(vm.operands.pop()))
	vm.operands.push(uint64(math.Float32bits(v)))
	vm.activeFrame.pc++
}

func f32ceil(vm *naiveVirtualMachine) {
	v := math.Float32frombits(uint32(vm.operands.pop()))
	vm.operands.push(uint64(math.Float32bits(float32(math.Ceil(float64(v))))))
	vm.activeFrame.pc++
}

func f32floor(vm *naiveVirtualMachine) {
	v := math.Float32frombits(uint32(vm.operands.pop()))
	vm.operands.push(uint64(math.Float32bits(float32(math.Floor(float64(v))))))
	vm.activeFrame.pc++
}

func f32trunc(vm *naiveVirtualMachine) {
	v := math.Float32frombits(uint32(vm.operands.pop()))
	vm.operands.push(uint64(math.Float32bits(float32(math.Trunc(float64(v))))))
	vm.activeFrame.pc++
}

func f32nearest(vm *naiveVirtualMachine) {
	// Borrowed from https://github.com/wasmerio/wasmer/blob/703bb4ee2ffb17b2929a194fc045a7e351b696e2/lib/vm/src/libcalls.rs#L77
	f := math.Float32frombits(uint32(vm.operands.pop()))
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
	vm.operands.push(uint64(math.Float32bits(f)))
	vm.activeFrame.pc++
}

func f32sqrt(vm *naiveVirtualMachine) {
	v := math.Float32frombits(uint32(vm.operands.pop()))
	vm.operands.push(uint64(math.Float32bits(float32(math.Sqrt(float64(v))))))
	vm.activeFrame.pc++
}

func f32add(vm *naiveVirtualMachine) {
	v := math.Float32frombits(uint32(vm.operands.pop())) + math.Float32frombits(uint32(vm.operands.pop()))
	vm.operands.push(uint64(math.Float32bits(v)))
	vm.activeFrame.pc++
}

func f32sub(vm *naiveVirtualMachine) {
	v2 := math.Float32frombits(uint32(vm.operands.pop()))
	v1 := math.Float32frombits(uint32(vm.operands.pop()))
	vm.operands.push(uint64(math.Float32bits(v1 - v2)))
	vm.activeFrame.pc++
}

func f32mul(vm *naiveVirtualMachine) {
	v := math.Float32frombits(uint32(vm.operands.pop())) * math.Float32frombits(uint32(vm.operands.pop()))
	vm.operands.push(uint64(math.Float32bits(v)))
	vm.activeFrame.pc++
}

func f32div(vm *naiveVirtualMachine) {
	v2 := math.Float32frombits(uint32(vm.operands.pop()))
	v1 := math.Float32frombits(uint32(vm.operands.pop()))
	vm.operands.push(uint64(math.Float32bits(v1 / v2)))
	vm.activeFrame.pc++
}

func f32min(vm *naiveVirtualMachine) {
	v2 := math.Float32frombits(uint32(vm.operands.pop()))
	v1 := math.Float32frombits(uint32(vm.operands.pop()))
	vm.operands.push(uint64(math.Float32bits(float32(min(float64(v1), float64(v2))))))
	vm.activeFrame.pc++
}

func f32max(vm *naiveVirtualMachine) {
	v2 := math.Float32frombits(uint32(vm.operands.pop()))
	v1 := math.Float32frombits(uint32(vm.operands.pop()))
	vm.operands.push(uint64(math.Float32bits(float32(max(float64(v1), float64(v2))))))
	vm.activeFrame.pc++
}

func f32copysign(vm *naiveVirtualMachine) {
	v2 := math.Float32frombits(uint32(vm.operands.pop()))
	v1 := math.Float32frombits(uint32(vm.operands.pop()))
	vm.operands.push(uint64(math.Float32bits(float32(math.Copysign(float64(v1), float64(v2))))))
	vm.activeFrame.pc++
}

func f64abs(vm *naiveVirtualMachine) {
	const mask = 1 << 63
	v := vm.operands.pop() &^ mask
	vm.operands.push(v)
	vm.activeFrame.pc++
}

func f64neg(vm *naiveVirtualMachine) {
	v := -math.Float64frombits(vm.operands.pop())
	vm.operands.push(math.Float64bits(v))
	vm.activeFrame.pc++
}

func f64ceil(vm *naiveVirtualMachine) {
	v := math.Float64frombits(vm.operands.pop())
	vm.operands.push(math.Float64bits(math.Ceil(v)))
	vm.activeFrame.pc++
}

func f64floor(vm *naiveVirtualMachine) {
	v := math.Float64frombits(vm.operands.pop())
	vm.operands.push(math.Float64bits(math.Floor(v)))
	vm.activeFrame.pc++
}

func f64trunc(vm *naiveVirtualMachine) {
	v := math.Float64frombits(vm.operands.pop())
	vm.operands.push(math.Float64bits(math.Trunc(v)))
	vm.activeFrame.pc++
}

func f64nearest(vm *naiveVirtualMachine) {
	// Borrowed from https://github.com/wasmerio/wasmer/blob/703bb4ee2ffb17b2929a194fc045a7e351b696e2/lib/vm/src/libcalls.rs#L77
	f := math.Float64frombits(vm.operands.pop())
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
	vm.operands.push(math.Float64bits(f))
	vm.activeFrame.pc++
}

func f64sqrt(vm *naiveVirtualMachine) {
	v := math.Float64frombits(vm.operands.pop())
	vm.operands.push(math.Float64bits(math.Sqrt(v)))
	vm.activeFrame.pc++
}

func f64add(vm *naiveVirtualMachine) {
	v := math.Float64frombits(vm.operands.pop()) + math.Float64frombits(vm.operands.pop())
	vm.operands.push(math.Float64bits(v))
	vm.activeFrame.pc++
}

func f64sub(vm *naiveVirtualMachine) {
	v2 := math.Float64frombits(vm.operands.pop())
	v1 := math.Float64frombits(vm.operands.pop())
	vm.operands.push(math.Float64bits(v1 - v2))
	vm.activeFrame.pc++
}

func f64mul(vm *naiveVirtualMachine) {
	v := math.Float64frombits(vm.operands.pop()) * math.Float64frombits(vm.operands.pop())
	vm.operands.push(math.Float64bits(v))
	vm.activeFrame.pc++
}

func f64div(vm *naiveVirtualMachine) {
	v2 := math.Float64frombits(vm.operands.pop())
	v1 := math.Float64frombits(vm.operands.pop())
	vm.operands.push(math.Float64bits(v1 / v2))
	vm.activeFrame.pc++
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

func f64min(vm *naiveVirtualMachine) {
	v2 := math.Float64frombits(vm.operands.pop())
	v1 := math.Float64frombits(vm.operands.pop())
	vm.operands.push(math.Float64bits(min(v1, v2)))
	vm.activeFrame.pc++
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

func f64max(vm *naiveVirtualMachine) {
	v2 := math.Float64frombits(vm.operands.pop())
	v1 := math.Float64frombits(vm.operands.pop())
	vm.operands.push(math.Float64bits(max(v1, v2)))
	vm.activeFrame.pc++
}

func f64copysign(vm *naiveVirtualMachine) {
	v2 := math.Float64frombits(vm.operands.pop())
	v1 := math.Float64frombits(vm.operands.pop())
	vm.operands.push(math.Float64bits(math.Copysign(v1, v2)))
	vm.activeFrame.pc++
}

func i32wrapi64(vm *naiveVirtualMachine) {
	vm.operands.push(uint64(uint32(vm.operands.pop())))
	vm.activeFrame.pc++
}

func i32truncf32s(vm *naiveVirtualMachine) {
	v := math.Trunc(float64(math.Float32frombits(uint32(vm.operands.pop()))))
	if math.IsNaN(v) {
		panic("invalid conversion")
	} else if v < math.MinInt32 || v > math.MaxInt32 {
		panic("integer overflow")
	}
	vm.operands.push(uint64(int32(v)))
	vm.activeFrame.pc++
}

func i32truncf32u(vm *naiveVirtualMachine) {
	v := math.Trunc(float64(math.Float32frombits(uint32(vm.operands.pop()))))
	if math.IsNaN(v) {
		panic("invalid conversion")
	} else if v < 0 || v > math.MaxUint32 {
		panic("integer overflow")
	}
	vm.operands.push(uint64(uint32(v)))
	vm.activeFrame.pc++
}

func i32truncf64s(vm *naiveVirtualMachine) {
	v := math.Trunc(math.Float64frombits(vm.operands.pop()))
	if math.IsNaN(v) {
		panic("invalid conversion")
	} else if v < math.MinInt32 || v > math.MaxInt32 {
		panic("integer overflow")
	}
	vm.operands.push(uint64(int32(v)))
	vm.activeFrame.pc++
}

func i32truncf64u(vm *naiveVirtualMachine) {
	v := math.Trunc(math.Float64frombits(vm.operands.pop()))
	if math.IsNaN(v) {
		panic("invalid conversion")
	} else if v < 0 || v > math.MaxUint32 {
		panic("integer overflow")
	}
	vm.operands.push(uint64(uint32(v)))
	vm.activeFrame.pc++
}

func i64extendi32s(vm *naiveVirtualMachine) {
	v := int64(int32(vm.operands.pop()))
	vm.operands.push(uint64(v))
	vm.activeFrame.pc++
}

func i64extendi32u(vm *naiveVirtualMachine) {
	v := uint64(uint32(vm.operands.pop()))
	vm.operands.push(v)
	vm.activeFrame.pc++
}

func i64truncf32s(vm *naiveVirtualMachine) {
	v := math.Trunc(float64(math.Float32frombits(uint32(vm.operands.pop()))))
	res := int64(v)
	if math.IsNaN(v) {
		panic("invalid conversion")
	} else if v < math.MinInt64 || v > 0 && res < 0 {
		panic("integer overflow")
	}
	vm.operands.push(uint64(res))
	vm.activeFrame.pc++
}

func i64truncf32u(vm *naiveVirtualMachine) {
	v := math.Trunc(float64(math.Float32frombits(uint32(vm.operands.pop()))))
	res := uint64(v)
	if math.IsNaN(v) {
		panic("invalid conversion")
	} else if v < 0 || v > float64(res) {
		panic("integer overflow")
	}
	vm.operands.push(res)
	vm.activeFrame.pc++
}

func i64truncf64s(vm *naiveVirtualMachine) {
	v := math.Trunc(math.Float64frombits(vm.operands.pop()))
	res := int64(v)
	if math.IsNaN(v) {
		panic("invalid conversion")
	} else if v < math.MinInt64 || v > 0 && res < 0 {
		panic("integer overflow")
	}
	vm.operands.push(uint64(res))
	vm.activeFrame.pc++
}

func i64truncf64u(vm *naiveVirtualMachine) {
	v := math.Trunc(math.Float64frombits(vm.operands.pop()))
	res := uint64(v)
	if math.IsNaN(v) {
		panic("invalid conversion")
	} else if v < 0 || v > float64(res) {
		panic("integer overflow")
	}
	vm.operands.push(res)
	vm.activeFrame.pc++
}

func f32converti32s(vm *naiveVirtualMachine) {
	v := float32(int32(vm.operands.pop()))
	vm.operands.push(uint64(math.Float32bits(v)))
	vm.activeFrame.pc++
}

func f32converti32u(vm *naiveVirtualMachine) {
	v := float32(uint32(vm.operands.pop()))
	vm.operands.push(uint64(math.Float32bits(v)))
	vm.activeFrame.pc++
}

func f32converti64s(vm *naiveVirtualMachine) {
	v := float32(int64(vm.operands.pop()))
	vm.operands.push(uint64(math.Float32bits(v)))
	vm.activeFrame.pc++
}

func f32converti64u(vm *naiveVirtualMachine) {
	v := float32(vm.operands.pop())
	vm.operands.push(uint64(math.Float32bits(v)))
	vm.activeFrame.pc++
}

func f32demotef64(vm *naiveVirtualMachine) {
	v := float32(math.Float64frombits(vm.operands.pop()))
	vm.operands.push(uint64(math.Float32bits(v)))
	vm.activeFrame.pc++
}

func f64converti32s(vm *naiveVirtualMachine) {
	v := float64(int32(vm.operands.pop()))
	vm.operands.push(math.Float64bits(v))
	vm.activeFrame.pc++
}

func f64converti32u(vm *naiveVirtualMachine) {
	v := float64(uint32(vm.operands.pop()))
	vm.operands.push(math.Float64bits(v))
	vm.activeFrame.pc++
}

func f64converti64s(vm *naiveVirtualMachine) {
	v := float64(int64(vm.operands.pop()))
	vm.operands.push(math.Float64bits(v))
	vm.activeFrame.pc++
}

func f64converti64u(vm *naiveVirtualMachine) {
	v := float64(vm.operands.pop())
	vm.operands.push(math.Float64bits(v))
	vm.activeFrame.pc++
}

func f64promotef32(vm *naiveVirtualMachine) {
	v := float64(math.Float32frombits(uint32(vm.operands.pop())))
	vm.operands.push(math.Float64bits(v))
	vm.activeFrame.pc++
}
