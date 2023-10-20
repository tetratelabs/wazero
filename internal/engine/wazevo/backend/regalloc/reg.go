package regalloc

import (
	"fmt"
	"math/bits"

	"github.com/tetratelabs/wazero/internal/engine/wazevo/ssa"
)

// VReg represents a register which is assigned to an SSA value. This is used to represent a register in the backend.
// A VReg may or may not be a physical register, and the info of physical register can be obtained by RealReg.
type VReg uint64

// VRegID is the lower 32bit of VReg, which is the pure identifier of VReg without RealReg info.
type VRegID uint32

const (
	MaxVRegID = ^VRegID(0)
)

// RealReg returns the RealReg of this VReg.
func (v VReg) RealReg() RealReg {
	return RealReg(v >> 32)
}

// IsRealReg returns true if this VReg is backed by a physical register.
func (v VReg) IsRealReg() bool {
	return v.RealReg() != RealRegInvalid
}

// FromRealReg returns a VReg from the given RealReg and RegType.
// This is used to represent a specific pre-colored register in the backend.
func FromRealReg(r RealReg, typ RegType) VReg {
	rid := VRegID(r)
	if rid > vRegIDReservedForRealNum {
		panic(fmt.Sprintf("invalid real reg %d", r))
	}
	return VReg(r).SetRealReg(r).SetRegType(typ)
}

// SetRealReg sets the RealReg of this VReg and returns the updated VReg.
func (v VReg) SetRealReg(r RealReg) VReg {
	return VReg(r)<<32 | (v & 0xff_00_ffffffff)
}

// RegType returns the RegType of this VReg.
func (v VReg) RegType() RegType {
	return RegType(v >> 40)
}

// SetRegType sets the RegType of this VReg and returns the updated VReg.
func (v VReg) SetRegType(t RegType) VReg {
	return VReg(t)<<40 | (v & 0x00_ff_ffffffff)
}

// ID returns the VRegID of this VReg.
func (v VReg) ID() VRegID {
	return VRegID(v & 0xffffffff)
}

// Valid returns true if this VReg is Valid.
func (v VReg) Valid() bool {
	return v.ID() != vRegIDInvalid && v.RegType() != RegTypeInvalid
}

// VRegIDMinSet is used to collect the minimum ID by type in a collection of
// virtual registers.
//
// We store the min values + 1 so the zero-value of the VRegIDMinSet is valid.
type VRegIDMinSet [NumRegType]VRegID

func (mins *VRegIDMinSet) Min(t RegType) VRegID {
	return mins[t] - 1
}

func (mins *VRegIDMinSet) Observe(v VReg) {
	if rt, id := v.RegType(), v.ID(); id < (mins[rt] - 1) {
		mins[rt] = id + 1
	}
}

// VRegTable is a data structure designed for fast association of program
// counters to virtual registers.
type VRegTable [NumRegType]VRegTypeTable

func (t *VRegTable) Contains(v VReg) bool {
	return t[v.RegType()].Contains(v.ID())
}

func (t *VRegTable) Lookup(v VReg) programCounter {
	return t[v.RegType()].Lookup(v.ID())
}

func (t *VRegTable) Insert(v VReg, p programCounter) {
	if v.IsRealReg() {
		panic("BUG: cannot insert real registers in virtual register table")
	}
	t[v.RegType()].Insert(v.ID(), p)
}

func (t *VRegTable) Range(f func(VReg, programCounter)) {
	for i := range t {
		t[i].Range(func(id VRegID, p programCounter) {
			f(VReg(id).SetRegType(RegType(i)), p)
		})
	}
}

func (t *VRegTable) Reset(minVRegIDs VRegIDMinSet) {
	for i := range t {
		t[i].Reset(minVRegIDs.Min(RegType(i)))
	}
}

// VRegTypeTable implements a table for virtual registers of a specific type.
//
// The parent VRegTable uses 3 instances of this type to maintain a table for
// each virtual register type.
//
// The virtual register type table uses a bitset to accelerate checking whether
// a virtual register exists in the table. The bitset also helps identify which
// slots in the program counter array are used, since the table is sparse and
// some locations may not be occupied and will have the value zero even though
// zero is a valid program counter value.
type VRegTypeTable struct {
	min VRegID
	set bitset
	pcs []programCounter
}

func (t *VRegTypeTable) Contains(id VRegID) bool {
	return t.set.has(uint(id - t.min))
}

func (t *VRegTypeTable) Lookup(id VRegID) programCounter {
	if id := int(id - t.min); t.set.has(uint(id)) {
		return t.pcs[id]
	}
	return -1
}

