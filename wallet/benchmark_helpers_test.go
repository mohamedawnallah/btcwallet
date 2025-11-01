package wallet

import (
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/hdkeychain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/integration/rpctest"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/chain"
	"github.com/btcsuite/btcwallet/waddrmgr"
	"github.com/btcsuite/btcwallet/walletdb"
	"github.com/btcsuite/btcwallet/wtxmgr"
	"github.com/stretchr/testify/require"
)

var errAccountNotFound = errors.New("account not found")

// growthFunc defines how a benchmark parameter should scale with iteration
// index. It takes an iteration index i (0-based) and returns the parameter
// value for that iteration. This allows flexible configuration of benchmark
// data sizes with different growth patterns (linear, exponential, logarithmic,
// etc.).
type growthFunc func(i int) int

// constantGrowth returns a constant value regardless of iteration.
func constantGrowth(i int) int {
	return 5
}

// linearGrowth scales the parameter value linearly.
func linearGrowth(i int) int {
	return 5 + (i * 5)
}

// exponentialGrowth scales the parameter value exponentially.
func exponentialGrowth(i int) int {
	return 1 << i
}

// mapRange maps fn over indices [start..end] (inclusive) and returns the
// results. This provides functional-style array generation for benchmarks.
//
//nolint:unparam
func mapRange(start, end int, fn growthFunc) []int {
	result := make([]int, end-start+1)
	for i := range result {
		result[i] = fn(start + i)
	}

	return result
}

// decimalWidth returns the number of characters in the decimal representation
// of given value.
func decimalWidth(value int) int {
	return len(strconv.Itoa(value))
}

// benchmarkWalletConfig holds configuration for benchmark wallet setup.
type benchmarkWalletConfig struct {
	// scopes is the key scopes to create accounts in.
	scopes []waddrmgr.KeyScope

	// numAccounts is the number of accounts to create.
	numAccounts int

	// numUTXOs is the number of UTXOs to create.
	numUTXOs int

	// numAddresses is the number of addresses to create.
	numAddresses int

	// miner is an optional btcd regtest harness. If provided, the wallet
	// will be connected to the miner via RPC for chain integration tests.
	miner *rpctest.Harness
}

// benchmarkWallet holds a wallet and its created UTXO outpoints.
type benchmarkWallet struct {
	*Wallet

	outpoints []wire.OutPoint

	// chainConn is the RPC connection to btcd if miner was provided.
	chainConn chain.Interface
}

// setupBenchmarkWallet creates a wallet with test data based on the provided
// configuration. It distributes accounts evenly across the specified scopes
// and returns the wallet along with the outpoints of all created UTXOs. If
// config.miner is provided, the wallet is connected to the btcd node via RPC.
func setupBenchmarkWallet(tb testing.TB,
	config benchmarkWalletConfig) *benchmarkWallet {

	tb.Helper()

	// Since testWallet requires a *testing.T, we can't pass the benchmark's
	// *testing.B. Instead, we create a setup *testing.T and manually fail
	// the benchmark if the setup fails.
	setupT := &testing.T{}
	w := testWallet(setupT)
	require.False(tb, setupT.Failed(), "testWallet setup failed")

	var chainConn *chain.RPCClient

	// If miner provided, connect wallet to btcd.
	if config.miner != nil {
		rpcConfig := config.miner.RPCConfig()
		clientConfig := &chain.RPCClientConfig{
			Conn:              &rpcConfig,
			Chain:             &chaincfg.RegressionNetParams,
			ReconnectAttempts: 20,
		}

		var err error

		chainConn, err = chain.NewRPCClientWithConfig(clientConfig)
		require.NoError(tb, err)

		err = chainConn.Start()
		require.NoError(tb, err)

		tb.Cleanup(func() {
			chainConn.Stop()
			chainConn.WaitForShutdown()
		})

		// Attach chain client to wallet.
		w.chainClient = chainConn
		w.chainClientLock.Lock()
		w.chainClientSynced = true
		w.chainClientLock.Unlock()
	}

	addresses := createTestAccounts(
		tb, w, config.scopes, config.numAccounts,
		config.numAddresses,
	)

	outpoints := createTestUTXOs(tb, w, addresses, config.numUTXOs)

	// Sync wallet to the block height where UTXOs were created.
	setSyncedToHeight(tb, w, 1)

	return &benchmarkWallet{
		Wallet:    w,
		outpoints: outpoints,
		chainConn: chainConn,
	}
}

