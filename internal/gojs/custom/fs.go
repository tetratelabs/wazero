package custom

const (
	NameCallback = "callback"

	NameFs        = "fs"
	NameFsOpen    = "open"
	NameFsStat    = "stat"
	NameFsFstat   = "fstat"
	NameFsLstat   = "lstat"
	NameFsClose   = "close"
	NameFsRead    = "read"
	NameFsWrite   = "write"
	NameFsReaddir = "readdir"
	NameFsMkdir   = "mkdir"
	NameFsRmdir   = "rmdir"
	NameFsUnlink  = "unlink"
)

// FsNameSection are the functions defined in the object named NameFs. Results
// here are those set to the current event object, but effectively are results
// of the host function.
var FsNameSection = map[string]*Names{
	NameFsOpen: {
		Name:        NameFsOpen,
		ParamNames:  []string{"name", "flags", "perm", NameCallback},
		ResultNames: []string{"err", "fd"},
	},
	NameFsStat: {
		Name:        NameFsStat,
		ParamNames:  []string{"name", NameCallback},
		ResultNames: []string{"err", "stat"},
	},
	NameFsFstat: {
		Name:        NameFsFstat,
		ParamNames:  []string{"fd", NameCallback},
		ResultNames: []string{"err", "stat"},
	},
	NameFsLstat: {
		Name:        NameFsLstat,
		ParamNames:  []string{"name", NameCallback},
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
		ParamNames:  []string{"name", NameCallback},
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
	NameFsUnlink: {
		Name:        NameFsUnlink,
		ParamNames:  []string{"path", NameCallback},
		ResultNames: []string{"err", "ok"},
	},
}
