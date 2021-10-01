package wasi

import (
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"reflect"

	"github.com/mathetake/gasm/hostfunc"
	"github.com/mathetake/gasm/wasm"
)

const (
	wasiUnstableName         = "wasi_unstable"
	wasiSnapshotPreview1Name = "wasi_snapshot_preview1"
)

type fileEntry struct {
	path    string
	fileSys FS
	file    File
}

type WASI struct {
	stdin io.Reader
	stdout,
	stderr io.Writer
	opened map[uint32]fileEntry
}

type Option func(*WASI)

func Stdin(reader io.Reader) Option {
	return func(w *WASI) {
		w.stdin = reader
	}
}

func Stdout(writer io.Writer) Option {
	return func(w *WASI) {
		w.stdout = writer
	}
}

func Stderr(writer io.Writer) Option {
	return func(w *WASI) {
		w.stderr = writer
	}
}

func Preopen(dir string, fileSys FS) Option {
	return func(w *WASI) {
		w.opened[uint32(len(w.opened))+3] = fileEntry{
			path:    dir,
			fileSys: fileSys,
		}
	}
}

func New(opts ...Option) *WASI {
	ret := &WASI{
		stdin:  os.Stdin,
		stdout: os.Stdout,
		stderr: os.Stderr,
		opened: map[uint32]fileEntry{},
	}

	// apply functional options
	for _, f := range opts {
		f(ret)
	}

	return ret
}

func (w *WASI) randUnusedFD() uint32 {
	fd := uint32(rand.Int31())
	for {
		if _, ok := w.opened[fd]; !ok {
			return fd
		}
		fd = (fd + 1) % (1 << 31)
	}
}

func (w *WASI) fd_prestat_get(vm *wasm.VirtualMachine) reflect.Value {
	body := func(fd uint32, bufPtr uint32) (err uint32) {
		if _, ok := w.opened[fd]; !ok {
			return EBADF
		}
		return ESUCCESS
	}
	return reflect.ValueOf(body)
}

func (w *WASI) fd_prestat_dir_name(vm *wasm.VirtualMachine) reflect.Value {
	body := func(fd uint32, pathPtr uint32, pathLen uint32) (err uint32) {
		f, ok := w.opened[fd]
		if !ok {
			return EINVAL
		}

		if uint32(len(f.path)) < pathLen {
			return ENAMETOOLONG
		}

		copy(vm.Memory[pathPtr:], f.path)
		return ESUCCESS
	}
	return reflect.ValueOf(body)
}

func (w *WASI) fd_fdstat_get(vm *wasm.VirtualMachine) reflect.Value {
	body := func(fd uint32, bufPtr uint32) (err uint32) {
		if _, ok := w.opened[fd]; !ok {
			return EBADF
		}
		binary.LittleEndian.PutUint64(vm.Memory[bufPtr+16:], R_FD_READ|R_FD_WRITE)
		return ESUCCESS
	}
	return reflect.ValueOf(body)
}

func (w *WASI) path_open(vm *wasm.VirtualMachine) reflect.Value {
	body := func(
		fd, dirFlags, pathPtr, pathLen, oFlags uint32,
		fsRightsBase, fsRightsInheriting uint64,
		fdFlags, fdPtr uint32,
	) (errno uint32) {
		dir, ok := w.opened[fd]
		if !ok || dir.fileSys == nil {
			return EINVAL
		}

		path := string(vm.Memory[pathPtr : pathPtr+pathLen])
		f, err := dir.fileSys.OpenWASI(dirFlags, path, oFlags, fsRightsBase, fsRightsInheriting, fdFlags)
		if err != nil {
			switch {
			case errors.Is(err, fs.ErrNotExist):
				return ENOENT
			default:
				return EINVAL
			}
		}

		newFD := w.randUnusedFD()

		w.opened[newFD] = fileEntry{
			file: f,
		}

		binary.LittleEndian.PutUint32(vm.Memory[fdPtr:], newFD)
		return ESUCCESS
	}
	return reflect.ValueOf(body)
}