// setSyncedToHeight updates the wallet's synced block height. This is useful
// for benchmark tests to ensure confirmation calculations work correctly.
func setSyncedToHeight(tb testing.TB, w *Wallet, height int32) {
	tb.Helper()

	err := walletdb.Update(w.db, func(tx walletdb.ReadWriteTx) error {
		addrmgrNs := tx.ReadWriteBucket(waddrmgrNamespaceKey)

		return w.addrStore.SetSyncedTo(addrmgrNs, &waddrmgr.BlockStamp{
			Height: height,
			Hash:   chainhash.Hash{},
		})
	})
	require.NoError(tb, err, "failed to set synced height to %d", height)
}

// createTestAccounts creates test accounts across the specified key scopes
// and returns all generated addresses.
func createTestAccounts(tb testing.TB, w *Wallet, scopes []waddrmgr.KeyScope,
	numAccounts, numAddresses int) []waddrmgr.ManagedAddress {

	tb.Helper()

	var allAddresses []waddrmgr.ManagedAddress

	err := walletdb.Update(w.db, func(tx walletdb.ReadWriteTx) error {
		// Distribute accounts across the specified key scopes.
		accountsPerScope := numAccounts / len(scopes)
		remainder := numAccounts % len(scopes)

		for i, scope := range scopes {
			scopeAccounts := accountsPerScope
			if i < remainder {
				// Distribute remainder accounts.
				scopeAccounts++
			}

			err := createAccountsInScope(
				w, tx, scope, scopeAccounts, numAddresses,
				i*accountsPerScope, &allAddresses,
			)
			if err != nil {
				return err
			}
		}

		return nil
	})

	require.NoError(tb, err, "failed to create test accounts: %v", err)

	return allAddresses
}

// createAccountsInScope creates accounts within a specific scope with unique
// naming across scopes.
func createAccountsInScope(w *Wallet, tx walletdb.ReadWriteTx,
	scope waddrmgr.KeyScope, numAccounts, numAddresses, offset int,
	allAddresses *[]waddrmgr.ManagedAddress) error {

	manager, err := w.addrStore.FetchScopedKeyManager(scope)
	if err != nil {
		return err
	}

	addrmgrNs := tx.ReadWriteBucket(waddrmgrNamespaceKey)

	for i := range numAccounts {
		name := fmt.Sprintf("bench-scope-%d-%d-account-%d",
			scope.Purpose, scope.Coin, offset+i)

		account, err := manager.NewAccount(addrmgrNs, name)
		if err != nil {
			return err
		}

		addrs, err := manager.NextExternalAddresses(
			addrmgrNs, account, uint32(numAddresses),
		)
		if err != nil {
			return err
		}

		*allAddresses = append(*allAddresses, addrs...)
	}

	return nil
}

// createTestUTXOs creates the specified number of test UTXOs using the provided
// addresses for benchmark data setup. It returns the outpoints of all created
// UTXOs.
func createTestUTXOs(tb testing.TB, w *Wallet,
	addresses []waddrmgr.ManagedAddress, numUTXOs int) []wire.OutPoint {

	tb.Helper()

	var outpoints []wire.OutPoint

	err := walletdb.Update(w.db, func(tx walletdb.ReadWriteTx) error {
		txmgrNs := tx.ReadWriteBucket(wtxmgrNamespaceKey)
		addrmgrNs := tx.ReadWriteBucket(waddrmgrNamespaceKey)
		msgTx := TstTx.MsgTx()

		blockMeta := &wtxmgr.BlockMeta{
			Block: wtxmgr.Block{
				Hash:   chainhash.Hash{},
				Height: 1,
			},
			Time: time.Now(),
		}

		for i := 0; i < numUTXOs && i < len(addresses); i++ {
			newMsgTx := wire.NewMsgTx(msgTx.Version)
			addr := addresses[i%len(addresses)]

			pkScript, err := txscript.PayToAddrScript(
				addr.Address(),
			)
			if err != nil {
				return err
			}

			// Add a dummy tx output to make it valid.
			amount := btcutil.Amount(100000 + i*1000)
			txOut := wire.NewTxOut(int64(amount), pkScript)
			newMsgTx.AddTxOut(txOut)

			// Add a dummy tx input to make it valid.
			prevHash := chainhash.Hash{}
			prevHash[0] = byte(i)
			txIn := wire.NewTxIn(
				wire.NewOutPoint(&prevHash, 0), nil, nil,
			)
			newMsgTx.AddTxIn(txIn)

			rec, err := wtxmgr.NewTxRecordFromMsgTx(
				newMsgTx, time.Now(),
			)
			if err != nil {
				return err
			}

			err = w.txStore.InsertTx(txmgrNs, rec, blockMeta)
			if err != nil {
				return err
			}

			// Mark the output as unspent.
			err = w.txStore.AddCredit(
				txmgrNs, rec, blockMeta, 0, false,
			)
			if err != nil {
				return err
			}

			err = w.addrStore.MarkUsed(
				addrmgrNs, addr.Address(),
			)
			if err != nil {
				return err
			}

			// Store the actual outpoint for later use.
			outpoints = append(outpoints, wire.OutPoint{
				Hash:  rec.Hash,
				Index: 0,
			})
		}

		return nil
	})

	require.NoError(tb, err, "failed to create test UTXOs: %v", err)

	return outpoints
}

