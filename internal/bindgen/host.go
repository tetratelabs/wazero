// Copyright (C) 2022 Print Tracker, LLC - All Rights Reserved
//
// Unauthorized copying of this file, via any medium is strictly prohibited
// as this source code is proprietary and confidential. Dissemination of this
// information or reproduction of this material is strictly forbidden unless
// prior written permission is obtained from Print Tracker, LLC.

package bindgen

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"math"
)

const (
	U8        uint32 = 1
	I8               = 2
	U16              = 3
	I16              = 4
	U32              = 5
	I32              = 6
	U64              = 7
	I64              = 8
	F32              = 9
	F64              = 10
	Bool             = 11
	Rune             = 12
	ByteArray        = 21
	I8Array          = 22
	U16Array         = 23
	I16Array         = 24
	U32Array         = 25
	I32Array         = 26
	U64Array         = 27
	I64Array         = 28
	String           = 31
)

// Bindgen allows for binding
type Bindgen struct {
	runtime    wazero.Runtime
	module     api.Module
	resultChan chan []interface{}
	errChan    chan error
}

func Instantiate(ctx context.Context, runtime wazero.Runtime) (*Bindgen, error) {
	b := &Bindgen{
		runtime:    runtime,
		resultChan: make(chan []interface{}, 1),
		errChan:    make(chan error, 1),
	}
	return b, b.init(ctx)
}

func (b *Bindgen) init(ctx context.Context) error {
	var err error
	_, err = b.runtime.NewModuleBuilder("wazero-bindgen").ExportFunctions(map[string]interface{}{
		"return_result": b.return_result,
		"return_error":  b.return_error,
	}).Instantiate(ctx, b.runtime)
	if err != nil {
		return err
	}
	return nil
}

func (b *Bindgen) Bind(module api.Module) {
	b.module = module
}

func (b *Bindgen) Execute(ctx context.Context, funcName string, inputs ...interface{}) ([]interface{}, error) {
	inputsCount := len(inputs)

	// allocate new frame for passing pointers
	allocateResult, err := b.module.ExportedFunction("allocate").Call(ctx, uint64(inputsCount*4*2))
	if err != nil {
		return nil, err
	}
	pointerOfPointers := allocateResult[0]
	defer b.module.ExportedFunction("deallocate").Call(ctx, pointerOfPointers, uint64(inputsCount*4*2))

	memory := b.module.Memory()
	if memory == nil {
		return nil, errors.New("Memory not found")
	}

	for idx, inp := range inputs {
		var pointer, lengthOfInput, byteLengthOfInput uint32
		var err error
		switch input := inp.(type) {
		case []byte:
			pointer, lengthOfInput, err = b.settleByteSlice(ctx, memory, input)
			byteLengthOfInput = lengthOfInput
		case []int8:
			pointer, lengthOfInput, err = b.settleI8Slice(ctx, memory, input)
			byteLengthOfInput = lengthOfInput
		case []uint16:
			pointer, lengthOfInput, err = b.settleU16Slice(ctx, memory, input)
			byteLengthOfInput = lengthOfInput * 2
		case []int16:
			pointer, lengthOfInput, err = b.settleI16Slice(ctx, memory, input)
			byteLengthOfInput = lengthOfInput * 2
		case []uint32:
			pointer, lengthOfInput, err = b.settleU32Slice(ctx, memory, input)
			byteLengthOfInput = lengthOfInput * 4
		case []int32:
			pointer, lengthOfInput, err = b.settleI32Slice(ctx, memory, input)
			byteLengthOfInput = lengthOfInput * 4
		case []uint64:
			pointer, lengthOfInput, err = b.settleU64Slice(ctx, memory, input)
			byteLengthOfInput = lengthOfInput * 8
		case []int64:
			pointer, lengthOfInput, err = b.settleI64Slice(ctx, memory, input)
			byteLengthOfInput = lengthOfInput * 8
		case bool:
			pointer, lengthOfInput, err = b.settleBool(ctx, memory, input)
			byteLengthOfInput = lengthOfInput
		case int8:
			pointer, lengthOfInput, err = b.settleI8(ctx, memory, input)
			byteLengthOfInput = lengthOfInput
		case uint8:
			pointer, lengthOfInput, err = b.settleU8(ctx, memory, input)
			byteLengthOfInput = lengthOfInput
		case int16:
			pointer, lengthOfInput, err = b.settleI16(ctx, memory, input)
			byteLengthOfInput = lengthOfInput * 2
		case uint16:
			pointer, lengthOfInput, err = b.settleU16(ctx, memory, input)
			byteLengthOfInput = lengthOfInput * 2
		case int32:
			pointer, lengthOfInput, err = b.settleI32(ctx, memory, input)
			byteLengthOfInput = lengthOfInput * 4
		case uint32:
			pointer, lengthOfInput, err = b.settleU32(ctx, memory, input)
			byteLengthOfInput = lengthOfInput * 4
		case int64:
			pointer, lengthOfInput, err = b.settleI64(ctx, memory, input)
			byteLengthOfInput = lengthOfInput * 8
		case uint64:
			pointer, lengthOfInput, err = b.settleU64(ctx, memory, input)
			byteLengthOfInput = lengthOfInput * 8
		case float32:
			pointer, lengthOfInput, err = b.settleF32(ctx, memory, input)
			byteLengthOfInput = lengthOfInput * 4
		case float64:
			pointer, lengthOfInput, err = b.settleF64(ctx, memory, input)
			byteLengthOfInput = lengthOfInput * 8
		case string:
			pointer, lengthOfInput, err = b.settleString(ctx, memory, input)
			byteLengthOfInput = lengthOfInput
		default:
			return nil, errors.New(fmt.Sprintf("Unsupported arg type %T", input))
		}
		if err != nil {
			return nil, err
		}
		b.putPointerOfPointer(ctx, uint32(pointerOfPointers), memory, idx, pointer, lengthOfInput)
		defer b.module.ExportedFunction("deallocate").Call(context.TODO(), uint64(pointer), uint64(byteLengthOfInput))
	}

	fn := b.module.ExportedFunction(funcName)
	if _, err := fn.Call(context.TODO(), pointerOfPointers, uint64(inputsCount)); err != nil {
		return nil, err
	}
	return b.executionResult()
}

