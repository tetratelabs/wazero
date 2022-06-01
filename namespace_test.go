package wazero

import (
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

// TestRuntime_Namespace ensures namespaces are independent.
func TestRuntime_Namespace(t *testing.T) {
	r := NewRuntime()
	defer r.Close(testCtx)

	// Compile a module to add to the runtime
	compiled, err := r.NewModuleBuilder("env").Compile(testCtx, NewCompileConfig())
	require.NoError(t, err)

	// Instantiate "env" into the runtime default namespace (base case)
	require.Nil(t, r.Module("env"))
	m1, err := r.InstantiateModule(testCtx, compiled, NewModuleConfig())
	require.NoError(t, err)
	require.Equal(t, m1, r.Module("env"))

	// NewNamespace does not inherit modules in the default namespace
	ns1 := r.NewNamespace(testCtx)
	require.Nil(t, ns1.Module("env"))

	// Ensure this namespace has a new instance of "env"
	m2, err := ns1.InstantiateModule(testCtx, compiled, NewModuleConfig())
	require.NoError(t, err)
	require.NotSame(t, m1, m2)

	// Ensure the next namespace is similarly independent.
	ns2 := r.NewNamespace(testCtx)
	m3, err := ns2.InstantiateModule(testCtx, compiled, NewModuleConfig())
	require.NoError(t, err)
	require.NotSame(t, m1, m3)
	require.NotSame(t, m2, m3)

	// Ensure we can't re-instantiate the same module multiple times.
	_, err = ns2.InstantiateModule(testCtx, compiled, NewModuleConfig())
	require.EqualError(t, err, "module[env] has already been instantiated")

	// Ensure we can instantiate the same module multiple times.
	m4, err := ns2.InstantiateModule(testCtx, compiled, NewModuleConfig().WithName("env2"))
	require.NoError(t, err)
	require.NotSame(t, m3, m4)

	// Ensure closing one namespace doesn't affect another
	require.NoError(t, ns2.Close(testCtx))
	require.Nil(t, ns2.Module("env"))
	require.Nil(t, ns2.Module("env2"))
	require.Equal(t, m1, r.Module("env"))
	require.Equal(t, m2, ns1.Module("env"))

	// Ensure closing the Runtime closes all namespaces
	require.NoError(t, r.Close(testCtx))
	require.Nil(t, r.Module("env"))
	require.Nil(t, ns1.Module("env"))
}
