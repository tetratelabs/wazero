package wasm

import (
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/tetratelabs/wazero/internal/wasmruntime"
)

// Exception represents a thrown WebAssembly exception.
type Exception struct {
	// Tag is the tag instance that was thrown.
	Tag *TagInstance
	// Params holds the argument values matching the tag's function type params.
	Params []uint64
	// ParamRefs holds an unordered slice of all Exceptions referenced by exnref typed entries in Params.
	ParamRefs []*Exception
	// ID is the handle guest code holds as an exnref. Zero is reserved for `ref.null exn`.
	ID Reference
}

// NewException builds an Exception.
func (s *ExceptionStore) NewException(
	tag *TagInstance, params []uint64, paramRefs []*Exception,
) *Exception {
	return &Exception{Tag: tag, Params: params, ID: s.nextID(), ParamRefs: paramRefs}
}

// IsExnref reports whether a value type is an exnref, nullable or not.
func IsExnref(vt ValueType) bool {
	return vt.Kind() == ValueTypeExnref.Kind()
}

// ExnrefSlot returns the Reference an exnref-typed global holds.
//
// Val is a uint64 because a global holds any value type, so where Reference is narrower
// this addresses half the field. That is sound only because every read and write of an
// exnref global goes through here, so the same half is always used, and the other stays
// zero from initialization.
func ExnrefSlot(g *GlobalInstance) *Reference {
	return (*Reference)(unsafe.Pointer(&g.Val))
}

// ExceptionStore records which exceptions a global or table slot names, and how many slots
// name each, so that the last slot letting go of one releases it. Nothing else is counted:
// a raise in flight, a call's held set, and the params of another exception all hold their
// exception by pointer, for the collector to follow.
//
// It is keyed by the handle guest code carries as an exnref, which is how a slot names an
// exception at all -- an opaque integer the collector cannot follow, which is the whole
// reason this exists.
type ExceptionStore struct {
	// last is the source of exception IDs.
	last atomic.Uint64
	mu   sync.Mutex
	m    map[Reference]exceptionEntry
}

// exceptionEntry is an exception and the number of slots that name it.
type exceptionEntry struct {
	exn   *Exception
	count int32
}

const (
	// exceptionHandleTag is in the high bits of every exception handle where a Reference is
	// 64 bits wide. Guest code cannot see a handle's bits, so they are free to choose, and
	// wazevo finds the handles its locals and operand stack hold by scanning the stack for
	// words equal to one. Untagged, handles would be small integers, which a loop counter
	// equals as often as not; tagged, an ordinary value almost never does.
	exceptionHandleTag      = 0xe4ce << exceptionHandleTagShift
	exceptionHandleTagShift = 48
)

// MayBeExceptionHandle reports whether w is shaped like an exception handle: a cheap test that
// rules out almost every word that is not one, before looking it up.
func MayBeExceptionHandle(w uint64) bool {
	return w>>exceptionHandleTagShift == exceptionHandleTag>>exceptionHandleTagShift
}

// nextID returns the next unused exception handle. It is never zero.
func (s *ExceptionStore) nextID() Reference {
	next := s.last.Add(1)
	h := next | exceptionHandleTag
	if uint64(Reference(h)) != h {
		// A Reference narrower than 64 bits has no room for the tag. Only the interpreter
		// runs there, and it does not scan for handles.
		h = next
	}
	if next >= 1<<exceptionHandleTagShift || uint64(Reference(h)) != h {
		// Handing out a wrapped handle would name a live exception.
		panic(wasmruntime.ErrRuntimeTooManyExceptions)
	}
	return Reference(h)
}

// Live reports how many exceptions a slot still names.
func (s *ExceptionStore) Live() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.m)
}

// StoreSlot is the write barrier for an exnref-typed global or table slot: it records exn
// as named by the slot, writes the slot, and drops what the slot named before. A nil exn
// stores `ref.null exn`.
//
// The slot access happens here rather than in the caller because the sequence must be
// atomic against another barrier on the same slot: globals and tables are shared across a
// store, which wazero allows concurrent calls into.
func (s *ExceptionStore) StoreSlot(slot *Reference, exn *Exception) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old := *slot
	if exn != nil {
		s.retainLocked(exn)
		*slot = exn.ID
	} else {
		*slot = 0
	}
	// Retain-then-release, so storing what was already there is not a special case.
	s.releaseLocked(old)
}

// CopySlot is the write barrier for a slot-to-slot move, which table.copy and table.init
// make. The source is read here rather than by the caller so that what it names cannot be
// dropped between the read and the retain.
func (s *ExceptionStore) CopySlot(dst, src *Reference) {
	s.mu.Lock()
	defer s.mu.Unlock()
	old, moved := *dst, *src
	if e, ok := s.m[moved]; ok {
		s.retainLocked(e.exn)
	}
	*dst = moved
	s.releaseLocked(old)
}

// LoadSlot is the read barrier: it reads an exnref-typed slot and returns what it names,
// under the lock a store takes so a concurrent store cannot drop it in between. It returns
// nil for `ref.null exn`.
func (s *ExceptionStore) LoadSlot(slot *Reference) *Exception {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.m[*slot].exn
}

// Retain records one more slot naming exn, for slots filled without going through a
// barrier: table.grow writes its own.
func (s *ExceptionStore) Retain(exn *Exception) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.retainLocked(exn)
}

// ReleaseModuleSlots drops what a closing module's own exnref globals and tables name.
// Nothing can reach those slots once the instance is gone. Called exactly once per module:
// a second pass would over-release.
//
// There is no counterpart for instantiation, because a module's own slots start out naming
// nothing: the only exnref a constant expression can produce is `ref.null exn`. It cannot
// produce more than that because global.get in a constant expression may only name an
// immutable global, whose own value came from a constant expression in turn. So everything
// these slots ever name was put there by guest code, through a barrier that counted it.
func (s *ExceptionStore) ReleaseModuleSlots(m *ModuleInstance) {
	s.mu.Lock()
	defer s.mu.Unlock()
	forEachModuleSlot(m, func(slot Reference) { s.releaseLocked(slot) })
}

// forEachModuleSlot calls f with each exnref-typed global and table slot the module defines
// itself.
func forEachModuleSlot(m *ModuleInstance, f func(slot Reference)) {
	src := m.Source
	for i := src.ImportGlobalCount; i < Index(len(m.Globals)); i++ {
		if g := m.Globals[i]; IsExnref(g.Type.ValType) {
			if m.Engine.OwnsGlobals() {
				lo, _ := m.Engine.GetGlobalValue(g.Index)
				f(Reference(lo))
			} else {
				f(Reference(g.Val))
			}
		}
	}
	for i := src.ImportTableCount; i < Index(len(m.Tables)); i++ {
		t := m.Tables[i]
		if t == nil || !IsExnref(t.Type) {
			continue
		}
		for j := range t.References {
			f(t.References[j])
		}
	}
}

func (s *ExceptionStore) retainLocked(exn *Exception) {
	if e, ok := s.m[exn.ID]; ok {
		e.count++
		s.m[exn.ID] = e
		return
	}
	if s.m == nil {
		s.m = make(map[Reference]exceptionEntry)
	}
	s.m[exn.ID] = exceptionEntry{exn: exn, count: 1}
}

func (s *ExceptionStore) releaseLocked(handle Reference) {
	e, ok := s.m[handle]
	if !ok {
		return
	}
	if e.count--; e.count > 0 {
		s.m[handle] = e
		return
	}
	// What its params name stays alive as long as it does, through paramRefs.
	delete(s.m, handle)
}
