package wallet

import (
	"bytes"
	"fmt"
	"sort"
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/stretchr/testify/require"
)

// BenchmarkGetTxAPI benchmarks GetTx API and its deprecated variant
// GetTransaction using identical test data across transactions with varying
// complexity (input/output counts). Test names start with transaction
// complexity to group API comparisons for benchstat analysis.
//
// Time Complexity Analysis:
// GetTx has no amortization - it's a read operation with consistent upper/tight
// bound cost every time. The time complexity is O(log n + I + O) where:
//   - n: number of transactions in the database (B-tree lookup)
//   - I: number of inputs in the transaction
//   - O: number of outputs in the transaction
func BenchmarkGetTxAPI(b *testing.B) {
	const (
		// startGrowthIteration is the starting iteration index for the
		// growth sequence.
		startGrowthIteration = 0

		// maxGrowthIteration is the maximum iteration index for the
		// growth sequence.
		maxGrowthIteration = 10
	)

	var (
		// accountGrowth uses constantGrowth since account count doesn't
		// affect the API's time complexity.
		accountGrowth = mapRange(
			startGrowthIteration, maxGrowthIteration,
			constantGrowth,
		)

		// addressGrowth uses constantGrowth since address count doesn't
		// affect the API's time complexity.
		addressGrowth = mapRange(
			startGrowthIteration, maxGrowthIteration,
			constantGrowth,
		)

		// walletTxsGrowth uses linearGrowth to test O(log n) B-tree
		// lookup scaling. As database size grows linearly, lookup time
		// should grow logarithmically, demonstrating sublinear scaling.
		walletTxsGrowth = mapRange(
			startGrowthIteration, maxGrowthIteration,
			linearGrowth,
		)

		// txIOGrowth uses symmetric exponentialGrowth for both inputs
		// and outputs to stress test the O(I + O) processing cost with
		// rapidly growing transaction complexity, exposing potential
		// performance bottlenecks in input/output iteration and address
		// extraction.
		txIOGrowth = mapRange(
			startGrowthIteration, maxGrowthIteration,
			exponentialGrowth,
		)

		walletTxsGrowthPadding = decimalWidth(
			walletTxsGrowth[len(walletTxsGrowth)-1],
		)

		txIOGrowthPadding = decimalWidth(
			txIOGrowth[len(txIOGrowth)-1],
		)

		scopes = []waddrmgr.KeyScope{waddrmgr.KeyScopeBIP0084}
	)

	for i := 0; i <= maxGrowthIteration; i++ {
		name := fmt.Sprintf("%0*d-UTXOs-%0*d-TxInputs-%0*d-TxOutputs",
			walletTxsGrowthPadding, walletTxsGrowth[i],
			txIOGrowthPadding, txIOGrowth[i],
			txIOGrowthPadding, txIOGrowth[i])

		b.Run(name, func(b *testing.B) {
			// Setup wallet once for both API benchmarks.
			bw := setupBenchmarkWallet(
				b, benchmarkWalletConfig{
					scopes:       scopes,
					numAccounts:  accountGrowth[i],
					numAddresses: addressGrowth[i],
					numWalletTxs: walletTxsGrowth[i],
					numTxInputs:  txIOGrowth[i],
					numTxOutputs: txIOGrowth[i],
				},
			)

			// Get a transaction hash from the middle of the dataset
			// for representative benchmarking.
			medianIndex := len(bw.confirmedTxs) / 2
			testTxHash := bw.confirmedTxs[medianIndex].TxHash()

			var (
				beforeResult *GetTransactionResult
				afterResult  *TxDetail
			)

			b.Run("0-Before", func(b *testing.B) {
				var (
					result      *GetTransactionResult
					firstResult *GetTransactionResult
					err         error
				)

				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; b.Loop(); i++ {
					result, err = bw.GetTransaction(
						testTxHash,
					)
					require.NoError(b, err)

					// Capture first result only.
					if i == 0 {
						firstResult = result
					}
				}

				require.Equal(
					b, firstResult, result,
					"GetTransaction API should be "+
						"idempotent",
				)

				beforeResult = result
			})

			b.Run("1-After", func(b *testing.B) {
				var (
					result      *TxDetail
					firstResult *TxDetail
					err         error
				)

				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; b.Loop(); i++ {
					result, err = bw.GetTx(
						b.Context(), testTxHash,
					)
					require.NoError(b, err)

					// Capture first result only.
					if i == 0 {
						firstResult = result
					}
				}

				require.Equal(
					b, firstResult, result,
					"GetTx API should be idempotent",
				)

				afterResult = result
			})

			// Verify API equivalence after benchmarks complete.
			// This ensures:
			//   - Both APIs return consistent results for the same
			//     transaction
			//   - The new API maintains compatibility with the
			//     legacy API
			//   - Regression prevention for future changes
			assertGetTxAPIsEquivalent(
				b, bw.Wallet, beforeResult, afterResult,
			)
		})
	}
}

