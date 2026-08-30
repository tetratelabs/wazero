package wasm

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

// exnrefTag is a tag whose params are all exnrefs, for building exceptions that reference
// other exceptions.
func exnrefTag(n int) *TagInstance {
	params := make([]ValueType, n)
	for i := range params {
		params[i] = ValueTypeExnref
	}
	return &TagInstance{Type: &FunctionType{Params: params}}
}

// newExn builds an exception whose exnref params name the given exceptions, the way an
// engine builds one for a raise whose params it holds.
func newExn(s *ExceptionStore, params ...*Exception) *Exception {
	values := make([]uint64, len(params))
	for i, p := range params {
		values[i] = uint64(p.ID)
	}
	return s.NewException(exnrefTag(len(params)), values, nil, params)
}

// named reports whether any slot still names the exception, which is what keeps it
// resolvable for a later call.
func named(s *ExceptionStore, e *Exception) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.m[e.ID]
	return ok
}

func TestExceptionStore_slotNamesAndReleases(t *testing.T) {
	var s ExceptionStore
	exn := newExn(&s)
	var slot Reference

	s.StoreSlot(&slot, exn)
	require.Equal(t, exn.ID, slot)
	require.Equal(t, exn, s.LoadSlot(&slot))
	require.Equal(t, 1, s.Live())

	// Overwriting with ref.null drops the last naming.
	s.StoreSlot(&slot, nil)
	require.Equal(t, Reference(0), slot)
	require.Nil(t, s.LoadSlot(&slot))
	require.Equal(t, 0, s.Live())
}

func TestExceptionStore_countsSlotsSeparately(t *testing.T) {
	var s ExceptionStore
	exn := newExn(&s)
	var a, b Reference

	s.StoreSlot(&a, exn)
	s.StoreSlot(&b, exn)
	require.Equal(t, 1, s.Live())

	// One slot letting go is not enough: the other still names it.
	s.StoreSlot(&a, nil)
	require.True(t, named(&s, exn))

	s.StoreSlot(&b, nil)
	require.False(t, named(&s, exn))
}

func TestExceptionStore_storingWhatIsAlreadyThere(t *testing.T) {
	var s ExceptionStore
	exn := newExn(&s)
	var slot Reference

	s.StoreSlot(&slot, exn)
	// A self-store retains and releases, leaving the slot's single naming intact rather
	// than doubling it or dropping it.
	s.StoreSlot(&slot, exn)
	require.Equal(t, 1, s.Live())

	s.StoreSlot(&slot, nil)
	require.Equal(t, 0, s.Live())
}

func TestExceptionStore_copySlot(t *testing.T) {
	var s ExceptionStore
	exn := newExn(&s)
	var src, dst Reference

	s.StoreSlot(&src, exn)
	s.CopySlot(&dst, &src)
	require.Equal(t, exn.ID, dst)

	// Both slots name it, so clearing the source is not enough.
	s.StoreSlot(&src, nil)
	require.True(t, named(&s, exn))
	s.StoreSlot(&dst, nil)
	require.False(t, named(&s, exn))
}

func TestExceptionStore_paramsNeedNoEntry(t *testing.T) {
	var s ExceptionStore
	child := newExn(&s)
	parent := newExn(&s, child)
	var slot Reference

	// Only the parent is named by a slot. The child stays reachable through the parent,
	// which is what lets a later call unpack it, without an entry of its own.
	s.StoreSlot(&slot, parent)
	require.Equal(t, 1, s.Live())
	require.False(t, named(&s, child))
	require.Equal(t, child, parent.ParamRefs[0])

	s.StoreSlot(&slot, nil)
	require.Equal(t, 0, s.Live())
}

func TestExceptionStore_paramNamedByItsOwnSlot(t *testing.T) {
	var s ExceptionStore
	child := newExn(&s)
	parent := newExn(&s, child)
	var parentSlot, childSlot Reference

	s.StoreSlot(&parentSlot, parent)
	// The guest parks the child somewhere of its own.
	s.StoreSlot(&childSlot, child)

	s.StoreSlot(&parentSlot, nil)
	require.False(t, named(&s, parent))
	require.Equal(t, child, s.LoadSlot(&childSlot))
}

func TestExceptionStore_unreferencedParamStillTraps(t *testing.T) {
	var s ExceptionStore
	child := newExn(&s)

	// A param the raising call could not resolve, which is what host code throwing back a
	// handle from an earlier call looks like. Nothing can reach it, so nothing keeps it
	// alive, but the params still carry the handle for the trap on use.
	parent := s.NewException(exnrefTag(1), []uint64{uint64(child.ID)}, nil, nil)
	require.Zero(t, len(parent.ParamRefs))
	require.Equal(t, uint64(child.ID), parent.Params[0])
}

func TestExceptionStore_loadSlot(t *testing.T) {
	var s ExceptionStore
	exn := newExn(&s)
	var slot Reference

	require.Nil(t, s.LoadSlot(&slot))

	s.StoreSlot(&slot, exn)
	require.Equal(t, exn, s.LoadSlot(&slot))
}

func TestExceptionStore_idsAreNeverReused(t *testing.T) {
	var s ExceptionStore
	first := newExn(&s)
	var slot Reference

	s.StoreSlot(&slot, first)
	s.StoreSlot(&slot, nil)

	// A handle to the released exception must not resolve to whatever comes next.
	second := newExn(&s)
	require.NotEqual(t, first.ID, second.ID)
	s.StoreSlot(&slot, second)
	require.False(t, named(&s, first))
}

