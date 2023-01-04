package values

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/gojs/goos"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_Values(t *testing.T) {
	t.Parallel()

	vs := NewValues()

	err := require.CapturePanic(func() {
		_ = vs.Get(goos.NextID)
	})
	require.EqualError(t, err, "id 18 is out of range 0")

	v1 := "foo"
	id1 := vs.Increment(v1)
	v2 := "bar"
	id2 := vs.Increment(v2)

	require.Equal(t, goos.NextID, id1)
	require.Equal(t, v1, vs.Get(id1))

	// Second value should be at a sequential position
	require.Equal(t, id1+1, id2)
	require.Equal(t, v2, vs.Get(id2))

	// Incrementing the ref count should return the same ID
	require.Equal(t, id1, vs.Increment(v1))
	require.Equal(t, v1, vs.Get(id1))

	// Decrement and we should still get the value
	vs.Decrement(id1)
	require.Equal(t, v1, vs.Get(id1))

	// Decrement again, and we should panic, as go should never attempt to
	// get a value it already decremented to zero.
	vs.Decrement(id1)
	err = require.CapturePanic(func() {
		_ = vs.Get(id1)
	})
	require.EqualError(t, err, "value for 18 was nil")

	// Since the ID is no longer in use, we should be able to revive it.
	require.Equal(t, id1, vs.Increment(v1))
	require.Equal(t, v1, vs.Get(id1))
}