// BenchmarkListTxnsAPI benchmarks ListTxns API and its deprecated variant
// GetTransactions using identical test data across varying block ranges and
// transaction densities. Test names start with complexity metrics to group API
// comparisons for benchstat analysis.
//
// Time Complexity Analysis:
// ListTxns has no amortization - it's a read operation with consistent
// upper/tight bound cost. The time complexity is O(B * T * (I + O)) where:
//   - B: number of blocks in the range [startHeight, endHeight]
//   - T: average transactions per block
//   - I: average inputs per transaction
//   - O: average outputs per transaction
//
// This simplifies to O(N) where N = total inputs + outputs across all
// transactions in the block range.
func BenchmarkListTxnsAPI(b *testing.B) {
	const (
		// startGrowthIteration is the starting iteration index for the
		// growth sequence.
		startGrowthIteration = 0

		// maxGrowthIteration is the maximum iteration index for the
		// growth sequence.
		maxGrowthIteration = 10
	)

	var (
		// accountGrowth uses constantGrowth since account count doesn't
		// affect the API's time complexity.
		accountGrowth = mapRange(
			startGrowthIteration, maxGrowthIteration,
			constantGrowth,
		)

		// addressGrowth uses constantGrowth since address count doesn't
		// affect the API's time complexity.
		addressGrowth = mapRange(
			startGrowthIteration, maxGrowthIteration,
			constantGrowth,
		)

		// walletTxsGrowth uses exponentialGrowth to stress test the
		// O(B * T) component - total transactions across blocks.
		walletTxsGrowth = mapRange(
			startGrowthIteration, maxGrowthIteration,
			exponentialGrowth,
		)

		// txIOGrowth uses exponentialGrowth for both inputs and outputs
		// to stress test the O(I + O) per-transaction processing cost
		// with rapidly growing transaction complexity.
		txIOGrowth = mapRange(
			startGrowthIteration, maxGrowthIteration,
			exponentialGrowth,
		)

		walletTxsGrowthPadding = decimalWidth(
			walletTxsGrowth[len(walletTxsGrowth)-1],
		)

		txIOGrowthPadding = decimalWidth(
			txIOGrowth[len(txIOGrowth)-1],
		)

		scopes = []waddrmgr.KeyScope{waddrmgr.KeyScopeBIP0084}
	)

	for i := 0; i <= maxGrowthIteration; i++ {
		name := fmt.Sprintf("%0*d-Txs-%0*d-TxInputs-%0*d-TxOutputs",
			walletTxsGrowthPadding, walletTxsGrowth[i],
			txIOGrowthPadding, txIOGrowth[i],
			txIOGrowthPadding, txIOGrowth[i])

		b.Run(name, func(b *testing.B) {
			// Setup wallet once for both API benchmarks.
			bw := setupBenchmarkWallet(
				b, benchmarkWalletConfig{
					scopes:       scopes,
					numAccounts:  accountGrowth[i],
					numAddresses: addressGrowth[i],
					numWalletTxs: walletTxsGrowth[i],
					numTxInputs:  txIOGrowth[i],
					numTxOutputs: txIOGrowth[i],
				},
			)

			// List all transactions (no height filter).
			// For GetTransactions (old): nil, nil means all blocks
			// For ListTxns (new): 0, -1 means all blocks
			// (0=genesis, -1=unlimited).
			var (
				startBlock  *BlockIdentifier
				endBlock    *BlockIdentifier
				startHeight int32 = 0
				endHeight   int32 = -1
			)

			var (
				beforeResult *GetTransactionsResult
				afterResult  []*TxDetail
			)

			b.Run("0-Before", func(b *testing.B) {
				var (
					result      *GetTransactionsResult
					firstResult *GetTransactionsResult
					err         error
				)

				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; b.Loop(); i++ {
					result, err = bw.GetTransactions(
						startBlock, endBlock, "", nil,
					)
					require.NoError(b, err)

					// Capture first result only.
					if i == 0 {
						firstResult = result
					}
				}

				require.Equal(
					b, firstResult, result,
					"GetTransactions API should be "+
						"idempotent",
				)

				beforeResult = result
			})

			b.Run("1-After", func(b *testing.B) {
				var (
					result      []*TxDetail
					firstResult []*TxDetail
					err         error
				)

				b.ReportAllocs()
				b.ResetTimer()

				for i := 0; b.Loop(); i++ {
					result, err = bw.ListTxns(
						b.Context(), startHeight,
						endHeight,
					)
					require.NoError(b, err)

					// Capture first result only.
					if i == 0 {
						firstResult = result
					}
				}

				require.Equal(
					b, firstResult, result,
					"ListTxns API should be idempotent ",
				)

				afterResult = result
			})

			// Verify API equivalence after benchmarks complete.
			// This ensures:
			//   - Both APIs return consistent results for the same
			//     block range
			//   - The new API maintains compatibility with the
			//     legacy API
			//   - Regression prevention for future changes
			assertListTxnsAPIsEquivalent(
				b, bw.Wallet, beforeResult, afterResult,
			)
		})
	}
}

