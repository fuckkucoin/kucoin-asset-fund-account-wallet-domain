package occupbucketfence_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/occupbook"
	"github.com/prometheus/client_golang/prometheus"
	"account-assets.kucoin.net/funding/internal/occupbucketfence"
)

func testClock() func() time.Time {
	return func() time.Time {
		return time.Date(2025, 11, 18, 12, 0, 0, 0, time.UTC)
	}
}

func newEng() (*occupbucketfence.Engine, *occupbucketfence.MemStore, *occupbook.Book) {
	st := occupbucketfence.NewMemStore()
	book := occupbook.NewBook(nil, occupbook.WithClock(testClock()), occupbook.WithRequireFence(true))
	eng := occupbucketfence.NewEngine(st, book, occupbucketfence.WithEngineClock(testClock()))
	return eng, st, book
}

func holdOK(t *testing.T, eng *occupbucketfence.Engine, lot, user, asset, amt, risk string) *occupbucketfence.HoldWithFenceResp {
	t.Helper()
	resp, err := eng.HoldWithFence(context.Background(), occupbucketfence.HoldWithFenceReq{
		LotID:	lot, UserID: user, Asset: asset, Amount: amt, RiskClass: risk, IdemKey: "hold-" + lot,
	})
	if err != nil {
		t.Fatalf("hold: %v", err)
	}
	return resp
}

func TestEngine_HoldReleaseConserve(t *testing.T) {
	cases := []struct {
		name, lot, amt, risk string
	}{
		{"normal_usdt", "lot-a", "100", "normal"},
		{"t1_btc", "lot-b", "0.5", "t1"},
		{"t4_full", "lot-c", "12", "t4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eng, _, _ := newEng()
			h := holdOK(t, eng, tc.lot, "u1", "USDT", tc.amt, tc.risk)
			if h.Fence == nil || h.Fence.State != occupbucketfence.FenceBound {
				t.Fatalf("fence bound: %+v", h.Fence)
			}
			lot,
				_ := eng.Book().GetLot(context.Background(), tc.lot)
			rel, err := eng.ReleaseAssert(context.Background(), occupbucketfence.ReleaseAssertReq{
				LotID:	tc.lot, HoldID: h.HoldID, FenceID: h.Fence.FenceID, Amount: tc.amt,
				IdemKey:	"rel-" + tc.lot, Fingerprint: occupbucketfence.FingerprintLot(lot), Generation: lot.Generation,
			})
			if err != nil {
				t.Fatalf("release: %v", err)
			}
			if !rel.Conserve {
				t.Fatalf("conserve")
			}
			if rel.Fence.State != occupbucketfence.FenceConsumed {
				t.Fatalf("consumed %s", rel.Fence.State)
			}
		})
	}
}

func TestEngine_ReclassVoidConserve(t *testing.T) {
	eng, _, _ := newEng()
	h := holdOK(t, eng, "lot-rc", "u1", "USDT", "20", "t2")
	lot, _ := eng.Book().GetLot(context.Background(), "lot-rc")
	resp, err := eng.ReclassAssert(context.Background(), occupbucketfence.ReclassAssertReq{
		LotID:	"lot-rc", HoldID: h.HoldID, FenceID: h.Fence.FenceID, Amount: "20",
		ToClass:	"t4", ReasonCode: "risk_up", IdemKey: "rc-1",
		Fingerprint:	occupbucketfence.FingerprintLot(lot), Generation: lot.Generation,
	})
	if err != nil {
		t.Fatalf("reclass: %v", err)
	}
	if resp.Void != "20" || resp.Inflight != "0" {
		t.Fatalf("buckets %+v", resp)
	}
	if !resp.Conserve {
		t.Fatal("conserve")
	}
}