func (b *Bindgen) Close(ctx context.Context) error {
	return b.module.Close(ctx)
}

func (b *Bindgen) executionResult() ([]interface{}, error) {
	select {
	case res := <-b.resultChan:
		return res, nil
	case err := <-b.errChan:
		return nil, err
	}
}

func (b *Bindgen) return_result(ctx context.Context, module api.Module, pointer uint32, size uint32) {
	memory := module.Memory()
	if memory == nil {
		return
	}

	data, ok := memory.Read(ctx, pointer, size*3*4)
	if !ok {
		b.errChan <- fmt.Errorf("Memory.Read(%d, %d) out of range", pointer, size*3*4)
		return
	}
	rets := make([]uint32, size*3)

	for i := 0; i < int(size*3); i++ {
		buf := bytes.NewBuffer(data[i*4 : (i+1)*4])
		var p uint32
		binary.Read(buf, binary.LittleEndian, &p)
		rets[i] = p
	}

	result := make([]interface{}, size)
	for i := 0; i < int(size); i++ {
		offset, byteCount := rets[i*3], rets[i*3+2]
		bytes, ok := memory.Read(ctx, offset, byteCount)
		if !ok {
			b.errChan <- fmt.Errorf("Memory.Read(%d, %d) out of range", offset, byteCount)
			return
		}
		switch rets[i*3+1] {
		case U8:
			result[i] = interface{}(b.getU8(bytes))
		case I8:
			result[i] = interface{}(b.getI8(bytes))
		case U16:
			result[i] = interface{}(b.getU16(bytes))
		case I16:
			result[i] = interface{}(b.getI16(bytes))
		case U32:
			result[i] = interface{}(b.getU32(bytes))
		case I32:
			result[i] = interface{}(b.getI32(bytes))
		case U64:
			result[i] = interface{}(b.getU64(bytes))
		case I64:
			result[i] = interface{}(b.getI64(bytes))
		case F32:
			result[i] = interface{}(b.getF32(bytes))
		case F64:
			result[i] = interface{}(b.getF64(bytes))
		case Bool:
			result[i] = interface{}(b.getBool(bytes))
		case Rune:
			result[i] = interface{}(b.getRune(bytes))
		case String:
			result[i] = interface{}(b.getString(bytes))
		case ByteArray:
			result[i] = interface{}(b.getByteSlice(bytes))
		case I8Array:
			result[i] = interface{}(b.getI8Slice(bytes))
		case U16Array:
			result[i] = interface{}(b.getU16Slice(bytes))
		case I16Array:
			result[i] = interface{}(b.getI16Slice(bytes))
		case U32Array:
			result[i] = interface{}(b.getU32Slice(bytes))
		case I32Array:
			result[i] = interface{}(b.getI32Slice(bytes))
		case U64Array:
			result[i] = interface{}(b.getU64Slice(bytes))
		case I64Array:
			result[i] = interface{}(b.getI64Slice(bytes))
		}
	}

	b.resultChan <- result
}