// assertGetTxAPIsEquivalent verifies that GetTransaction (legacy) and GetTx
// (new) return equivalent data for the same transaction.
func assertGetTxAPIsEquivalent(b *testing.B, w *Wallet,
	before *GetTransactionResult, after *TxDetail) {

	b.Helper()

	require.NotNil(b, before)
	require.NotNil(b, after)

	// Convert TxDetail to GetTransactionResult for comparison.
	afterConverted, err := fillGetTxResultConstruct(w, after)
	require.NoError(b, err)

	// Compare the entire structures.
	require.Equal(b, before, afterConverted)
}

// assertListTxnsAPIsEquivalent verifies that GetTransactions (legacy) and
// ListTxns (new) return equivalent data for the same block range.
func assertListTxnsAPIsEquivalent(b *testing.B, w *Wallet,
	before *GetTransactionsResult, after []*TxDetail) {

	b.Helper()

	require.NotNil(b, before)
	require.NotNil(b, after)

	// Convert all TxDetails to GetTransactionsResult.
	var results []*GetTransactionsResult
	for _, detail := range after {
		result, err := fillGetTxnsResultConstruct(w, detail)
		require.NoError(b, err)
		results = append(results, result)
	}

	// Group mined transactions by block height and sort.
	minedBlocks := fillBlocksConstruct(results)

	// Collect all unmined transactions.
	var unminedTxs []TransactionSummary
	for _, result := range results {
		unminedTxs = append(unminedTxs, result.UnminedTransactions...)
	}

	after_converted := &GetTransactionsResult{
		MinedTransactions:   minedBlocks,
		UnminedTransactions: unminedTxs,
	}

	// Sort both results for deterministic comparison.
	sort.Slice(before.MinedTransactions, func(i, j int) bool {
		return before.MinedTransactions[i].Height <
			before.MinedTransactions[j].Height
	})

	sort.Slice(after_converted.MinedTransactions, func(i, j int) bool {
		return after_converted.MinedTransactions[i].Height <
			after_converted.MinedTransactions[j].Height
	})

	sort.Slice(before.UnminedTransactions, func(i, j int) bool {
		return before.UnminedTransactions[i].Timestamp <
			before.UnminedTransactions[j].Timestamp
	})

	sort.Slice(after_converted.UnminedTransactions, func(i, j int) bool {
		return after_converted.UnminedTransactions[i].Timestamp <
			after_converted.UnminedTransactions[j].Timestamp
	})

	require.Equal(
		b, before, after_converted,
		"GetTransactions and ListTxns APIs should return equivalent "+
			"data",
	)
}

