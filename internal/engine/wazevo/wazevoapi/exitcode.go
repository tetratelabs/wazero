package wazevoapi

// ExitCode is an exit code of an execution of a function.
type ExitCode uint32

const (
	ExitCodeOK ExitCode = iota
	ExitCodeGrowStack
	ExitCodeUnreachable
	ExitCodeMemoryOutOfBounds
)

// String implements fmt.Stringer.
func (e ExitCode) String() string {
	switch e {
	case ExitCodeOK:
		return "ok"
	case ExitCodeGrowStack:
		return "grow_stack"
	case ExitCodeUnreachable:
		return "unreachable"
	case ExitCodeMemoryOutOfBounds:
		return "memory_out_of_bounds"
	}
	panic("TODO")
}
