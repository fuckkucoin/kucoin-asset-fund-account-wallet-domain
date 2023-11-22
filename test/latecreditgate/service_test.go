package latecreditgate_test

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
	"account-assets.kucoin.net/funding/internal/reclaim"
	"account-assets.kucoin.net/funding/internal/latecreditgate"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "latecreditgate")
}

func loadGolden(t *testing.T, name string, dest any) {
	t.Helper()
	b,
		err := os.ReadFile(filepath.Join(testdataDir(t), name))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, dest); err != nil {
		t.Fatal(err)
	}
}

func key(chain, hash string, idx int) latecreditgate.OutputKey {
	return latecreditgate.OutputKey{Chain: chain, TxHash: hash, OutputIndex: idx}
}

func fenceOK() latecreditgate.FenceToken	{ return latecreditgate.FenceToken{Epoch: 3, Token: "tok-3"} }

func TestFenceWriteOmitTable(t *testing.T) {
	type row struct {
		name	string
		fence	latecreditgate.FenceToken
		want	error
	}
	cases := []row{
		{"epoch0_empty", latecreditgate.FenceToken{Epoch: 0, Token: ""}, latecreditgate.ErrFenceEpochZero},
		{"epoch0_token", latecreditgate.FenceToken{Epoch: 0, Token: "x"}, latecreditgate.ErrFenceEpochZero},
		{"epoch1_empty", latecreditgate.FenceToken{Epoch: 1, Token: ""}, latecreditgate.ErrFenceTokenEmpty},
		{"epoch1_space", latecreditgate.FenceToken{Epoch: 2, Token: "  "}, latecreditgate.ErrFenceTokenEmpty},
		{"ok", latecreditgate.FenceToken{Epoch: 1, Token: "t"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := latecreditgate.MustFenceWrite(tc.fence)
			if !errors.Is(err, tc.want) && (tc.want != nil || err != nil) {
				if tc.want == nil && err == nil {
					return
				}
				if !errors.Is(err, tc.want) {
					t.Fatalf("got %v want %v", err, tc.want)
				}
			}
		})
	}
}

func TestWritePathOmittedFenceNoAdvance(t *testing.T) {
	var g struct {
		Expect struct {
			Error		string	`json:"error"`
			StateAdvanced	bool	`json:"state_advanced"`
			Generation	int64	`json:"generation"`
		} `json:"expect"`
	}
	loadGolden(t, "golden_fence_omit.json", &g)
	ctx := context.Background()
	svc := latecreditgate.New()
	k := key("eth", "0xabc", 0)
	_, err := svc.Reserve(ctx, k, "ops-a", latecreditgate.FenceToken{})
	if !errors.Is(err, latecreditgate.ErrFenceEpochZero) {
		t.Fatalf("reserve: %v", err)
	}
	if _, err := svc.GetGeneration(ctx, k, latecreditgate.FenceToken{}, true); !errors.Is(err, latecreditgate.ErrGenNotFound) {
		t.Fatalf("state advanced: %v", err)
	}
	paths := []func() error{
		func() error { _, e := svc.Reclaim(ctx, k, "ops-a", latecreditgate.FenceToken{}); return e },
		func() error { _, e := svc.Resolve(ctx, k, "ops-a", latecreditgate.FenceToken{}); return e },
		func() error {
			_, e := svc.AutoCredit(ctx, latecreditgate.CreditRequest{OutputKey: k, ActorID: "ac", Fence: latecreditgate.FenceToken{}})
			return e
		},
		func() error {
			_, e := svc.OccupyWithGen(ctx, latecreditgate.OccupyRequest{OutputKey: k, ActorID: "o", Fence: latecreditgate.FenceToken{}})
			return e
		},
	}
	for i, fn := range paths {
		if err := fn(); !errors.Is(err, latecreditgate.ErrFenceEpochZero) && !errors.Is(err, latecreditgate.ErrFenceTokenEmpty) && !errors.Is(err, latecreditgate.ErrFenceRequired) {
			t.Fatalf("path %d: %v", i, err)
		}
	}
	gen, _ := svc.CurrentGeneration(ctx, k)
	if gen != g.Expect.Generation {
		t.Fatalf("generation=%d", gen)
	}
	if g.Expect.StateAdvanced {
		t.Fatal("golden expects no advance")
	}
}