func (w *WASI) fd_write(vm *wasm.VirtualMachine) reflect.Value {
	body := func(fd uint32, iovsPtr uint32, iovsLen uint32, nwrittenPtr uint32) (err uint32) {
		var writer io.Writer

		switch fd {
		case 1:
			writer = w.stdout
		case 2:
			writer = w.stderr
		default:
			f, ok := w.opened[fd]
			if !ok || f.file == nil {
				return EBADF
			}
			writer = f.file
		}

		var nwritten uint32
		for i := uint32(0); i < iovsLen; i++ {
			iovPtr := iovsPtr + i*8
			offset := binary.LittleEndian.Uint32(vm.Memory[iovPtr:])
			l := binary.LittleEndian.Uint32(vm.Memory[iovPtr+4:])
			n, err := writer.Write(vm.Memory[offset : offset+l])
			if err != nil {
				panic(err)
			}
			nwritten += uint32(n)
		}
		binary.LittleEndian.PutUint32(vm.Memory[nwrittenPtr:], nwritten)
		return ESUCCESS
	}
	return reflect.ValueOf(body)
}

func (w *WASI) fd_read(vm *wasm.VirtualMachine) reflect.Value {
	body := func(fd uint32, iovsPtr uint32, iovsLen uint32, nreadPtr uint32) (err uint32) {
		var reader io.Reader

		switch fd {
		case 0:
			reader = w.stdin
		default:
			f, ok := w.opened[fd]
			if !ok || f.file == nil {
				return EBADF
			}
			reader = f.file
		}

		var nread uint32
		for i := uint32(0); i < iovsLen; i++ {
			iovPtr := iovsPtr + i*8
			offset := binary.LittleEndian.Uint32(vm.Memory[iovPtr:])
			l := binary.LittleEndian.Uint32(vm.Memory[iovPtr+4:])
			n, err := reader.Read(vm.Memory[offset : offset+l])
			nread += uint32(n)
			if errors.Is(err, io.EOF) {
				break
			} else if err != nil {
				return EIO
			}
		}
		binary.LittleEndian.PutUint32(vm.Memory[nreadPtr:], nread)
		return ESUCCESS
	}
	return reflect.ValueOf(body)
}

func (w *WASI) fd_close(vm *wasm.VirtualMachine) reflect.Value {
	body := func(fd uint32) (err uint32) {
		f, ok := w.opened[fd]
		if !ok {
			return EBADF
		}

		if f.file != nil {
			f.file.Close()
		}

		delete(w.opened, fd)

		return ESUCCESS
	}
	return reflect.ValueOf(body)
}

func (w *WASI) Modules() map[string]*wasm.Module {
	b := hostfunc.NewModuleBuilder()
	for _, wasiName := range []string{
		wasiUnstableName,
		wasiSnapshotPreview1Name,
	} {
		b.MustSetFunction(wasiName, "proc_exit", proc_exit)
		b.MustSetFunction(wasiName, "fd_write", w.fd_write)
		b.MustSetFunction(wasiName, "environ_sizes_get", environ_sizes_get)
		b.MustSetFunction(wasiName, "environ_get", environ_get)
		b.MustSetFunction(wasiName, "fd_prestat_get", w.fd_prestat_get)
		b.MustSetFunction(wasiName, "fd_prestat_dir_name", w.fd_prestat_dir_name)
		b.MustSetFunction(wasiName, "fd_fdstat_get", w.fd_fdstat_get)
		b.MustSetFunction(wasiName, "fd_close", w.fd_close)
		b.MustSetFunction(wasiName, "fd_read", w.fd_read)
		b.MustSetFunction(wasiName, "path_open", w.path_open)
		b.MustSetFunction(wasiName, "args_get", args_get)
		b.MustSetFunction(wasiName, "args_sizes_get", args_sizes_get)
	}
	return b.Done()
}

func args_sizes_get(vm *wasm.VirtualMachine) reflect.Value {
	body := func(argcPtr uint32, argvPtr uint32) (err uint32) {
		binary.LittleEndian.PutUint32(vm.Memory[argcPtr:], 0)
		binary.LittleEndian.PutUint32(vm.Memory[argvPtr:], 0)
		// not implemented yet
		return 0
	}
	return reflect.ValueOf(body)
}

func args_get(vm *wasm.VirtualMachine) reflect.Value {
	body := func(uint32, uint32) (err uint32) {
		// not implemented yet
		return 0
	}
	return reflect.ValueOf(body)
}

func proc_exit(vm *wasm.VirtualMachine) reflect.Value {
	body := func(uint32) {
		// not implemented yet
	}
	return reflect.ValueOf(body)
}

func environ_sizes_get(vm *wasm.VirtualMachine) reflect.Value {
	body := func(uint32, uint32) (err uint32) {
		// not implemented yet
		return 0
	}
	return reflect.ValueOf(body)
}

func environ_get(vm *wasm.VirtualMachine) reflect.Value {
	body := func(uint32, uint32) (err uint32) {
		// not implemented yet
		return 0
	}
	return reflect.ValueOf(body)
}
