package batchresiduefence_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/batchresiduefence"
)

func TestPatrolExpiresFence(t *testing.T) {
	v := batchresiduefence.NewStaticViewer()
	putFailed(v, "b-e", "i-e", "u-1", "USDT", "1")
	base := time.Date(2021, 12, 18, 10, 0, 0, 0, time.UTC)
	now := base
	e := batchresiduefence.New(batchresiduefence.Config{
		Store:	batchresiduefence.NewMemStore(), Viewer: v,
		Clock:		func() time.Time { return now },
		FenceTTL:	1 * time.Minute, HalfTTL: 1 * time.Minute,
	})
	mustIssue(t, e, "b-e", "i-e", "ops-a", "u-1", "USDT", "1")
	now = base.Add(2 * time.Minute)
	rep, err := e.PatrolResidue(context.Background(), 50)
	if err != nil {
		t.Fatal(err)
	}
	if rep.ExpiredFences != 1 {
		t.Fatalf("expired %d", rep.ExpiredFences)
	}
}

func TestPatrolAbandonHalf(t *testing.T) {
	v := batchresiduefence.NewStaticViewer()
	putFailed(v, "b-ah", "i-ah", "u-1", "USDT", "1")
	base := time.Date(2021, 12, 18, 10, 0, 0, 0, time.UTC)
	now := base
	e := batchresiduefence.New(batchresiduefence.Config{
		Store:	batchresiduefence.NewMemStore(), Viewer: v,
		Clock:		func() time.Time { return now },
		FenceTTL:	30 * time.Minute, HalfTTL: 1 * time.Minute,
	})
	_, err := e.SubmitAssert(context.Background(), batchresiduefence.SubmitAssertReq{
		BatchID:	"b-ah", ItemIdem: "i-ah", Actor: "ops-a", HalfCut: true,
		CutReason:	"cut", FreezeRef: "fr", UserID: "u-1", Asset: "USDT", Amount: "1",
	})
	if err != nil {
		t.Fatal(err)
	}
	now = base.Add(2 * time.Minute)
	rep, err := e.PatrolResidue(context.Background(), 20)
	if err != nil {
		t.Fatal(err)
	}
	if rep.AbandonedHalf != 1 {
		t.Fatalf("abandon %d", rep.AbandonedHalf)
	}
}

func TestExportUnexported(t *testing.T) {
	v := batchresiduefence.NewStaticViewer()
	putFailed(v, "b-x", "i-x", "u-1", "USDT", "1")
	e := testEng(t, v)
	f := mustIssue(t, e, "b-x", "i-x", "ops-a", "u-1", "USDT", "1")
	_, _ = e.RetryWithFence(context.Background(), batchresiduefence.RetryWithFenceReq{
		BatchID:	"b-x", ItemIdem: "i-x", FenceID: f.ID, Actor: "ops-a", AttemptID: "w",
	})
	other := batchresiduefence.ResidueFence{
		ID:	"other-x", BatchID: "b-x", ItemIdem: "i-x",
		FreezeFingerprint:	f.FreezeFingerprint, Generation: 9,
		TTL:		f.TTL,
		IssuedAt:	f.IssuedAt, ExpireAt: f.ExpireAt,
		State:	batchresiduefence.FenceIssued, Actor: "ops-b", UserID: "u-1", Asset: "USDT", Amount: "1",
	}
	_ = e.store.SaveFence(context.Background(), other)
	_, _ = e.RetryWithFence(context.Background(), batchresiduefence.RetryWithFenceReq{
		BatchID:	"b-x", ItemIdem: "i-x", FenceID: other.ID, Actor: "ops-b", AttemptID: "l",
	})
	n,
		err := e.ExportUnexported(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if n < 1 {
		t.Fatalf("exported %d", n)
	}
}

func TestAbandonHalfManual(t *testing.T) {
	v := batchresiduefence.NewStaticViewer()
	putFailed(v, "b-m", "i-m", "u-1", "USDT", "1")
	e := testEng(t, v)
	_, err := e.SubmitAssert(context.Background(), batchresiduefence.SubmitAssertReq{
		BatchID:	"b-m", ItemIdem: "i-m", Actor: "ops-a", HalfCut: true,
		UserID:	"u-1", Asset: "USDT", Amount: "1", FreezeRef: "r",
	})
	if err != nil {
		t.Fatal(err)
	}
	h, err := e.AbandonHalf(context.Background(), "b-m", "i-m", "ops-a")
	if err != nil || h.State != batchresiduefence.HalfAbandoned {
		t.Fatalf("%v %#v", err, h)
	}
}

func TestMemStoreRoundtrip(t *testing.T) {
	m := batchresiduefence.NewMemStore()
	ctx := context.Background()
	if err := m.SaveFence(ctx, batchresiduefence.ResidueFence{ID: "f1", BatchID: "b", ItemIdem: "i", State: batchresiduefence.FenceIssued}); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := m.GetFence(ctx, "f1"); !ok {
		t.Fatal("missing")
	}
	if rows, _ := m.ListFencesByItem(ctx, "b", "i"); len(rows) != 1 {
		t.Fatal(rows)
	}
	if rows, _ := m.ListLiveFences(ctx, 1); len(rows) != 1 {
		t.Fatal(rows)
	}
	_ = m.SaveProof(ctx, batchresiduefence.BatchDupFreezeProof{ID: "p1", BatchID: "b"})
	_ = m.SaveHalf(ctx, batchresiduefence.HalfSubmitCase{ID: "h1", BatchID: "b", ItemIdem: "i", State: batchresiduefence.HalfOpen})
	if _, ok, _ := m.GetOpenHalf(ctx, "b", "i"); !ok {
		t.Fatal("half")
	}
	_ = m.SaveSlot(ctx, batchresiduefence.FreezeSlot{ItemIdem: "i", Status: batchresiduefence.FreezeSlotActive})
	_ = m.SaveMutex(ctx, batchresiduefence.MutexHold{BatchID: "b", Kind: batchresiduefence.MutexSubmit})
	_ = m.SaveAudit(ctx, batchresiduefence.AuditEvent{ID: "a", BatchID: "b"})
	_, _ = m.ListProofs(ctx, "b")
	_, _ = m.ListAudits(ctx, "b", 10)
	_ = m.DeleteMutex(ctx, "b")
}
