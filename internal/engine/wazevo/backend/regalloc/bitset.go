package regalloc

import "math/bits"

// vrSet implements a set for virtual registers.
//
// The virtual register type set uses a bitset to minimize the memory footprint,
// registers ids are offseted by the minimum entry, and represented by a bit bit
// set to 1 in the bitset.
type vrSet struct {
	min VRegID
	set bitset
}

func (s *vrSet) contains(id VRegID) bool {
	return s.set.has(uint(id - s.min))
}

func (s *vrSet) insert(id VRegID) {
	if id < s.min {
		panic("inserting a register with a lower id than the minimum")
	}
	s.set.set(uint(id - s.min))
}

func (s *vrSet) reset(minVRegID VRegID) {
	s.min = minVRegID
	s.set.reset()
}

func (s *vrSet) Range(f func(VRegID)) {
	s.set.scan(func(i uint) { f(VRegID(i) + s.min) })
}

type bitset struct {
	bits []uint64
	// Most of the bitset values have short backing arrays, to reduce the memory
	// footprint we use this buffer as backing array for storing up to 320 bits.
	// When more bits need to be stored, the backing array are offloaded to the
	// heap.
	buf [5]uint64
}

func (b *bitset) reset() {
	b.bits, b.buf = b.bits[:0], [5]uint64{}
}

func (b *bitset) scan(f func(uint)) {
	for i, v := range b.bits {
		for j := uint(i * 64); v != 0; j++ {
			n := uint(bits.TrailingZeros64(v))
			j += n
			v >>= n + 1
			f(j)
		}
	}
}

func (b *bitset) has(i uint) bool {
	index, shift := i/64, i%64
	return index < uint(len(b.bits)) && ((b.bits[index] & (1 << shift)) != 0)
}

func (b *bitset) set(i uint) {
	index, shift := i/64, i%64
	if index >= uint(len(b.bits)) {
		if index < uint(len(b.buf)) {
			b.bits = b.buf[:]
		} else {
			b.bits = append(b.bits, make([]uint64, (index+1)-uint(len(b.bits)))...)
			b.buf = [5]uint64{}
		}
	}
	b.bits[index] |= 1 << shift
}
