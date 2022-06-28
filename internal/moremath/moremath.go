package moremath

import (
	"math"
)

// https://www.w3.org/TR/2022/WD-wasm-core-2-20220419/syntax/values.html#floating-point
const (
	// F32CanonicalNaNBits is the 32-bit float where payload's MSB equals 1 and others are all zero.
	F32CanonicalNaNBits = uint32(0x7fc0_0000)
	// F32CanonicalNaNBitsMask can be used to judge the value `v` is canonical nan as "v&F32CanonicalNaNBitsMask == F32CanonicalNaNBits"
	F32CanonicalNaNBitsMask = uint32(0x7fff_ffff)
	// F64CanonicalNaNBits is the 64-bit float where payload's MSB equals 1 and others are all zero.
	F64CanonicalNaNBits = uint64(0x7ff8_0000_0000_0000)
	// F64CanonicalNaNBitsMask can be used to judge the value `v` is canonical nan as "v&F64CanonicalNaNBitsMask == F64CanonicalNaNBits"
	F64CanonicalNaNBitsMask = uint64(0x7fff_ffff_ffff_ffff)
	// F32ArithmeticNaNPayloadMSB is used to extract the most significant bit of payload of 32-bit arithmetic NaN values
	F32ArithmeticNaNPayloadMSB = uint32(0x0040_0000)
	// F32ExponentMask is used to extract the exponent of 32-bit floating point.
	F32ExponentMask = uint32(0x7f80_0000)
	// F32ArithmeticNaNBits is an example 32-bit arithmetic NaN.
	F32ArithmeticNaNBits = F32CanonicalNaNBits | 0b1 // Set first bit to make this as an arithmetic NaN.
	// F64ArithmeticNaNPayloadMSB is used to extract the most significant bit of payload of 64-bit arithmetic NaN values
	F64ArithmeticNaNPayloadMSB = uint64(0x0008_0000_0000_0000)
	// F64ExponentMask is used to extract the exponent of 64-bit floating point.
	F64ExponentMask = uint64(0x7ff0_0000_0000_0000)
	// F64ArithmeticNaNBits is an example 64-bit arithmetic NaN.
	F64ArithmeticNaNBits = F64CanonicalNaNBits | 0b1 // Set first bit to make this as an arithmetic NaN.
)

// f64ReturnNaNBinOp returns a 64-bit NaN value following the NaN propagation procedure as in
// https://www.w3.org/TR/2022/WD-wasm-core-2-20220419/exec/numerics.html#nan-propagation
func f64ReturnNaNBinOp(xb, yb uint64) float64 {
	if (xb&F64CanonicalNaNBitsMask == F64CanonicalNaNBits) || (yb&F64CanonicalNaNBitsMask == F64CanonicalNaNBits) {
		return math.Float64frombits(F64CanonicalNaNBits)
	}
	// This case, we can return *one of* arithmetic value (meaning that this is un-deterministic as pointed by Wasm spec).
	// Here, we return the fixed F64ArithmeticNaNBits to have determinism.
	return math.Float64frombits(F64ArithmeticNaNBits)
}

// f64ReturnNaNUniOp returns a 64-bit NaN value following the NaN propagation procedure as in
// https://www.w3.org/TR/2022/WD-wasm-core-2-20220419/exec/numerics.html#nan-propagation
func f64ReturnNaNUniOp(xb uint64) float64 {
	if xb&F64CanonicalNaNBitsMask == F64CanonicalNaNBits {
		return math.Float64frombits(F64CanonicalNaNBits)
	}
	// This case, we can return *one of* arithmetic value (meaning that this is un-deterministic as pointed by Wasm spec).
	// Here, we return the fixed F64ArithmeticNaNBits to have determinism.
	return math.Float64frombits(F64ArithmeticNaNBits)
}

