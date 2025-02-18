# Note: Some tests might fail if Go has been installed with homebrew because
# the file system layout is different than what the tests expect.
# Easiest fix is to install without using brew.
goroot?=$(shell go env GOROOT)/src
# Note: The standard binary zig distribution does not ship some testdata.
# You should override with the zig source code path, otherwise some tests will fail.
zigroot?=$(shell dirname $(shell which zig))

zig_bin=$(shell pwd)/testdata/zig
tinygo_bin=$(shell pwd)/testdata/tinygo
gowasip1_bin=$(shell pwd)/testdata/go


# embed/internal/embedtest deals with some paths that are not available here, ignore.
tinygo_tests=container/heap \
	container/list \
	container/ring \
	crypto/des \
	crypto/md5 \
	crypto/rc4 \
	crypto/sha1 \
	crypto/sha256 \
	crypto/sha512 \
	encoding \
	encoding/ascii85 \
	encoding/base32 \
	encoding/csv \
	encoding/hex \
	go/scanner \
	hash \
	hash/adler32 \
	hash/crc64 \
	hash/fnv \
	html \
	internal/itoa \
	internal/profile \
	math \
	math/cmplx \
	net \
	net/http/internal/ascii \
	net/mail \
	os \
	path \
	reflect \
	sync \
	testing \
	testing/iotest \
	text/scanner \
	unicode \
	unicode/utf16 \
	unicode/utf8

gowasip1_tests=archive/tar \
	bufio \
	bytes \
	context \
	encoding/ascii85 \
	encoding/asn1 \
	encoding/base32 \
	encoding/base64 \
	encoding/binary \
	encoding/csv \
	encoding/gob \
	encoding/hex \
	encoding/json \
	encoding/pem \
	encoding/xml \
	errors \
	expvar \
	flag \
	fmt \
	hash \
	hash/adler32 \
	hash/crc32 \
	hash/crc64 \
	hash/fnv \
	hash/maphash \
	io \
	io/fs \
	io/ioutil \
	log \
	log/syslog \
	maps \
	math \
	math/big \
	math/bits \
	math/cmplx \
	math/rand \
	mime \
	mime/multipart \
	mime/quotedprintable \
	os \
	os/exec \
	os/signal \
	os/user \
	path \
	reflect \
	regexp \
	regexp/syntax \
	runtime \
	runtime/internal/atomic \
	runtime/internal/math \
	runtime/internal/sys \
	slices \
	sort \
	strconv \
	strings \
	sync \
	sync/atomic \
	syscall \
	testing \
	testing/fstest \
	testing/iotest \
	testing/quick \
	time

all: build.zig build.tinygo build.gowasip1


.PHONY: build.zig
build.zig:
	# --test-no-exec allows building of the test Wasm binary without executing command.
	# We use find because the test.wasm will be something like ./zig-cache/o/dd6df1361b2134adc5eee9d027495436/test.wasm
	@mkdir -p $(zig_bin)
	@cd $(zigroot) && zig test --test-no-exec -target wasm32-wasi --zig-lib-dir $(zigroot)/lib $(zigroot)/lib/std/std.zig
	@cp $$(find $(zigroot) -name test.wasm) $(zig_bin)/test.wasm
	# The generated test binary is large and produces skewed results in favor of the optimized compiler.
	# We also generate a stripped, optimized binary with wasm-opt.
	@wasm-opt $(zig_bin)/test.wasm -O --strip-dwarf -o $(zig_bin)/test-opt.wasm

.PHONY: build.tinygo
build.tinygo:
	@mkdir -p $(tinygo_bin)
	for value in $(tinygo_tests); do\
		echo Building $${value}... ;\
		tinygo test -target wasip1 -c -o $(tinygo_bin)/$$(echo $$value | sed -e 's/\//_/g').test $${value} 2>&1 >/dev/null  ;\
	done

.PHONY: build.gowasip1
build.gowasip1:
	@mkdir -p $(gowasip1_bin)
	@cd '$(goroot)'; \
	for value in $(gowasip1_tests); do\
		if [ -d ./$${value} ]; then\
			GOOS=wasip1 GOARCH=wasm go test -v -c -o $(gowasip1_bin)/$$(echo $$value | sed -e 's/\//_/g').test ./$${value}; \
		fi;\
	done
