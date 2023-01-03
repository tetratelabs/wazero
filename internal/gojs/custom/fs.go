package custom

const (
	NameCallback = "callback"

	NameFs          = "fs"
	NameFsOpen      = "open"
	NameFsStat      = "stat"
	NameFsFstat     = "fstat"
	NameFsLstat     = "lstat"
	NameFsClose     = "close"
	NameFsWrite     = "write"
	NameFsRead      = "read"
	NameFsReaddir   = "readdir"
	NameFsMkdir     = "mkdir"
	NameFsRmdir     = "rmdir"
	NameFsRename    = "rename"
	NameFsUnlink    = "unlink"
	NameFsUtimes    = "utimes"
	NameFsChmod     = "chmod"
	NameFsFchmod    = "fchmod"
	NameFsChown     = "chown"
	NameFsFchown    = "fchown"
	NameFsLchown    = "lchown"
	NameFsTruncate  = "truncate"
	NameFsFtruncate = "ftruncate"
	NameFsReadlink  = "readlink"
	NameFsLink      = "link"
	NameFsSymlink   = "symlink"
	NameFsFsync     = "fsync"
)

// FsNameSection are the functions defined in the object named NameFs. Results
// here are those set to the current event object, but effectively are results
// of the host function.
var FsNameSection = map[string]*Names{
	NameFsOpen: {
		Name:        NameFsOpen,
		ParamNames:  []string{"path", "flags", "perm", NameCallback},
		ResultNames: []string{"err", "fd"},
	},
	NameFsStat: {
		Name:        NameFsStat,
		ParamNames:  []string{"path", NameCallback},
		ResultNames: []string{"err", "stat"},
	},
	NameFsFstat: {
		Name:        NameFsFstat,
		ParamNames:  []string{"fd", NameCallback},
		ResultNames: []string{"err", "stat"},
	},
	NameFsLstat: {
		Name:        NameFsLstat,
		ParamNames:  []string{"path", NameCallback},
		ResultNames: []string{"err", "stat"},
	},
	NameFsClose: {
		Name:        NameFsClose,
		ParamNames:  []string{"fd", NameCallback},
		ResultNames: []string{"err", "ok"},
	},
	NameFsRead: {
		Name:        NameFsRead,
		ParamNames:  []string{"fd", "buf", "offset", "byteCount", "fOffset", NameCallback},
		ResultNames: []string{"err", "n"},
	},
	NameFsWrite: {
		Name:        NameFsWrite,
		ParamNames:  []string{"fd", "buf", "offset", "byteCount", "fOffset", NameCallback},
		ResultNames: []string{"err", "n"},
	},
	NameFsReaddir: {
		Name:        NameFsReaddir,
		ParamNames:  []string{"path", NameCallback},
		ResultNames: []string{"err", "dirents"},
	},
	NameFsMkdir: {
		Name:        NameFsMkdir,
		ParamNames:  []string{"path", "perm", NameCallback},
		ResultNames: []string{"err", "fd"},
	},
	NameFsRmdir: {
		Name:        NameFsRmdir,
		ParamNames:  []string{"path", NameCallback},
		ResultNames: []string{"err", "ok"},
	},
	NameFsRename: {
		Name:        NameFsRename,
		ParamNames:  []string{"from", "to", NameCallback},
		ResultNames: []string{"err", "ok"},
	},
	NameFsUnlink: {
		Name:        NameFsUnlink,
		ParamNames:  []string{"path", NameCallback},
		ResultNames: []string{"err", "ok"},
	},
	NameFsUtimes: {
		Name:        NameFsUtimes,
		ParamNames:  []string{"path", "atime", "mtime", NameCallback},
		ResultNames: []string{"err", "ok"},
	},
	NameFsChmod: {
		Name:        NameFsChmod,
		ParamNames:  []string{"path", "mode", NameCallback},
		ResultNames: []string{"err", "ok"},
	},
	NameFsFchmod: {
		Name:        NameFsFchmod,
		ParamNames:  []string{"fd", "mode", NameCallback},
		ResultNames: []string{"err", "ok"},
	},
	NameFsChown: {
		Name:        NameFsChown,
		ParamNames:  []string{"path", "uid", "gid", NameCallback},
		ResultNames: []string{"err", "ok"},
	},
	NameFsFchown: {
		Name:        NameFsFchown,
		ParamNames:  []string{"fd", "uid", "gid", NameCallback},
		ResultNames: []string{"err", "ok"},
	},
	NameFsLchown: {
		Name:        NameFsLchown,
		ParamNames:  []string{"path", "uid", "gid", NameCallback},
		ResultNames: []string{"err", "ok"},
	},
	NameFsTruncate: {
		Name:        NameFsTruncate,
		ParamNames:  []string{"path", "length", NameCallback},
		ResultNames: []string{"err", "ok"},
	},
	NameFsFtruncate: {
		Name:        NameFsFtruncate,
		ParamNames:  []string{"fd", "length", NameCallback},
		ResultNames: []string{"err", "ok"},
	},
	NameFsReadlink: {
		Name:        NameFsReadlink,
		ParamNames:  []string{"path", NameCallback},
		ResultNames: []string{"err", "dst"},
	},
	NameFsLink: {
		Name:        NameFsLink,
		ParamNames:  []string{"path", "link", NameCallback},
		ResultNames: []string{"err", "ok"},
	},
	NameFsSymlink: {
		Name:        NameFsSymlink,
		ParamNames:  []string{"path", "link", NameCallback},
		ResultNames: []string{"err", "ok"},
	},
	NameFsFsync: {
		Name:        NameFsFsync,
		ParamNames:  []string{"fd", NameCallback},
		ResultNames: []string{"err", "ok"},
	},
}
