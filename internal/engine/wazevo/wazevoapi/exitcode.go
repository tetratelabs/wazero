package wazevoapi

type ExitCode byte

const (
	ExitCodeOK ExitCode = iota
	ExitCodeGrowStack
	ExitCodeUnreachable
	ExitCodeCount
)
