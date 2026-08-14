package wasm

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"unsafe"

	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/internal/internalapi"
	"github.com/tetratelabs/wazero/internal/wasmruntime"
)

// Exception represents a thrown WebAssembly exception that has been exposed to guest code via a catch.
type Exception struct {
	// Tag is the tag instance that was thrown.
	Tag *TagInstance
	// Params holds the argument values matching the tag's function type params.
	Params []uint64
	// ParamRefs holds an unordered slice of all Exceptions referenced by exnref typed entries in Params.
	ParamRefs []*Exception
	// ID is the handle guest code holds as an exnref. Zero is reserved for `ref.null exn`.
	ID Reference
	// Origin is where this was thrown, in an engine-specific representation, used for the
	// "originally thrown at" section of an uncaught exception's stack trace.
	Origin any
}

// NewException builds an Exception.
func (s *ExceptionStore) NewException(
	tag *TagInstance, params []uint64, origin any, paramRefs []*Exception,
) *Exception {
	return &Exception{Tag: tag, Params: params, ID: s.nextID(), Origin: origin, ParamRefs: paramRefs}
}

// IsExnref reports whether a value type is an exnref, nullable or not.
func IsExnref(vt ValueType) bool {
	return vt.Kind() == ValueTypeExnref.Kind()
}

// ExnrefInSignature reports whether a function type mentions exnref, which no function
// reachable from the host may.
//
// An exnref names an exception only for the duration of the call into wasm that produced it:
// the handle is an index into that call's own table, and nothing outside can resolve one. So
// a handle passed in from the host names nothing, and one passed out is dead the moment it
// arrives. Rather than accept a value with no meaning in either direction, wazero declines
// to make such a function callable at all.
//
// This says nothing about wasm-to-wasm calls, where a handle never leaves the call that made
// it, so a module is free to define, export and import functions of these types.
func ExnrefInSignature(typ *FunctionType) bool {
	for _, vt := range typ.Params {
		if IsExnref(vt) {
			return true
		}
	}
	for _, vt := range typ.Results {
		if IsExnref(vt) {
			return true
		}
	}
	return false
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

// hostCallable is the api.Function for fn, or one that refuses every call when fn's signature
// puts it out of the host's reach. Everything that hands an api.Function to the host goes
// through here, which is what makes the boundary rule hold wherever the handle came from.
//
// Refusing at the call rather than returning nil keeps the export honest: it exists, its
// definition is there to introspect, and the error says why it cannot be called -- where a nil
// would have been indistinguishable from no such export.
func hostCallable(def *FunctionDefinition, fn api.Function) api.Function {
	if !ExnrefInSignature(def.Functype) {
		return fn
	}
	return &uncallableFunction{def: def}
}

// uncallableFunction is an api.Function that exists only to explain itself. See hostCallable.
type uncallableFunction struct {
	internalapi.WazeroOnly
	def *FunctionDefinition
}

// Definition implements api.Function.
func (u *uncallableFunction) Definition() api.FunctionDefinition { return u.def }

// Call implements api.Function.
func (u *uncallableFunction) Call(context.Context, ...uint64) ([]uint64, error) {
	return nil, u.err()
}

// CallWithStack implements api.Function.
func (u *uncallableFunction) CallWithStack(context.Context, []uint64) error { return u.err() }

func (u *uncallableFunction) err() error {
	return fmt.Errorf("function %s has an exnref in its signature, so it cannot be called from the host",
		u.def.DebugName())
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

// nextID returns the next unused exception handle. It is never zero.
func (s *ExceptionStore) nextID() Reference {
	next := s.last.Add(1)
	if uint64(Reference(next)) != next {
		// The handle space is as wide as a Reference, so this is unreachable where that is
		// 64 bits. Narrower, handing out a wrapped handle would name a live exception.
		panic(wasmruntime.ErrRuntimeTooManyExceptions)
	}
	return Reference(next)
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
