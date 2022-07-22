package compiler

import (
	"github.com/tetratelabs/wazero/internal/asm"
	"github.com/tetratelabs/wazero/internal/wazeroir"
)

// compileDropRange adds instruction to drop the values on the target range
// in the architecture independent way.
func compileDropRange(c compiler, r *wazeroir.InclusiveRange) (err error) {
	locationStack := c.runtimeValueLocationStack()
	if r == nil {
		return
	} else if r.Start == 0 {
		for i := 0; i <= r.End; i++ {
			if loc := locationStack.pop(); loc.onRegister() {
				locationStack.releaseRegister(loc)
			}
		}
		return
	}

	// If the top value is alive, we must ensure that it is not located as a conditional.
	// Otherwise, the conditional flag might end up modified by the following operation.
	if err = c.maybeCompileMoveTopConditionalToGeneralPurposeRegister(); err != nil {
		return
	}

	// liveValues are must be pushed backed after dropping the target range.
	liveValues := locationStack.stack[locationStack.sp-uint64(r.Start) : locationStack.sp]
	// dropValues are the values on the drop target range.
	dropValues := locationStack.stack[locationStack.sp-uint64(r.End) : locationStack.sp-uint64(r.Start)+1]
	for _, dv := range dropValues {
		locationStack.releaseRegister(dv)
	}

	// These booleans are true if a live value of that type is currently located on the memory stack.
	// In order to migrate these values, we need a register of the corresponding type.
	var gpTmp, vecTmp = asm.NilRegister, asm.NilRegister
	for _, l := range liveValues {
		if l.onStack() {
			if l.getRegisterType() == registerTypeGeneralPurpose && gpTmp == asm.NilRegister {
				gpTmp, err = c.allocateRegister(registerTypeGeneralPurpose)
				if err != nil {
					return err
				}
			} else if l.getRegisterType() == registerTypeVector && vecTmp == asm.NilRegister {
				vecTmp, err = c.allocateRegister(registerTypeVector)
				if err != nil {
					return err
				}
			}
		}
	}

	// Reset the stack pointer below the end.
	locationStack.sp -= uint64(len(liveValues) + len(dropValues))

	// Push back the live values again.
	for _, live := range liveValues {
		if live.valueType == runtimeValueTypeV128Hi {
			// Higher bits of vector was already handled together with the lower bits.
			continue
		}

		previouslyOnStack := live.onStack()
		if previouslyOnStack {
			// If the value is on the stack, load the value on the old location into the temporary value,
			// and then write it back to the new memory location below.
			switch live.getRegisterType() {
			case registerTypeGeneralPurpose:
				live.setRegister(gpTmp)
			case registerTypeVector:
				live.setRegister(vecTmp)
			}
			// Load the value into tmp.
			c.compileLoadValueOnStackToRegister(live)
		}

		var newLocation *runtimeValueLocation
		if live.valueType == runtimeValueTypeV128Lo {
			newLocation = c.pushVectorRuntimeValueLocationOnRegister(live.register)
		} else {
			newLocation = c.pushRuntimeValueLocationOnRegister(live.register, live.valueType)
		}

		if previouslyOnStack {
			// This case, the location is on the temporary register. Therefore,
			// we have to release the value there into the *new* memory location
			// so that the tmp can be used for subsequent live value migrations.
			c.compileReleaseRegisterToStack(newLocation)
		}
	}
	return
}
