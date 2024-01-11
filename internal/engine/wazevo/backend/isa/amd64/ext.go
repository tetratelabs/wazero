package amd64

// extMode represents the mode of extension in movzx/movsx.
type extMode byte

const (
	// extModeBL represents Byte -> Longword.
	extModeBL extMode = iota
	// extModeBQ represents Byte -> Quadword.
	extModeBQ
	// extModeWL represents Word -> Longword.
	extModeWL
	// extModeWQ represents Word -> Quadword.
	extModeWQ
	// extModeLQ represents Longword -> Quadword.
	extModeLQ
)

// String implements fmt.Stringer.
func (e extMode) String() string {
	switch e {
	case extModeBL:
		return "bl"
	case extModeBQ:
		return "bq"
	case extModeWL:
		return "wl"
	case extModeWQ:
		return "wq"
	case extModeLQ:
		return "lq"
	default:
		panic("BUG: invalid ext mode")
	}
}

func (e extMode) sizes() (from, to byte) {
	switch e {
	case extModeBL:
		return 1, 4
	case extModeBQ:
		return 1, 8
	case extModeWL:
		return 2, 4
	case extModeWQ:
		return 2, 8
	case extModeLQ:
		return 4, 8
	default:
		panic("BUG: invalid ext mode")
	}
}
