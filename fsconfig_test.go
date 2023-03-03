package wazero

import (
	"testing"

	"github.com/tetratelabs/wazero/internal/sysfs"
	testfs "github.com/tetratelabs/wazero/internal/testing/fs"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// TestFSConfig only tests the cases that change the inputs to sysfs.NewRootFS.
func TestFSConfig(t *testing.T) {
	base := NewFSConfig()

	testFS := testfs.FS{}
	testFS2 := testfs.FS{"/": &testfs.File{}}

	tests := []struct {
		name     string
		input    FSConfig
		expected sysfs.FS
	}{
		{
			name:     "empty",
			input:    base,
			expected: sysfs.UnimplementedFS{},
		},
		{
			name:     "WithFSMount",
			input:    base.WithFSMount(testFS, "/"),
			expected: sysfs.Adapt(testFS),
		},
		{
			name:     "WithFSMount overwrites",
			input:    base.WithFSMount(testFS, "/").WithFSMount(testFS2, "/"),
			expected: sysfs.Adapt(testFS2),
		},
		{
			name:     "WithFsMount nil",
			input:    base.WithFSMount(nil, "/"),
			expected: sysfs.UnimplementedFS{},
		},
		{
			name:     "WithDirMount overwrites",
			input:    base.WithFSMount(testFS, "/").WithDirMount(".", "/"),
			expected: sysfs.NewDirFS("."),
		},
		{
			name:  "Composition",
			input: base.WithReadOnlyDirMount(".", "/").WithDirMount("/tmp", "/tmp"),
			expected: func() sysfs.FS {
				f, err := sysfs.NewRootFS(
					[]sysfs.FS{sysfs.NewReadFS(sysfs.NewDirFS(".")), sysfs.NewDirFS("/tmp")},
					[]string{"/", "/tmp"},
				)
				require.NoError(t, err)
				return f
			}(),
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			sysCtx, err := tc.input.(*fsConfig).toFS()
			require.NoError(t, err)
			require.Equal(t, tc.expected, sysCtx)
		})
	}
}

func TestFSConfig_Errors(t *testing.T) {
	tests := []struct {
		name        string
		input       FSConfig
		expectedErr string
	}{
		{
			name:        "multi-level path not yet supported",
			input:       NewFSConfig().WithDirMount(".", "/usr/bin"),
			expectedErr: "only single-level guest paths allowed: [.:/usr/bin]",
		},
	}
	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.input.(*fsConfig).toFS()
			require.EqualError(t, err, tc.expectedErr)
		})
	}
}

func TestFSConfig_clone(t *testing.T) {
	fc := NewFSConfig().(*fsConfig)
	fc.guestPathToFS["/"] = 0

	cloned := fc.clone()

	// Make post-clone changes
	fc.guestPaths = []string{"/"}
	fc.guestPathToFS["/"] = 1

	// Ensure the guestPathToFS map is not shared
	require.Equal(t, map[string]int{"/": 1}, fc.guestPathToFS)
	require.Equal(t, map[string]int{"/": 0}, cloned.guestPathToFS)

	// Ensure the guestPaths slice is not shared
	require.Zero(t, len(cloned.guestPaths))
}
