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

		// walletOutputsPerTx is held constant at 1 to isolate and
		// purely measure the impact of concurrent broadcast with
		// varying concurrency level and address reuse pattern without
		// confounding from output count variation.
		walletOutputsPerTx = 1

		// useSparseOwnership determines whether wallet-owned outputs
		// are placed sparsely throughout the transaction outputs.
		// Setting this to false since there is only one output per
		// transaction, avoiding confounding from sparse ownership
		// patterns.
		useSparseOwnership = false
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
				b, concurrencyLevelsGrowth[i],
				broadcastBenchmarkConfig{
					txPoolSize:         txPoolSize,
					walletOutputsPerTx: walletOutputsPerTx,
					useNewAPI:          false,
					sameAddress:        useSameAddress,
					sparseOwnership:    useSparseOwnership,
				},
			)
		})

		b.Run(name+"/1-After", func(b *testing.B) {
			benchmarkConcurrentBroadcast(
				b, concurrencyLevelsGrowth[i],
				broadcastBenchmarkConfig{
					txPoolSize:         txPoolSize,
					walletOutputsPerTx: walletOutputsPerTx,
					useNewAPI:          true,
					sameAddress:        useSameAddress,
					sparseOwnership:    useSparseOwnership,
				},
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

		// walletOutputsPerTx is held constant at 1 to isolate and
		// purely measure the impact of concurrent broadcast with
		// varying concurrency level and address reuse pattern without
		// confounding from output count variation.
		walletOutputsPerTx = 1

		// useSparseOwnership determines whether wallet-owned outputs
		// are placed sparsely throughout the transaction outputs.
		// Setting this to false since there is only one output per
		// transaction, avoiding confounding from sparse ownership
		// patterns.
		useSparseOwnership = false
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
				b, concurrencyLevelsGrowth[i],
				broadcastBenchmarkConfig{
					txPoolSize:         txPoolSize,
					walletOutputsPerTx: walletOutputsPerTx,
					useNewAPI:          false,
					sameAddress:        useSameAddress,
					sparseOwnership:    useSparseOwnership,
				},
			)
		})

		b.Run(name+"/1-After", func(b *testing.B) {
			benchmarkConcurrentBroadcast(
				b, concurrencyLevelsGrowth[i],
				broadcastBenchmarkConfig{
					txPoolSize:         txPoolSize,
					walletOutputsPerTx: walletOutputsPerTx,
					useNewAPI:          true,
					sameAddress:        useSameAddress,
					sparseOwnership:    useSparseOwnership,
				},
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

		// walletOutputsPerTx is held constant at 1 to isolate and
		// purely measure the impact of sequential broadcast with
		// varying txPoolSize and address reuse pattern without
		// confounding from output count variation.
		walletOutputsPerTx = 1

		// useSparseOwnership determines whether wallet-owned outputs
		// are placed sparsely throughout the transaction outputs.
		// Setting this to false since there is only one output per
		// transaction, avoiding confounding from sparse ownership
		// patterns.
		useSparseOwnership = false
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
			txPoolSize := uint32(txPoolSizes[i])
			benchmarkSequentialBroadcast(
				b, broadcastBenchmarkConfig{
					txPoolSize:         txPoolSize,
					walletOutputsPerTx: walletOutputsPerTx,
					useNewAPI:          false,
					sameAddress:        useSameAddress,
					sparseOwnership:    useSparseOwnership,
				},
			)
		})

		b.Run(name+"/1-After", func(b *testing.B) {
			txPoolSize := uint32(txPoolSizes[i])
			benchmarkSequentialBroadcast(
				b, broadcastBenchmarkConfig{
					txPoolSize:         txPoolSize,
					walletOutputsPerTx: walletOutputsPerTx,
					useNewAPI:          true,
					sameAddress:        useSameAddress,
					sparseOwnership:    useSparseOwnership,
				},
			)
		})
	}
}

