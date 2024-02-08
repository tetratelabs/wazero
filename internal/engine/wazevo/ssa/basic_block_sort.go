//go:build go1.21

package ssa

import (
	"slices"
)

func sortBlocks(blocks []*basicBlock) {
	slices.SortFunc(blocks, func(i, j *basicBlock) int {
		if j.ReturnBlock() {
			return 1
		}
		if i.ReturnBlock() {
			return -1
		}
		iRoot, jRoot := i.rootInstr, j.rootInstr
		if iRoot == nil || jRoot == nil { // For testing.
			return 1
		}
		return j.rootInstr.id - i.rootInstr.id
	})
}
