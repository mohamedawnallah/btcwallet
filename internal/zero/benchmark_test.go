// Copyright (c) 2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package zero_test

import (
	"testing"

	. "github.com/btcsuite/btcwallet/internal/zero"
)

var (
	bytes32 = make([]byte, 32) // typical key size
	bytes64 = make([]byte, 64) // passphrase hash size
	bytea32 = new([32]byte)
	bytea64 = new([64]byte)
)

// xor is the "slow" byte zeroing implementation which this package
// originally replaced.  If this function benchmarks faster than the
// functions exported by this package in a future Go version (perhaps
// by calling runtime.memclr), replace the "optimized" versions with
// this.
func xor(b []byte) {
	for i := range b {
		b[i] ^= b[i]
	}
}

// zrange is an alternative zero implementation that, while currently
// slower than the functions provided by this package, may be faster
// in a future Go release.  Switch to this or the xor implementation
// if they ever become faster.
func zrange(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// BenchmarkXor benchmarks the xor zeroing implementation across different
// byte slice sizes.
func BenchmarkXor(b *testing.B) {
	sizes := []struct {
		name string
		data []byte
	}{
		{"32-bytes", bytes32},
		{"64-bytes", bytes64},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				xor(size.data)
			}
		})
	}
}

// BenchmarkRange benchmarks the range zeroing implementation across different
// byte slice sizes.
func BenchmarkRange(b *testing.B) {
	sizes := []struct {
		name string
		data []byte
	}{
		{"32-bytes", bytes32},
		{"64-bytes", bytes64},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				zrange(size.data)
			}
		})
	}
}

// BenchmarkBytes benchmarks the Bytes function across different byte slice
// sizes.
func BenchmarkBytes(b *testing.B) {
	sizes := []struct {
		name string
		data []byte
	}{
		{"32-bytes", bytes32},
		{"64-bytes", bytes64},
	}

	for _, size := range sizes {
		b.Run(size.name, func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				Bytes(size.data)
			}
		})
	}
}

// BenchmarkBytea benchmarks the Bytea functions across different array sizes.
func BenchmarkBytea(b *testing.B) {
	b.Run("32-bytes", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			Bytea32(bytea32)
		}
	})

	b.Run("64-bytes", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			Bytea64(bytea64)
		}
	})
}
