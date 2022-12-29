package writefs

import (
	"fmt"
	"log"
	"os"
	"path"
	"syscall"
)

func Main() {
	// Create a test directory
	dir := path.Join(os.TempDir(), "dir")
	err := os.Mkdir(dir, 0o700)
	if err != nil {
		log.Panicln(err)
		return
	}
	defer os.Remove(dir)

	// Create a test file in that directory
	file := path.Join(dir, "file")
	err = os.WriteFile(file, []byte{}, 0o600)
	if err != nil {
		log.Panicln(err)
		return
	}
	defer os.Remove(file)

	// Ensure stat works, particularly mode.
	for _, path := range []string{dir, file} {
		if stat, err := os.Stat(path); err != nil {
			log.Panicln(err)
		} else {
			fmt.Println(path, "mode", stat.Mode())
		}
	}

	// Test removing a non-empty empty directory
	if err = syscall.Rmdir(dir); err != syscall.ENOTEMPTY {
		log.Panicln("unexpected error", err)
	}

	// Test unlinking a file
	if err = syscall.Rmdir(file); err != syscall.ENOTDIR {
		log.Panicln("unexpected error", err)
	}
	if err = syscall.Unlink(file); err != nil {
		log.Panicln("unexpected error", err)
	}

	// Test removing an empty directory
	if err = syscall.Unlink(dir); err != syscall.EISDIR {
		log.Panicln("unexpected error", err)
	}
	if err = syscall.Rmdir(dir); err != nil {
		log.Panicln("unexpected error", err)
	}

	// shouldn't fail
	if err = os.RemoveAll(dir); err != nil {
		log.Panicln(err)
		return
	}
}