func (b *Bindgen) return_error(ctx context.Context, module api.Module, pointer uint32, size uint32) {
	memory := module.Memory()
	if memory == nil {
		return
	}
	data, ok := memory.Read(ctx, pointer, size)
	if !ok {
		b.errChan <- fmt.Errorf("Memory.Read(%d, %d) out of range", pointer, size)
		return
	}
	result := make([]byte, size)
	copy(result, data)
	if result != nil {
		b.errChan <- errors.New(string(result))
	}
}

func (b *Bindgen) putPointerOfPointer(ctx context.Context, pointerOfPointers uint32, memory api.Memory, inputIdx int, pointer uint32, lengthOfInput uint32) {
	// set data for pointer of pointer
	pointerBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(pointerBytes, pointer)
	memory.Write(ctx, uint32(uint(pointerOfPointers)+uint(inputIdx*4*2)), pointerBytes)
	lengthBytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(lengthBytes, lengthOfInput)
	memory.Write(ctx, uint32(uint(pointerOfPointers)+uint(inputIdx*4*2+4)), lengthBytes)
}

func (b *Bindgen) settleByteSlice(ctx context.Context, memory api.Memory, input []byte) (uint32, uint32, error) {
	lengthOfInput := uint32(len(input))
	allocateResult, err := b.module.ExportedFunction("allocate").Call(ctx, uint64(lengthOfInput))
	if err != nil {
		return 0, 0, err
	}
	pointer := allocateResult[0]
	if ok := memory.Write(ctx, uint32(pointer), input); !ok {
		return 0, 0, fmt.Errorf("Memory.Write error")
	}
	return uint32(pointer), lengthOfInput, nil
}

func (b *Bindgen) settleI8Slice(ctx context.Context, memory api.Memory, input []int8) (uint32, uint32, error) {
	lengthOfInput := uint32(len(input))
	allocateResult, err := b.module.ExportedFunction("allocate").Call(ctx, uint64(lengthOfInput))
	if err != nil {
		return 0, 0, err
	}
	pointer := allocateResult[0]

	data := make([]byte, lengthOfInput)
	for i := 0; i < int(lengthOfInput); i++ {
		data[i] = byte(input[i])
	}

	if ok := memory.Write(ctx, uint32(pointer), data); !ok {
		return 0, 0, fmt.Errorf("Memory.Write error")
	}
	return uint32(pointer), lengthOfInput, nil
}

func (b *Bindgen) settleU16Slice(ctx context.Context, memory api.Memory, input []uint16) (uint32, uint32, error) {
	lengthOfInput := uint32(len(input))
	allocateResult, err := b.module.ExportedFunction("allocate").Call(ctx, uint64(lengthOfInput*2))
	if err != nil {
		return 0, 0, err
	}
	pointer := allocateResult[0]

	data := make([]byte, lengthOfInput*2)
	for i := 0; i < int(lengthOfInput); i++ {
		binary.LittleEndian.PutUint16(data[i*2:(i+1)*2], input[i])
	}

	if ok := memory.Write(ctx, uint32(pointer), data); !ok {
		return 0, 0, fmt.Errorf("Memory.Write error")
	}
	return uint32(pointer), lengthOfInput, nil
}

