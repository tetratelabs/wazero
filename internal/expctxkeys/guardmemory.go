package expctxkeys

// GuardPageMemoryKey is a context.Context Value key. Its value must be true
// to enable guard-page-backed linear memory, which lets the compiler omit
// per-access memory bounds checks. See experimental.WithGuardPageMemory.
type GuardPageMemoryKey struct{}
