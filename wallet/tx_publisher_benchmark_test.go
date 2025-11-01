// Copyright (c) 2025 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wallet

import (
	"fmt"
	"testing"
)

// BenchmarkBroadcastAPIConcurrentANDSameAddressUsed benchmarks the new
// Broadcast API vs old PublishTransaction API under concurrent load with the
// same address per transaction.
func BenchmarkBroadcastAPIConcurrentANDSameAddressUsed(b *testing.B) {
	const (
		// txPoolSize is the number of transactions to pre-create and
		// reuse across benchmark iterations. Should be at least as
		// large as the highest concurrency level.
		txPoolSize = 1 << 14

		// startGrowthIteration is the starting iteration index for the
		// growth sequence.
		startGrowthIteration = 0

		// maxGrowthIteration is the maximum iteration index for the
		// growth sequence.
		maxGrowthIteration = 14

		// useSameAddress determines whether all transactions in the
		// pool send to the same wallet address (true) or to unique
		// addresses (false). This deliberately enables comprehensive
		// benchmarking unique/duplicate addrs codepaths.
		useSameAddress = true
	)

	var (
		concurrencyLevelsGrowth = mapRange(
			startGrowthIteration, maxGrowthIteration,
			exponentialGrowth,
		)

		padding = decimalWidth(
			concurrencyLevelsGrowth[len(concurrencyLevelsGrowth)-1],
		)
	)

	for i := 0; i <= maxGrowthIteration; i++ {
		name := fmt.Sprintf("TxPool-%d-Concurrent-%0*d",
			txPoolSize, padding, concurrencyLevelsGrowth[i])

		b.Run(name+"/0-Before", func(b *testing.B) {
			benchmarkConcurrentBroadcast(
				b, concurrencyLevelsGrowth[i], txPoolSize,
				false, useSameAddress,
			)
		})

		b.Run(name+"/1-After", func(b *testing.B) {
			benchmarkConcurrentBroadcast(
				b, concurrencyLevelsGrowth[i], txPoolSize, true,
				useSameAddress,
			)
		})
	}
}

// BenchmarkBroadcastAPIConcurrentANDUniqueAddressesUsed benchmarks the new
// Broadcast API vs old PublishTransaction API under concurrent load with unique
// addresses per transaction.
func BenchmarkBroadcastAPIConcurrentANDUniqueAddressesUsed(b *testing.B) {
	const (
		// txPoolSize is the number of transactions to pre-create and
		// reuse across benchmark iterations. Should be at least as
		// large as the highest concurrency level.
		txPoolSize = 1 << 14

		// startGrowthIteration is the starting iteration index for the
		// growth sequence.
		startGrowthIteration = 0

		// maxGrowthIteration is the maximum iteration index for the
		// growth sequence.
		maxGrowthIteration = 14

		// useSameAddress determines whether all transactions in the
		// pool send to the same wallet address (true) or to unique
		// addresses (false). This deliberately enables comprehensive
		// benchmarking unique/duplicate addrs codepaths.
		useSameAddress = false
	)

	var (
		concurrencyLevelsGrowth = mapRange(
			startGrowthIteration, maxGrowthIteration,
			exponentialGrowth,
		)

		padding = decimalWidth(
			concurrencyLevelsGrowth[len(concurrencyLevelsGrowth)-1],
		)
	)

	for i := 0; i <= maxGrowthIteration; i++ {
		name := fmt.Sprintf("TxPool-%d-Concurrent-%0*d",
			txPoolSize, padding, concurrencyLevelsGrowth[i])

		b.Run(name+"/0-Before", func(b *testing.B) {
			benchmarkConcurrentBroadcast(
				b, concurrencyLevelsGrowth[i], txPoolSize,
				false, useSameAddress,
			)
		})

		b.Run(name+"/1-After", func(b *testing.B) {
			benchmarkConcurrentBroadcast(
				b, concurrencyLevelsGrowth[i], txPoolSize,
				true, useSameAddress,
			)
		})
	}
}

// BenchmarkBroadcastAPISequentialANDSameAddressUsed benchmarks the new
// Broadcast API vs old PublishTransaction API under sequential load with the
// same address per transaction.
func BenchmarkBroadcastAPISequentialANDSameAddressUsed(b *testing.B) {
	const (
		// startGrowthIteration is the starting iteration index for the
		// growth sequence.
		startGrowthIteration = 0

		// maxGrowthIteration is the maximum iteration index for the
		// growth sequence.
		maxGrowthIteration = 14

		// useSameAddress determines whether all transactions in the
		// pool send to the same wallet address (true) or to unique
		// addresses (false). This deliberately enables comprehensive
		// benchmarking for unique and duplicate addrs codepaths.
		useSameAddress = true
	)

	var (
		txPoolSizes = mapRange(
			startGrowthIteration, maxGrowthIteration,
			exponentialGrowth,
		)

		padding = decimalWidth(txPoolSizes[len(txPoolSizes)-1])
	)

	for i := 0; i <= maxGrowthIteration; i++ {
		// numInputs equals txPoolSize, with each transaction using one
		// input from the miner's split UTXOs.
		numInputs := txPoolSizes[i]

		// numOutputs is 2x txPoolSize because each transaction has two
		// outputs: one payment output to the wallet address and one
		// change output back to the miner.
		numOutputs := 2 * txPoolSizes[i]

		name := fmt.Sprintf("TxPool-%0*d-Inputs-%d-Outputs-%d",
			padding, txPoolSizes[i], numInputs, numOutputs)

		b.Run(name+"/0-Before", func(b *testing.B) {
			benchmarkSequentialBroadcast(
				b, uint32(txPoolSizes[i]), useSameAddress,
				false,
			)
		})

		b.Run(name+"/1-After", func(b *testing.B) {
			benchmarkSequentialBroadcast(
				b, uint32(txPoolSizes[i]), useSameAddress, true,
			)
		})
	}
}