// generateAccountName generates a consistent account name and number for
// benchmarking based on the given number of accounts and scopes. It returns
// the first account name and number in the last scope, which provides a good
// heuristic case for evaluating search performance.
func generateAccountName(numAccounts int,
	scopes []waddrmgr.KeyScope) (string, uint32) {

	accountsPerScope := numAccounts / len(scopes)

	lastScopeIndex := len(scopes) - 1
	lastScope := scopes[lastScopeIndex]
	lastScopeOffset := lastScopeIndex * accountsPerScope

	accountName := fmt.Sprintf("bench-scope-%d-%d-account-%d",
		lastScope.Purpose, lastScope.Coin, lastScopeOffset)

	// Account numbers start from 1, not 0. Account 0 is reserved for
	// "default".
	accountNumber := uint32(lastScopeOffset + 1)

	return accountName, accountNumber
}

// generateTestExtendedKey generates a test extended public key for benchmarking
// ImportAccount operations. It uses a deterministic seed based on the
// seed index to ensure consistent and unique results across benchmark runs.
func generateTestExtendedKey(tb testing.TB,
	seedIndex int) (*hdkeychain.ExtendedKey, uint32, waddrmgr.AddressType) {

	tb.Helper()

	// Use a simple deterministic seed based on seed index.
	seed := make([]byte, 32)
	for j := range seed {
		seed[j] = byte(seedIndex + j)
	}

	// Create master key from seed.
	masterKey, err := hdkeychain.NewMaster(seed, &chaincfg.TestNet3Params)
	require.NoError(tb, err)

	// Derive account key for BIP0084 (m/84'/1'/seedIndex').
	purpose, err := masterKey.Derive(hdkeychain.HardenedKeyStart + 84)
	require.NoError(tb, err)

	coin, err := purpose.Derive(hdkeychain.HardenedKeyStart + 1)
	require.NoError(tb, err)

	account, err := coin.Derive(
		hdkeychain.HardenedKeyStart + uint32(seedIndex),
	)
	require.NoError(tb, err)

	accountPubKey, err := account.Neuter()
	require.NoError(tb, err)

	return accountPubKey, uint32(seedIndex), waddrmgr.WitnessPubKey
}

// getMedianTestAddress returns a median address from a median account for
// benchmarking purposes.
func getTestAddress(tb testing.TB, w *Wallet, numAccounts int) btcutil.Address {
	tb.Helper()

	medianAccount := uint32(numAccounts / 2)
	addresses, err := w.AccountAddresses(medianAccount)
	require.NoError(tb, err)

	return addresses[len(addresses)/2]
}

// markAddressAsUsed marks an address as used in the wallet database. This is
// useful for making benchmark iterations idempotent.
func markAddressAsUsed(b *testing.B, w *Wallet, addr btcutil.Address) {
	b.Helper()

	err := walletdb.Update(w.db, func(tx walletdb.ReadWriteTx) error {
		addrmgrNs := tx.ReadWriteBucket(waddrmgrNamespaceKey)

		manager, err := w.addrStore.FetchScopedKeyManager(
			waddrmgr.KeyScopeBIP0044,
		)
		if err != nil {
			return err
		}

		return manager.MarkUsed(addrmgrNs, addr)
	})
	require.NoError(b, err)
}

// getTestUtxoOutpoint returns a median UTXO outpoint from the provided list
// for benchmarking purposes. It returns the outpoint from the middle of the
// list to provide a representative test case.
func getTestUtxoOutpoint(outpoints []wire.OutPoint) wire.OutPoint {
	medianIndex := len(outpoints) / 2
	return outpoints[medianIndex]
}

