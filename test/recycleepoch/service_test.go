package recycleepoch_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/recycleepoch"
)

func testdataDir(t *testing.T) string {
	t.Helper()
	_,
		file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "recycleepoch")
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

type clock struct {
	mu	sync.Mutex
	t	time.Time
}

func newClock(t time.Time) *clock	{ return &clock{t: t.UTC()} }

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) Add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func setupMarkReadyAssign(t *testing.T, svc *recycleepoch.Service, clk *clock, chain, addr, prev, next string, cool time.Duration) *recycleepoch.RecycleEpoch {
	t.Helper()
	ctx := context.Background()
	_, err := svc.MarkWithEpoch(ctx, recycleepoch.MarkRequest{
		Chain:	chain, Address: addr, PrevUser: prev, ActorID: "ops",
		IdemKey:	"mark-1", CoolUntil: clk.Now().Add(cool),
	})
	if err != nil {
		t.Fatal(err)
	}
	clk.Add(cool + time.Second)
	if _, err := svc.ReadyAssert(ctx, recycleepoch.ReadyRequest{Chain: chain, Address: addr, ActorID: "ops"}); err != nil {
		t.Fatal(err)
	}
	e, _, err := svc.AssignUser(ctx, recycleepoch.AssignRequest{Chain: chain, Address: addr, NextUser: next, ActorID: "ops", IdemKey: "as-1"})
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestLateObserveOldUserGolden(t *testing.T) {
	var g struct {
		Input	struct {
			Chain		string	`json:"chain"`
			Address		string	`json:"address"`
			PrevUser	string	`json:"prev_user"`
			NextUser	string	`json:"next_user"`
			ObserveUser	string	`json:"observe_user"`
			Asset		string	`json:"asset"`
			Amount		string	`json:"amount"`
			TxHash		string	`json:"tx_hash"`
			CarriedGen	int64	`json:"carried_gen"`
		}	`json:"input"`
		Expect	recycleepoch.ScenarioExpect	`json:"expect"`
	}
	loadGolden(t, "golden_late_observe_old_user.json", &g)
	clk := newClock(time.Date(2021,
		8, 14, 12, 0, 0, 0, time.UTC))
	svc := recycleepoch.New().WithClock(clk.Now)
	e := setupMarkReadyAssign(t, svc, clk, g.Input.Chain, g.Input.Address, g.Input.PrevUser, g.Input.NextUser, time.Second)
	if e.State != recycleepoch.EpochAssigned || e.Generation != g.Expect.Generation {
		t.Fatalf("epoch %+v", e)
	}
	rec, err := svc.ObserveCreditAssertEpoch(context.Background(), recycleepoch.ObserveCreditRequest{
		Chain:	g.Input.Chain, Address: g.Input.Address, UserID: g.Input.ObserveUser,
		CarriedGen:	g.Input.CarriedGen,
		TxHash:		g.Input.TxHash, Asset: g.Input.Asset,
		Amount:	g.Input.Amount, ActorID: "deposit", IdemKey: "late-1",
	})
	if !errors.Is(err, recycleepoch.ErrWrongUserCredit) {
		t.Fatalf("err=%v", err)
	}
	if rec == nil || rec.CreditPosted() {
		t.Fatalf("credit posted: %+v", rec)
	}
	if err.Error() != g.Expect.Error {
		t.Fatalf("err text %q", err.Error())
	}
	ps, _ := svc.ListProofsBySlot(context.Background(), g.Input.Chain, g.Input.Address)
	if len(ps) == 0 || ps[0].Kind != recycleepoch.ProofLateObserveOldUser || !ps[0].FailClosed {
		t.Fatalf("proofs %+v", ps)
	}
	if string(ps[0].Kind) != g.Expect.ProofKind {
		t.Fatalf("kind %s", ps[0].Kind)
	}
}

func TestStaleEpochGolden(t *testing.T) {
	var g struct {
		Input	struct {
			Chain		string	`json:"chain"`
			Address		string	`json:"address"`
			PrevUser	string	`json:"prev_user"`
			NextUser	string	`json:"next_user"`
			CarriedGen	int64	`json:"carried_gen"`
			CurrentGen	int64	`json:"current_gen"`
		}	`json:"input"`
		Expect	recycleepoch.ScenarioExpect	`json:"expect"`
	}
	loadGolden(t, "golden_stale_epoch.json", &g)
	clk := newClock(time.Date(2021, 8, 14, 13, 0, 0, 0, time.UTC))
	svc := recycleepoch.New().WithClock(clk.Now)
	ctx := context.Background()
	setupMarkReadyAssign(t, svc, clk, g.Input.Chain, g.Input.Address, g.Input.PrevUser, g.Input.NextUser, time.Second)
	// 再回收推进世代。
	_, err := svc.MarkWithEpoch(ctx, recycleepoch.MarkRequest{
		Chain:	g.Input.Chain, Address: g.Input.Address, PrevUser: g.Input.NextUser,
		ActorID:	"ops", IdemKey: "mark-2", CoolUntil: clk.Now().Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	clk.Add(2 * time.Second)
	if _, err := svc.ReadyAssert(ctx, recycleepoch.ReadyRequest{Chain: g.Input.Chain, Address: g.Input.Address, ActorID: "ops"}); err != nil {
		t.Fatal(err)
	}
	e, _, err := svc.AssignUser(ctx, recycleepoch.AssignRequest{
		Chain:	g.Input.Chain, Address: g.Input.Address, NextUser: "userC", ActorID: "ops", IdemKey: "as-2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if e.Generation != g.Expect.Generation {
		t.Fatalf("gen=%d", e.Generation)
	}
	rec, err := svc.ObserveCreditAssertEpoch(ctx, recycleepoch.ObserveCreditRequest{
		Chain:	g.Input.Chain, Address: g.Input.Address, UserID: "userC",
		CarriedGen:	g.Input.CarriedGen, TxHash: "0xstale01", Asset: "USDT", Amount: "1",
		ActorID:	"deposit", IdemKey: "stale-1",
	})
	if !errors.Is(err, recycleepoch.ErrEpochStale) {
		t.Fatalf("err=%v", err)
	}
	if rec.CreditPosted() {
		t.Fatal("credit")
	}
	ps,
		_ := svc.ListProofsBySlot(ctx, g.Input.Chain, g.Input.Address)
	found := false
	for _, p := range ps {
		if p.Kind == recycleepoch.ProofStaleEpoch {
			found = true
		}
	}
	if !found {
		t.Fatalf("no stale proof: %+v", ps)
	}
}

func TestReadyBeforeCoolGolden(t *testing.T) {
	var g struct {
		Input	struct {
			Chain		string	`json:"chain"`
			Address		string	`json:"address"`
			PrevUser	string	`json:"prev_user"`
			NextUser	string	`json:"next_user"`
			CoolSec		int	`json:"cool_sec"`
		}	`json:"input"`
		Expect	struct {
			ReadyError	string	`json:"ready_error"`
			ForceError	string	`json:"force_error"`
			CreditPosted	bool	`json:"credit_posted"`
			FailClosed	bool	`json:"fail_closed"`
			EpochState	string	`json:"epoch_state"`
		}	`json:"expect"`
	}
	loadGolden(t, "golden_ready_before_cool.json", &g)
	clk := newClock(time.Date(2021, 8, 14, 14, 0, 0, 0, time.UTC))
	svc := recycleepoch.New().WithClock(clk.Now)
	ctx := context.Background()
	_, err := svc.MarkWithEpoch(ctx, recycleepoch.MarkRequest{
		Chain:	g.Input.Chain, Address: g.Input.Address, PrevUser: g.Input.PrevUser,
		ActorID:	"ops", IdemKey: "m", CoolUntil: clk.Now().Add(time.Duration(g.Input.CoolSec) * time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.ReadyAssert(ctx, recycleepoch.ReadyRequest{Chain: g.Input.Chain, Address: g.Input.Address, ActorID: "ops"})
	if !errors.Is(err, recycleepoch.ErrCoolNotElapsed) {
		t.Fatalf("ready %v", err)
	}
	if err.Error() != g.Expect.ReadyError {
		t.Fatalf("ready text %q", err)
	}
	_, err = svc.ReadyAssert(ctx, recycleepoch.ReadyRequest{Chain: g.Input.Chain, Address: g.Input.Address, ActorID: "ops", Force: true})
	if !errors.Is(err, recycleepoch.ErrForcedReuse) {
		t.Fatalf("force %v", err)
	}
	if err.Error() != g.Expect.ForceError {
		t.Fatalf("force text %q", err)
	}
	_, _, err = svc.AssignUser(ctx, recycleepoch.AssignRequest{
		Chain:	g.Input.Chain, Address: g.Input.Address, NextUser: g.Input.NextUser, ActorID: "ops", IdemKey: "x",
	})
	if !errors.Is(err, recycleepoch.ErrForcedReuse) && !errors.Is(err, recycleepoch.ErrNotReady) {
		t.Fatalf("assign %v", err)
	}
	rec, err := svc.ObserveCreditAssertEpoch(ctx, recycleepoch.ObserveCreditRequest{
		Chain:	g.Input.Chain, Address: g.Input.Address, UserID: g.Input.PrevUser,
		CarriedGen:	1, TxHash: "0xforce01", Asset: "USDT", Amount: "1",
		ActorID:	"deposit", IdemKey: "force-obs",
	})
	if rec != nil && rec.CreditPosted() {
		t.Fatal("forced credit")
	}
	if err == nil {
		t.Fatal("expected observe fail")
	}
	e, _ := svc.GetEpoch(ctx, g.Input.Chain, g.Input.Address)
	if string(e.State) != g.Expect.EpochState {
		t.Fatalf("state %s", e.State)
	}
}

func TestNewUserMatchGolden(t *testing.T) {
	var g struct {
		Input	struct {
			Chain		string	`json:"chain"`
			Address		string	`json:"address"`
			PrevUser	string	`json:"prev_user"`
			NextUser	string	`json:"next_user"`
			Asset		string	`json:"asset"`
			Amount		string	`json:"amount"`
			TxHash		string	`json:"tx_hash"`
			CarriedGen	int64	`json:"carried_gen"`
		}	`json:"input"`
		Expect	recycleepoch.ScenarioExpect	`json:"expect"`
	}
	loadGolden(t, "golden_new_user_match.json", &g)
	clk := newClock(time.Date(2021,
		8, 14, 15, 0, 0, 0, time.UTC))
	svc := recycleepoch.New().WithClock(clk.Now)
	setupMarkReadyAssign(t, svc, clk, g.Input.Chain, g.Input.Address, g.Input.PrevUser, g.Input.NextUser, time.Second)
	rec, err := svc.ObserveCreditAssertEpoch(context.Background(), recycleepoch.ObserveCreditRequest{
		Chain:		g.Input.Chain,
		Address:	g.Input.Address, UserID: g.Input.NextUser,
		CarriedGen:	g.Input.CarriedGen, TxHash: g.Input.TxHash, Asset: g.Input.Asset,
		Amount:	g.Input.Amount, ActorID: "deposit", IdemKey: "ok-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !rec.CreditPosted() || rec.Outcome != recycleepoch.OutcomeCredited {
		t.Fatalf("%+v", rec)
	}
}

func TestObserveIdempotentGolden(t *testing.T) {
	var g struct {
		Input	struct {
			IdemKey string `json:"idem_key"`
		}	`json:"input"`
		Expect	struct {
			CreditPosted	bool	`json:"credit_posted"`
			SameObserveID	bool	`json:"same_observe_id"`
		}	`json:"expect"`
	}
	loadGolden(t, "golden_idempotent_observe.json", &g)
	clk := newClock(time.Date(2021, 8, 14, 16, 0, 0, 0, time.UTC))
	svc := recycleepoch.New().WithClock(clk.Now)
	addr := "0xeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee"
	setupMarkReadyAssign(t, svc, clk, "ETH", addr, "userA", "userB", time.Second)
	req := recycleepoch.ObserveCreditRequest{
		Chain:	"ETH", Address: addr, UserID: "userB", CarriedGen: 1,
		TxHash:	"0xidem01", Asset: "USDT", Amount: "2", ActorID: "deposit", IdemKey: g.Input.IdemKey,
	}
	a, err := svc.ObserveCreditAssertEpoch(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.ObserveCreditAssertEpoch(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if a.ObserveID != b.ObserveID {
		t.Fatalf("idem %s %s", a.ObserveID, b.ObserveID)
	}
	if !b.CreditPosted() {
		t.Fatal("credit")
	}
}

func TestMissingEpochFailClosed(t *testing.T) {
	svc := recycleepoch.New()
	_, err := svc.ObserveCreditAssertEpoch(context.Background(), recycleepoch.ObserveCreditRequest{
		Chain:	"ETH", Address: "0xffffffffffffffffffffffffffffffffffffffff",
		UserID:	"userA", CarriedGen: 1, TxHash: "0xmiss", Asset: "USDT", Amount: "1",
		ActorID:	"d", IdemKey: "m1",
	})
	if !errors.Is(err, recycleepoch.ErrEpochMissing) {
		t.Fatalf("%v", err)
	}
}

func TestDeniedObserveReplayFailClosed(t *testing.T) {
	svc := recycleepoch.New()
	req := recycleepoch.ObserveCreditRequest{
		Chain:	"ETH", Address: "0xffffffffffffffffffffffffffffffffffffffff",
		UserID:	"userA", CarriedGen: 1, TxHash: "0xmiss", Asset: "USDT", Amount: "1",
		ActorID:	"d", IdemKey: "denied-replay",
	}
	a, err := svc.ObserveCreditAssertEpoch(context.Background(), req)
	if !errors.Is(err, recycleepoch.ErrEpochMissing) {
		t.Fatalf("first %v", err)
	}
	b,
		err := svc.ObserveCreditAssertEpoch(context.Background(), req)
	if err == nil || b.CreditPosted() {
		t.Fatal("denied observe replay must not succeed")
	}
	if a.ObserveID != b.ObserveID {
		t.Fatalf("idem %s %s", a.ObserveID, b.ObserveID)
	}
	if !errors.Is(err, recycleepoch.ErrCreditBlocked) {
		t.Fatalf("replay %v", err)
	}
}

func TestMarkIdempotent(t *testing.T) {
	clk := newClock(time.Date(2021, 8, 14, 17, 0, 0, 0, time.UTC))
	svc := recycleepoch.New().WithClock(clk.Now)
	req := recycleepoch.MarkRequest{
		Chain:	"ETH", Address: "0x1111111111111111111111111111111111111111",
		PrevUser:	"userA", ActorID: "ops", IdemKey: "same", CoolUntil: clk.Now().Add(time.Hour),
	}
	a, err := svc.MarkWithEpoch(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := svc.MarkWithEpoch(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if a.EpochID != b.EpochID || a.Generation != b.Generation {
		t.Fatalf("%+v %+v", a, b)
	}
}

func TestParallelObserveRace(t *testing.T) {
	clk := newClock(time.Date(2021, 8, 14, 18, 0, 0, 0, time.UTC))
	svc := recycleepoch.New().WithClock(clk.Now)
	addr := "0x2222222222222222222222222222222222222222"
	setupMarkReadyAssign(t, svc, clk, "ETH", addr, "userA", "userB", time.Second)
	var ok, deny atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			user := "userA"
			if i%2 == 0 {
				user = "userB"
			}
			_, err := svc.ObserveCreditAssertEpoch(context.Background(), recycleepoch.ObserveCreditRequest{
				Chain:	"ETH", Address: addr, UserID: user, CarriedGen: 1,
				TxHash:	"0xrace" + time.Now().Format("150405.000000000"),
				Asset:	"USDT", Amount: "1", ActorID: "d",
				IdemKey:	"race-" + user + "-" + time.Now().Format("150405.000000000") + string(rune('a'+i)),
			})
			if err == nil {
				ok.Add(1)
			} else {
				deny.Add(1)
			}
		}(i)
	}
	wg.Wait()
	if ok.Load() == 0 {
		t.Fatal("expected some userB credits")
	}
	if deny.Load() == 0 {
		t.Fatal("expected userA denies")
	}
}

func TestWorkerCoolPump(t *testing.T) {
	clk := newClock(time.Date(2021, 8, 14, 19, 0, 0, 0, time.UTC))
	svc := recycleepoch.New().WithClock(clk.Now)
	addr := "0x3333333333333333333333333333333333333333"
	_, err := svc.MarkWithEpoch(context.Background(), recycleepoch.MarkRequest{
		Chain:	"ETH", Address: addr, PrevUser: "userA", ActorID: "ops",
		IdemKey:	"w", CoolUntil: clk.Now().Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	clk.Add(2 * time.Second)
	w := recycleepoch.NewWorker(svc, recycleepoch.WorkerConfig{BatchSize: 10, AutoReadyOnElapsed: true}, nil)
	if n := w.PumpCoolOnce(context.Background()); n == 0 {
		t.Fatal("pump")
	}
	e, err := svc.GetEpoch(context.Background(), "ETH", addr)
	if err != nil || e.State != recycleepoch.EpochReady {
		t.Fatalf("%v %+v", err, e)
	}
}

func TestSnapshotRestartProofs(t *testing.T) {
	clk := newClock(time.Date(2021, 8, 14, 20, 0, 0, 0, time.UTC))
	mem := recycleepoch.NewMemStore()
	svc := recycleepoch.NewWithStore(mem).WithClock(clk.Now)
	addr := "0x4444444444444444444444444444444444444444"
	setupMarkReadyAssign(t, svc, clk, "ETH", addr, "userA", "userB", time.Second)
	_, _ = svc.ObserveCreditAssertEpoch(context.Background(), recycleepoch.ObserveCreditRequest{
		Chain:	"ETH", Address: addr, UserID: "userA", CarriedGen: 1,
		TxHash:	"0xrst", Asset: "USDT", Amount: "1", ActorID: "d", IdemKey: "rst",
	})
	snap := mem.Snapshot()
	svc2 := recycleepoch.NewWithStore(snap).WithClock(clk.Now)
	ps,
		err := svc2.DrainOpenProofs(context.Background(), 50)
	if err != nil || len(ps) == 0 {
		t.Fatalf("drain %v %d", err, len(ps))
	}
}

func TestFSMTransitions(t *testing.T) {
	rows := []struct {
		from, to	recycleepoch.EpochState
		ok		bool
	}{
		{"", recycleepoch.EpochCooling, true},
		{recycleepoch.EpochCooling, recycleepoch.EpochReady, true},
		{recycleepoch.EpochReady, recycleepoch.EpochAssigned, true},
		{recycleepoch.EpochAssigned, recycleepoch.EpochCooling, true},
		{recycleepoch.EpochCooling, recycleepoch.EpochAssigned, false},
		{recycleepoch.EpochRetired, recycleepoch.EpochReady, false},
		{recycleepoch.EpochAssigned, recycleepoch.EpochRetired, true},
	}
	for _, r := range rows {
		if recycleepoch.AllowedTransition(r.from, r.to) != r.ok {
			t.Fatalf("%s→%s want %v", r.from, r.to, r.ok)
		}
	}
}

func TestQuarantineDisposition(t *testing.T) {
	clk := newClock(time.Date(2021,
		8, 14, 21, 0, 0, 0, time.UTC))
	svc := recycleepoch.New().WithClock(clk.Now).WithDisposition(recycleepoch.DispQuarantine)
	addr := "0x5555555555555555555555555555555555555555"
	setupMarkReadyAssign(t, svc, clk, "ETH", addr, "userA", "userB", time.Second)
	rec, err := svc.ObserveCreditAssertEpoch(context.Background(), recycleepoch.ObserveCreditRequest{
		Chain:	"ETH", Address: addr, UserID: "userA", CarriedGen: 1,
		TxHash:	"0xq", Asset: "USDT", Amount: "1", ActorID: "d", IdemKey: "q1",
	})
	if !errors.Is(err, recycleepoch.ErrQuarantined) {
		t.Fatalf("%v", err)
	}
	if rec.Outcome != recycleepoch.OutcomeQuarantined || rec.CreditPosted() {
		t.Fatalf("%+v", rec)
	}
}

func TestRetireBlocksCredit(t *testing.T) {
	clk := newClock(time.Date(2021,
		8, 14, 22, 0, 0, 0, time.UTC))
	svc := recycleepoch.New().WithClock(clk.Now)
	addr := "0x6666666666666666666666666666666666666666"
	setupMarkReadyAssign(t, svc, clk, "ETH", addr, "userA", "userB", time.Second)
	if _, err := svc.Retire(context.Background(), recycleepoch.RetireRequest{Chain: "ETH", Address: addr, ActorID: "ops"}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.ObserveCreditAssertEpoch(context.Background(), recycleepoch.ObserveCreditRequest{
		Chain:	"ETH", Address: addr, UserID: "userB", CarriedGen: 1,
		TxHash:	"0xret", Asset: "USDT", Amount: "1", ActorID: "d", IdemKey: "ret",
	})
	if !errors.Is(err, recycleepoch.ErrEpochRetired) {
		t.Fatalf("%v", err)
	}
}

func TestViewAndInspect(t *testing.T) {
	clk := newClock(time.Date(2021, 8, 14, 23, 0, 0, 0, time.UTC))
	svc := recycleepoch.New().WithClock(clk.Now)
	addr := "0x7777777777777777777777777777777777777777"
	setupMarkReadyAssign(t, svc, clk, "ETH", addr, "userA", "userB", time.Second)
	v, err := svc.View(context.Background(), "ETH", addr)
	if err != nil || v.Epoch == nil || v.Bind == nil {
		t.Fatalf("%v %+v", err, v)
	}
	if _, err := svc.InspectBindings(context.Background(), 50); err != nil {
		t.Fatal(err)
	}
}

func TestAckProof(t *testing.T) {
	clk := newClock(time.Date(2021, 8, 15, 0, 0, 0, 0, time.UTC))
	svc := recycleepoch.New().WithClock(clk.Now)
	addr := "0x8888888888888888888888888888888888888888"
	setupMarkReadyAssign(t, svc, clk, "ETH", addr, "userA", "userB", time.Second)
	_, _ = svc.ObserveCreditAssertEpoch(context.Background(), recycleepoch.ObserveCreditRequest{
		Chain:	"ETH", Address: addr, UserID: "userA", CarriedGen: 1,
		TxHash:	"0xack", Asset: "USDT", Amount: "1", ActorID: "d", IdemKey: "ack",
	})
	ps, _ := svc.DrainOpenProofs(context.Background(), 10)
	if len(ps) == 0 {
		t.Fatal("no proof")
	}
	got, err := svc.AckProof(context.Background(), recycleepoch.AckProofRequest{ProofID: ps[0].ProofID, ActorID: "ops"})
	if err != nil || got.State != recycleepoch.ProofAcked {
		t.Fatalf("%v %+v", err, got)
	}
	if _, err := svc.AckProof(context.Background(), recycleepoch.AckProofRequest{ProofID: ps[0].ProofID, ActorID: "ops"}); !errors.Is(err, recycleepoch.ErrProofAlreadyAcked) {
		t.Fatalf("%v", err)
	}
}

func TestAddrpoolHint(t *testing.T) {
	h := RecycleEpochHintShape()
	if h == "" {
		t.Fatal("hint")
	}
}

func RecycleEpochHintShape() string {
	return recycleepoch.SlotCanon("eth", "0xabc")
}

func TestCounters(t *testing.T) {
	var c recycleepoch.Counters
	c.NoteObserve(nil)
	c.NoteObserve(recycleepoch.ErrEpochStale)
	c.NoteReady(recycleepoch.ErrCoolNotElapsed)
	c.NoteProofOpen()
	c.NoteProofAck()
	m := c.Snapshot()
	if m["recycleepoch_observe_ok"] != 1 || m["recycleepoch_stale_epoch"] != 1 {
		t.Fatalf("%v", m)
	}
}

func TestHTTPStatusMapping(t *testing.T) {
	if recycleepoch.StatusOf(recycleepoch.ErrWrongUserCredit) != 409 {
		t.Fatal("conflict")
	}
	if recycleepoch.StatusOf(recycleepoch.ErrEpochNotFound) != 404 {
		t.Fatal("404")
	}
	if recycleepoch.StatusOf(recycleepoch.ErrActorEmpty) != 400 {
		t.Fatal("400")
	}
}

func TestDigestAndSort(t *testing.T) {
	ps := []recycleepoch.MiscreditProof{
		{Kind: recycleepoch.ProofStaleEpoch, AttemptUser: "b", FailClosed: true, OpenedAt: time.Unix(2, 0)},
		{Kind: recycleepoch.ProofLateObserveOldUser, AttemptUser: "a", FailClosed: true, OpenedAt: time.Unix(1, 0)},
	}
	out := recycleepoch.SortProofs(ps)
	if out[0].Kind != recycleepoch.ProofLateObserveOldUser {
		t.Fatal(out[0].Kind)
	}
	d := recycleepoch.DigestProof(out[0])
	if !d.FailClosed || d.CreditOK {
		t.Fatal(d)
	}
}