// BenchmarkBroadcastAPISequentialANDUniqueAddressesUsed benchmarks the new
// Broadcast API vs old PublishTransaction API under sequential load with unique
// addresses per transaction.
func BenchmarkBroadcastAPISequentialANDUniqueAddressesUsed(b *testing.B) {
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
		// benchmarking unique/duplicate addrs codepaths.
		useSameAddress = false

		// walletOutputsPerTx is held constant at 1 to isolate and
		// purely measure the impact of sequential broadcast with
		// varying txPoolSize and address reuse pattern without
		// confounding from output count variation.
		walletOutputsPerTx = 1

		// useSparseOwnership determines whether wallet-owned outputs
		// are placed sparsely throughout the transaction outputs.
		// Setting this to false since there is only one output per
		// transaction, avoiding confounding from sparse ownership
		// patterns.
		useSparseOwnership = false
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
			txPoolSize := uint32(txPoolSizes[i])
			benchmarkSequentialBroadcast(
				b, broadcastBenchmarkConfig{
					txPoolSize:         txPoolSize,
					walletOutputsPerTx: walletOutputsPerTx,
					useNewAPI:          false,
					sameAddress:        useSameAddress,
					sparseOwnership:    useSparseOwnership,
				},
			)
		})

		b.Run(name+"/1-After", func(b *testing.B) {
			txPoolSize := uint32(txPoolSizes[i])
			benchmarkSequentialBroadcast(
				b, broadcastBenchmarkConfig{
					txPoolSize:         txPoolSize,
					walletOutputsPerTx: walletOutputsPerTx,
					useNewAPI:          true,
					sameAddress:        useSameAddress,
					sparseOwnership:    useSparseOwnership,
				},
			)
		})
	}
}

// BenchmarkBroadcastAPIMultiOutputSameAddress benchmarks the optimization's
// maximum impact when transactions have many wallet-owned outputs all paying
// to the same wallet address.
func BenchmarkBroadcastAPIMultiOutputSameAddress(b *testing.B) {
	const (
		// txPoolSize is small because each transaction is expensive
		// to create with many wallet-owned outputs.
		txPoolSize = 100

		// useSameAddress determines whether all transactions in the
		// pool send to the same wallet address (true) or to unique
		// addresses (false). This deliberately enables comprehensive
		// benchmarking unique/duplicate addrs codepaths.
		useSameAddress = true

		// useSparseOwnership determines whether wallet-owned outputs
		// are placed sparsely throughout the transaction outputs.
		// Setting this to false since all outputs are wallet-owned in
		// this benchmark, avoiding confounding from sparse ownership
		// patterns.
		useSparseOwnership = false
	)

	var (
		walletOutputCounts = mapRange(0, 14, exponentialGrowth)

		padding = decimalWidth(
			walletOutputCounts[len(walletOutputCounts)-1],
		)
	)

	for _, walletOutputsPerTx := range walletOutputCounts {
		name := fmt.Sprintf("TxPool-%d-WalletOutputsPerTx-%0*d",
			txPoolSize, padding, walletOutputsPerTx)

		b.Run(name+"/0-Before", func(b *testing.B) {
			benchmarkSequentialBroadcast(
				b, broadcastBenchmarkConfig{
					txPoolSize:         txPoolSize,
					walletOutputsPerTx: walletOutputsPerTx,
					useNewAPI:          false,
					sameAddress:        useSameAddress,
					sparseOwnership:    useSparseOwnership,
				},
			)
		})

		b.Run(name+"/1-After", func(b *testing.B) {
			benchmarkSequentialBroadcast(
				b, broadcastBenchmarkConfig{
					txPoolSize:         txPoolSize,
					walletOutputsPerTx: walletOutputsPerTx,
					useNewAPI:          true,
					sameAddress:        useSameAddress,
					sparseOwnership:    useSparseOwnership,
				},
			)
		})
	}
}
