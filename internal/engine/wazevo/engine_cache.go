package wazevo

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"runtime"

	"github.com/tetratelabs/wazero/experimental"
	"github.com/tetratelabs/wazero/internal/engine/wazevo/wazevoapi"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/u32"
	"github.com/tetratelabs/wazero/internal/u64"
	"github.com/tetratelabs/wazero/internal/wasm"
)

func (e *engine) addCompiledModule(module *wasm.Module, cm *compiledModule) (err error) {
	e.addCompiledModuleToMemory(module, cm)
	if !module.IsHostModule && e.fileCache != nil {
		err = e.addCompiledModuleToCache(module, cm)
	}
	return
}

func (e *engine) getCompiledModule(module *wasm.Module, listeners []experimental.FunctionListener, ensureTermination bool) (cm *compiledModule, ok bool, err error) {
	cm, ok = e.getCompiledModuleFromMemory(module)
	if ok {
		return
	}
	cm, ok, err = e.getCompiledModuleFromCache(module)
	if ok {
		cm.parent = e
		cm.module = module
		cm.sharedFunctions = e.sharedFunctions
		cm.ensureTermination = ensureTermination
		cm.offsets = wazevoapi.NewModuleContextOffsetData(module, len(listeners) > 0)
		if len(listeners) > 0 {
			cm.listeners = listeners
			cm.listenerBeforeTrampolines = make([]*byte, len(module.TypeSection))
			cm.listenerAfterTrampolines = make([]*byte, len(module.TypeSection))
			for i := range module.TypeSection {
				typ := &module.TypeSection[i]
				before, after := e.getListenerTrampolineForType(typ)
				cm.listenerBeforeTrampolines[i] = before
				cm.listenerAfterTrampolines[i] = after
			}
		}
		e.addCompiledModuleToMemory(module, cm)
		cm.entryPreambles = make([]*byte, len(module.TypeSection))
		for i := range cm.entryPreambles {
			cm.entryPreambles[i] = e.getEntryPreambleForType(&module.TypeSection[i])
		}
	}
	return
}

func (e *engine) addCompiledModuleToMemory(m *wasm.Module, cm *compiledModule) {
	e.mux.Lock()
	defer e.mux.Unlock()
	e.compiledModules[m.ID] = cm
	if len(cm.executable) > 0 {
		e.addCompiledModuleToSortedList(cm)
	}
}

func (e *engine) getCompiledModuleFromMemory(module *wasm.Module) (cm *compiledModule, ok bool) {
	e.mux.RLock()
	defer e.mux.RUnlock()
	cm, ok = e.compiledModules[module.ID]
	return
}

func (e *engine) addCompiledModuleToCache(module *wasm.Module, cm *compiledModule) (err error) {
	if e.fileCache == nil || module.IsHostModule {
		return
	}
	err = e.fileCache.Add(module.ID, serializeCompiledModule(e.wazeroVersion, cm))
	return
}

func (e *engine) getCompiledModuleFromCache(module *wasm.Module) (cm *compiledModule, hit bool, err error) {
	if e.fileCache == nil || module.IsHostModule {
		return
	}

	// Check if the entries exist in the external cache.
	var cached io.ReadCloser
	cached, hit, err = e.fileCache.Get(module.ID)
	if !hit || err != nil {
		return
	}

	// Otherwise, we hit the cache on external cache.
	// We retrieve *code structures from `cached`.
	var staleCache bool
	// Note: cached.Close is ensured to be called in deserializeCodes.
	cm, staleCache, err = deserializeCompiledModule(e.wazeroVersion, cached)
	if err != nil {
		hit = false
		return
	} else if staleCache {
		return nil, false, e.fileCache.Delete(module.ID)
	}
	return
}

const magic = "WAZEVO"

