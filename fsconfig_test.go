package wazero

import (
	"testing"

	"github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/internal/sysfs"
	testfs "github.com/tetratelabs/wazero/internal/testing/fs"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

// TestFSConfig only tests the cases that change the inputs to sysfs.ValidatePreopens.
func TestFSConfig(t *testing.T) {
	base := NewFSConfig()

	testFS := testfs.FS{}
	testFS2 := testfs.FS{"/": &testfs.File{}}

	tests := []struct {
		name               string
		input              FSConfig
		expectedFS         []sys.FS
		expectedGuestPaths []string
	}{
		{
			name:  "empty",
			input: base,
		},
		{
			name:               "WithFSMount",
			input:              base.WithFSMount(testFS, "/"),
			expectedFS:         []sys.FS{&sysfs.AdaptFS{FS: testFS}},
			expectedGuestPaths: []string{"/"},
		},
		{
			name:               "WithFSMount overwrites",
			input:              base.WithFSMount(testFS, "/").WithFSMount(testFS2, "/"),
			expectedFS:         []sys.FS{&sysfs.AdaptFS{FS: testFS2}},
			expectedGuestPaths: []string{"/"},
		},
		{
			name:  "WithFsMount nil",
			input: base.WithFSMount(nil, "/"),
		},
		{
			name:               "WithDirMount overwrites",
			input:              base.WithFSMount(testFS, "/").WithDirMount(".", "/"),
			expectedFS:         []sys.FS{sysfs.DirFS(".")},
			expectedGuestPaths: []string{"/"},
		},
		{
			name:               "multiple",
			input:              base.WithReadOnlyDirMount(".", "/").WithDirMount("/tmp", "/tmp"),
			expectedFS:         []sys.FS{&sysfs.ReadFS{FS: sysfs.DirFS(".")}, sysfs.DirFS("/tmp")},
			expectedGuestPaths: []string{"/", "/tmp"},
		},
	}

	for _, tt := range tests {
		tc := tt

		t.Run(tc.name, func(t *testing.T) {
			fs, guestPaths := tc.input.(*fsConfig).preopens()
			require.Equal(t, tc.expectedFS, fs)
			require.Equal(t, tc.expectedGuestPaths, guestPaths)
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