// generateTestTapscript generates a test tapscript for benchmarking purposes.
// It creates a simple script that checks a signature against the provided
// public key, wraps it in a tap leaf, and returns a complete Tapscript
// structure ready for import.
func generateTestTapscript(tb testing.TB,
	pubKey *btcec.PublicKey) waddrmgr.Tapscript {

	tb.Helper()

	script, err := txscript.NewScriptBuilder().
		AddData(pubKey.SerializeCompressed()).
		AddOp(txscript.OP_CHECKSIG).
		Script()
	require.NoError(tb, err)

	leaf := txscript.NewTapLeaf(txscript.BaseLeafVersion, script)

	return waddrmgr.Tapscript{
		Type: waddrmgr.TapscriptTypeFullTree,
		ControlBlock: &txscript.ControlBlock{
			InternalKey: pubKey,
		},
		Leaves: []txscript.TapLeaf{leaf},
	}
}

// generateTestTxOut generates a test TxOut for benchmarking purposes.
// It creates a TxOut with the provided address as the PkScript.
func generateTestTxOut(tb testing.TB, addr btcutil.Address) wire.TxOut {
	tb.Helper()

	pkScript, err := txscript.PayToAddrScript(addr)
	require.NoError(tb, err)

	return wire.TxOut{
		Value:    1e8,
		PkScript: pkScript,
	}
}

// leaseAllOutputs leases all outputs in the wallet with unique lock IDs. This
// is used to set up benchmarks for ListLeasedOutputs where we want to maximize
// the N+1 query impact when comparing the new vs deprecated ListLeasedOutputs
// APIs.
func leaseAllOutputs(tb testing.TB, w *Wallet, outpoints []wire.OutPoint,
	duration time.Duration) {

	tb.Helper()

	for i, outpoint := range outpoints {
		lockID := wtxmgr.LockID{byte(i)}
		_, err := w.LeaseOutput(
			tb.Context(), lockID, outpoint, duration,
		)
		require.NoError(tb, err, "failed to lease output %v", outpoint)
	}
}

// setupMiner creates and starts an isolated btcd node via rpctest for
// integration benchmarks. The node runs with minimal configuration (no peers,
// no DNS seeds) suitable for controlled testing. Cleanup is handled
// automatically via b.Cleanup.
func setupMiner(b *testing.B, netParams *chaincfg.Params,
	additionalArgs ...string) *rpctest.Harness {

	b.Helper()

	baseArgs := []string{
		// Enable debug logging mode for all subsystems.
		"--debuglevel=debug",

		// Disable DNS seeding. It is not needed in the test
		// environment.
		"--nodnsseed",

		// Disable listening for incoming peer connections. Though it
		// would be overridden later by the rpctest framework for
		// listening on localhost (127.0.0.1:<os_free_port>).
		"--nolisten",

		// Disable stall detection. It is designed for controlled test
		// environment.
		"--nostalldetect",

		// Disable peer banning. It is not needed in the test
		// environment.
		"--nobanning",

		// Set max inbound/outbound peers to 0. It is not needed in the
		// test environment.
		"--maxpeers=0",
	}

	extraArgs := append([]string(nil), baseArgs...)
	extraArgs = append(extraArgs, additionalArgs...)

	miner, err := rpctest.New(
		netParams,
		nil,
		extraArgs,
		"",
	)
	b.Cleanup(func() {
		require.NoError(b, miner.TearDown())
	})
	require.NoError(b, err)

	err = miner.SetUp(true, 1)
	require.NoError(b, err)

	return miner
}

