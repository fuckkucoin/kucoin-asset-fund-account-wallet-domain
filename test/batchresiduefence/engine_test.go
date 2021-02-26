package batchresiduefence_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"account-assets.kucoin.net/funding/internal/batchresiduefence"
)

func TestIssueAndRetryHappy(t *testing.T) {
	v := batchresiduefence.NewStaticViewer()
	putFailed(v, "b-ok", "i-ok", "u-1", "USDT", "8")
	e := testEng(t, v)
	f := mustIssue(t, e, "b-ok", "i-ok", "ops-a", "u-1", "USDT", "8")
	resp, err := e.RetryWithFence(context.Background(), batchresiduefence.RetryWithFenceReq{
		BatchID:	"b-ok", ItemIdem: "i-ok", FenceID: f.ID, Actor: "ops-a", Generation: f.Generation,
	})
	if err != nil || !resp.Allowed {
		t.Fatalf("retry: %v %#v", err, resp)
	}
	if resp.Fence.State != batchresiduefence.FenceFrozen {
		t.Fatalf("state %s", resp.Fence.State)
	}
}

func TestNoFenceFailClosed(t *testing.T) {
	v := batchresiduefence.NewStaticViewer()
	putFailed(v, "b-nf", "i-nf", "u-1", "USDT", "1")
	e := testEng(t, v)
	_, err := e.RetryWithFence(context.Background(), batchresiduefence.RetryWithFenceReq{BatchID: "b-nf", ItemIdem: "i-nf", Actor: "ops-a"})
	if !errors.Is(err, batchresiduefence.ErrFailClosed) {
		t.Fatalf("got %v", err)
	}
}

func TestSubmittedForbidsRetry(t *testing.T) {
	v := batchresiduefence.NewStaticViewer()
	v.Put(batchresiduefence.BatchItemView{BatchID: "b-s", ItemIdem: "i-s", Phase: batchresiduefence.PhaseSubmitted, UserID: "u-1", Asset: "BTC", Amount: "1"})
	e := testEng(t, v)
	_, err := e.RetryWithFence(context.Background(), batchresiduefence.RetryWithFenceReq{BatchID: "b-s", ItemIdem: "i-s", FenceID: "x", Actor: "ops-a"})
	if !errors.Is(err, batchresiduefence.ErrItemSubmitted) {
		t.Fatalf("got %v", err)
	}
}

func TestSkippedForbidsRetry(t *testing.T) {
	v := batchresiduefence.NewStaticViewer()
	v.Put(batchresiduefence.BatchItemView{BatchID: "b-k", ItemIdem: "i-k", Phase: batchresiduefence.PhaseSkipped, UserID: "u-1", Asset: "BTC", Amount: "1"})
	e := testEng(t, v)
	_, err := e.RetryWithFence(context.Background(), batchresiduefence.RetryWithFenceReq{BatchID: "b-k", ItemIdem: "i-k", FenceID: "x", Actor: "ops-a"})
	if !errors.Is(err, batchresiduefence.ErrItemSkipped) {
		t.Fatalf("got %v", err)
	}
}

func TestConcurrentRetryDupProof(t *testing.T) {
	v := batchresiduefence.NewStaticViewer()
	putFailed(v, "b-c", "i-c", "u-9", "ETH", "3")
	e := testEng(t, v)
	f := mustIssue(t, e, "b-c", "i-c", "ops-a", "u-9", "ETH", "3")
	_, err := e.RetryWithFence(context.Background(), batchresiduefence.RetryWithFenceReq{
		BatchID:	"b-c", ItemIdem: "i-c", FenceID: f.ID, Actor: "ops-a", AttemptID: "a1",
	})
	if err != nil {
		t.Fatal(err)
	}
	f2, err := e.IssueFence(context.Background(), batchresiduefence.IssueFenceReq{
		BatchID:	"b-c", ItemIdem: "i-c", Actor: "ops-b", UserID: "u-9", Asset: "ETH", Amount: "3",
	})
	if err != nil {
		t.Fatal(err)
	}
	// 已有 live 围栏时 Issue 复用同一条，第二腿必须换一条伪造占用冲突：另签发后强行换槽
	if f2.ID == f.ID {
		// 直接第二腿同一围栏会复用槽成功；用独立围栏并发第二签发
		store := e.store
		other := batchresiduefence.ResidueFence{
			ID:	"fence-other", BatchID: "b-c", ItemIdem: "i-c",
			FreezeFingerprint:	f.FreezeFingerprint, Generation: 2,
			TTL:		f.TTL,
			IssuedAt:	f.IssuedAt, ExpireAt: f.ExpireAt,
			State:	batchresiduefence.FenceIssued, Actor: "ops-b", UserID: "u-9", Asset: "ETH", Amount: "3",
		}
		if err := store.SaveFence(context.Background(), other); err != nil {
			t.Fatal(err)
		}
		resp, err := e.RetryWithFence(context.Background(), batchresiduefence.RetryWithFenceReq{
			BatchID:	"b-c", ItemIdem: "i-c", FenceID: other.ID, Actor: "ops-b", AttemptID: "a2",
		})
		if !errors.Is(err, batchresiduefence.ErrDupFreeze) || resp.DupProof == nil {
			t.Fatalf("want dup proof got %v %#v", err, resp)
		}
	}
}

