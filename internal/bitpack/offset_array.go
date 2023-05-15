package bitpack

import (
	"math"
)

// OffsetArray is an interface representing read-only views of arrays of 64 bits
// offsets.
type OffsetArray interface {
	// Returns the value at index i.
	//
	// The method complexity may be anywhere between O(1) and O(N).
	Index(i int) uint64
	// Returns the number of offsets in the array.
	//
	// The method complexity must be O(1).
	Len() int
}

// OffsetArrayLen is a helper function to access the length of an offset array.
// It is similar to calling Len on the array but handles the special case where
// the array is nil, in which case it returns zero.
func OffsetArrayLen(array OffsetArray) int {
	if array != nil {
		return array.Len()
	}
	return 0
}

// NewOffsetArray constructs a new array of offsets from the slice of values
// passed as argument. The slice is not retained, the returned array always
// holds a copy of the values.
//
// The underlying implementation of the offset array applies a compression
// mechanism derived from Frame-of-Reference and Delta Encoding to minimize
// the memory footprint of the array. This compression model works best when
// the input is made of ordered values, otherwise the deltas between values
// are likely to be too large to benefit from delta encoding.
//
// See https://lemire.me/blog/2012/02/08/effective-compression-using-frame-of-reference-and-delta-coding/
func NewOffsetArray(values []uint64) OffsetArray {
	if len(values) == 0 {
		return emptyOffsetArray{}
	}
	if len(values) <= smallOffsetArrayCapacity {
		return newSmallOffsetArray(values)
	}

	maxDelta := uint64(0)
	lastValue := values[0]
	// TODO: the pre-processing we perform here can be optimized using SIMD
	// instructions.
	for _, value := range values[1:] {
		if delta := value - lastValue; delta > maxDelta {
			maxDelta = delta
		}
		lastValue = value
	}

	switch {
	case maxDelta > math.MaxUint32:
		return newOffsetArray(values)
	case maxDelta > math.MaxUint16:
		return newDeltaArray[uint32](values)
	case maxDelta > math.MaxUint8:
		return newDeltaArray[uint16](values)
	default:
		return newDeltaArray[uint8](values)
	}
}

type offsetArray struct {
	values []uint64
}

func newOffsetArray(values []uint64) *offsetArray {
	a := &offsetArray{
		values: make([]uint64, len(values)),
	}
	copy(a.values, values)
	return a
}

func (a *offsetArray) Index(i int) uint64 {
	return a.values[i]
}

func (a *offsetArray) Len() int {
	return len(a.values)
}

type emptyOffsetArray struct{}

func (emptyOffsetArray) Index(int) uint64 {
	panic("index out of bounds")
}

func (emptyOffsetArray) Len() int {
	return 0
}

const smallOffsetArrayCapacity = 7

type smallOffsetArray struct {
	length int
	values [smallOffsetArrayCapacity]uint64
}

func newSmallOffsetArray(values []uint64) *smallOffsetArray {
	a := &smallOffsetArray{length: len(values)}
	copy(a.values[:], values)
	return a
}

func (a *smallOffsetArray) Index(i int) uint64 {
	if i < 0 || i >= a.length {
		panic("index out of bounds")
	}
	return a.values[i]
}

func (a *smallOffsetArray) Len() int {
	return a.length
}

type uintType interface {
	uint8 | uint16 | uint32 | uint64
}

type deltaArray[T uintType] struct {
	deltas     []T
	firstValue uint64
}

func newDeltaArray[T uintType](values []uint64) *deltaArray[T] {
	a := &deltaArray[T]{
		deltas:     make([]T, len(values)-1),
		firstValue: values[0],
	}
	lastValue := values[0]
	for i, value := range values[1:] {
		a.deltas[i] = T(value - lastValue)
		lastValue = value
	}
	return a
}

func (a *deltaArray[T]) Index(i int) uint64 {
	if i < 0 || i >= a.Len() {
		panic("index out of bounds")
	}
	value := a.firstValue
	// TODO: computing the prefix sum can be vectorized;
	// see https://en.algorithmica.org/hpc/algorithms/prefix/
	for _, delta := range a.deltas[:i] {
		value += uint64(delta)
	}
	return value
}

func (a *deltaArray[T]) Len() int {
	return len(a.deltas) + 1
}