func serializeCompiledModule(wazeroVersion string, cm *compiledModule) io.Reader {
	buf := bytes.NewBuffer(nil)
	// First 6 byte: WAZEVO header.
	buf.WriteString(magic)
	// Next 1 byte: length of version:
	buf.WriteByte(byte(len(wazeroVersion)))
	// Version of wazero.
	buf.WriteString(wazeroVersion)
	// Number of *code (== locally defined functions in the module): 4 bytes.
	buf.Write(u32.LeBytes(uint32(len(cm.functionOffsets))))
	for _, offset := range cm.functionOffsets {
		// The offset of this function in the executable (8 bytes).
		buf.Write(u64.LeBytes(uint64(offset)))
	}
	// The length of code segment (8 bytes).
	buf.Write(u64.LeBytes(uint64(len(cm.executable))))
	// Append the native code.
	buf.Write(cm.executable)
	return bytes.NewReader(buf.Bytes())
}

func deserializeCompiledModule(wazeroVersion string, reader io.ReadCloser) (cm *compiledModule, staleCache bool, err error) {
	defer reader.Close()
	cacheHeaderSize := len(magic) + 1 /* version size */ + len(wazeroVersion) + 4 /* number of functions */

	// Read the header before the native code.
	header := make([]byte, cacheHeaderSize)
	n, err := reader.Read(header)
	if err != nil {
		return nil, false, fmt.Errorf("compilationcache: error reading header: %v", err)
	}

	if n != cacheHeaderSize {
		return nil, false, fmt.Errorf("compilationcache: invalid header length: %d", n)
	}

	for i := 0; i < len(magic); i++ {
		if magic[i] != header[i] {
			return nil, false, fmt.Errorf(
				"compilationcache: invalid magic number: got %s but want %s", magic, header[:len(magic)])
		}
	}

	// Check the version compatibility.
	versionSize := int(header[len(magic)])

	cachedVersionBegin, cachedVersionEnd := len(magic)+1, len(magic)+1+versionSize
	if cachedVersionEnd >= len(header) {
		staleCache = true
		return
	} else if cachedVersion := string(header[cachedVersionBegin:cachedVersionEnd]); cachedVersion != wazeroVersion {
		staleCache = true
		return
	}

	functionsNum := binary.LittleEndian.Uint32(header[len(header)-4:])
	cm = &compiledModule{functionOffsets: make([]int, functionsNum)}

	var eightBytes [8]byte
	for i := uint32(0); i < functionsNum; i++ {
		// Read the offset of each function in the executable.
		var offset uint64
		if offset, err = readUint64(reader, &eightBytes); err != nil {
			err = fmt.Errorf("compilationcache: error reading func[%d] executable offset: %v", i, err)
			return
		}
		cm.functionOffsets[i] = int(offset)
	}

	executableLen, err := readUint64(reader, &eightBytes)
	if err != nil {
		err = fmt.Errorf("compilationcache: error reading executable size: %v", err)
		return
	}

	if executableLen > 0 {
		executable, err := platform.MmapCodeSegment(int(executableLen))
		if err != nil {
			err = fmt.Errorf("compilationcache: error mmapping executable (len=%d): %v", executableLen, err)
			return nil, false, err
		}

		_, err = io.ReadFull(reader, executable)
		if err != nil {
			err = fmt.Errorf("compilationcache: error reading executable (len=%d): %v", executableLen, err)
			return nil, false, err
		}

		if runtime.GOARCH == "arm64" {
			// On arm64, we cannot give all of rwx at the same time, so we change it to exec.
			if err = platform.MprotectRX(executable); err != nil {
				return nil, false, err
			}
		}
		cm.executable = executable
	}
	return
}

// readUint64 strictly reads an uint64 in little-endian byte order, using the
// given array as a buffer. This returns io.EOF if less than 8 bytes were read.
func readUint64(reader io.Reader, b *[8]byte) (uint64, error) {
	s := b[0:8]
	n, err := reader.Read(s)
	if err != nil {
		return 0, err
	} else if n < 8 { // more strict than reader.Read
		return 0, io.EOF
	}

	// Read the u64 from the underlying buffer.
	ret := binary.LittleEndian.Uint64(s)
	return ret, nil
}
