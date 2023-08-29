package wazevoapi

// These consts are used various places in the wazevo implementations.
// Instead of defining them in each file, we define them here so that we can quickly iterate on
// debugging without spending "where do we have debug logging?" time.

// ----- Debug logging -----
// These consts must be disabled by default. Enable them only when debugging.

const (
	FrontEndLoggingEnabled = false
	SSALoggingEnabled      = false
	RegAllocLoggingEnabled = false
)

// ----- Output prints -----
// These consts must be disabled by default. Enable them only when debugging.

const (
	PrintSSA                       = false
	PrintOptimizedSSA              = false
	PrintBlockLaidOutSSA           = false
	PrintSSAToBackendIRLowering    = false
	PrintRegisterAllocated         = false
	PrintFinalizedMachineCode      = false
	PrintMachineCodeHexPerFunction = false
)

// ----- Validations -----
// These consts must be enabled by default until we reach the point where we can disable them (e.g. multiple days of fuzzing passes).

const (
	RegAllocValidationEnabled = true
	SSAValidationEnabled      = true
)