func (b *Bindgen) settleI16Slice(ctx context.Context, memory api.Memory, input []int16) (uint32, uint32, error) {
	lengthOfInput := uint32(len(input))
	allocateResult, err := b.module.ExportedFunction("allocate").Call(ctx, uint64(lengthOfInput*2))
	if err != nil {
		return 0, 0, err
	}
	pointer := allocateResult[0]

	data := make([]byte, lengthOfInput*2)
	for i := 0; i < int(lengthOfInput); i++ {
		binary.LittleEndian.PutUint16(data[i*2:(i+1)*2], uint16(input[i]))
	}

	if ok := memory.Write(ctx, uint32(pointer), data); !ok {
		return 0, 0, fmt.Errorf("Memory.Write error")
	}
	return uint32(pointer), lengthOfInput, nil
}

func (b *Bindgen) settleU32Slice(ctx context.Context, memory api.Memory, input []uint32) (uint32, uint32, error) {
	lengthOfInput := uint32(len(input))
	allocateResult, err := b.module.ExportedFunction("allocate").Call(ctx, uint64(lengthOfInput*4))
	if err != nil {
		return 0, 0, err
	}
	pointer := allocateResult[0]

	data := make([]byte, lengthOfInput*4)
	for i := 0; i < int(lengthOfInput); i++ {
		binary.LittleEndian.PutUint32(data[i*4:(i+1)*4], input[i])
	}

	if ok := memory.Write(ctx, uint32(pointer), data); !ok {
		return 0, 0, fmt.Errorf("Memory.Write error")
	}

	return uint32(pointer), lengthOfInput, nil
}

func (b *Bindgen) settleI32Slice(ctx context.Context, memory api.Memory, input []int32) (uint32, uint32, error) {
	lengthOfInput := uint32(len(input))
	allocateResult, err := b.module.ExportedFunction("allocate").Call(ctx, uint64(lengthOfInput*4))
	if err != nil {
		return 0, 0, err
	}
	pointer := allocateResult[0]
	data := make([]byte, lengthOfInput*4)
	for i := 0; i < int(lengthOfInput); i++ {
		binary.LittleEndian.PutUint32(data[i*4:(i+1)*4], uint32(input[i]))
	}
	if ok := memory.Write(ctx, uint32(pointer), data); !ok {
		return 0, 0, fmt.Errorf("Memory.Write error")
	}
	return uint32(pointer), lengthOfInput, nil
}

func (b *Bindgen) settleU64Slice(ctx context.Context, memory api.Memory, input []uint64) (uint32, uint32, error) {
	lengthOfInput := uint32(len(input))
	allocateResult, err := b.module.ExportedFunction("allocate").Call(ctx, uint64(lengthOfInput*8))
	if err != nil {
		return 0, 0, err
	}
	pointer := allocateResult[0]

	data := make([]byte, lengthOfInput*8)
	for i := 0; i < int(lengthOfInput); i++ {
		binary.LittleEndian.PutUint64(data[i*8:(i+1)*8], input[i])
	}

	if ok := memory.Write(ctx, uint32(pointer), data); !ok {
		return 0, 0, fmt.Errorf("Memory.Write error")
	}

	return uint32(pointer), lengthOfInput, nil
}

func (b *Bindgen) settleI64Slice(ctx context.Context, memory api.Memory, input []int64) (uint32, uint32, error) {
	lengthOfInput := uint32(len(input))
	allocateResult, err := b.module.ExportedFunction("allocate").Call(ctx, uint64(lengthOfInput*8))
	if err != nil {
		return 0, 0, err
	}
	pointer := allocateResult[0]

	data := make([]byte, lengthOfInput*8)
	for i := 0; i < int(lengthOfInput); i++ {
		binary.LittleEndian.PutUint64(data[i*8:(i+1)*8], uint64(input[i]))
	}

	if ok := memory.Write(ctx, uint32(pointer), data); !ok {
		return 0, 0, fmt.Errorf("Memory.Write error")
	}

	return uint32(pointer), lengthOfInput, nil
}