// f32ReturnNaNBinOp returns a 32-bit NaN value following the NaN propagation procedure as in
// https://www.w3.org/TR/2022/WD-wasm-core-2-20220419/exec/numerics.html#nan-propagation
func f32ReturnNaNBinOp(xb, yb uint32) float32 {
	if (xb&F32CanonicalNaNBitsMask == F32CanonicalNaNBits) || (yb&F32CanonicalNaNBitsMask == F32CanonicalNaNBits) {
		return math.Float32frombits(F32CanonicalNaNBits)
	}
	// This case, we can return *one of* arithmetic value (meaning that this is un-deterministic as pointed by Wasm spec).
	// Here, we return the fixed F32ArithmeticNaNBits to have determinism.
	return math.Float32frombits(F32ArithmeticNaNBits)
}

// f32ReturnNaNUniOp returns a 32-bit NaN value following the NaN propagation procedure as in
// https://www.w3.org/TR/2022/WD-wasm-core-2-20220419/exec/numerics.html#nan-propagation
func f32ReturnNaNUniOp(xb uint32) float32 {
	if xb&F32CanonicalNaNBitsMask == F32CanonicalNaNBits {
		return math.Float32frombits(F32CanonicalNaNBits)
	}
	// This case, we can return *one of* arithmetic value (meaning that this is un-deterministic as pointed by Wasm spec).
	// Here, we return the fixed F32ArithmeticNaNBits to have determinism.
	return math.Float32frombits(F32ArithmeticNaNBits)
}