// fillGetTxResultConstruct converts a TxDetail to GetTransactionResult format.
func fillGetTxResultConstruct(w *Wallet,
	detail *TxDetail) (*GetTransactionResult, error) {

	txSummary, err := fillTxSummaryConstruct(w, detail)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to summary: %w", err)
	}

	result := &GetTransactionResult{
		Summary:       *txSummary,
		Height:        detail.Block.Height,
		Confirmations: detail.Confirmations,
		Timestamp:     detail.Block.Timestamp,
	}

	// Set BlockHash only if transaction is confirmed.
	if detail.Block != nil && detail.Block.Height >= 0 {
		result.BlockHash = &detail.Block.Hash
	}

	return result, nil
}

// fillGetTxnsResultConstruct converts a single TxDetail to
// GetTransactionsResult format, wrapping it in the appropriate block structure.
func fillGetTxnsResultConstruct(w *Wallet,
	detail *TxDetail) (*GetTransactionsResult, error) {

	txSummary, err := fillTxSummaryConstruct(w, detail)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to summary: %w", err)
	}

	result := &GetTransactionsResult{
		MinedTransactions:   []Block{},
		UnminedTransactions: []TransactionSummary{},
	}

	if detail.Block != nil && detail.Block.Height >= 0 {
		// Mined transaction - create block with single tx.
		block := Block{
			Hash:         &detail.Block.Hash,
			Height:       detail.Block.Height,
			Timestamp:    detail.Block.Timestamp,
			Transactions: []TransactionSummary{*txSummary},
		}
		result.MinedTransactions = append(
			result.MinedTransactions, block,
		)
	} else {
		// Unmined transaction.
		result.UnminedTransactions = append(
			result.UnminedTransactions, *txSummary,
		)
	}

	return result, nil
}

// convertTxDetailToTransactionSummary converts a single TxDetail to
// TransactionSummary format.
func fillTxSummaryConstruct(w *Wallet,
	detail *TxDetail) (*TransactionSummary, error) {

	// Deserialize the raw transaction to get full tx data.
	var msgTx wire.MsgTx
	err := msgTx.Deserialize(bytes.NewReader(detail.RawTx))
	if err != nil {
		return nil, fmt.Errorf("failed to deserialize tx: %w", err)
	}

	// Build MyInputs from PrevOuts that are ours.
	myInputs, err := fillMyInputsConstruct(w, detail)
	if err != nil {
		return nil, err
	}

	// Build MyOutputs from Outputs that are ours.
	myOutputs, err := fillMyOutputsConstruct(w, detail)
	if err != nil {
		return nil, err
	}

	txSummary := &TransactionSummary{
		Hash:        &detail.Hash,
		Transaction: detail.RawTx,
		Fee:         detail.Fee,
		Timestamp:   detail.ReceivedTime.Unix(),
		Label:       detail.Label,
		Tx:          &msgTx,
		MyInputs:    myInputs,
		MyOutputs:   myOutputs,
	}

	return txSummary, nil
}