func (b *Bindgen) settleBool(ctx context.Context, memory api.Memory, input bool) (uint32, uint32, error) {
	lengthOfInput := uint32(1)
	allocateResult, err := b.module.ExportedFunction("allocate").Call(ctx, uint64(lengthOfInput))
	if err != nil {
		return 0, 0, err
	}
	pointer := allocateResult[0]

	var bt byte = 0
	if input {
		bt = 1
	}
	if ok := memory.Write(ctx, uint32(pointer), []byte{bt}); !ok {
		return 0, 0, fmt.Errorf("Memory.Write error")
	}
	return uint32(pointer), lengthOfInput, nil
}

func (b *Bindgen) settleRune(ctx context.Context, memory api.Memory, input rune) (uint32, uint32, error) {
	lengthOfInput := uint32(4)
	allocateResult, err := b.module.ExportedFunction("allocate").Call(ctx, uint64(lengthOfInput))
	if err != nil {
		return 0, 0, err
	}
	pointer := allocateResult[0]

	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, uint32(input))
	if ok := memory.Write(ctx, uint32(pointer), bytes); !ok {
		return 0, 0, fmt.Errorf("Memory.Write error")
	}

	return uint32(pointer), lengthOfInput, nil
}

func (b *Bindgen) settleI8(ctx context.Context, memory api.Memory, input int8) (uint32, uint32, error) {
	lengthOfInput := uint32(1)
	allocateResult, err := b.module.ExportedFunction("allocate").Call(ctx, uint64(lengthOfInput))
	if err != nil {
		return 0, 0, err
	}
	pointer := allocateResult[0]

	if ok := memory.Write(ctx, uint32(pointer), []byte{byte(input)}); !ok {
		return 0, 0, fmt.Errorf("Memory.Write error")
	}

	return uint32(pointer), lengthOfInput, nil
}

func (b *Bindgen) settleU8(ctx context.Context, memory api.Memory, input uint8) (uint32, uint32, error) {
	lengthOfInput := uint32(1)
	allocateResult, err := b.module.ExportedFunction("allocate").Call(ctx, uint64(lengthOfInput))
	if err != nil {
		return 0, 0, err
	}
	pointer := allocateResult[0]

	if ok := memory.Write(ctx, uint32(pointer), []byte{byte(input)}); !ok {
		return 0, 0, fmt.Errorf("Memory.Write error")
	}

	return uint32(pointer), lengthOfInput, nil
}

func (b *Bindgen) settleI16(ctx context.Context, memory api.Memory, input int16) (uint32, uint32, error) {
	lengthOfInput := uint32(1)
	allocateResult, err := b.module.ExportedFunction("allocate").Call(ctx, uint64(lengthOfInput*2))
	if err != nil {
		return 0, 0, err
	}
	pointer := allocateResult[0]

	bytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(bytes, uint16(input))
	if ok := memory.Write(ctx, uint32(pointer), bytes); !ok {
		return 0, 0, fmt.Errorf("Memory.Write error")
	}

	return uint32(pointer), lengthOfInput, nil
}

func (b *Bindgen) settleU16(ctx context.Context, memory api.Memory, input uint16) (uint32, uint32, error) {
	lengthOfInput := uint32(1)
	allocateResult, err := b.module.ExportedFunction("allocate").Call(ctx, uint64(lengthOfInput*2))
	if err != nil {
		return 0, 0, err
	}
	pointer := allocateResult[0]

	bytes := make([]byte, 2)
	binary.LittleEndian.PutUint16(bytes, input)
	if ok := memory.Write(ctx, uint32(pointer), bytes); !ok {
		return 0, 0, fmt.Errorf("Memory.Write error")
	}

	return uint32(pointer), lengthOfInput, nil
}

func (b *Bindgen) settleI32(ctx context.Context, memory api.Memory, input int32) (uint32, uint32, error) {
	lengthOfInput := uint32(1)
	allocateResult, err := b.module.ExportedFunction("allocate").Call(ctx, uint64(lengthOfInput*4))
	if err != nil {
		return 0, 0, err
	}
	pointer := allocateResult[0]

	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, uint32(input))
	if ok := memory.Write(ctx, uint32(pointer), bytes); !ok {
		return 0, 0, fmt.Errorf("Memory.Write error")
	}

	return uint32(pointer), lengthOfInput, nil
}

