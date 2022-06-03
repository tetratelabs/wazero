package platform

import (
	"context"
	"testing"
	"time"

	"github.com/tetratelabs/wazero/internal/testing/require"
)

func Test_Walltime(t *testing.T) {
	now := time.Now().Unix()
	sec, nsec := Walltime(context.Background())

	// Loose test that the second variant is close to now.
	// The only thing that could flake this is a time adjustment during the test.
	require.True(t, now == sec || now == sec-1)

	// Verify bounds of nanosecond fraction as measuring it precisely won't work.
	require.True(t, nsec >= 0)
	require.True(t, nsec < int32(time.Second.Nanoseconds()))
}

func Test_Nanotime(t *testing.T) {
	delta := time.Since(nanoBase).Nanoseconds()
	nanos := Nanotime(context.Background())

	// It takes more than a nanosecond to make the two clock readings required
	// to implement time.Now. Hence, delta will always be less than nanos.
	require.True(t, delta < nanos)
}
