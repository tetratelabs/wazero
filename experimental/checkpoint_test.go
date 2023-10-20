package experimental_test

import (
	"context"
	_ "embed"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func TestSnapshotNestedWasmInvocation(t *testing.T) {
	ctx := context.Background()

	rt := wazero.NewRuntime(ctx)
	defer rt.Close(ctx)

	sidechannel := 0

	_, err := rt.NewHostModuleBuilder("example").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, snapshotPtr uint32) int32 {
			defer func() {
				sidechannel = 10
			}()
			snapshot := ctx.Value(experimental.SnapshotterKey{}).(experimental.Snapshotter).Snapshot()
			snapshots := ctx.Value(snapshotsKey{}).(*[]experimental.Snapshot)
			idx := len(*snapshots)
			*snapshots = append(*snapshots, snapshot)
			ok := mod.Memory().WriteUint32Le(snapshotPtr, uint32(idx))
			require.True(t, ok)

			_, err := mod.ExportedFunction("restore").Call(ctx, uint64(snapshotPtr))
			require.NoError(t, err)

			return 2
		}).
		Export("snapshot").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, snapshotPtr uint32) {
			idx, ok := mod.Memory().ReadUint32Le(snapshotPtr)
			require.True(t, ok)
			snapshots := ctx.Value(snapshotsKey{}).(*[]experimental.Snapshot)
			snapshot := (*snapshots)[idx]

			snapshot.Restore([]uint64{12})
		}).
		Export("restore").
		Instantiate(ctx)
	require.NoError(t, err)

	mod, err := rt.Instantiate(ctx, snapshotWasm)
	require.NoError(t, err)

	var snapshots []experimental.Snapshot
	ctx = context.WithValue(ctx, snapshotsKey{}, &snapshots)
	ctx = context.WithValue(ctx, experimental.EnableSnapshotterKey{}, struct{}{})

	snapshotPtr := uint64(0)
	res, err := mod.ExportedFunction("snapshot").Call(ctx, snapshotPtr)
	require.NoError(t, err)
	// return value from restore
	require.Equal(t, uint64(12), res[0])
	// Host function defers within the call stack work fine
	require.Equal(t, 10, sidechannel)
}

func TestSnapshotMultipleWasmInvocations(t *testing.T) {
	ctx := context.Background()

	rt := wazero.NewRuntime(ctx)
	defer rt.Close(ctx)

	_, err := rt.NewHostModuleBuilder("example").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, snapshotPtr uint32) int32 {
			snapshot := ctx.Value(experimental.SnapshotterKey{}).(experimental.Snapshotter).Snapshot()
			snapshots := ctx.Value(snapshotsKey{}).(*[]experimental.Snapshot)
			idx := len(*snapshots)
			*snapshots = append(*snapshots, snapshot)
			ok := mod.Memory().WriteUint32Le(snapshotPtr, uint32(idx))
			require.True(t, ok)

			return 0
		}).
		Export("snapshot").
		NewFunctionBuilder().
		WithFunc(func(ctx context.Context, mod api.Module, snapshotPtr uint32) {
			idx, ok := mod.Memory().ReadUint32Le(snapshotPtr)
			require.True(t, ok)
			snapshots := ctx.Value(snapshotsKey{}).(*[]experimental.Snapshot)
			snapshot := (*snapshots)[idx]

			snapshot.Restore([]uint64{12})
		}).
		Export("restore").
		Instantiate(ctx)
	require.NoError(t, err)

	mod, err := rt.Instantiate(ctx, snapshotWasm)
	require.NoError(t, err)

	var snapshots []experimental.Snapshot
	ctx = context.WithValue(ctx, snapshotsKey{}, &snapshots)
	ctx = context.WithValue(ctx, experimental.EnableSnapshotterKey{}, struct{}{})

	snapshotPtr := uint64(0)
	res, err := mod.ExportedFunction("snapshot").Call(ctx, snapshotPtr)
	require.NoError(t, err)
	// snapshot returned zero
	require.Equal(t, uint64(0), res[0])

	res, err = mod.ExportedFunction("restore").Call(ctx, snapshotPtr)
	// Fails, snapshot and restore are called from different wasm invocations.
	require.Error(t, err)
}
