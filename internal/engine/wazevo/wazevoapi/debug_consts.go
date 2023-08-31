package wazevoapi

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"time"
)

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
	PrintSSA                                 = false
	PrintOptimizedSSA                        = false
	PrintBlockLaidOutSSA                     = false
	PrintSSAToBackendIRLowering              = false
	PrintRegisterAllocated                   = false
	PrintFinalizedMachineCode                = false
	PrintMachineCodeHexPerFunction           = printMachineCodeHexPerFunctionUnmodified || PrintMachineCodeHexPerFunctionDisassemblable //nolint
	printMachineCodeHexPerFunctionUnmodified = false
	// PrintMachineCodeHexPerFunctionDisassemblable prints the machine code while modifying the actual result
	// to make it disassemblable. This is useful when debugging the final machine code. See the places where this is used for detail.
	// When this is enabled, functions must not be called.
	PrintMachineCodeHexPerFunctionDisassemblable = false
)

// ----- Validations -----
// These consts must be enabled by default until we reach the point where we can disable them (e.g. multiple days of fuzzing passes).

const (
	RegAllocValidationEnabled = true
	SSAValidationEnabled      = true
)

// ----- Deterministic compilation verifier -----

const (
	// DeterministicCompilationVerifierEnabled enables the deterministic compilation verifier. This is disabled by default
	// since the operation is expensive. But when in doubt, enable this to make sure the compilation is deterministic.
	DeterministicCompilationVerifierEnabled = true
	DeterministicCompilationVerifyingIter   = 20
)

type (
	verifierState struct {
		randomizedIndexes []int
		r                 *rand.Rand
		values            map[string]string
	}
	verifierStateContextKey struct{}
	currentFunctionNameKey  struct{}
)

// NewDeterministicCompilationVerifierContext creates a new context with the deterministic compilation verifier used per wasm.Module.
func NewDeterministicCompilationVerifierContext(ctx context.Context, localFunctions int) context.Context {
	randomizedIndexes := make([]int, localFunctions)
	for i := range randomizedIndexes {
		randomizedIndexes[i] = i
	}
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	return context.WithValue(ctx, verifierStateContextKey{}, &verifierState{
		r: r, randomizedIndexes: randomizedIndexes, values: map[string]string{},
	})
}

// DeterministicCompilationVerifierRandomizeIndexes randomizes the indexes for the deterministic compilation verifier.
// To get the randomized index, use DeterministicCompilationVerifierGetRandomizedLocalFunctionIndex.
func DeterministicCompilationVerifierRandomizeIndexes(ctx context.Context) {
	verifierCtx := ctx.Value(verifierStateContextKey{}).(*verifierState)
	r := verifierCtx.r
	r.Shuffle(len(verifierCtx.randomizedIndexes), func(i, j int) {
		verifierCtx.randomizedIndexes[i], verifierCtx.randomizedIndexes[j] = verifierCtx.randomizedIndexes[j], verifierCtx.randomizedIndexes[i]
	})
}

// DeterministicCompilationVerifierGetRandomizedLocalFunctionIndex returns the randomized index for the given `index`
// which is assigned by DeterministicCompilationVerifierRandomizeIndexes.
func DeterministicCompilationVerifierGetRandomizedLocalFunctionIndex(ctx context.Context, index int) int {
	verifierCtx := ctx.Value(verifierStateContextKey{}).(*verifierState)
	ret := verifierCtx.randomizedIndexes[index]
	return ret
}

// DeterministicCompilationVerifierSetCurrentFunctionName sets the current function name to the given `functionName`.
func DeterministicCompilationVerifierSetCurrentFunctionName(ctx context.Context, functionName string) context.Context {
	return context.WithValue(ctx, currentFunctionNameKey{}, functionName)
}

// VerifyOrSetDeterministicCompilationContextValue verifies that the `newValue` is the same as the previous value for the given `scope`
// and the current function name. If the previous value doesn't exist, it sets the value to the given `newValue`.
//
// If the verification fails, this prints the diff and exits the process.
func VerifyOrSetDeterministicCompilationContextValue(ctx context.Context, scope string, newValue string) {
	fn := ctx.Value(currentFunctionNameKey{}).(string)
	key := fn + ": " + scope
	verifierCtx := ctx.Value(verifierStateContextKey{}).(*verifierState)
	oldValue, ok := verifierCtx.values[key]
	if !ok {
		verifierCtx.values[key] = newValue
		return
	}
	if oldValue != newValue {
		fmt.Printf(
			`BUG: Deterministic compilation failed for function%s at scope="%s".

This is mostly due to (but might not be limited to):
	* Resetting ssa.Builder, backend.Compiler or frontend.Compiler, etc doens't work as expected, and the compilation has been affected by the previous iterations.
	* Using a map with non-deterministic iteration order.

---------- [old] ----------
%s

---------- [new] ----------
%s
`,
			fn, scope, oldValue, newValue,
		)
		os.Exit(1)
	}
}
