package core

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"porcupin/backend/db"
)

// gatedIPFSNode is a mockIPFSNode whose Pin blocks until released, while
// tracking concurrent in-flight Pin calls. Used to verify worker-pool bounds
// and serialization invariants of BackupManager.
//
// The gate channel is buffered so pre-arming N releases never blocks the
// releaser goroutine even if not every release ends up being consumed (e.g.
// when a test exits dispatch early). Tests must size the buffer to >= the
// number of releases they intend to send.
type gatedIPFSNode struct {
	mockIPFSNode
	gate        chan struct{}
	inFlight    int64 // current concurrent Pin calls
	maxInFlight int64 // observed peak concurrency
}

// newGatedIPFSNode constructs a gated mock with the given gate buffer size.
// Pass capacity >= the planned number of release() calls to avoid the
// releaser blocking if dispatch returns early.
func newGatedIPFSNode(gateCapacity int) *gatedIPFSNode {
	g := &gatedIPFSNode{
		gate: make(chan struct{}, gateCapacity),
	}
	g.pinned = make(map[string]bool)
	g.sizes = make(map[string]int64)
	g.repoPath = "/tmp/mock-ipfs"
	return g
}

func (g *gatedIPFSNode) Pin(ctx context.Context, cid string, timeout time.Duration) error {
	cur := atomic.AddInt64(&g.inFlight, 1)
	defer atomic.AddInt64(&g.inFlight, -1)
	for {
		prev := atomic.LoadInt64(&g.maxInFlight)
		if cur <= prev || atomic.CompareAndSwapInt64(&g.maxInFlight, prev, cur) {
			break
		}
	}
	select {
	case <-g.gate:
	case <-ctx.Done():
		return ctx.Err()
	}
	g.mu.Lock()
	g.pinned[cid] = true
	g.mu.Unlock()
	return nil
}

// release queues one credit on the gate. Non-blocking because gate is buffered.
func (g *gatedIPFSNode) release() { g.gate <- struct{}{} }

// releaseAll closes the gate so any in-flight or future Pin returns immediately
// (receive on a closed channel never blocks).
func (g *gatedIPFSNode) releaseAll() { close(g.gate) }

// seedPending inserts n pending IPFS assets attached to a single NFT so
// ProcessPendingAssets has something to chew on.
func seedPending(t *testing.T, database *db.Database, n int) {
	t.Helper()
	nft := &db.NFT{TokenID: "1", ContractAddress: "KT1test", WalletAddress: "tz1test"}
	if err := database.SaveNFT(nft); err != nil {
		t.Fatalf("SaveNFT: %v", err)
	}
	for i := 0; i < n; i++ {
		a := &db.Asset{
			URI:    "ipfs://QmTestAsset" + strconv.Itoa(i),
			NFTID:  nft.ID,
			Status: db.StatusPending,
		}
		if err := database.SaveAsset(a); err != nil {
			t.Fatalf("SaveAsset[%d]: %v", i, err)
		}
	}
}

// TestProcessPendingAssets_BoundsConcurrency verifies the fix for the OOM
// path: with N pending assets and M workers, no more than M Pin calls are
// ever in flight simultaneously. Pre-fix, all N goroutines were spawned
// upfront and blocked on the worker channel.
func TestProcessPendingAssets_BoundsConcurrency(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()
	cfg.Backup.MaxConcurrency = 3

	gated := newGatedIPFSNode(64)
	bm := &BackupManager{
		ipfs:    gated,
		db:      database,
		config:  cfg,
		workers: make(chan struct{}, cfg.Backup.MaxConcurrency),
	}

	const pending = 20
	seedPending(t, database, pending)

	// Pre-arm one release per pending asset. Buffered gate (sized = pending)
	// ensures these never block, so if dispatch exits early we don't leak a
	// goroutine waiting to send.
	for i := 0; i < pending; i++ {
		gated.release()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	bm.ProcessPendingAssets(ctx, pending)

	peak := atomic.LoadInt64(&gated.maxInFlight)
	if peak == 0 {
		t.Fatal("no Pin calls were observed — dispatch never started")
	}
	if peak > int64(cfg.Backup.MaxConcurrency) {
		t.Errorf("peak concurrent Pin calls = %d, want <= %d (MaxConcurrency)",
			peak, cfg.Backup.MaxConcurrency)
	}
}

// TestHeavyOpMu_SerializesProcessPendingAssets verifies the second OOM fix:
// ProcessPendingAssets must acquire heavyOpMu. We prove this by holding the
// mutex externally and confirming a fresh call cannot make progress until
// we release.
func TestHeavyOpMu_SerializesProcessPendingAssets(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()
	cfg.Backup.MaxConcurrency = 2

	gated := newGatedIPFSNode(64)
	gated.releaseAll() // Pin returns immediately for whoever holds the mutex
	bm := &BackupManager{
		ipfs:    gated,
		db:      database,
		config:  cfg,
		workers: make(chan struct{}, cfg.Backup.MaxConcurrency),
	}

	seedPending(t, database, 4)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	bm.heavyOpMu.Lock()
	done := make(chan struct{})
	go func() {
		bm.ProcessPendingAssets(ctx, 4)
		close(done)
	}()
	select {
	case <-done:
		bm.heavyOpMu.Unlock()
		t.Fatal("ProcessPendingAssets ran while heavyOpMu was held externally — serialization broken")
	case <-time.After(200 * time.Millisecond):
		// Good: blocked on the mutex as expected.
	}
	bm.heavyOpMu.Unlock()
	<-done // let it finish so the test doesn't leak the goroutine
}

// TestHeavyOpMu_SerializesAcrossOperations verifies the mutex also serializes
// across DIFFERENT heavy operations (SyncWallet vs VerifyAndFixPins vs
// ProcessPendingAssets), not just same-op concurrency. We can't easily run
// the full SyncWallet here without an indexer, but we can directly prove the
// mutex covers both ProcessPendingAssets and VerifyAndFixPins.
func TestHeavyOpMu_SerializesAcrossOperations(t *testing.T) {
	database := testDB(t)
	cfg := testConfig()
	gated := newGatedIPFSNode(64)
	gated.releaseAll() // never block on Pin
	bm := &BackupManager{
		ipfs:    gated,
		db:      database,
		config:  cfg,
		workers: make(chan struct{}, cfg.Backup.MaxConcurrency),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bm.heavyOpMu.Lock()
	verifyDone := make(chan struct{})
	go func() {
		_, _ = bm.VerifyAndFixPins(ctx)
		close(verifyDone)
	}()
	select {
	case <-verifyDone:
		bm.heavyOpMu.Unlock()
		t.Fatal("VerifyAndFixPins ran while heavyOpMu was held — not serialized")
	case <-time.After(200 * time.Millisecond):
	}
	bm.heavyOpMu.Unlock()
	<-verifyDone
}
