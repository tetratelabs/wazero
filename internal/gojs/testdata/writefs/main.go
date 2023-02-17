package writefs

import (
	"errors"
	"fmt"
	"io/fs"
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
	// Next, chmod it (tests Fchmod)
	if err = f.Chmod(0o400); err != nil {
		log.Panicln(err)
	}
	if stat, err := f.Stat(); err != nil {
		log.Panicln(err)
	} else if mode := stat.Mode() & fs.ModePerm; mode != 0o400 {
		log.Panicln("expected mode = 0o400", mode)
	}
	// Finally, close it.
	if err = f.Close(); err != nil {
		log.Panicln(err)
	}

	// Revert to writeable
	if err = syscall.Chmod(file1, 0o600); err != nil {
		log.Panicln(err)
	}
	if stat, err := os.Stat(file1); err != nil {
		log.Panicln(err)
	} else if mode := stat.Mode() & fs.ModePerm; mode != 0o600 {
		log.Panicln("expected mode = 0o600", mode)
	}

	// Check the file was truncated.
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
	dirAtimeNsec, dirMtimeNsec, dirDev, dirInode := statFields(dir)
	fmt.Println("dir times:", dirAtimeNsec, dirMtimeNsec)

	// Ensure we were able to read the dev and inode.
	//
	// Note: The size of syscall.Stat_t.Dev (32-bit) in js is smaller than
	// linux (64-bit), so we can't compare its real value against the host.
	if dirDev == 0 {
		log.Panicln("expected dir dev != 0", dirDev)
	}
	if dirInode == 0 {
		log.Panicln("expected dir inode != 0", dirInode)
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

	// Compare stat after renaming.
	atimeNsec, mtimeNsec, dev, inode := statFields(dir1)
	// atime shouldn't change as we didn't access (re-open) the directory.
	if atimeNsec != dirAtimeNsec {
		log.Panicln("expected dir atimeNsec = previous value", atimeNsec, dirAtimeNsec)
	}
	// mtime should change because we renamed the directory.
	if mtimeNsec <= dirMtimeNsec {
		log.Panicln("expected dir mtimeNsec > previous value", mtimeNsec, dirMtimeNsec)
	}
	// dev/inode shouldn't change during rename.
	if dev != dirDev {
		log.Panicln("expected dir dev = previous value", dev, dirDev)
	}
	if inode != dirInode {
		log.Panicln("expected dir inode = previous value", dev, dirInode)
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
