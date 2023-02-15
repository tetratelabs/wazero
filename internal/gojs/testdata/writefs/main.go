package writefs

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path"
	"syscall"
	"time"
)

func Main() {
	// Create a test directory
	dir := path.Join(os.TempDir(), "dir")
	dir1 := path.Join(os.TempDir(), "dir1")
	err := os.Mkdir(dir, 0o700)
	if err != nil {
		log.Panicln(err)
		return
	}
	defer os.Remove(dir)

	// Create a test file in that directory
	file := path.Join(dir, "file")
	file1 := path.Join(os.TempDir(), "file1")
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

	// Now, test that syscall.WriteAt works
	f, err := os.OpenFile(file1, os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		log.Panicln(err)
	}
	defer f.Close()

	// Write segments to the file, noting map iteration isn't ordered.
	bytes := []byte("wazero")
	for o, b := range map[int][]byte{3: bytes[3:], 0: bytes[:3]} {
		n, err := f.WriteAt(b, int64(o))
		if err != nil {
			log.Panicln(err)
		} else if n != 3 {
			log.Panicln("expected 3, but wrote", n)
		}
	}

	// Now, use ReadAt (tested in testfs package) to verify everything wrote!
	if _, err = f.ReadAt(bytes, 0); err != nil {
		log.Panicln(err)
	} else if string(bytes) != "wazero" {
		log.Panicln("unexpected contents:", string(bytes))
	}

	// Next, truncate it.
	if err = f.Truncate(2); err != nil {
		log.Panicln(err)
	}
	// Next, sync it.
	if err = f.Sync(); err != nil {
		log.Panicln(err)
	}
	if err = f.Close(); err != nil {
		log.Panicln(err)
	}
	if bytes, err := os.ReadFile(file1); err != nil {
		log.Panicln(err)
	} else if string(bytes) != "wa" {
		log.Panicln("unexpected contents:", string(bytes))
	}

	// Now, truncate it by path
	if err = os.Truncate(file1, 1); err != nil {
		log.Panicln(err)
	} else if bytes, err := os.ReadFile(file1); err != nil {
		log.Panicln(err)
	} else if string(bytes) != "w" {
		log.Panicln("unexpected contents:", string(bytes))
	}

	// Test removing a non-empty empty directory
	if err = syscall.Rmdir(dir); err != syscall.ENOTEMPTY {
		log.Panicln("unexpected error", err)
	}

	// Test updating the mod time of a file, noting JS has millis precision.
	atime := time.Unix(123, 4*1e6)
	mtime := time.Unix(567, 8*1e6)

	// Ensure errors propagate
	if err = os.Chtimes("noexist", atime, mtime); !errors.Is(err, syscall.ENOENT) {
		log.Panicln("unexpected error", err)
	}

	// Now, try a real update.
	if err = os.Chtimes(dir, atime, mtime); err != nil {
		log.Panicln("unexpected error", err)
	}

	// Ensure the times translated properly.
	if st, err := os.Stat(dir); err != nil {
		log.Panicln("unexpected error", err)
	} else {
		atimeNsec, mtimeNsec, _ := statTimes(st)
		fmt.Println("times:", atimeNsec, mtimeNsec)

		// statDeviceInode cannot be tested against real device values because
		// the size of d.Dev (32-bit) in js is smaller than linux (64-bit).
		//
		// We can't test the real inode of dir, though we could /tmp as that
		// file is visible on the host. However, we haven't yet implemented
		// platform.StatDeviceInode on windows, so we couldn't run that test
		// in CI. For now, this only tests there is no compilation problem or
		// runtime panic.
		_, _ = statDeviceInode(st)
	}

	// Test renaming a file, noting we can't verify error numbers as they
	// vary per operating system.
	if err = syscall.Rename(file, dir); err == nil {
		log.Panicln("expected error")
	}
	if err = syscall.Rename(file, file1); err != nil {
		log.Panicln("unexpected error", err)
	}

	// Test renaming a directory
	if err = syscall.Rename(dir, file1); err == nil {
		log.Panicln("expected error")
	}
	if err = syscall.Rename(dir, dir1); err != nil {
		log.Panicln("unexpected error", err)
	}

	// Test unlinking a file
	if err = syscall.Rmdir(file1); err != syscall.ENOTDIR {
		log.Panicln("unexpected error", err)
	}
	if err = syscall.Unlink(file1); err != nil {
		log.Panicln("unexpected error", err)
	}

	// Test removing an empty directory
	if err = syscall.Unlink(dir1); err != syscall.EISDIR {
		log.Panicln("unexpected error", err)
	}
	if err = syscall.Rmdir(dir1); err != nil {
		log.Panicln("unexpected error", err)
	}

	// shouldn't fail
	if err = os.RemoveAll(dir1); err != nil {
		log.Panicln(err)
		return
	}
}
