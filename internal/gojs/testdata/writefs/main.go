package writefs

import (
	"fmt"
	"log"
	"os"
	"path"
)

func Main() {
	dir := path.Join(os.TempDir(), "dir")
	err := os.Mkdir(dir, 0o700)
	if err != nil {
		log.Panicln(err)
		return
	}
	defer os.Remove(dir)

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

	// should fail due to non-empty dir
	if err = os.Remove(dir); err == nil {
		log.Panicln("shouldn't be able to remove", dir)
	}

	// shouldn't fail
	if err = os.RemoveAll(dir); err != nil {
		log.Panicln(err)
		return
	}
}