// fillMyInputsConstruct builds TransactionSummaryInput slice from PrevOuts
// that belong to the wallet.
func fillMyInputsConstruct(w *Wallet,
	detail *TxDetail) ([]TransactionSummaryInput, error) {

	var myInputs []TransactionSummaryInput
	for i, prevOut := range detail.PrevOuts {
		if !prevOut.IsOurs {
			continue
		}

		// Look up the previous output to get account and amount.
		var (
			account uint32
			amount  btcutil.Amount
		)

		err := walletdb.View(w.db, func(dbtx walletdb.ReadTx) error {
			txmgrNs := dbtx.ReadBucket(wtxmgrNamespaceKey)
			addrmgrNs := dbtx.ReadBucket(waddrmgrNamespaceKey)

			// Get the previous transaction output details.
			prevTxDetails, err := w.txStore.TxDetails(
				txmgrNs, &prevOut.OutPoint.Hash,
			)
			if err != nil {
				return err
			}

			if prevTxDetails == nil {
				return fmt.Errorf("previous tx not found")
			}

			// Get the output amount.
			if int(prevOut.OutPoint.Index) >=
				len(prevTxDetails.MsgTx.TxOut) {
				return fmt.Errorf("output index out of range")
			}
			prevOutIndex := prevOut.OutPoint.Index
			txOut := prevTxDetails.MsgTx.TxOut[prevOutIndex]
			amount = btcutil.Amount(txOut.Value)

			// Look up account from output script.
			_, addrs, _, err := txscript.ExtractPkScriptAddrs(
				txOut.PkScript, w.chainParams,
			)
			if err == nil && len(addrs) > 0 {
				_, account, err = w.addrStore.AddrAccount(
					addrmgrNs, addrs[0],
				)
				if err != nil {
					return err
				}
			}

			return nil
		})

		if err != nil {
			return nil, fmt.Errorf("failed to lookup input %d: %w",
				i, err)
		}

		myInputs = append(
			myInputs, TransactionSummaryInput{
				Index:           uint32(i),
				PreviousAccount: account,
				PreviousAmount:  amount,
			},
		)
	}

	return myInputs, nil
}

// fillMyOutputsConstruct builds TransactionSummaryOutput slice from Outputs
// that belong to the wallet.
func fillMyOutputsConstruct(w *Wallet,
	detail *TxDetail) ([]TransactionSummaryOutput, error) {

	var myOutputs []TransactionSummaryOutput
	for _, output := range detail.Outputs {
		if !output.IsOurs {
			continue
		}

		var (
			account  uint32
			internal bool
		)

		err := walletdb.View(w.db, func(dbtx walletdb.ReadTx) error {
			addrmgrNs := dbtx.ReadBucket(waddrmgrNamespaceKey)

			// Look up managed address.
			if len(output.Addresses) > 0 {
				ma, err := w.addrStore.Address(
					addrmgrNs, output.Addresses[0],
				)
				if err != nil {
					return err
				}

				account = ma.InternalAccount()
				internal = ma.Internal()
			}

			return nil
		})

		if err != nil {
			return nil, fmt.Errorf(
				"failed to lookup output %d: %w", output.Index,
				err)
		}

		myOutputs = append(myOutputs, TransactionSummaryOutput{
			Index:    uint32(output.Index),
			Account:  account,
			Internal: internal,
		})
	}

	return myOutputs, nil
}

// fillBlocksConstruct groups transactions by block height and returns a
// slice of blocks.
func fillBlocksConstruct(results []*GetTransactionsResult) []Block {
	// Use a map to group transactions by block height.
	blockMap := make(map[int32]*Block)

	for _, result := range results {
		for _, block := range result.MinedTransactions {
			if existing, ok := blockMap[block.Height]; ok {
				// Append to existing block.
				existing.Transactions = append(
					existing.Transactions,
					block.Transactions...,
				)
			} else {
				// Create new block entry.
				blockCopy := block
				blockMap[block.Height] = &blockCopy
			}
		}
	}

	// Convert map to slice. The order doesn't need to be guarnteed since
	// it can be sorted by the caller if the order property is of
	// interest.
	blocks := make([]Block, 0, len(blockMap))
	for _, block := range blockMap {
		blocks = append(blocks, *block)
	}

	return blocks
}