// exnrefSlotEngine stands in for either kind of engine, since where a global's value lives
// depends on which: one that owns globals keeps them in its own memory (wazevo, in the
// module context), and one that does not leaves GlobalInstance.Val as the storage (the
// interpreter).
type exnrefSlotEngine struct {
	*mockModuleEngine
	ownsGlobals bool
	globals     map[Index]uint64
}

func (e *exnrefSlotEngine) OwnsGlobals() bool { return e.ownsGlobals }

func (e *exnrefSlotEngine) GetGlobalValue(idx Index) (lo, hi uint64) {
	if !e.ownsGlobals {
		panic("BUG: asked an engine that does not own globals for a global's value")
	}
	return e.globals[idx], 0
}

// moduleWithExnrefSlots builds a minimal instance owning one exnref global naming e, and an
// exnref table holding tableSlots, in the state guest code leaves them: every slot was
// written through a barrier, so the store counts one naming per slot. ownsGlobals picks
// which of the two places the global's value is put, so callers can exercise both engines'
// storage.
//
// The namings are counted from the slots this builds rather than by walking the instance, so
// that a test of how the instance is walked cannot agree with itself.
func moduleWithExnrefSlots(s *ExceptionStore, e *Exception, ownsGlobals bool, tableSlots ...Reference) *ModuleInstance {
	eng := &exnrefSlotEngine{mockModuleEngine: &mockModuleEngine{}, ownsGlobals: ownsGlobals}
	g := &GlobalInstance{Type: GlobalType{ValType: ValueTypeExnref}}
	if ownsGlobals {
		g.Me, g.Index = eng, 0
		eng.globals = map[Index]uint64{0: uint64(e.ID)}
	} else {
		g.Val = uint64(e.ID)
	}
	s.Retain(e) // the global.
	m := &ModuleInstance{
		Source:  &Module{},
		Engine:  eng,
		Globals: []*GlobalInstance{g},
	}
	if len(tableSlots) > 0 {
		m.Tables = []*TableInstance{{Type: ValueTypeExnref, References: tableSlots}}
		for range tableSlots {
			s.Retain(e)
		}
	}
	return m
}

// forEachEngineKind runs f for both places a global's value can live.
func forEachEngineKind(t *testing.T, f func(t *testing.T, ownsGlobals bool)) {
	t.Run("engine owns globals", func(t *testing.T) { f(t, true) })
	t.Run("instance owns globals", func(t *testing.T) { f(t, false) })
}

func TestExceptionStore_moduleSlotsRelease(t *testing.T) {
	forEachEngineKind(t, func(t *testing.T, ownsGlobals bool) {
		var s ExceptionStore
		exn := newExn(&s)

		// Something outside the module names it too, so releasing the module's two slots
		// has to leave that one alone.
		var slot Reference
		s.StoreSlot(&slot, exn)

		m := moduleWithExnrefSlots(&s, exn, ownsGlobals, exn.ID)
		require.True(t, named(&s, exn))

		// Closing the module drops both of its slots' namings, and nothing more.
		s.ReleaseModuleSlots(m)
		require.True(t, named(&s, exn))

		s.StoreSlot(&slot, nil)
		require.False(t, named(&s, exn))
		require.Equal(t, 0, s.Live())
	})
}

func TestExceptionStore_moduleSlotsSkipImports(t *testing.T) {
	forEachEngineKind(t, func(t *testing.T, ownsGlobals bool) {
		var s ExceptionStore
		exn := newExn(&s)

		// The one global and one table are imported, so they belong to the module that
		// defined them and are that module's to release.
		m := moduleWithExnrefSlots(&s, exn, ownsGlobals, exn.ID)
		m.Source = &Module{ImportGlobalCount: 1, ImportTableCount: 1}

		s.ReleaseModuleSlots(m)
		require.True(t, named(&s, exn))
	})
}

// TestExceptionStore_moduleSlotsReadEngineOwnedGlobals pins where the value of an exnref
// global is read from when the engine owns globals: the engine, not GlobalInstance.Val.
//
// Val is only a staging area there -- instantiation writes the initial value, the engine
// copies it into its own memory, and nothing updates Val again -- so a `global.set` of an
// exnref leaves Val naming something other than what the global now holds. Reading it on
// close would release the wrong exception and leak the right one.
func TestExceptionStore_moduleSlotsReadEngineOwnedGlobals(t *testing.T) {
	var s ExceptionStore
	held, stale := newExn(&s), newExn(&s)

	// stale is named by a slot of its own, so releasing it by mistake is visible as a
	// naming that goes away without its own holder letting go.
	var staleSlot Reference
	s.StoreSlot(&staleSlot, stale)

	// No table, so the global is the only thing under test.
	m := moduleWithExnrefSlots(&s, held, true)
	m.Globals[0].Val = uint64(stale.ID) // what instantiation left behind.

	s.ReleaseModuleSlots(m)

	// The global named held, so that is what was released -- and stale is untouched.
	require.False(t, named(&s, held))
	require.True(t, named(&s, stale))

	s.StoreSlot(&staleSlot, nil)
	require.Equal(t, 0, s.Live())
}