// createBenchmarkTransactions creates a pool of signed, unconfirmed
// transactions from the miner's funds to a wallet address. These transactions
// are NOT broadcast and are reused across benchmark iterations to test wallet
// broadcast performance.
//
// TODO(mohamedawnallah): refactor this <-> no need to skip the linter
//
//nolint:cyclop
func createBenchmarkTransactions(b *testing.B, miner *rpctest.Harness,
	w *Wallet, keyScope waddrmgr.KeyScope, txPoolSize,
	blocksToMine uint32, sameAddress bool) []*wire.MsgTx {

	b.Helper()

	var (
		// Split outputs: large enough to cover final tx output + fees +
		// change. It is 0.01 BTC per split output.
		splitOutputAmt = int64(0.01 * btcutil.SatoshiPerBitcoin)

		// Final tx outputs: smaller so there's room for fees. It is
		// 0.005 BTC per final tx.
		finalOutputAmt = int64(0.005 * btcutil.SatoshiPerBitcoin)

		feeRateSatPerByte = btcutil.Amount(10)

		addChangeOutput = true

		// Initial blocks to mine for coinbase maturity. It is used in
		// case of there is no user-defined value provided.
		initialBlocks = uint32(1000)
	)

	if blocksToMine != 0 {
		initialBlocks = blocksToMine
	}

	b.Logf("Mining %d blocks for coinbase maturity", initialBlocks)
	_, err := miner.Client.Generate(initialBlocks)
	require.NoError(b, err)

	// Query how many mature coinbases are available.
	blockCount, err := miner.Client.GetBlockCount()
	require.NoError(b, err)

	const coinbaseMaturity = 100

	numMatureCoinbases := uint32(blockCount) - coinbaseMaturity

	// Determine how many coinbases to split. We want to create at least
	// txPoolSize UTXOs, so calculate minimum splits needed. Each split
	// creates outputsPerSplit UTXOs. Keeping outputsPerSplit in a
	// reasonable range to avoid huge transactions.
	const outputsPerSplit = 100

	numSplitsNeeded := (txPoolSize + outputsPerSplit - 1) / outputsPerSplit

	// Don't split more coinbases than available.
	if numSplitsNeeded > numMatureCoinbases {
		numSplitsNeeded = numMatureCoinbases
	}

	totalUTXOs := numSplitsNeeded * outputsPerSplit

	b.Logf("Pass 1: Using %d of %d mature coinbases, splitting each into "+
		"%d outputs of %d sats = %d total UTXOs (need %d)",
		numSplitsNeeded, numMatureCoinbases, outputsPerSplit,
		splitOutputAmt, totalUTXOs, txPoolSize)

	splitCoinbasesToMiner(
		b, miner, numSplitsNeeded, outputsPerSplit, splitOutputAmt,
		feeRateSatPerByte,
	)

	// Pass 2: Create final transactions from miner's split UTXOs to wallet.
	b.Logf("Pass 2: Creating %d transactions from miner to wallet",
		txPoolSize)

	txs := make([]*wire.MsgTx, txPoolSize)

	// Generate addresses conditionally based on address generation flag.
	//
	// TODO(mohamedawnallah): refactor this <-> no need to skip the linter
	//
	//nolint:nestif
	if sameAddress {
		addr, err := w.CurrentAddress(
			waddrmgr.DefaultAccountNum, keyScope,
		)
		require.NoError(b, err)

		for i := range txPoolSize {
			pkScript, err := txscript.PayToAddrScript(addr)
			require.NoError(b, err)

			// Create transaction from miner to wallet address.
			outputs := []*wire.TxOut{
				{
					Value:    finalOutputAmt,
					PkScript: pkScript,
				},
			}

			tx, err := miner.CreateTransaction(
				outputs, feeRateSatPerByte, addChangeOutput,
			)
			require.NoError(b, err)

			txs[i] = tx
		}

		b.Logf("Created %d unbroadcast transactions from miner to "+
			"wallet address %s", txPoolSize, addr.String())
	} else {
		// Generate a unique address for each transaction.
		for i := range txPoolSize {
			addr, err := w.NewAddressDeprecated(
				waddrmgr.DefaultAccountNum, keyScope,
			)
			require.NoError(b, err)

			pkScript, err := txscript.PayToAddrScript(addr)
			require.NoError(b, err)

			// Create transaction from miner to unique wallet
			// address.
			outputs := []*wire.TxOut{
				{
					Value:    finalOutputAmt,
					PkScript: pkScript,
				},
			}

			tx, err := miner.CreateTransaction(
				outputs, feeRateSatPerByte, addChangeOutput,
			)
			require.NoError(b, err)

			txs[i] = tx
		}

		b.Logf("Created %d unbroadcast transactions from miner to "+
			"%d unique wallet addresses", txPoolSize, txPoolSize)
	}

	return txs
}

