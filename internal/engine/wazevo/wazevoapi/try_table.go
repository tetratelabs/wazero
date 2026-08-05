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
}