func (t *VRegTypeTable) Insert(id VRegID, p programCounter) {
	if p < 0 {
		panic("BUG: cannot insert negative program counter in virtual register table")
	}
	i := int(id - t.min)
	if len(t.pcs) <= i {
		t.pcs = append(t.pcs, make([]programCounter, (i+1)-len(t.pcs))...)
	}
	t.set.set(uint(i))
	t.pcs[i] = p
}

func (t *VRegTypeTable) Range(f func(VRegID, programCounter)) {
	t.set.scan(func(i uint) { f(VRegID(i)+t.min, t.pcs[i]) })
}

func (t *VRegTypeTable) Reset(minVRegID VRegID) {
	t.min = minVRegID
	t.set.reset()
	t.pcs = nil
}

// VRegSet is a data structure designed for fast lookup in a set of virtual
// registers.
//
// The type maintains one instance of VRegTypeSet for each register type:
// invalid, int, and float. It dispatches between the sub sets based on the
// type of virtual registers that are inserted.
//
// Note that there is no way to delete entries, the sets are constructed and
// then used until they are either reset or go unused.
type VRegSet [NumRegType]VRegTypeSet

func (s *VRegSet) Contains(v VReg) bool {
	return s[v.RegType()].Contains(v.ID())
}

func (s *VRegSet) Insert(v VReg) {
	if v.IsRealReg() {
		panic("BUG: cannot insert real registers in virtual register table")
	}
	s[v.RegType()].Insert(v.ID())
}

func (s *VRegSet) Range(f func(VReg)) {
	for i := range s {
		s[i].Range(func(id VRegID) {
			f(VReg(id).SetRegType(RegType(i)))
		})
	}
}

func (s *VRegSet) Reset(minVRegIDs VRegIDMinSet) {
	for i := range s {
		s[i].Reset(minVRegIDs.Min(RegType(i)))
	}
}

// VRegTypeSet implements a set for virtual registers of a specific type.
//
// The parent VRegSet uses 3 instances of this type to maintain a table for each
// virtual register type.
//
// The virtual register type set uses a bitset to minimize the memory footprint,
// registers ids are offseted by the minimum entry, and represented by a bit bit
// set to 1 in the bitset.
type VRegTypeSet struct {
	min VRegID
	set bitset
}

func (s *VRegTypeSet) Contains(id VRegID) bool {
	return s.set.has(uint(id - s.min))
}

func (s *VRegTypeSet) Insert(id VRegID) {
	s.set.set(uint(id - s.min))
}

func (s *VRegTypeSet) Range(f func(VRegID)) {
	s.set.scan(func(i uint) { f(VRegID(i) + s.min) })
}

func (s *VRegTypeSet) Reset(minVRegID VRegID) {
	s.min = minVRegID
	s.set.reset()
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
	b.bits, b.buf = nil, [5]uint64{}
}

func (b *bitset) scan(f func(uint)) {
	for i, v := range b.bits {
		for j := uint(i * 64); v != 0; j++ {
			n := uint(bits.TrailingZeros64(v))
			j += n
			v >>= (n + 1)
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

// RealReg represents a physical register.
type RealReg byte

const RealRegInvalid RealReg = 0

const (
	vRegIDInvalid            VRegID = 1 << 31
	VRegIDNonReservedBegin          = vRegIDReservedForRealNum
	vRegIDReservedForRealNum VRegID = 128
	VRegInvalid                     = VReg(vRegIDInvalid)
)

// String implements fmt.Stringer.
func (r RealReg) String() string {
	switch r {
	case RealRegInvalid:
		return "invalid"
	default:
		return fmt.Sprintf("r%d", r)
	}
}

// String implements fmt.Stringer.
func (v VReg) String() string {
	if v.IsRealReg() {
		return fmt.Sprintf("r%d", v.ID())
	}
	return fmt.Sprintf("v%d?", v.ID())
}

// RegType represents the type of a register.
type RegType byte

const (
	RegTypeInvalid RegType = iota
	RegTypeInt
	RegTypeFloat
	NumRegType
)

// String implements fmt.Stringer.
func (r RegType) String() string {
	switch r {
	case RegTypeInt:
		return "int"
	case RegTypeFloat:
		return "float"
	default:
		return "invalid"
	}
}

// RegTypeOf returns the RegType of the given ssa.Type.
func RegTypeOf(p ssa.Type) RegType {
	switch p {
	case ssa.TypeI32, ssa.TypeI64:
		return RegTypeInt
	case ssa.TypeF32, ssa.TypeF64, ssa.TypeV128:
		return RegTypeFloat
	default:
		panic("invalid type")
	}
}

const RealRegsNumMax = 128
