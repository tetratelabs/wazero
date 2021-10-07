package wasi

import (
	"encoding/binary"
	"errors"
	"io"
	"io/fs"
	"math/rand"
	"os"
	"reflect"

	"github.com/mathetake/gasm/wasm"
)

const (
	wasiUnstableName         = "wasi_unstable"
	wasiSnapshotPreview1Name = "wasi_snapshot_preview1"
)

type WASIEnvirnment struct {
	stdin io.Reader
	stdout,
	stderr io.Writer
	opened map[uint32]fileEntry
}

func (w *WASIEnvirnment) RegisterToVirtualMachine(vm *wasm.VirtualMachine) (err error) {
	for _, wasiName := range []string{
		wasiUnstableName,
		wasiSnapshotPreview1Name,
	} {
		err = vm.AddHostFunction(wasiName, "proc_exit", reflect.ValueOf(proc_exit))
		if err != nil {
			return err
		}
		err = vm.AddHostFunction(wasiName, "fd_write", reflect.ValueOf(w.fd_write))
		if err != nil {
			return err
		}
		err = vm.AddHostFunction(wasiName, "environ_sizes_get", reflect.ValueOf(environ_sizes_get))
		if err != nil {
			return err
		}
		err = vm.AddHostFunction(wasiName, "environ_get", reflect.ValueOf(environ_get))
		if err != nil {
			return err
		}
		err = vm.AddHostFunction(wasiName, "fd_prestat_get", reflect.ValueOf(w.fd_prestat_get))
		if err != nil {
			return err
		}
		err = vm.AddHostFunction(wasiName, "fd_prestat_dir_name", reflect.ValueOf(w.fd_prestat_dir_name))
		if err != nil {
			return err
		}
		err = vm.AddHostFunction(wasiName, "fd_fdstat_get", reflect.ValueOf(w.fd_fdstat_get))
		if err != nil {
			return err
		}
		err = vm.AddHostFunction(wasiName, "fd_close", reflect.ValueOf(w.fd_close))
		if err != nil {
			return err
		}
		err = vm.AddHostFunction(wasiName, "fd_read", reflect.ValueOf(w.fd_read))
		if err != nil {
			return err
		}
		err = vm.AddHostFunction(wasiName, "path_open", reflect.ValueOf(w.path_open))
		if err != nil {
			return err
		}
		err = vm.AddHostFunction(wasiName, "args_get", reflect.ValueOf(args_get))
		if err != nil {
			return err
		}
		err = vm.AddHostFunction(wasiName, "args_sizes_get", reflect.ValueOf(args_sizes_get))
		if err != nil {
			return err
		}
	}
	return nil
}

type fileEntry struct {
	path    string
	fileSys FS
	file    File
}

type Option func(*WASIEnvirnment)

func Stdin(reader io.Reader) Option {
	return func(w *WASIEnvirnment) {
		w.stdin = reader
	}
}

func Stdout(writer io.Writer) Option {
	return func(w *WASIEnvirnment) {
		w.stdout = writer
	}
}

func Stderr(writer io.Writer) Option {
	return func(w *WASIEnvirnment) {
		w.stderr = writer
	}
}

func Preopen(dir string, fileSys FS) Option {
	return func(w *WASIEnvirnment) {
		w.opened[uint32(len(w.opened))+3] = fileEntry{
			path:    dir,
			fileSys: fileSys,
		}
	}
}

