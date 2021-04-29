package feebumpgate_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/feebumpgate"
)

func TestGoldenDoubleBumpRetire(t *testing.T) {
	g, err := feebumpgate.LoadGolden("double_bump_retire.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	eng := feebumpgate.NewEngine(feebumpgate.NewMemStore())
	ctx := context.Background()
	first, err := eng.SignFeeBumpWithGen(ctx, g.firstReq())
	if err != nil {
		t.Fatal(err)
	}
	second, err := eng.SignFeeBumpWithGen(ctx, g.secondReq(first.Gen.NewBroadcastHash))
	if err != nil {
		t.Fatal(err)
	}
	held, err := eng.Store().CountHeld(ctx, g.TransferID)
	if err != nil {
		t.Fatal(err)
	}
	if held != g.Expected.HeldAfterSecond {
		t.Fatalf("held=%d want %d", held, g.Expected.HeldAfterSecond)
	}
	_, err = eng.BroadcastReplace(ctx, feebumpgate.BroadcastReq{
		GenID:	first.Gen.GenID, TransferID: g.TransferID, Actor: g.Actor,
		BroadcastHash:	first.Gen.NewBroadcastHash,
	})
	if err == nil || err.Error() != g.Expected.OldBroadcastError {
		t.Fatalf("old broadcast err=%v want %s", err, g.Expected.OldBroadcastError)
	}
	if _, err := eng.BroadcastReplace(ctx, feebumpgate.BroadcastReq{
		GenID:		second.Gen.GenID,
		TransferID:	g.TransferID, Actor: g.Actor,
		BroadcastHash:	second.Gen.NewBroadcastHash,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestGoldenPrevHashMismatch(t *testing.T) {
	g, err := feebumpgate.LoadGolden("prev_hash_mismatch.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	eng := feebumpgate.NewEngine(feebumpgate.NewMemStore())
	ctx := context.Background()
	first, err := eng.SignFeeBumpWithGen(ctx, g.firstReq())
	if err != nil {
		t.Fatal(err)
	}
	_, err = eng.SignFeeBumpWithGen(ctx, g.secondReq(first.Gen.NewBroadcastHash))
	if err == nil || err.Error() != g.Expected.SecondError {
		t.Fatalf("second err=%v want %s", err, g.Expected.SecondError)
	}
	held,
		_ := eng.Store().CountHeld(ctx, g.TransferID)
	if held != g.Expected.HeldAfterSecond {
		t.Fatalf("held=%d", held)
	}
	proofs, _ := eng.ExportProofs(ctx, g.TransferID)
	found := false
	for _, p := range proofs {
		if string(p.Kind) == g.Expected.ProofKind {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing proof %s", g.Expected.ProofKind)
	}
}

func TestGoldenRetireFailOrphan(t *testing.T) {
	g, err := feebumpgate.LoadGolden("retire_fail_orphan.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	inner := feebumpgate.NewMemStore()
	eng := feebumpgate.NewEngine(&failRetireStore{Store: inner, failOn: g.TransferID})
	ctx := context.Background()
	first, err := eng.SignFeeBumpWithGen(ctx, g.firstReq())
	if err != nil {
		t.Fatal(err)
	}
	_, err = eng.SignFeeBumpWithGen(ctx, g.secondReq(first.Gen.NewBroadcastHash))
	if err == nil || err.Error() != g.Expected.SecondError {
		t.Fatalf("second err=%v want %s", err, g.Expected.SecondError)
	}
	_, err = eng.SignFeeBumpWithGen(ctx, feebumpgate.SignWithGenReq{
		TransferID:	g.TransferID, Chain: g.Chain, To: g.To, Amount: g.Amount,
		Asset:	g.Asset, NewFee: "0.00800000", DestinationTag: g.DestinationTag,
		PrevBroadcastHash:	first.Gen.NewBroadcastHash, Actor: g.Actor, IdempotencyKey: g.TransferID + ":3",
	})
	if !errors.Is(err, feebumpgate.ErrOrphanOpen) {
		t.Fatalf("further sign err=%v", err)
	}
}

func TestGoldenOldGenBroadcast(t *testing.T) {
	g, err := feebumpgate.LoadGolden("old_gen_broadcast.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	eng := feebumpgate.NewEngine(feebumpgate.NewMemStore())
	ctx := context.Background()
	first, err := eng.SignFeeBumpWithGen(ctx, g.firstReq())
	if err != nil {
		t.Fatal(err)
	}
	second, err := eng.SignFeeBumpWithGen(ctx, g.secondReq(first.Gen.NewBroadcastHash))
	if err != nil {
		t.Fatal(err)
	}
	_, err = eng.BroadcastReplace(ctx, feebumpgate.BroadcastReq{
		GenID:	first.Gen.GenID, TransferID: g.TransferID, Actor: g.Actor,
		BroadcastHash:	first.Gen.NewBroadcastHash,
	})
	if err == nil || err.Error() != g.Expected.OldBroadcastError {
		t.Fatalf("old err=%v", err)
	}
	if _, err := eng.BroadcastReplace(ctx, feebumpgate.BroadcastReq{
		GenID:	second.Gen.GenID, TransferID: g.TransferID, Actor: g.Actor,
		BroadcastHash:	second.Gen.NewBroadcastHash,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestGoldenNoGenFailClosed(t *testing.T) {
	g, err := feebumpgate.LoadGolden("no_gen_fail_closed.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	eng := feebumpgate.NewEngine(feebumpgate.NewMemStore())
	ctx := context.Background()
	_, err = eng.BroadcastReplace(ctx, feebumpgate.BroadcastReq{TransferID: g.TransferID, Actor: g.Actor})
	if !errors.Is(err, feebumpgate.ErrNoGen) {
		t.Fatalf("err=%v", err)
	}
	_, err = eng.AssertOccupancy(ctx, feebumpgate.AssertOccReq{TransferID: g.TransferID, Actor: g.Actor})
	if !errors.Is(err, feebumpgate.ErrNoGen) {
		t.Fatalf("assert err=%v", err)
	}
}

func TestTableSignValidation(t *testing.T) {
	eng := feebumpgate.NewEngine(feebumpgate.NewMemStore())
	ctx := context.Background()
	base := feebumpgate.SignWithGenReq{
		TransferID:	"t1", Chain: "BTC", To: "bc1qxx", Amount: "1", Asset: "BTC",
		NewFee:	"0.0001", Actor: "a",
	}
	cases := []struct {
		name	string
		mut	func(*feebumpgate.SignWithGenReq,)
		want	error
	}{
		{"ok", func(*feebumpgate.SignWithGenReq,) {}, nil},
		{"no transfer", func(r *feebumpgate.SignWithGenReq,) { r.TransferID = "" }, feebumpgate.ErrTransfer},
		{"no chain", func(r *feebumpgate.SignWithGenReq,) { r.Chain = "" }, feebumpgate.ErrChain},
		{"no dest", func(r *feebumpgate.SignWithGenReq,) { r.To = "" }, feebumpgate.ErrDest},
		{"no amount", func(r *feebumpgate.SignWithGenReq,) { r.Amount = "" }, feebumpgate.ErrAmount},
		{"no fee", func(r *feebumpgate.SignWithGenReq,) { r.NewFee = "" }, feebumpgate.ErrFee},
		{"bad fee", func(r *feebumpgate.SignWithGenReq,) { r.NewFee = "x" }, feebumpgate.ErrFee},
		{"no actor", func(r *feebumpgate.SignWithGenReq,) { r.Actor = "" }, feebumpgate.ErrActor},
		{"no asset", func(r *feebumpgate.SignWithGenReq,) { r.Asset = "" }, feebumpgate.ErrAsset},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := base
			req.IdempotencyKey = tc.name
			tc.mut(&req)
			_, err := eng.SignFeeBumpWithGen(ctx, req)
			if tc.want == nil {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want %v", err, tc.want)
			}
		})
	}
}

func TestFeeNotRaised(t *testing.T) {
	eng := feebumpgate.NewEngine(feebumpgate.NewMemStore())
	ctx := context.Background()
	first, err := eng.SignFeeBumpWithGen(ctx, feebumpgate.SignWithGenReq{
		TransferID:	"t-fee", Chain: "BTC", To: "bc1q", Amount: "1", Asset: "BTC",
		NewFee:	"0.00020000", Actor: "a", IdempotencyKey: "k1",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = eng.SignFeeBumpWithGen(ctx, feebumpgate.SignWithGenReq{
		TransferID:	"t-fee", Chain: "BTC", To: "bc1q", Amount: "1", Asset: "BTC",
		NewFee:	"0.00010000", PrevBroadcastHash: first.Gen.NewBroadcastHash,
		Actor:	"a", IdempotencyKey: "k2",
	})
	if !errors.Is(err, feebumpgate.ErrFeeNotRaised) {
		t.Fatalf("err=%v", err)
	}
}

func TestIdempotentSign(t *testing.T) {
	eng := feebumpgate.NewEngine(feebumpgate.NewMemStore())
	ctx := context.Background()
	req := feebumpgate.SignWithGenReq{
		TransferID:	"t-idem", Chain: "BTC", To: "bc1q", Amount: "1", Asset: "BTC",
		NewFee:	"0.00010000", Actor: "a", IdempotencyKey: "same",
	}
	a, err := eng.SignFeeBumpWithGen(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := eng.SignFeeBumpWithGen(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if a.Gen.GenID != b.Gen.GenID {
		t.Fatalf("idem mismatch")
	}
}

func TestRetireExpiredAndFinalize(t *testing.T) {
	fixed := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	eng := feebumpgate.NewEngine(feebumpgate.NewMemStore(), feebumpgate.WithClock(func() time.Time { return fixed }), feebumpgate.WithConfig(feebumpgate.GateConfig{OccTTL: time.Minute}))
	ctx := context.Background()
	got, err := eng.SignFeeBumpWithGen(ctx, feebumpgate.SignWithGenReq{
		TransferID:	"t-ttl", Chain: "BTC", To: "bc1q", Amount: "1", Asset: "BTC",
		NewFee:	"0.00010000", Actor: "a", IdempotencyKey: "ttl",
	})
	if err != nil {
		t.Fatal(err)
	}
	n,
		err := eng.RetireExpired(ctx, fixed.Add(2*time.Minute), 10)
	if err != nil || n != 1 {
		t.Fatalf("retired=%d err=%v", n, err)
	}
	occ, err := eng.Store().GetOccByGen(ctx, got.Gen.GenID)
	if err != nil || occ.State != feebumpgate.OccRetired {
		t.Fatalf("occ=%v err=%v", occ.State, err)
	}
	if n != 1 {
		t.Fatalf("want 1 retired")
	}
}

func TestBroadcastThenFinalize(t *testing.T) {
	eng := feebumpgate.NewEngine(feebumpgate.NewMemStore())
	ctx := context.Background()
	got, err := eng.SignFeeBumpWithGen(ctx, feebumpgate.SignWithGenReq{
		TransferID:	"t-fin2", Chain: "BTC", To: "bc1q", Amount: "1", Asset: "BTC",
		NewFee:	"0.00010000", Actor: "a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := eng.BroadcastReplace(ctx, feebumpgate.BroadcastReq{
		GenID:	got.Gen.GenID, TransferID: "t-fin2", Actor: "a",
		BroadcastHash:	got.Gen.NewBroadcastHash,
	}); err != nil {
		t.Fatal(err)
	}
	fin, err := eng.Finalize(ctx, got.Gen.GenID, "a")
	if err != nil {
		t.Fatal(err)
	}
	if fin.State != feebumpgate.GenFinal {
		t.Fatalf("state=%s", fin.State)
	}
}

func TestWorkerScan(t *testing.T) {
	store := feebumpgate.NewMemStore()
	eng := feebumpgate.NewEngine(store)
	w := feebumpgate.NewWorker(eng,
		store, feebumpgate.NewMetrics(), feebumpgate.WorkerConfig{BatchSize: 8})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w.Run(ctx)
	if _, err := w.ScanOrphans(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := w.ScanExpired(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestDetectOrphanSignedWithoutRetire(t *testing.T) {
	store := feebumpgate.NewMemStore()
	eng := feebumpgate.NewEngine(store)
	ctx := context.Background()
	a, err := eng.SignFeeBumpWithGen(ctx, feebumpgate.SignWithGenReq{
		TransferID:	"t-orp", Chain: "BTC", To: "bc1q", Amount: "1", Asset: "BTC",
		NewFee:	"0.00010000", Actor: "a",
	})
	if err != nil {
		t.Fatal(err)
	}
	// 人工制造双 held：再插一条占用。
	now := time.Now().UTC()
	_ = store.SaveOcc(ctx, feebumpgate.BroadcastOccupancy{
		OccID:	"ghost", GenID: a.Gen.GenID + "-ghost", TransferID: "t-orp",
		Generation:	99, BroadcastHash: "x", OccupancyFP: "y", Asset: "BTC",
		Amount:	"1", Fee: "0.0002", State: feebumpgate.OccHeld, HeldAt: now, ExpireAt: now.Add(time.Hour),
		Actor:	"x", CreatedAt: now, UpdatedAt: now,
	})
	n, err := eng.DetectOrphans(ctx, 16)
	if err != nil || n < 1 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestTestdataPresent(t *testing.T) {
	dir := feebumpgate.TestdataDir()
	names := []string{
		"double_bump_retire.golden.json",
		"prev_hash_mismatch.golden.json",
		"retire_fail_orphan.golden.json",
		"old_gen_broadcast.golden.json",
		"no_gen_fail_closed.golden.json",
	}
	for _, n := range names {
		if _, err := feebumpgate.LoadGolden(n); err != nil {
			t.Fatalf("%s: %v (dir=%s)", n, err, dir)
		}
	}
	if filepath.Base(dir) != "feebumpgate" {
		t.Fatalf("dir=%s", dir)
	}
}

func TestAssertMaterialStale(t *testing.T) {
	eng := feebumpgate.NewEngine(feebumpgate.NewMemStore())
	ctx := context.Background()
	first, err := eng.SignFeeBumpWithGen(ctx, feebumpgate.SignWithGenReq{
		TransferID:	"t-mat", Chain: "BTC", To: "bc1q", Amount: "1", Asset: "BTC",
		NewFee:	"0.00010000", Actor: "a", DestinationTag: "m",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := eng.SignFeeBumpWithGen(ctx, feebumpgate.SignWithGenReq{
		TransferID:	"t-mat", Chain: "BTC", To: "bc1q", Amount: "1", Asset: "BTC",
		NewFee:	"0.00020000", Actor: "a", DestinationTag: "m",
		PrevBroadcastHash:	first.Gen.NewBroadcastHash,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = eng.AssertMaterial(ctx, first.Gen.GenID, feebumpgate.MaterialBind{TransferID: "t-mat", NewFee: first.Gen.NewFee})
	if !errors.Is(err, feebumpgate.ErrGenStale) {
		t.Fatalf("err=%v", err)
	}
	if err := eng.AssertMaterial(ctx, second.Gen.GenID, feebumpgate.MaterialBind{
		TransferID:	"t-mat", NewFee: second.Gen.NewFee, DestinationTag: "m",
	}); err != nil {
		t.Fatal(err)
	}
}

type failRetireStore struct {
	feebumpgate.Store

	failOn	string
}

func (s *failRetireStore) CASOcc(ctx context.Context, occID string, from, to feebumpgate.OccState, patch feebumpgate.BroadcastOccupancy,) error {
	if to == feebumpgate.OccRetired && s.failOn != "" {
		occ, err := s.Store.GetOcc(ctx, occID)
		if err == nil && occ.TransferID == s.failOn {
			return feebumpgate.ErrRetireFailed
		}
	}
	return s.Store.CASOcc(ctx, occID, from, to, patch)
}

func TestNewEngineNilStore(t *testing.T) {
	if feebumpgate.NewEngine(nil).Store() == nil {
		t.Fatal("expected mem store")
	}
}

func TestForceRetire(t *testing.T) {
	eng := feebumpgate.NewEngine(feebumpgate.NewMemStore())
	ctx := context.Background()
	got, err := eng.SignFeeBumpWithGen(ctx, feebumpgate.SignWithGenReq{
		TransferID:	"t-fr", Chain: "BTC", To: "bc1q", Amount: "1", Asset: "BTC",
		NewFee:	"0.00010000", Actor: "ops",
	})
	if err != nil {
		t.Fatal(err)
	}
	occ, err := eng.ForceRetire(ctx, feebumpgate.RetireReq{GenID: got.Gen.GenID, Actor: "ops", Force: true, Why: "ops"})
	if err != nil {
		t.Fatal(err)
	}
	if occ.State != feebumpgate.OccRetired {
		t.Fatalf("state=%s", occ.State)
	}
}
// temporary guard removed after the production check