func (b *Bindgen) settleU32(ctx context.Context, memory api.Memory, input uint32) (uint32, uint32, error) {
	lengthOfInput := uint32(1)
	allocateResult, err := b.module.ExportedFunction("allocate").Call(ctx, uint64(lengthOfInput*4))
	if err != nil {
		return 0, 0, err
	}
	pointer := allocateResult[0]

	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, input)
	if ok := memory.Write(ctx, uint32(pointer), bytes); !ok {
		return 0, 0, fmt.Errorf("Memory.Write error")
	}

	return uint32(pointer), lengthOfInput, nil
}

func (b *Bindgen) settleI64(ctx context.Context, memory api.Memory, input int64) (uint32, uint32, error) {
	lengthOfInput := uint32(1)
	allocateResult, err := b.module.ExportedFunction("allocate").Call(ctx, uint64(lengthOfInput*8))
	if err != nil {
		return 0, 0, err
	}
	pointer := allocateResult[0]

	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytes, uint64(input))
	if ok := memory.Write(ctx, uint32(pointer), bytes); !ok {
		return 0, 0, fmt.Errorf("Memory.Write error")
	}

	return uint32(pointer), lengthOfInput, nil
}

func (b *Bindgen) settleU64(ctx context.Context, memory api.Memory, input uint64) (uint32, uint32, error) {
	lengthOfInput := uint32(1)
	allocateResult, err := b.module.ExportedFunction("allocate").Call(ctx, uint64(lengthOfInput*8))
	if err != nil {
		return 0, 0, err
	}
	pointer := allocateResult[0]

	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytes, input)
	if ok := memory.Write(ctx, uint32(pointer), bytes); !ok {
		return 0, 0, fmt.Errorf("Memory.Write error")
	}

	return uint32(pointer), lengthOfInput, nil
}

func (b *Bindgen) settleF32(ctx context.Context, memory api.Memory, input float32) (uint32, uint32, error) {
	lengthOfInput := uint32(1)
	allocateResult, err := b.module.ExportedFunction("allocate").Call(ctx, uint64(lengthOfInput*4))
	if err != nil {
		return 0, 0, err
	}
	pointer := allocateResult[0]

	bytes := make([]byte, 4)
	binary.LittleEndian.PutUint32(bytes, math.Float32bits(input))
	if ok := memory.Write(ctx, uint32(pointer), bytes); !ok {
		return 0, 0, fmt.Errorf("Memory.Write error")
	}

	return uint32(pointer), lengthOfInput, nil
}

func (b *Bindgen) settleF64(ctx context.Context, memory api.Memory, input float64) (uint32, uint32, error) {
	lengthOfInput := uint32(1)
	allocateResult, err := b.module.ExportedFunction("allocate").Call(ctx, uint64(lengthOfInput*8))
	if err != nil {
		return 0, 0, err
	}
	pointer := allocateResult[0]

	bytes := make([]byte, 8)
	binary.LittleEndian.PutUint64(bytes, math.Float64bits(input))
	if ok := memory.Write(ctx, uint32(pointer), bytes); !ok {
		return 0, 0, fmt.Errorf("Memory.Write error")
	}

	return uint32(pointer), lengthOfInput, nil
}

func (b *Bindgen) settleString(ctx context.Context, memory api.Memory, input string) (uint32, uint32, error) {
	lengthOfInput := uint32(len([]byte(input)))
	allocateResult, err := b.module.ExportedFunction("allocate").Call(ctx, uint64(lengthOfInput))
	if err != nil {
		return 0, 0, err
	}
	pointer := allocateResult[0]

	if ok := memory.Write(ctx, uint32(pointer), []byte(input)); !ok {
		return 0, 0, fmt.Errorf("Memory.Write error")
	}

	return uint32(pointer), lengthOfInput, nil
}

func (b *Bindgen) getU8(d []byte) uint8 {
	return uint8(d[0])
}

func (b *Bindgen) getI8(d []byte) int8 {
	return int8(d[0])
}

