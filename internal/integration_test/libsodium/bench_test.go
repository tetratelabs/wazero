package libsodium

import (
	"context"
	"crypto/rand"
	"embed"
	"io"
	"os"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/internal/platform"
	"github.com/tetratelabs/wazero/internal/testing/require"
)

//go:embed testdata/*
var tests embed.FS

func BenchmarkLibsodium(b *testing.B) {
	if !platform.CompilerSupported() {
		b.Skip()
	}

	cases, err := tests.ReadDir("testdata")
	require.NoError(b, err)
	if len(cases) < 10 {
		b.Skip("skipping libsodium bench because wasm files not found. `make libsodium` to download the binaries.")
	}

	ctx := context.Background()
	r := wazero.NewRuntimeWithConfig(ctx, wazero.NewRuntimeConfigCompiler())
	defer r.Close(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	// Some tests are skipped because they are taking too long to run, but sure it is possible to run them.
	for _, c := range []struct {
		name string
	}{
		//{name: "box7"},
		{name: "box_easy2"},
		{name: "kdf_hkdf"},
		{name: "auth5"},
		{name: "stream2"},
		{name: "aead_xchacha20poly1305"},
		{name: "hash3"},
		{name: "aead_chacha20poly1305"},
		{name: "auth"},
		//{name: "core_ed25519_h2c"},
		{name: "onetimeauth"},
		{name: "aead_aegis256"},
		{name: "scalarmult_ristretto255"},
		//{name: "core_ristretto255"},
		{name: "stream3"},
		//{name: "pwhash_scrypt"},
		{name: "shorthash"},
		{name: "scalarmult"},
		{name: "chacha20"},
		//{name: "pwhash_argon2id"},
		{name: "onetimeauth7"},
		{name: "scalarmult7"},
		{name: "auth3"},
		{name: "stream4"},
		{name: "hash"},
		//{name: "sign"},
		{name: "auth2"},
		{name: "scalarmult6"},
		{name: "ed25519_convert"},
		{name: "box_seal"},
		{name: "secretbox7"},
		{name: "pwhash_argon2i"},
		{name: "secretstream_xchacha20poly1305"},
		{name: "codecs"},
		{name: "scalarmult_ed25519"},
		{name: "sodium_utils"},
		{name: "scalarmult5"},
		{name: "xchacha20"},
		{name: "secretbox8"},
		{name: "box2"},
		{name: "core3"},
		{name: "siphashx24"},
		{name: "generichash"},
		{name: "aead_chacha20poly13052"},
		{name: "randombytes"},
		{name: "scalarmult8"},
		//{name: "pwhash_scrypt_ll"},
		{name: "kx"},
		{name: "stream"},
		{name: "auth7"},
		{name: "generichash2"},
		{name: "box_seed"},
		{name: "keygen"},
		{name: "metamorphic"},
		{name: "secretbox_easy2"},
		{name: "sign2"},
		//{name: "core_ed25519"},
		{name: "box_easy"},
		{name: "secretbox2"},
		//{name: "box8"},
		{name: "box"},
		{name: "kdf"},
		{name: "secretbox_easy"},
		{name: "onetimeauth2"},
		{name: "generichash3"},
		{name: "scalarmult2"},
		{name: "aead_aegis128l"},
		{name: "auth6"},
		{name: "secretbox"},
		{name: "verify1"},
	} {
		b.Run(c.name, func(b *testing.B) {
			path := "testdata/" + c.name + ".wasm"
			wasm, err := tests.ReadFile(path)
			require.NoError(b, err)

			cfg := wazero.NewModuleConfig().
				WithStdout(io.Discard).
				WithStderr(io.Discard).
				WithStdin(os.Stdin).
				WithRandSource(rand.Reader).
				WithFSConfig(wazero.NewFSConfig()).
				WithSysNanosleep().
				WithSysNanotime().
				WithSysWalltime().
				WithArgs(c.name + ".wasm")

			compiled, err := r.CompileModule(ctx, wasm)
			require.NoError(b, err)

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				mod, err := r.InstantiateModule(ctx, compiled, cfg.WithName(""))
				require.NoError(b, err)
				err = mod.Close(ctx)
				require.NoError(b, err)
			}
		})
	}
}
