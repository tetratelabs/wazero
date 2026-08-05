// Compile with Emscripten (legacy wasm EH), then translate to new EH with Binaryen:
//
//   emcc -fwasm-exceptions -o cpp_exceptions_legacy.wasm cpp_exceptions.cpp \
//     -s EXPORTED_FUNCTIONS='["_test_no_throw","_test_catch_specific","_test_catch_base","_test_rethrow"]' \
//     -s ERROR_ON_UNDEFINED_SYMBOLS=0 -s STANDALONE_WASM -O2 --no-entry
//   wasm-opt --translate-to-exnref \
//     --enable-exception-handling --enable-reference-types --enable-bulk-memory \
//     --enable-bulk-memory-opt --enable-multivalue --enable-mutable-globals \
//     --enable-sign-ext --enable-nontrapping-float-to-int \
//     -o cpp_exceptions.wasm cpp_exceptions_legacy.wasm

#include <exception>
#include <stdexcept>

class MyException : public std::exception {
    int code_;
public:
    MyException(int code) : code_(code) {}
    int code() const { return code_; }
    const char* what() const noexcept override { return "MyException"; }
};

static int may_throw(int v) {
    if (v < 0) {
        throw MyException(v);
    }
    if (v == 0) {
        throw std::runtime_error("zero");
    }
    return v * 2;
}

extern "C" {

// Returns may_throw(21), expected 42. Returns -1 on unexpected exception.
int test_no_throw() {
    try {
        return may_throw(21);
    } catch (...) {
        return -1;
    }
}

// Throws MyException(-1), catches it, returns the code. Expected: -1.
int test_catch_specific() {
    try {
        may_throw(-1);
        return 0; // should not reach
    } catch (const MyException& e) {
        return e.code();
    }
}

// Throws std::runtime_error, catches via std::exception base. Returns 1 on match, 0 otherwise.
int test_catch_base() {
    try {
        may_throw(0);
        return 0;
    } catch (const std::exception& e) {
        return 1;
    }
}

// Throws, catches, rethrows, re-catches. Returns the code from re-catch. Expected: -42.
int test_rethrow() {
    try {
        try {
            may_throw(-42);
        } catch (const MyException&) {
            throw;
        }
    } catch (const MyException& e) {
        return e.code();
    }
    return 0;
}

} // extern "C"
