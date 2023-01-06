package syscallfstest

import (
	"errors"
	"sort"
	"testing"

	"github.com/tetratelabs/wazero/internal/syscallfs"
)

// MakeFS is a function type used to instantiate read-write file
// systems during tests.
//
// The function receives the *testing.T instance created to run the test that
// the file system is created for.
//
// If cleanup needs to be done to tear down the file system, the MakeFS function
// must register a cleanup function to the test using testing.(*T).Cleanup.
//
// If the file system creation fails, the function must abort the test by calling
// testing.(*T).Fatal or testing.(*T).Fatalf.
type MakeFS func(*testing.T) syscallfs.FS

// TestFS represents test suites for file systems.
//
// The test suite maps test names to the function implementing the test.
type TestFS map[string]func(*testing.T, syscallfs.FS)

// Run runs the test suite, calling makeFS to create a file system to run each
// test against.
func (tests TestFS) Run(t *testing.T, makeFS MakeFS) {
	names := make([]string, 0, len(tests))
	for name := range tests {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) { tests[name](t, makeFS(t)) })
	}
}

// RunFunc creates a function which can be passed to testing.(*T).Run to run the
// tests.
//
// The makeFS function is called for each test to create a file system for each
// test that compose the suite.
func (tests TestFS) RunFunc(makeFS MakeFS) func(*testing.T) {
	return func(t *testing.T) { tests.Run(t, makeFS) }
}

func expect(want error, test func(syscallfs.FS) error) func(*testing.T, syscallfs.FS) {
	return func(t *testing.T, fsys syscallfs.FS) {
		if got := test(fsys); !errors.Is(got, want) {
			if want == nil {
				t.Errorf("unexpected error: %v", got)
			} else {
				t.Errorf("error mismatch: want=%v got=%v", want, got)
			}
		}
	}
}