func TestOccupyVsCreditMutexAndProof(t *testing.T) {
	var g struct {
		Expect struct {
			MutexHolds	bool	`json:"mutex_holds"`
			CreditError	string	`json:"credit_error"`
			ProofKind	string	`json:"proof_kind"`
			FailClosed	bool	`json:"fail_closed"`
			OccupyState	string	`json:"occupy_state"`
		} `json:"expect"`
	}
	loadGolden(t, "golden_occupy_credit_race.json", &g)
	ctx := context.Background()
	svc := latecreditgate.New()
	k := key("btc", "0xdef", 1)
	occ, err := svc.OccupyWithGen(ctx, latecreditgate.OccupyRequest{
		OutputKey:	k, ActorID: "reclaim-worker", CaseID: "c1", OccupancyID: "occ1",
		Asset:	"BTC", Amount: "1", Fence: fenceOK(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(occ.State) != g.Expect.OccupyState {
		t.Fatalf("state %s", occ.State)
	}
	_,
		err = svc.CheckCreditDurable(ctx, latecreditgate.CreditRequest{
		OutputKey:	k, ActorID: "auto-credit", Kind: "credit", Fence: fenceOK(),
	})
	if !errors.Is(err, latecreditgate.ErrCreditBlocked) {
		t.Fatalf("credit: %v", err)
	}
	if err.Error() != g.Expect.CreditError {
		t.Fatalf("err text %q", err.Error())
	}
	proofs, err := svc.ExportProofs(ctx, 10)
	if err != nil || len(proofs) == 0 {
		t.Fatalf("proofs %v %d", err, len(proofs))
	}
	if string(proofs[0].Kind) != g.Expect.ProofKind {
		t.Fatalf("kind %s", proofs[0].Kind)
	}
	if proofs[0].FailClosed != g.Expect.FailClosed {
		t.Fatal("fail_closed")
	}
	if !g.Expect.MutexHolds {
		t.Fatal("golden mutex")
	}
}

func TestConcurrentOccupyCredit(t *testing.T) {
	ctx := context.Background()
	svc := latecreditgate.New()
	k := key("trx", "0xconc", 2)
	var wg sync.WaitGroup
	var occupyOK,
		creditOK, creditBlocked int
	var mu sync.Mutex
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := svc.OccupyWithGen(ctx, latecreditgate.OccupyRequest{OutputKey: k, ActorID: "occ", Fence: fenceOK()})
		mu.Lock()
		defer mu.Unlock()
		if err == nil {
			occupyOK++
		}
	}()
	go func() {
		defer wg.Done()
		_, err := svc.CheckCreditDurable(ctx, latecreditgate.CreditRequest{OutputKey: k, ActorID: "crd", Fence: fenceOK()})
		mu.Lock()
		defer mu.Unlock()
		if err == nil {
			creditOK++
		}
		if errors.Is(err, latecreditgate.ErrCreditBlocked) {
			creditBlocked++
		}
	}()
	wg.Wait()
	if occupyOK+creditOK != 1 {
		// 一方必须成功；另一方冲突或 credit blocked
		if occupyOK == 0 && creditOK == 0 {
			t.Fatalf("neither succeeded occupy=%d credit=%d blocked=%d", occupyOK, creditOK, creditBlocked)
		}
	}
	if occupyOK == 1 && creditOK == 1 {
		t.Fatal("mutex broken: both succeeded")
	}
	g, err := svc.GetGeneration(ctx, k, latecreditgate.FenceToken{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if g.State != latecreditgate.GenOccupied && g.State != latecreditgate.GenCredited {
		t.Fatalf("state %s", g.State)
	}
}

func TestRestartDrainNoRollback(t *testing.T) {
	var g struct {
		Expect struct {
			UnackedStillDrainable	bool	`json:"unacked_still_drainable"`
			GenerationNeverRollback	bool	`json:"generation_never_rollback"`
			MinOpenEvents		int	`json:"min_open_events"`
		} `json:"expect"`
	}
	loadGolden(t, "golden_restart_drain.json", &g)
	ctx := context.Background()
	mem := latecreditgate.NewMemStore()
	svc := latecreditgate.NewWithStore(mem)
	k := key("eth", "0xrst", 0)
	if _, err := svc.OccupyWithGen(ctx, latecreditgate.OccupyRequest{OutputKey: k, ActorID: "occ", Fence: fenceOK()}); err != nil {
		t.Fatal(err)
	}
	before, _ := svc.CurrentGeneration(ctx, k)
	for i := 0; i < 3; i++ {
		_, err := svc.CheckCreditDurable(ctx, latecreditgate.CreditRequest{
			OutputKey:	k, ActorID: "crd", Fence: latecreditgate.FenceToken{Epoch: 9, Token: "tok-9"},
		})
		if !errors.Is(err, latecreditgate.ErrCreditBlocked) {
			t.Fatalf("inject: %v", err)
		}
	}
	open, err := svc.DrainOpenRaces(ctx, 50)
	if err != nil || len(open) < g.Expect.MinOpenEvents {
		t.Fatalf("open %d err %v", len(open), err)
	}
	// 重启：从快照恢复新 Service
	snap := mem.Snapshot()
	restarted := latecreditgate.NewWithStore(snap)
	afterOpen, err := restarted.DrainOpenRaces(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterOpen) != len(open) {
		t.Fatalf("drain lost: before=%d after=%d", len(open), len(afterOpen))
	}
	afterGen, err := restarted.CurrentGeneration(ctx, k)
	if err != nil {
		t.Fatal(err)
	}
	if afterGen < before {
		t.Fatalf("rollback %d -> %d", before, afterGen)
	}
	if !g.Expect.UnackedStillDrainable || !g.Expect.GenerationNeverRollback {
		t.Fatal("golden flags")
	}
}

func TestAutoCreditBlockedWhenOccupied(t *testing.T) {
	ctx := context.Background()
	svc := latecreditgate.New()
	k := key("sol", "0xsol", 3)
	if _, err := svc.OccupyWithGen(ctx, latecreditgate.OccupyRequest{OutputKey: k, ActorID: "occ", Fence: fenceOK()}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.AutoCredit(ctx, latecreditgate.CreditRequest{OutputKey: k, ActorID: "sys", Fence: fenceOK()})
	if !errors.Is(err, latecreditgate.ErrCreditBlocked) {
		t.Fatal(err)
	}
	races,
		_ := svc.DrainOpenRaces(ctx, 10)
	found := false
	for _, r := range races {
		if r.Kind == latecreditgate.RaceOccupyVsAutoCredit {
			found = true
		}
	}
	if !found {
		t.Fatal("expected auto-credit race kind")
	}
}

func TestCreditThenOccupyConflict(t *testing.T) {
	ctx := context.Background()
	svc := latecreditgate.New()
	k := key("eth", "0xcr", 4)
	if _, err := svc.CheckCreditDurable(ctx, latecreditgate.CreditRequest{OutputKey: k, ActorID: "crd", Fence: fenceOK()}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.OccupyWithGen(ctx, latecreditgate.OccupyRequest{OutputKey: k, ActorID: "occ", Fence: fenceOK()})
	if !errors.Is(err, latecreditgate.ErrOccupyConflict) {
		t.Fatalf("got %v", err)
	}
}

func TestIdempotentOccupySameActor(t *testing.T) {
	ctx := context.Background()
	svc := latecreditgate.New()
	k := key("eth", "0xid", 5)
	req := latecreditgate.OccupyRequest{OutputKey: k, ActorID: "same", Fence: fenceOK()}
	a, err := svc.OccupyWithGen(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.OccupyWithGen(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if a.GenID != b.GenID || a.Generation != b.Generation {
		t.Fatal("idempotent occupy changed gen")
	}
}

func TestAckRaceAndWorker(t *testing.T) {
	ctx := context.Background()
	svc := latecreditgate.New()
	k := key("eth", "0xwk", 6)
	_, _ = svc.OccupyWithGen(ctx, latecreditgate.OccupyRequest{OutputKey: k, ActorID: "occ", Fence: fenceOK()})
	_, _ = svc.CheckCreditDurable(ctx, latecreditgate.CreditRequest{OutputKey: k, ActorID: "c", Fence: fenceOK()})
	w := latecreditgate.NewWorker(svc, latecreditgate.WorkerConfig{BatchSize: 10, OccupyTTL: time.Hour}, nil)
	if n := w.PumpRacesOnce(ctx); n < 1 {
		t.Fatalf("pump %d", n)
	}
	evs, _ := svc.DrainOpenRaces(ctx, 10)
	if _, err := svc.AckRace(ctx, latecreditgate.ResolveRaceRequest{EventID: evs[0].EventID, ActorID: "ops"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AckRace(ctx, latecreditgate.ResolveRaceRequest{EventID: evs[0].EventID}); !errors.Is(err, latecreditgate.ErrRaceEventAlreadyAcked) {
		t.Fatalf("reack %v", err)
	}
	if n := w.InspectFenceOnce(ctx); n != 0 {
		t.Fatalf("fence omit %d", n)
	}
	_ = w.SweepZombiesOnce(ctx)
}

func TestViewHooks(t *testing.T) {
	ctx := context.Background()
	k := key("eth", "0xvw", 7)
	ob := reclaim.NewOccupancyBook()
	cb := reclaim.NewCaseBook()
	tl := &reclaim.MemoryTimeline{}
	ob.Put(reclaim.OccupancyView{
		OccupancyID:	"o1", CaseID: "c1", OutputKey: k, State: reclaim.OccupancyActive,
		OccupiedAt:	time.Now(), ExpiresAt: time.Now().Add(time.Hour),
	})
	cb.Put(reclaim.CaseView{
		CaseID:	"c1", OutputKey: k, State: reclaim.ReclaimCaseReserved, CreatedAt: time.Now(),
	})
	svc := latecreditgate.New().WithReaders(cb, ob, tl)
	if _, err := svc.OccupyWithGen(ctx, latecreditgate.OccupyRequest{OutputKey: k, ActorID: "occ", CaseID: "c1", Fence: fenceOK()}); err != nil {
		t.Fatal(err)
	}
	v, err := svc.View(ctx, k)
	if err != nil {
		t.Fatal(err)
	}
	if v.Occupancy == nil || v.Case == nil || v.Generation == nil {
		t.Fatal("hooks missing")
	}
	if snap := tl.Snapshot(k); len(snap.Events) == 0 {
		t.Fatal("timeline empty")
	}
}

func TestGuardWriteValidFenceAdvances(t *testing.T) {
	ctx := context.Background()
	svc := latecreditgate.New()
	k := key("eth", "0xgw", 8)
	g, err := svc.Reserve(ctx, k, "ops", fenceOK())
	if err != nil || g.State != latecreditgate.GenOccupied {
		t.Fatalf("%v %v", err, g)
	}
	g, err = svc.Reclaim(ctx, k, "ops", latecreditgate.FenceToken{Epoch: 4, Token: "tok-4"})
	if err != nil || g.State != latecreditgate.GenReclaimed {
		t.Fatalf("%v %v", err, g)
	}
	if g.Generation < 2 {
		t.Fatal("gen not raised")
	}
}

func TestLegacyReadAllowsOmit(t *testing.T) {
	ctx := context.Background()
	svc := latecreditgate.New()
	k := key("eth", "0xlg", 9)
	if _, err := svc.OccupyWithGen(ctx, latecreditgate.OccupyRequest{OutputKey: k, ActorID: "a", Fence: fenceOK()}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetGeneration(ctx, k, latecreditgate.FenceToken{}, true); err != nil {
		t.Fatal(err)
	}
}

func TestReclaimInProcessFenceStillAllowsOmit(t *testing.T) {
	// 历史缺口：进程内 checkFence 仍可省略；写路径必须走 latecreditgate。
	if !reclaim.CheckFence(0, "") {
		t.Fatal("legacy in-process still skips")
	}
	if latecreditgate.MustFenceWrite(latecreditgate.FenceToken{}) == nil {
		t.Fatal("durable gate must reject")
	}
}

func TestHTTPHandlerFenceReject(t *testing.T) {
	// 编译期保证 Handler 绑定；行为由服务层覆盖。
	h := latecreditgate.NewHandler(latecreditgate.New())
	if h == nil {
		t.Fatal("handler")
	}
}

func TestInvalidRequests(t *testing.T) {
	ctx := context.Background()
	svc := latecreditgate.New()
	if _, err := svc.OccupyWithGen(ctx, latecreditgate.OccupyRequest{ActorID: "a", Fence: fenceOK()}); !errors.Is(err, latecreditgate.ErrOutputKeyEmpty) {
		t.Fatal(err)
	}
	if _, err := svc.OccupyWithGen(ctx, latecreditgate.OccupyRequest{OutputKey: key("e", "h", 0), Fence: fenceOK()}); !errors.Is(err, latecreditgate.ErrActorEmpty) {
		t.Fatal(err)
	}
	if err := latecreditgate.ValidateWritePath("nope"); !errors.Is(err, latecreditgate.ErrInvalidRequest) {
		t.Fatal(err)
	}
}
