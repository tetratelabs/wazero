package compiler

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"

	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/u32"
	"github.com/tetratelabs/wazero/internal/u64"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func (e *engine) deleteCodes(module *wasm.Module) {
	e.mux.Lock()
	defer e.mux.Unlock()
	delete(e.codes, module.ID)

	// Note: we do not call e.externCache.Delete, as the lifetime of
	// the content is up to the implementation of extencache.ExternCache interface.
}

func (e *engine) addCodes(module *wasm.Module, codes []*code) (err error) {
	e.addCodesToMemory(module, codes)
	err = e.addCodesToExternCache(module, codes)
	return
}

func (e *engine) getCodes(module *wasm.Module) (codes []*code, ok bool, err error) {
	codes, ok = e.getCodesFromMemory(module)
	if ok {
		return
	}
	codes, ok, err = e.getCodesFromExternCache(module)
	if ok {
		e.addCodesToMemory(module, codes)
	}
	return
}

func (e *engine) addCodesToMemory(module *wasm.Module, codes []*code) {
	e.mux.Lock()
	defer e.mux.Unlock()
	e.codes[module.ID] = codes
}

func (e *engine) getCodesFromMemory(module *wasm.Module) (codes []*code, ok bool) {
	e.mux.RLock()
	defer e.mux.RUnlock()
	codes, ok = e.codes[module.ID]
	return
}

func (e *engine) addCodesToExternCache(module *wasm.Module, codes []*code) (err error) {
	if e.externCache == nil {
		return
	}
	err = e.externCache.Add(module.ID, serializeCodes(codes))
	return
}

func (e *engine) getCodesFromExternCache(module *wasm.Module) (codes []*code, hit bool, err error) {
	if e.externCache == nil {
		return
	}

	// Check if the entries exist in the external cache.
	var cached io.ReadCloser
	cached, hit, err = e.externCache.Get(module.ID)
	if !hit || err != nil {
		return
	}
	defer cached.Close()

	// Otherwise, we hit the cache on external cache.
	// We retrieve *code structures from `cached`.
	var staleCache bool
	codes, staleCache, err = deserializeCodes(cached)
	if err != nil {
		hit = false
		return
	} else if staleCache {
		return nil, false, e.externCache.Delete(module.ID)
	}

	for i, c := range codes {
		c.indexInModule = wasm.Index(i)
		c.sourceModule = module
	}
	return
}

var (
	wazeroMagic     = "WAZERO"
	version         = "1.0.0-dev"
	cacheHeaderSize = len(wazeroMagic) + 1 /* version size */ + len(version) + 4 /* number of functions */
)

func serializeCodes(codes []*code) io.Reader {
	buf := bytes.NewBuffer(nil)
	// First 6 byte: WAZERO header.
	buf.WriteString(wazeroMagic)
	// Next 1 byte: length of version:
	buf.WriteByte(byte(len(version)))
	// Version of wazero.
	buf.WriteString(version)
	// Number of *code (== locally defined functions in the module): 4 bytes.
	buf.Write(u32.LeBytes(uint32(len(codes))))
	for _, c := range codes {
		// The stack pointer ceil (8 bytes).
		buf.Write(u64.LeBytes(c.stackPointerCeil))
		// The length of code segment (8 bytes).
		buf.Write(u64.LeBytes(uint64(len(c.codeSegment))))
		// Append the native code.
		buf.Write(c.codeSegment)
	}
	return bytes.NewReader(buf.Bytes())
}

func deserializeCodes(reader io.Reader) (codes []*code, staleCache bool, err error) {
	// Read the header before the native code.
	header := make([]byte, cacheHeaderSize)
	n, err := reader.Read(header)
	if err != nil {
		return nil, false, err
	}

	if n != cacheHeaderSize {
		return nil, false, fmt.Errorf("invalid header length: %d", n)
	}

	// Check the version compatibility.
	versionSize := int(header[len(wazeroMagic)])
	cachedVersion := string(header[len(wazeroMagic)+1 : len(wazeroMagic)+1+versionSize])
	if cachedVersion != version {
		staleCache = true
		return
	}

	functionsNum := binary.LittleEndian.Uint32(header[len(header)-4:])
	codes = make([]*code, 0, functionsNum)

	var eightBytes [8]byte
	for i := uint32(0); i < functionsNum; i++ {
		c := &code{}

		// Read the stack pointer ceil.
		_, err = reader.Read(eightBytes[:])
		if err != nil {
			err = fmt.Errorf("reading stack pointer ceil: %v", err)
			break
		}

		c.stackPointerCeil = binary.LittleEndian.Uint64(eightBytes[:])

		// Read (and mmap) the native code.
		_, err = reader.Read(eightBytes[:])
		if err != nil {
			err = fmt.Errorf("reading native code size: %v", err)
			break
		}

		c.codeSegment, err = platform.MmapCodeSegment(reader, int(binary.LittleEndian.Uint64(eightBytes[:])))
		if err != nil {
			err = fmt.Errorf("mmaping function: %v", err)
			break
		}
		codes = append(codes, c)
	}

	if err != nil {
		for _, c := range codes {
			if errMunmap := platform.MunmapCodeSegment(c.codeSegment); errMunmap != nil {
				// Munmap failure shouldn't happen.
				panic(errMunmap)
			}
		}
		codes = nil
	}
	return
}