func (b *Bindgen) getU16(d []byte) (r uint16) {
	buf := bytes.NewBuffer(d)
	binary.Read(buf, binary.LittleEndian, &r)
	return
}

func (b *Bindgen) getI16(d []byte) (r int16) {
	buf := bytes.NewBuffer(d)
	binary.Read(buf, binary.LittleEndian, &r)
	return
}

func (b *Bindgen) getU32(d []byte) (r uint32) {
	buf := bytes.NewBuffer(d)
	binary.Read(buf, binary.LittleEndian, &r)
	return
}

func (b *Bindgen) getI32(d []byte) (r int32) {
	buf := bytes.NewBuffer(d)
	binary.Read(buf, binary.LittleEndian, &r)
	return
}

func (b *Bindgen) getU64(d []byte) (r uint64) {
	buf := bytes.NewBuffer(d)
	binary.Read(buf, binary.LittleEndian, &r)
	return
}

func (b *Bindgen) getI64(d []byte) (r int64) {
	buf := bytes.NewBuffer(d)
	binary.Read(buf, binary.LittleEndian, &r)
	return
}

func (b *Bindgen) getF32(d []byte) float32 {
	buf := bytes.NewBuffer(d)
	var p uint32
	binary.Read(buf, binary.LittleEndian, &p)
	return math.Float32frombits(p)
}

func (b *Bindgen) getF64(d []byte) float64 {
	buf := bytes.NewBuffer(d)
	var p uint64
	binary.Read(buf, binary.LittleEndian, &p)
	return math.Float64frombits(p)
}

func (b *Bindgen) getBool(d []byte) bool {
	return d[0] == byte(1)
}

func (b *Bindgen) getRune(d []byte) rune {
	buf := bytes.NewBuffer(d)
	var p uint32
	binary.Read(buf, binary.LittleEndian, &p)
	return rune(p)
}

func (b *Bindgen) getString(d []byte) string {
	return string(d)
}

func (b *Bindgen) getByteSlice(d []byte) []byte {
	x := make([]byte, len(d))
	copy(x, d)
	return x
}

func (b *Bindgen) getI8Slice(d []byte) []int8 {
	r := make([]int8, len(d))
	for i, v := range d {
		r[i] = int8(v)
	}
	return r
}

func (b *Bindgen) getU16Slice(d []byte) []uint16 {
	r := make([]uint16, len(d)/2)
	for i := 0; i < len(r); i++ {
		buf := bytes.NewBuffer(d[i*2 : (i+1)*2])
		binary.Read(buf, binary.LittleEndian, &r[i])
	}
	return r
}

func (b *Bindgen) getI16Slice(d []byte) []int16 {
	r := make([]int16, len(d)/2)
	for i := 0; i < len(r); i++ {
		buf := bytes.NewBuffer(d[i*2 : (i+1)*2])
		binary.Read(buf, binary.LittleEndian, &r[i])
	}
	return r

}

func (b *Bindgen) getU32Slice(d []byte) []uint32 {
	r := make([]uint32, len(d)/4)
	for i := 0; i < len(r); i++ {
		buf := bytes.NewBuffer(d[i*4 : (i+1)*4])
		binary.Read(buf, binary.LittleEndian, &r[i])
	}
	return r

}

func (b *Bindgen) getI32Slice(d []byte) []int32 {
	r := make([]int32, len(d)/4)
	for i := 0; i < len(r); i++ {
		buf := bytes.NewBuffer(d[i*4 : (i+1)*4])
		binary.Read(buf, binary.LittleEndian, &r[i])
	}
	return r

}

func (b *Bindgen) getU64Slice(d []byte) []uint64 {
	r := make([]uint64, len(d)/8)
	for i := 0; i < len(r); i++ {
		buf := bytes.NewBuffer(d[i*8 : (i+1)*8])
		binary.Read(buf, binary.LittleEndian, &r[i])
	}
	return r

}

func (b *Bindgen) getI64Slice(d []byte) []int64 {
	r := make([]int64, len(d)/8)
	for i := 0; i < len(r); i++ {
		buf := bytes.NewBuffer(d[i*8 : (i+1)*8])
		binary.Read(buf, binary.LittleEndian, &r[i])
	}
	return r

}
