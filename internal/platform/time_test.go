package platform

import (
	"testing"
	"time"

	"github.com/tetratelabs/wazero/internal/testing/require"
	"github.com/tetratelabs/wazero/sys"
)

func Test_NewFakeWalltime(t *testing.T) {
	wt := NewFakeWalltime()

	// Base should be the same as FakeEpochNanos
	sec, nsec := wt()
	ft := time.UnixMicro(FakeEpochNanos / time.Microsecond.Nanoseconds()).UTC()
	require.Equal(t, ft, time.Unix(sec, int64(nsec)).UTC())

	// next reading should increase by 1ms
	sec, nsec = wt()
	require.Equal(t, ft.Add(time.Millisecond), time.Unix(sec, int64(nsec)).UTC())
}

func Test_NewFakeNanotime(t *testing.T) {
	nt := NewFakeNanotime()

	require.Equal(t, int64(0), nt())

	// next reading should increase by 1ms
	require.Equal(t, int64(time.Millisecond), nt())
}

func Test_Walltime(t *testing.T) {
	now := time.Now().Unix()
	sec, nsec := Walltime()

	// Loose test that the second variant is close to now.
	// The only thing that could flake this is a time adjustment during the test.
	require.True(t, now == sec || now == sec-1)

	// Verify bounds of nanosecond fraction as measuring it precisely won't work.
	require.True(t, nsec >= 0)
	require.True(t, nsec < int32(time.Second.Nanoseconds()))
}

func Test_Nanotime(t *testing.T) {
	tests := []struct {
		name     string
		nanotime sys.Nanotime
	}{
		{"Nanotime", Nanotime},
		{"nanotimePortable", nanotimePortable},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			delta := time.Since(nanoBase).Nanoseconds()
			nanos := Nanotime()

			// It takes more than a nanosecond to make the two clock readings required
			// to implement time.Now. Hence, delta will always be less than nanos.
			require.True(t, delta <= nanos)
		})
	}
}

func Test_Nanotime_ensure_monotonic(t *testing.T) {
	n1 := Nanotime()
	time.Sleep(time.Millisecond)
	n2 := Nanotime()
	require.True(t, n1 < n2)
}

func Test_Nanosleep(t *testing.T) {
	// In CI, Nanosleep(50ms) returned after 197ms.
	// As we can't control the platform clock, we have to be lenient
	ns := int64(50 * time.Millisecond)
	max := ns * 5

	start := Nanotime()
	Nanosleep(ns)
	duration := Nanotime() - start

	require.True(t, duration > 0 && duration < max, "Nanosleep(%d) slept for %d", ns, duration)
}
