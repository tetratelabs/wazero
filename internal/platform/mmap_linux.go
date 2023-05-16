package platform

import (
	"math/bits"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/tetratelabs/wazero/internal/features"
)

const (
	// https://man7.org/linux/man-pages/man2/mmap.2.html
	__MAP_HUGE_SHIFT = 26
	__MAP_HUGETLB    = 0x40000
)

var (
	hugePagesOnce    sync.Once
	hugePagesConfigs []hugePagesConfig
)

type hugePagesConfig struct {
	size int
	flag int
}

func hasHugePages() bool {
	// Lazy-initialize to give the application an opportunity to enable the
	// "hugepages" feature.
	hugePagesOnce.Do(initHugePageConfigs)
	return len(hugePagesConfigs) != 0
}

func initHugePageConfigs() {
	if !features.Have("hugepages") {
		return
	}
	dirents, err := os.ReadDir("/sys/kernel/mm/hugepages/")
	if err != nil {
		return
	}

	for _, dirent := range dirents {
		name := dirent.Name()
		if !strings.HasPrefix(name, "hugepages-") {
			continue
		}
		if !strings.HasSuffix(name, "kB") {
			continue
		}
		n, err := strconv.ParseUint(name[10:len(name)-2], 10, 64)
		if err != nil {
			continue
		}
		if bits.OnesCount64(n) != 1 {
			continue
		}
		n *= 1024
		hugePagesConfigs = append(hugePagesConfigs, hugePagesConfig{
			size: int(n),
			flag: int(bits.TrailingZeros64(n)<<__MAP_HUGE_SHIFT) | __MAP_HUGETLB,
		})
	}

	sort.Slice(hugePagesConfigs, func(i, j int) bool {
		return hugePagesConfigs[i].size > hugePagesConfigs[j].size
	})
}

func mmapCodeSegment(size, prot int) ([]byte, error) {
	flags := syscall.MAP_ANON | syscall.MAP_PRIVATE

	if hasHugePages() {
		for _, hugePagesConfig := range hugePagesConfigs {
			if (size & (hugePagesConfig.size - 1)) != 0 {
				continue
			}
			b, err := syscall.Mmap(-1, 0, size, prot, flags|hugePagesConfig.flag)
			if err != nil {
				continue
			}
			return b, nil
		}
	}

	return syscall.Mmap(-1, 0, size, prot, flags)
}
