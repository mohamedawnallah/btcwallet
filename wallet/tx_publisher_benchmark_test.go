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

		padding = calculatePadding(
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
