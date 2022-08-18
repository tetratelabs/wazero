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

	// Note: we do not call e.Cache.Delete, as the lifetime of
	// the content is up to the implementation of extencache.Cache interface.
}

func (e *engine) addCodes(module *wasm.Module, codes []*code) (err error) {
	e.addCodesToMemory(module, codes)
	err = e.addCodesToCache(module, codes)
	return
}

func (e *engine) getCodes(module *wasm.Module) (codes []*code, ok bool, err error) {
	codes, ok = e.getCodesFromMemory(module)
	if ok {
		return
	}
	codes, ok, err = e.getCodesFromCache(module)
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

func (e *engine) addCodesToCache(module *wasm.Module, codes []*code) (err error) {
	if e.Cache == nil {
		return
	}
	err = e.Cache.Add(module.ID, serializeCodes(e.wazeroVersion, codes))
	return
}

func (e *engine) getCodesFromCache(module *wasm.Module) (codes []*code, hit bool, err error) {
	if e.Cache == nil {
		return
	}

	// Check if the entries exist in the external cache.
	var cached io.ReadCloser
	cached, hit, err = e.Cache.Get(module.ID)
	if !hit || err != nil {
		return
	}
	defer cached.Close()

	// Otherwise, we hit the cache on external cache.
	// We retrieve *code structures from `cached`.
	var staleCache bool
	codes, staleCache, err = deserializeCodes(e.wazeroVersion, cached)
	if err != nil {
		hit = false
		return
	} else if staleCache {
		return nil, false, e.Cache.Delete(module.ID)
	}

	for i, c := range codes {
		c.indexInModule = wasm.Index(i)
		c.sourceModule = module
	}
	return
}

var (
	wazeroMagic = "WAZERO"
	// version must be synced with the tag of the wazero library.

)

func serializeCodes(wazeroVersion string, codes []*code) io.Reader {
	buf := bytes.NewBuffer(nil)
	// First 6 byte: WAZERO header.
	buf.WriteString(wazeroMagic)
	// Next 1 byte: length of version:
	buf.WriteByte(byte(len(wazeroVersion)))
	// Version of wazero.
	buf.WriteString(wazeroVersion)
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

func deserializeCodes(wazeroVersion string, reader io.Reader) (codes []*code, staleCache bool, err error) {
	cacheHeaderSize := len(wazeroMagic) + 1 /* version size */ + len(wazeroVersion) + 4 /* number of functions */

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

	cachedVersionBegin, cachedVersionEnd := len(wazeroMagic)+1, len(wazeroMagic)+1+versionSize
	if cachedVersionEnd >= len(header) {
		staleCache = true
		return
	} else if cachedVersion := string(header[cachedVersionBegin:cachedVersionEnd]); cachedVersion != wazeroVersion {
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