// splitCoinbasesToMiner splits coinbase outputs into many smaller UTXOs by
// sending them to the miner's own address. This creates abundant UTXOs for
// creating many transactions without running out of coins.
func splitCoinbasesToMiner(b *testing.B, miner *rpctest.Harness,
	numSplits, outputsPerSplit uint32, amtPerOutput int64,
	feeRate btcutil.Amount) {

	b.Helper()

	// Get miner's address.
	minerAddr, err := miner.NewAddress()
	require.NoError(b, err)

	pkScript, err := txscript.PayToAddrScript(minerAddr)
	require.NoError(b, err)

	for range numSplits {
		// Create transaction with many outputs to miner's own address.
		outputs := make([]*wire.TxOut, outputsPerSplit)
		for j := range outputsPerSplit {
			outputs[j] = &wire.TxOut{
				Value:    amtPerOutput,
				PkScript: pkScript,
			}
		}

		tx, err := miner.CreateTransaction(outputs, feeRate, true)
		require.NoError(b, err)

		_, err = miner.Client.SendRawTransaction(tx, false)
		require.NoError(b, err)
	}

	// Final confirmation.
	_, err = miner.Client.Generate(1)
	require.NoError(b, err)

	b.Logf("Split complete: created %d miner UTXOs",
		numSplits*outputsPerSplit)
}

// selectTransactions selects a subset of transactions from the pool for each
// benchmark iteration. The benchmarkIteration parameter ensures different
// transactions are selected across b.N iterations, making each iteration
// idempotent. Assumes candidatesCount < len(pool) to avoid selecting the same
// transaction twice within a single iteration.
func selectBenchmarkTransactions(pool []*wire.MsgTx, candidatesCount,
	benchmarkIteration int) []*wire.MsgTx {

	selected := make([]*wire.MsgTx, candidatesCount)

	for i := range candidatesCount {
		// Cycle through the pool.
		idx := (benchmarkIteration*candidatesCount + i) % len(pool)
		selected[i] = pool[idx]
	}

	return selected
}

// benchmarkConcurrentBroadcast runs the core benchmark logic for concurrent
// broadcast operations.
//
//nolint:unparam
func benchmarkConcurrentBroadcast(b *testing.B, numConcurrentTxs int,
	txPoolSize uint32, useNewAPI, sameAddress bool) {

	b.Helper()

	keyScope := waddrmgr.KeyScopeBIP0084

	miner := setupMiner(b, &chaincfg.RegressionNetParams)

	bw := setupBenchmarkWallet(b, benchmarkWalletConfig{
		scopes: []waddrmgr.KeyScope{keyScope},
		miner:  miner,
	})
	w := bw.Wallet

	// Create a pool of transactions using the miner's funds.
	txPool := createBenchmarkTransactions(
		b, miner, w, keyScope, txPoolSize, 0, sameAddress,
	)

	b.Logf("Broadcasting %d concurrent transactions per benchmark "+
		"iteration ...", numConcurrentTxs)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; b.Loop(); i++ {
		txs := selectBenchmarkTransactions(txPool, numConcurrentTxs, i)

		if useNewAPI {
			broadcastConcurrentNewAPI(b, w, txs)
		} else {
			broadcastConcurrentOldAPI(b, w, txs)
		}

		b.StopTimer()

		// Check mempool size after broadcast to verify how many
		// transactions actually made it and detect false positives
		// (broadcasts that appeared to succeed but didn't reach the
		// mempool).
		mempoolTxs, err := miner.Client.GetRawMempool()
		if err != nil {
			b.Logf("Warning: failed to get mempool: %v", err)
		}

		if testing.Verbose() {
			b.Logf("Iteration %d: Attempted to broadcast %d txs, "+
				"mempool now contains %d txs", i,
				numConcurrentTxs, len(mempoolTxs))
		}

		// Mine a block to confirm transactions and clear the mempool
		// for the next iteration. This makes each iteration idempotent,
		// preventing mempool conflicts when reusing the transaction
		// pool across b.N iterations.
		_, err = miner.Client.Generate(1)
		if err != nil {
			b.Logf("Warning: failed to mine block: %v", err)
		}

		b.StartTimer()
	}
}

// broadcastConcurrentNewAPI broadcasts transactions concurrently using the new
// Broadcast API.
func broadcastConcurrentNewAPI(b *testing.B, w *Wallet, txs []*wire.MsgTx) {
	b.Helper()

	const txLabel = "broadcastConcurrentNewAPI"

	var wg sync.WaitGroup

	for _, tx := range txs {
		wg.Add(1)

		go func(tx *wire.MsgTx) {
			defer wg.Done()

			err := w.Broadcast(b.Context(), tx, txLabel)
			if err != nil {
				b.Logf("Broadcast error for tx %s: %v",
					tx.TxHash(), err)
			}
		}(tx)
	}

	wg.Wait()
}