func TestHalfCutResumeNoSecondFull(t *testing.T) {
	v := batchresiduefence.NewStaticViewer()
	putFailed(v, "b-h", "i-h", "u-3", "ETH", "2.25")
	e := testEng(t, v)
	sa, err := e.SubmitAssert(context.Background(), batchresiduefence.SubmitAssertReq{
		BatchID:	"b-h", ItemIdem: "i-h", Actor: "ops-a", PhaseAfter: batchresiduefence.PhaseFailed,
		HalfCut:	true, CutReason: "interrupt", FreezeRef: "frz-1",
		UserID:	"u-3", Asset: "ETH", Amount: "2.25",
	})
	if err != nil || sa.Fence == nil || sa.Half == nil {
		t.Fatalf("half: %v %#v", err, sa)
	}
	resp, err := e.RetryWithFence(context.Background(), batchresiduefence.RetryWithFenceReq{
		BatchID:	"b-h", ItemIdem: "i-h", FenceID: sa.Fence.ID, Actor: "ops-b", ResumeOnly: true,
	})
	if err != nil || !resp.Allowed || !resp.ResumeOnly {
		t.Fatalf("resume: %v %#v", err, resp)
	}
	if resp.Half == nil || resp.Half.State != batchresiduefence.HalfResumed {
		t.Fatalf("half %#v", resp.Half)
	}
	ex, err := e.ExportDupProofs(context.Background(), "b-h")
	if err != nil {
		t.Fatal(err)
	}
	if ex.Count != 0 {
		t.Fatalf("no dup expected, got %d", ex.Count)
	}
}

func TestGenerationMismatch(t *testing.T) {
	v := batchresiduefence.NewStaticViewer()
	putFailed(v, "b-g", "i-g", "u-1", "USDT", "1")
	e := testEng(t, v)
	f := mustIssue(t, e, "b-g", "i-g", "ops-a", "u-1", "USDT", "1")
	_, err := e.RetryWithFence(context.Background(), batchresiduefence.RetryWithFenceReq{
		BatchID:	"b-g", ItemIdem: "i-g", FenceID: f.ID, Actor: "ops-a", Generation: f.Generation + 3,
	})
	if !errors.Is(err, batchresiduefence.ErrGeneration) {
		t.Fatalf("got %v", err)
	}
}

func TestReleaseThenRetry(t *testing.T) {
	v := batchresiduefence.NewStaticViewer()
	putFailed(v, "b-r", "i-r", "u-1", "USDT", "1")
	e := testEng(t, v)
	f := mustIssue(t, e, "b-r", "i-r", "ops-a", "u-1", "USDT", "1")
	if _, err := e.ReleaseFence(context.Background(), f.ID, "ops-a"); err != nil {
		t.Fatal(err)
	}
	_, err := e.RetryWithFence(context.Background(), batchresiduefence.RetryWithFenceReq{
		BatchID:	"b-r", ItemIdem: "i-r", FenceID: f.ID, Actor: "ops-a",
	})
	if !errors.Is(err, batchresiduefence.ErrFenceReleased) {
		t.Fatalf("got %v", err)
	}
}

func TestParallelFirstFreeze(t *testing.T) {
	v := batchresiduefence.NewStaticViewer()
	putFailed(v, "b-p", "i-p", "u-1", "USDT", "4")
	e := testEng(t, v)
	f := mustIssue(t, e, "b-p", "i-p", "ops-a", "u-1", "USDT", "4")
	var wg sync.WaitGroup
	errc := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_, err := e.RetryWithFence(context.Background(), batchresiduefence.RetryWithFenceReq{
				BatchID:	"b-p", ItemIdem: "i-p", FenceID: f.ID, Actor: "ops-a", AttemptID: "p",
			})
			errc <- err
		}(i)
	}
	wg.Wait()
	close(errc)
	ok := 0
	for err := range errc {
		if err == nil {
			ok++
		}
	}
	if ok == 0 {
		t.Fatal("at least one freeze must succeed")
	}
}

func TestSQLStatementsNonEmpty(t *testing.T) {
	d := batchresiduefence.DefaultSQLDialect()
	for _, s := range d.AllStatements() {
		if s == "" {
			t.Fatal("empty sql")
		}
	}
}

func TestCatalogComplete(t *testing.T) {
	if len(batchresiduefence.ReasonCatalog()) < 10 {
		t.Fatal("catalog thin")
	}
	if len(batchresiduefence.AuditCatalog()) < 8 {
		t.Fatal("audit thin")
	}
	if _, ok := batchresiduefence.FindReason(batchresiduefence.ReasonDupFreeze); !ok {
		t.Fatal("missing dup")
	}
}

func TestSnapshotSummary(t *testing.T) {
	v := batchresiduefence.NewStaticViewer()
	putFailed(v, "b-sum", "i-sum", "u-1", "USDT", "1")
	e := testEng(t, v)
	mustIssue(t, e, "b-sum", "i-sum", "ops-a", "u-1", "USDT", "1")
	sum, err := e.Summary(context.Background(), "b-sum")
	if err != nil || sum.LiveFences != 1 {
		t.Fatalf("%v %#v", err, sum)
	}
	if _, err := e.Snapshot(context.Background(), "b-sum"); err != nil {
		t.Fatal(err)
	}
}
