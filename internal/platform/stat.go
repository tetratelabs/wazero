package platform

import "os"

// StatTimes returns platform-specific values if os.FileInfo Sys is available.
// Otherwise, it returns the mod time for all values.
func StatTimes(t os.FileInfo) (atimeSec, atimeNsec, mtimeSec, mtimeNsec, ctimeSec, ctimeNsec int64) {
	if t.Sys() == nil { // possibly fake filesystem
		return mtimes(t)
	}
	return statTimes(t)
}

func mtimes(t os.FileInfo) (int64, int64, int64, int64, int64, int64) {
	mtime := t.ModTime().UnixNano()
	mtimeSec := mtime / 1e9
	mtimeNsec := mtime % 1e9
	return mtimeSec, mtimeNsec, mtimeSec, mtimeNsec, mtimeSec, mtimeNsec
}