// WasmCompatMin64 is the Wasm spec compatible variant of math.Min for 64-bit floating points.
func WasmCompatMin64(x, y float64) float64 {
	switch {
	case math.IsNaN(x) || math.IsNaN(y):
		return f64ReturnNaNBinOp(math.Float64bits(x), math.Float64bits(y))
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

// WasmCompatMin32 is the Wasm spec compatible variant of math.Min for 32-bit floating points.
func WasmCompatMin32(x, y float32) float32 {
	x64, y64 := float64(x), float64(y)
	switch {
	case math.IsNaN(x64) || math.IsNaN(y64):
		return f32ReturnNaNBinOp(math.Float32bits(x), math.Float32bits(y))
	case math.IsInf(x64, -1) || math.IsInf(y64, -1):
		return float32(math.Inf(-1))
	case x == 0 && x == y:
		if math.Signbit(x64) {
			return x
		}
		return y
	}
	if x < y {
		return x
	}
	return y
}

// WasmCompatMax64 is the Wasm spec compatible variant of math.Max for 64-bit floating points.
func WasmCompatMax64(x, y float64) float64 {
	switch {
	case math.IsNaN(x) || math.IsNaN(y):
		return f64ReturnNaNBinOp(math.Float64bits(x), math.Float64bits(y))
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

// WasmCompatMax32 is the Wasm spec compatible variant of math.Max for 32-bit floating points.
func WasmCompatMax32(x, y float32) float32 {
	x64, y64 := float64(x), float64(y)
	switch {
	case math.IsNaN(x64) || math.IsNaN(y64):
		return f32ReturnNaNBinOp(math.Float32bits(x), math.Float32bits(y))
	case math.IsInf(x64, 1) || math.IsInf(y64, 1):
		return float32(math.Inf(1))
	case x == 0 && x == y:
		if math.Signbit(x64) {
			return y
		}
		return x
	}
	if x > y {
		return x
	}
	return y
}

// WasmCompatNearestF32 is the Wasm spec compatible variant of math.Round, used for Nearest instruction.
// For example, this converts 1.9 to 2.0, and this has the semantics of LLVM's rint intrinsic.
//
// Ex. math.Round(-4.5) results in -5 while this results in -4.
//
// See https://llvm.org/docs/LangRef.html#llvm-rint-intrinsic.
func WasmCompatNearestF32(f float32) float32 {
	if math.IsNaN(float64(f)) {
		return f32ReturnNaNUniOp(math.Float32bits(f))
	}

	// TODO: look at https://github.com/bytecodealliance/wasmtime/pull/2171 and reconsider this algorithm
	if f != 0 {
		ceil := float32(math.Ceil(float64(f)))
		floor := float32(math.Floor(float64(f)))
		distToCeil := math.Abs(float64(f - ceil))
		distToFloor := math.Abs(float64(f - floor))
		h := ceil / 2.0
		if distToCeil < distToFloor {
			f = ceil
		} else if distToCeil == distToFloor && float32(math.Floor(float64(h))) == h {
			f = ceil
		} else {
			f = floor
		}
	}
	return f
}

// WasmCompatNearestF64 is the Wasm spec compatible variant of math.Round, used for Nearest instruction.
// For example, this converts 1.9 to 2.0, and this has the semantics of LLVM's rint intrinsic.
//
// Ex. math.Round(-4.5) results in -5 while this results in -4.
//
// See https://llvm.org/docs/LangRef.html#llvm-rint-intrinsic.
func WasmCompatNearestF64(f float64) float64 {
	if math.IsNaN(f) {
		return f64ReturnNaNUniOp(math.Float64bits(f))
	}

	// TODO: look at https://github.com/bytecodealliance/wasmtime/pull/2171 and reconsider this algorithm
	if f != 0 {
		ceil := math.Ceil(f)
		floor := math.Floor(f)
		distToCeil := math.Abs(f - ceil)
		distToFloor := math.Abs(f - floor)
		h := ceil / 2.0
		if distToCeil < distToFloor {
			f = ceil
		} else if distToCeil == distToFloor && math.Floor(h) == h {
			f = ceil
		} else {
			f = floor
		}
	}
	return f
}

// WasmCompatCeilF32 is the same as math.Ceil on 32-bit except that
// the returned NaN value follows the Wasm specification on NaN
// propagation.
// https://www.w3.org/TR/2022/WD-wasm-core-2-20220419/exec/numerics.html#nan-propagation
func WasmCompatCeilF32(f float32) float32 {
	if math.IsNaN(float64(f)) {
		return f32ReturnNaNUniOp(math.Float32bits(f))
	}
	return float32(math.Ceil(float64(f)))
}

// WasmCompatCeilF64 is the same as math.Ceil on 64-bit except that
// the returned NaN value follows the Wasm specification on NaN
// propagation.
// https://www.w3.org/TR/2022/WD-wasm-core-2-20220419/exec/numerics.html#nan-propagation
func WasmCompatCeilF64(f float64) float64 {
	if math.IsNaN(f) {
		return f64ReturnNaNUniOp(math.Float64bits(f))
	}
	return math.Ceil(f)
}

// WasmCompatFloorF32 is the same as math.Floor on 32-bit except that
// the returned NaN value follows the Wasm specification on NaN
// propagation.
// https://www.w3.org/TR/2022/WD-wasm-core-2-20220419/exec/numerics.html#nan-propagation
func WasmCompatFloorF32(f float32) float32 {
	if math.IsNaN(float64(f)) {
		return f32ReturnNaNUniOp(math.Float32bits(f))
	}
	return float32(math.Floor(float64(f)))
}

// WasmCompatFloorF64 is the same as math.Floor on 64-bit except that
// the returned NaN value follows the Wasm specification on NaN
// propagation.
// https://www.w3.org/TR/2022/WD-wasm-core-2-20220419/exec/numerics.html#nan-propagation
func WasmCompatFloorF64(f float64) float64 {
	if math.IsNaN(f) {
		return f64ReturnNaNUniOp(math.Float64bits(f))
	}
	return math.Floor(f)
}

// WasmCompatTruncF32 is the same as math.Trunc on 32-bit except that
// the returned NaN value follows the Wasm specification on NaN
// propagation.
// https://www.w3.org/TR/2022/WD-wasm-core-2-20220419/exec/numerics.html#nan-propagation
func WasmCompatTruncF32(f float32) float32 {
	if math.IsNaN(float64(f)) {
		return f32ReturnNaNUniOp(math.Float32bits(f))
	}
	return float32(math.Trunc(float64(f)))
}

// WasmCompatTruncF64 is the same as math.Trunc on 64-bit except that
// the returned NaN value follows the Wasm specification on NaN
// propagation.
// https://www.w3.org/TR/2022/WD-wasm-core-2-20220419/exec/numerics.html#nan-propagation
func WasmCompatTruncF64(f float64) float64 {
	if math.IsNaN(f) {
		return f64ReturnNaNUniOp(math.Float64bits(f))
	}
	return math.Trunc(f)
}
