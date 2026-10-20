package expctxkeys

// InterpreterLazyLowering is a context.Context Value key.
// Its associated value should be a bool: when true, the interpreter lowers
// each function body on its first call instead of during compilation.
type InterpreterLazyLowering struct{}