// broadcastConcurrentOldAPI broadcasts transactions concurrently using the old
// PublishTransaction API.
func broadcastConcurrentOldAPI(b *testing.B, w *Wallet, txs []*wire.MsgTx) {
	b.Helper()

	const txLabel = "broadcastConcurrentOldAPI"

	var wg sync.WaitGroup

	for _, tx := range txs {
		wg.Add(1)

		go func(tx *wire.MsgTx) {
			defer wg.Done()

			err := w.PublishTransaction(tx, txLabel)
			if err != nil {
				b.Logf("PublishTransaction error for tx %s: %v",
					tx.TxHash(), err)
			}
		}(tx)
	}

	wg.Wait()
}

// benchmarkSequentialBroadcast runs the core sequential benchmark logic,
// parameterized by txPoolSize and whether the same address is used.
func benchmarkSequentialBroadcast(b *testing.B, txPoolSize uint32,
	sameAddress bool, useNewAPI bool) {

	b.Helper()

	keyScope := waddrmgr.KeyScopeBIP0084

	miner := setupMiner(b, &chaincfg.RegressionNetParams)

	bw := setupBenchmarkWallet(b, benchmarkWalletConfig{
		scopes: []waddrmgr.KeyScope{keyScope},
		miner:  miner,
	})
	w := bw.Wallet

	// Create transaction pool.
	txPool := createBenchmarkTransactions(
		b, miner, w, keyScope, txPoolSize, 0, sameAddress,
	)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; b.Loop(); i++ {
		tx := txPool[i%len(txPool)]

		if useNewAPI {
			err := w.Broadcast(b.Context(), tx, "sequential-after")
			if err != nil {
				b.Logf("Broadcast error: %v", err)
			}
		} else {
			err := w.PublishTransaction(tx, "sequential-before")
			if err != nil {
				b.Logf("PublishTransaction error: %v", err)
			}
		}

		b.StopTimer()

		// Check mempool size after broadcast to verify how many
		// transactions actually made it and detect false positives
		// (broadcasts that appeared to succeed but didn't reach the
		// mempool).
		mempoolTxs, err := miner.Client.GetRawMempool()
		if err != nil {
			b.Logf("Warning: failed to get mempool: %v", err)
		}

		if testing.Verbose() {
			b.Logf("Iteration %d: Attempted to broadcast tx, "+
				"mempool now contains %d txs", i,
				len(mempoolTxs))
		}

		// Mine a block to confirm transactions and clear the mempool
		// for the next iteration. This makes each iteration idempotent,
		// preventing mempool conflicts when reusing the transaction
		// pool across b.N iterations.
		_, err = miner.Client.Generate(1)
		if err != nil {
			b.Logf("Warning: failed to mine block: %v", err)
		}

		b.StartTimer()
	}
}

// listAccountsDeprecated wraps the deprecated Accounts API to satisfy the same
// contract as ListAccounts by calling Accounts API across all active key scopes
// and aggregating the results.
func listAccountsDeprecated(w *Wallet) (*AccountsResult, error) {
	var (
		allAccounts      []AccountResult
		finalBlockHash   chainhash.Hash
		finalBlockHeight int32
		scopeManagers    = w.addrStore.ActiveScopedKeyManagers()
	)

	for _, scopeMgr := range scopeManagers {
		scope := scopeMgr.Scope()

		result, err := w.Accounts(scope)
		if err != nil {
			return nil, err
		}

		allAccounts = append(allAccounts, result.Accounts...)

		finalBlockHash = result.CurrentBlockHash
		finalBlockHeight = result.CurrentBlockHeight
	}

	return &AccountsResult{
		Accounts:           allAccounts,
		CurrentBlockHash:   finalBlockHash,
		CurrentBlockHeight: finalBlockHeight,
	}, nil
}

