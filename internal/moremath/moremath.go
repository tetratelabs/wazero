package internal

import "math"

// math.Min doen't comply with the Wasm spec, so we borrow from the original
// with a change that either one of NaN results in NaN even if another is -Inf.
// https://github.com/golang/go/blob/1d20a362d0ca4898d77865e314ef6f73582daef0/src/math/dim.go#L74-L91
func WasmCompatMin(x, y float64) float64 {
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

// math.Max doen't comply with the Wasm spec, so we borrow from the original
// with a change that either one of NaN results in NaN even if another is Inf.
// https://github.com/golang/go/blob/1d20a362d0ca4898d77865e314ef6f73582daef0/src/math/dim.go#L42-L59
func WasmCompatMax(x, y float64) float64 {
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
