package jit

// init initializes variables for the amd64 architecture
func init() {
	newArchContext = newArchContextImpl
}

// archContext is embedded in callEngine in order to store architecture-specific data.
// For amd64, this is empty.
type archContext struct{}

// newArchContextImpl implements newArchContext for amd64 architecture.
func newArchContextImpl() (ret archContext) { return }
