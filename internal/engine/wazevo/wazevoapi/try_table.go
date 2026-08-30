package wazevoapi

// CatchClauseInstance is a runtime catch clause with resolved tag index.
type CatchClauseInstance struct {
	Kind     byte   // wasm.CatchKindCatch, etc.
	TagIndex uint32 // module-local tag index
}

// TryTableInfo holds try_table metadata assigned during compilation
// and looked up at runtime by try_table ID.
type TryTableInfo struct {
	CatchClauses []CatchClauseInstance
	NumLocals    int
	// ReuseLocals is true for nested same-function try_tables that share
	// the enclosing try_table's locals save area instead of allocating.
	ReuseLocals bool
	// ExnrefLocals lists which of the function's locals hold exnrefs, in order.
	//
	// The locals a handler sees are the ones from the throw, carried across the rewind in
	// the save area -- but the rewind puts the reference counts back the way they were at
	// the try_table's entry, which is not where those values came from. This is what lets
	// the runtime find the handles the save area still names and count them again.
	ExnrefLocals []uint32
}