func NewEnvironment(opts ...Option) *WASIEnvirnment {
	ret := &WASIEnvirnment{
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

func (w *WASIEnvirnment) randUnusedFD() uint32 {
	fd := uint32(rand.Int31())
	for {
		if _, ok := w.opened[fd]; !ok {
			return fd
		}
		fd = (fd + 1) % (1 << 31)
	}
}

func (w *WASIEnvirnment) fd_prestat_get(vm *wasm.VirtualMachine, fd uint32, bufPtr uint32) (err uint32) {
	if _, ok := w.opened[fd]; !ok {
		return EBADF
	}
	return ESUCCESS
}

func (w *WASIEnvirnment) fd_prestat_dir_name(vm *wasm.VirtualMachine, fd uint32, pathPtr uint32, pathLen uint32) (err uint32) {
	f, ok := w.opened[fd]
	if !ok {
		return EINVAL
	}

	if uint32(len(f.path)) < pathLen {
		return ENAMETOOLONG
	}

	copy(vm.CurrentMemory()[pathPtr:], f.path)
	return ESUCCESS
}

func (w *WASIEnvirnment) fd_fdstat_get(vm *wasm.VirtualMachine, fd uint32, bufPtr uint32) (err uint32) {
	if _, ok := w.opened[fd]; !ok {
		return EBADF
	}
	binary.LittleEndian.PutUint64(vm.CurrentMemory()[bufPtr+16:], R_FD_READ|R_FD_WRITE)
	return ESUCCESS
}

func (w *WASIEnvirnment) path_open(vm *wasm.VirtualMachine, fd, dirFlags, pathPtr, pathLen, oFlags uint32,
	fsRightsBase, fsRightsInheriting uint64,
	fdFlags, fdPtr uint32) (errno uint32) {
	dir, ok := w.opened[fd]
	if !ok || dir.fileSys == nil {
		return EINVAL
	}

	path := string(vm.CurrentMemory()[pathPtr : pathPtr+pathLen])
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

	binary.LittleEndian.PutUint32(vm.CurrentMemory()[fdPtr:], newFD)
	return ESUCCESS
}

func (w *WASIEnvirnment) fd_write(vm *wasm.VirtualMachine, fd uint32, iovsPtr uint32, iovsLen uint32, nwrittenPtr uint32) (err uint32) {
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
		offset := binary.LittleEndian.Uint32(vm.CurrentMemory()[iovPtr:])
		l := binary.LittleEndian.Uint32(vm.CurrentMemory()[iovPtr+4:])
		n, err := writer.Write(vm.CurrentMemory()[offset : offset+l])
		if err != nil {
			panic(err)
		}
		nwritten += uint32(n)
	}
	binary.LittleEndian.PutUint32(vm.CurrentMemory()[nwrittenPtr:], nwritten)
	return ESUCCESS
}

func (w *WASIEnvirnment) fd_read(vm *wasm.VirtualMachine, fd uint32, iovsPtr uint32, iovsLen uint32, nreadPtr uint32) (err uint32) {
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
		offset := binary.LittleEndian.Uint32(vm.CurrentMemory()[iovPtr:])
		l := binary.LittleEndian.Uint32(vm.CurrentMemory()[iovPtr+4:])
		n, err := reader.Read(vm.CurrentMemory()[offset : offset+l])
		nread += uint32(n)
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return EIO
		}
	}
	binary.LittleEndian.PutUint32(vm.CurrentMemory()[nreadPtr:], nread)
	return ESUCCESS
}

func (w *WASIEnvirnment) fd_close(vm *wasm.VirtualMachine, fd uint32) (err uint32) {
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

func args_sizes_get(vm *wasm.VirtualMachine, argcPtr uint32, argvPtr uint32) (err uint32) {
	// not implemented yet
	binary.LittleEndian.PutUint32(vm.CurrentMemory()[argcPtr:], 0)
	binary.LittleEndian.PutUint32(vm.CurrentMemory()[argvPtr:], 0)
	return 0
}

func args_get(*wasm.VirtualMachine, uint32, uint32) (err uint32) {
	// not implemented yet
	return
}

func proc_exit(*wasm.VirtualMachine, uint32) {
	// not implemented yet
}

func environ_sizes_get(*wasm.VirtualMachine, uint32, uint32) (err uint32) {
	// not implemented yet
	return
}

func environ_get(*wasm.VirtualMachine, uint32, uint32) (err uint32) {
	return
}