// listAccountsByNameDeprecated wraps the deprecated Accounts API to satisfy the
// same contract as ListAccountsByName by calling Accounts API across all active
// key scopes, filtering by account name, and aggregating the results.
func listAccountsByNameDeprecated(w *Wallet,
	name string) (*AccountsResult, error) {

	var (
		matchingAccounts []AccountResult
		finalBlockHash   chainhash.Hash
		finalBlockHeight int32
		scopeManagers    = w.addrStore.ActiveScopedKeyManagers()
	)

	for _, scopeMgr := range scopeManagers {
		scope := scopeMgr.Scope()

		result, err := w.Accounts(scope)
		if err != nil {
			return nil, err
		}

		// Filter accounts by name from this scope's results.
		for _, account := range result.Accounts {
			if account.AccountName == name {
				matchingAccounts = append(
					matchingAccounts, account,
				)
			}
		}

		finalBlockHash = result.CurrentBlockHash
		finalBlockHeight = result.CurrentBlockHeight
	}

	return &AccountsResult{
		Accounts:           matchingAccounts,
		CurrentBlockHash:   finalBlockHash,
		CurrentBlockHeight: finalBlockHeight,
	}, nil
}

// getAccountDeprecated wraps the deprecated Accounts API to satisfy the same
// contract as GetAccount by calling Accounts API across all active key scopes
// and filtering by account name.
func getAccountDeprecated(w *Wallet, scope waddrmgr.KeyScope,
	accountName string) (*AccountResult, error) {

	result, err := w.Accounts(scope)
	if err != nil {
		return nil, err
	}

	for _, account := range result.Accounts {
		if account.AccountName == accountName {
			return &account, nil
		}
	}

	return nil, fmt.Errorf("%w: %s", errAccountNotFound, accountName)
}

// getBalanceDeprecated wraps the deprecated Accounts API to satisfy the same
// contract as GetBalance by calling Accounts API across all active key scopes
// and filtering by account name.
func getBalanceDeprecated(w *Wallet, scope waddrmgr.KeyScope,
	accountName string, _ int32) (btcutil.Amount, error) {

	result, err := w.Accounts(scope)
	if err != nil {
		return 0, err
	}

	for _, account := range result.Accounts {
		if account.AccountName == accountName {
			// The deprecated Accounts API doesn't support
			// confirmation filtering. It always returns total
			// balance.
			return account.TotalBalance, nil
		}
	}

	return 0, fmt.Errorf("%w: %s", errAccountNotFound, accountName)
}

// listAddressesDeprecated wraps the deprecated AccountAddresses and
// TotalReceivedForAddr APIs to satisfy the same contract as ListAddresses by
// calling the old APIs and aggregating the results with balances.
func listAddressesDeprecated(w *Wallet,
	accountID uint32) ([]AddressProperty, error) {

	addresses, err := w.AccountAddresses(accountID)
	if err != nil {
		return nil, err
	}

	allProperties := make([]AddressProperty, 0, len(addresses))

	for _, addr := range addresses {
		balance, err := w.TotalReceivedForAddr(addr, 0)
		if err != nil {
			return nil, err
		}

		allProperties = append(allProperties, AddressProperty{
			Address: addr,
			Balance: balance,
		})
	}

	return allProperties, nil
}

// getUtxoDeprecated wraps the deprecated FetchOutpointInfo API to satisfy the
// same contract as GetUtxo by calling FetchOutpointInfo and performing
// additional lookups to construct a complete Utxo struct. This demonstrates
// the inefficiency of the old API which returns raw data requiring the caller
// to perform multiple additional lookups.
func getUtxoDeprecated(w *Wallet, prevOut wire.OutPoint) (*Utxo, error) {
	_, txOut, confs, err := w.FetchOutpointInfo(&prevOut)
	if err != nil {
		return nil, err
	}

	// Additional lookup 1: Extract address from pkScript.
	addr := extractAddrFromPKScript(txOut.PkScript, w.chainParams)
	if addr == nil {
		return nil, ErrNotMine
	}

	// Additional lookup 2: Get address details (spendability, account,
	// address type) from the address manager.
	var (
		spendable bool
		account   string
		addrType  waddrmgr.AddressType
	)

	err = walletdb.View(w.db, func(tx walletdb.ReadTx) error {
		addrmgrNs := tx.ReadBucket(waddrmgrNamespaceKey)
		spendable, account, addrType = w.addrStore.AddressDetails(
			addrmgrNs, addr,
		)

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Additional lookup 3: Check if the output is locked.
	locked := w.LockedOutpoint(prevOut)

	return &Utxo{
		OutPoint:      prevOut,
		Amount:        btcutil.Amount(txOut.Value),
		PkScript:      txOut.PkScript,
		Confirmations: int32(confs),
		Spendable:     spendable,
		Address:       addr,
		Account:       account,
		AddressType:   addrType,
		Locked:        locked,
	}, nil
}
