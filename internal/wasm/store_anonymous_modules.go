package wasm

import (
	"sync"
	"unsafe"
)

const numAnonymousModuleListShards = 256

type (
	anonymousModuleList      [numAnonymousModuleListShards]anonymousModuleListShard
	anonymousModuleListShard struct {
		set   map[*ModuleInstance]struct{}
		mutex sync.RWMutex
	}
)

func newAnonymousModuleList() *anonymousModuleList {
	ret := &anonymousModuleList{}
	for i := range ret {
		shard := &ret[i]
		shard.set = make(map[*ModuleInstance]struct{})
	}
	return ret
}

func (s *anonymousModuleList) add(foo *ModuleInstance) {
	shardKey := uintptr(unsafe.Pointer(foo))
	shard := s.getShard(shardKey)
	shard.mutex.Lock()
	shard.set[foo] = struct{}{}
	shard.mutex.Unlock()
}

func (s *anonymousModuleList) delete(foo *ModuleInstance) {
	shardKey := uintptr(unsafe.Pointer(foo))
	shard := s.getShard(shardKey)
	shard.mutex.Lock()
	delete(shard.set, foo)
	shard.mutex.Unlock()
}

func (s *anonymousModuleList) getShard(key uintptr) *anonymousModuleListShard {
	return &s[hashPtr(key)%numAnonymousModuleListShards]
}

func hashPtr(key uintptr) uintptr {
	key = (key ^ (key >> 30)) * 0xbf58476d1ce4e5b9
	key = (key ^ (key >> 27)) * 0x94d049bb133111eb
	key = key ^ (key >> 31)
	return key
}