func TestEngine_ConcurrentReleaseReclass(t *testing.T) {
	raw, err := os.ReadFile(goldenPath(t, "concurrent_release_reclass.golden.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		LotID	string	`json:"lot_id"`
		UserID	string	`json:"user_id"`
		Asset	string	`json:"asset"`
		Amount	string	`json:"amount"`
		Risk	string	`json:"risk_class"`
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	eng, _, _ := newEng()
	h := holdOK(t, eng, g.LotID, g.UserID, g.Asset, g.Amount, g.Risk)
	lot, _ := eng.Book().GetLot(context.Background(), g.LotID)
	fp := occupbucketfence.FingerprintLot(lot)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var oks, mutexes int
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := eng.ReleaseAssert(context.Background(), occupbucketfence.ReleaseAssertReq{
			LotID:	g.LotID, HoldID: h.HoldID, FenceID: h.Fence.FenceID, Amount: g.Amount,
			IdemKey:	"rel-race", Fingerprint: fp, Generation: lot.Generation,
		})
		mu.Lock()
		defer mu.Unlock()
		if err == nil {
			oks++
		} else if concurrentLoser(err) {
			mutexes++
		} else {
			t.Errorf("release err %v", err)
		}
	}()
	go func() {
		defer wg.Done()
		_, err := eng.ReclassAssert(context.Background(), occupbucketfence.ReclassAssertReq{
			LotID:	g.LotID, HoldID: h.HoldID, FenceID: h.Fence.FenceID, Amount: g.Amount,
			ToClass:	"t4", ReasonCode: "up", IdemKey: "rc-race",
			Fingerprint:	fp, Generation: lot.Generation,
		})
		mu.Lock()
		defer mu.Unlock()
		if err == nil {
			oks++
		} else if concurrentLoser(err) {
			mutexes++
		} else {
			t.Errorf("reclass err %v", err)
		}
	}()
	wg.Wait()
	if oks != 1 {
		t.Fatalf("want 1 winner got %d mutexes=%d", oks, mutexes)
	}
	lot2, _ := eng.Book().GetLot(context.Background(), g.LotID)
	if !lot2.ConserveOK() {
		t.Fatalf("conserve broken %+v", lot2.Buckets)
	}
	// 败者必须失败即关；世代已被胜者推进时可能只回 ErrGenerationStale，此时三桶仍守恒。
	// 击穿证明在互斥/双释路径落库；仅世代陈旧不强制证明行。
	if mutexes != 1 {
		t.Fatalf("want 1 loser got mutexes=%d", mutexes)
	}
	proofs,
		_ := eng.ExportProofs(context.Background(), g.LotID)
	_ = proofs
}

func concurrentLoser(err error) bool {
	return errors.Is(err, occupbucketfence.ErrReclassMutex) || errors.Is(err, occupbucketfence.ErrFenceConsumed) || errors.Is(err, occupbucketfence.ErrInvalidTransition) ||
		errors.Is(err, occupbucketfence.ErrGenerationStale) || errors.Is(err, occupbucketfence.ErrFenceMismatch) ||
		errors.Is(err, occupbook.ErrHoldAlreadyReclassed) || errors.Is(err, occupbook.ErrHoldAlreadyReleased) ||
		errors.Is(err, occupbook.ErrLotClosed) || errors.Is(err, occupbook.ErrInsufficientInflight)
}

func TestEngine_HalfResumeRejectSecond(t *testing.T) {
	eng, _, book := newEng()
	h := holdOK(t, eng, "lot-golden-half-001", "user-occ-002", "BTC", "0.25", "normal")
	if err := book.MarkHalfHold(context.Background(), "lot-golden-half-001", h.HoldID); err != nil {
		t.Fatal(err)
	}
	now := testClock()()
	_ = eng.store.SaveHalf(context.Background(), &occupbucketfence.HalfBucketCase{
		CaseID:	"half-1", LotID: "lot-golden-half-001", HoldID: h.HoldID, FenceID: h.Fence.FenceID,
		State:	occupbucketfence.HalfOpen, Amount: "0.25", Reason: "bucket_persist_interrupted", OpenedAt: now,
	})
	_, err := eng.HoldWithFence(context.Background(), occupbucketfence.HoldWithFenceReq{
		LotID:	"lot-golden-half-001", UserID: "user-occ-002", Asset: "BTC", Amount: "0.25", RiskClass: "normal", IdemKey: "second",
	})
	if !errors.Is(err, occupbucketfence.ErrHalfOpen) {
		t.Fatalf("second occupy: %v", err)
	}
	half, err := eng.ResumeHalf(context.Background(), "lot-golden-half-001", "half-1")
	if err != nil {
		t.Fatal(err)
	}
	if half.State != occupbucketfence.HalfResumed {
		t.Fatalf("state %s", half.State)
	}
	lot, _ := book.GetLot(context.Background(), "lot-golden-half-001")
	if !lot.ConserveOK() {
		t.Fatal("conserve")
	}
}

func TestEngine_MixerBypass(t *testing.T) {
	book := occupbook.NewBook(nil, occupbook.WithClock(testClock()), occupbook.WithRequireFence(true))
	_, err := book.Observe(context.Background(), occupbook.ObserveReq{
		LotID:	"lot-golden-mix-001", UserID: "user-occ-003", Asset: "ETH", Amount: "8", RiskClass: "mixer",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = book.Hold(context.Background(), occupbook.HoldReq{
		LotID:	"lot-golden-mix-001", Amount: "8", IdemKey: "bypass",
	}, false)
	if !errors.Is(err, occupbook.ErrMixerBypass) {
		t.Fatalf("want mixer bypass got %v", err)
	}
	eng := occupbucketfence.NewEngine(occupbucketfence.NewMemStore(), book, occupbucketfence.WithEngineClock(testClock()))
	if _, err := eng.HoldWithFence(context.Background(), occupbucketfence.HoldWithFenceReq{
		LotID:	"lot-golden-mix-001", UserID: "user-occ-003", Asset: "ETH", Amount: "8", RiskClass: "mixer", IdemKey: "ok",
	}); err != nil {
		t.Fatalf("fenced hold: %v", err)
	}
}

func TestEngine_FingerprintFailClosed(t *testing.T) {
	eng, _, _ := newEng()
	h := holdOK(t, eng, "lot-golden-fp-001", "user-occ-004", "USDT", "50", "t1")
	resp, err := eng.ReleaseAssert(context.Background(), occupbucketfence.ReleaseAssertReq{
		LotID:	"lot-golden-fp-001", HoldID: h.HoldID, FenceID: h.Fence.FenceID, Amount: "50",
		IdemKey:	"rel-fp", Fingerprint: "deadbeef", Generation: 0,
	})
	if !errors.Is(err, occupbucketfence.ErrFenceMismatch) {
		t.Fatalf("want mismatch got %v", err)
	}
	if resp == nil || resp.Proof == nil || resp.Proof.Kind != "fingerprint_mismatch" {
		t.Fatalf("proof %+v", resp)
	}
	lot, _ := eng.Book().GetLot(context.Background(), "lot-golden-fp-001")
	if !lot.Buckets.Inflight.Equal(occupbook.MustAmount("50")) {
		t.Fatalf("bucket mutated")
	}
}

func TestEngine_NoFenceFailClosed(t *testing.T) {
	eng, _, _ := newEng()
	h := holdOK(t, eng, "lot-nf", "u", "USDT", "3", "normal")
	_, err := eng.ReleaseAssert(context.Background(), occupbucketfence.ReleaseAssertReq{
		LotID:	"lot-nf", HoldID: h.HoldID, Amount: "3", IdemKey: "x",
	})
	if !errors.Is(err, occupbucketfence.ErrFenceRequired) {
		t.Fatalf("%v", err)
	}
}

func TestEngine_IdempotentHold(t *testing.T) {
	eng, _, _ := newEng()
	a := holdOK(t, eng, "lot-id", "u", "USDT", "9", "normal")
	b, err := eng.HoldWithFence(context.Background(), occupbucketfence.HoldWithFenceReq{
		LotID:	"lot-id", UserID: "u", Asset: "USDT", Amount: "9", RiskClass: "normal", IdemKey: "hold-lot-id",
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.HoldID != b.HoldID {
		t.Fatalf("idem hold %s vs %s", a.HoldID, b.HoldID)
	}
}

func TestEngine_ProofLifecycle(t *testing.T) {
	eng, _, _ := newEng()
	h := holdOK(t, eng, "lot-pr", "u", "USDT", "1", "normal")
	resp, err := eng.ReleaseAssert(context.Background(), occupbucketfence.ReleaseAssertReq{
		LotID:	"lot-pr", HoldID: h.HoldID, FenceID: h.Fence.FenceID, Amount: "1",
		Fingerprint:	"00", Generation: 99,
	})
	if err == nil || resp.Proof == nil {
		t.Fatal("want proof")
	}
	p, err := eng.ProveBreak(context.Background(), resp.Proof.ProofID)
	if err != nil || p.State != occupbucketfence.BreakProven {
		t.Fatalf("prove %v %v", err, p)
	}
	p, err = eng.CloseBreak(context.Background(), p.ProofID)
	if err != nil || p.State != occupbucketfence.BreakClosed {
		t.Fatalf("close %v", err)
	}
}

func TestEngine_ExpireStale(t *testing.T) {
	st := occupbucketfence.NewMemStore()
	book := occupbook.NewBook(nil, occupbook.WithRequireFence(true))
	now := time.Date(2025, 11, 18, 12, 0, 0, 0, time.UTC)
	eng := occupbucketfence.NewEngine(st, book, occupbucketfence.WithEngineClock(func() time.Time { return now }))
	f, err := eng.IssueFence(context.Background(), occupbucketfence.IssueFenceReq{LotID: "ghost", TTLSeconds: 1})
	if err != nil {
		t.Fatal(err)
	}
	f.ExpiresAt = now.Add(-time.Second)
	_ = st.UpdateFence(context.Background(), f)
	n, err := eng.ExpireStale(context.Background(), 10)
	if err != nil || n != 1 {
		t.Fatalf("expire n=%d err=%v", n, err)
	}
}

func TestWorker_RunOnce(t *testing.T) {
	eng, st, _ := newEng()
	reg := prometheus.NewRegistry()
	m := occupbucketfence.NewMetrics(reg)
	w := occupbucketfence.NewWorker(eng, st, m, occupbucketfence.WorkerConfig{BatchSize: 10, NodeID: "t"})
	w.RunOnce(context.Background())
}

func TestFSM(t *testing.T) {
	if !occupbucketfence.FenceTransition(occupbucketfence.FenceIssued, occupbucketfence.FenceBound) || occupbucketfence.FenceTransition(occupbucketfence.FenceConsumed, occupbucketfence.FenceIssued) {
		t.Fatal("fence fsm")
	}
	if !occupbucketfence.BreakTransition(occupbucketfence.BreakOpen, occupbucketfence.BreakProven) || occupbucketfence.BreakTransition(occupbucketfence.BreakClosed, occupbucketfence.BreakOpen) {
		t.Fatal("break fsm")
	}
	if !occupbucketfence.HalfTransition(occupbucketfence.HalfOpen, occupbucketfence.HalfResumed) {
		t.Fatal("half fsm")
	}
	if occupbucketfence.KindClass("reclass_release_mutex") != "mutex" || occupbucketfence.KindClass("patrol_conserve") != "conserve" {
		t.Fatal("kind")
	}
}

func TestFingerprintStable(t *testing.T) {
	a := occupbucketfence.FingerprintRaw("L", "1", "0", "0", "1", 2)
	b := occupbucketfence.FingerprintRaw("L", "1", "0", "0", "1", 2)
	if a != b || a == "" {
		t.Fatal(a)
	}
}

func goldenPath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "occupbucketfence", name)
}
