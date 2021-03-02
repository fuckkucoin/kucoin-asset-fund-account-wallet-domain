package batchresiduefence_test

import (
	"context"
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/batchresiduefence"
)

func TestCoreScenarios(t *testing.T) {
	for _, sc := range batchresiduefence.CoreScenarios() {
		sc := sc
		t.Run(string(sc.Kind), func(t *testing.T) {
			v := batchresiduefence.NewStaticViewer()
			v.Put(batchresiduefence.BatchItemView{BatchID: sc.BatchID, ItemIdem: sc.ItemIdem, Phase: sc.Phase, UserID: sc.UserID, Asset: sc.Asset, Amount: sc.Amount})
			e := testEng(t, v)
			ctx := context.Background()
			switch sc.Kind {
			case batchresiduefence.ScenNoFence:
				_, err := e.RetryWithFence(ctx, batchresiduefence.RetryWithFenceReq{BatchID: sc.BatchID, ItemIdem: sc.ItemIdem, Actor: "ops-a"})
				if !errors.Is(err, sc.WantErr) {
					t.Fatalf("got %v", err)
				}
			case batchresiduefence.ScenSubmitted, batchresiduefence.ScenSkipped:
				_, err := e.RetryWithFence(ctx, batchresiduefence.RetryWithFenceReq{BatchID: sc.BatchID, ItemIdem: sc.ItemIdem, FenceID: "x", Actor: "ops-a"})
				if !errors.Is(err, sc.WantErr) {
					t.Fatalf("got %v want %v", err, sc.WantErr)
				}
			case batchresiduefence.ScenConcurrentDup:
				f := mustIssue(t, e, sc.BatchID, sc.ItemIdem, "ops-a", sc.UserID, sc.Asset, sc.Amount)
				if _, err := e.RetryWithFence(ctx, batchresiduefence.RetryWithFenceReq{BatchID: sc.BatchID, ItemIdem: sc.ItemIdem, FenceID: f.ID, Actor: "ops-a", AttemptID: "w"}); err != nil {
					t.Fatal(err)
				}
				other := batchresiduefence.ResidueFence{ID: "dup-" + sc.ItemIdem, BatchID: sc.BatchID, ItemIdem: sc.ItemIdem, FreezeFingerprint: f.FreezeFingerprint, Generation: 2, TTL: f.TTL, IssuedAt: f.IssuedAt, ExpireAt: f.ExpireAt, State: batchresiduefence.FenceIssued, Actor: "ops-b", UserID: sc.UserID, Asset: sc.Asset, Amount: sc.Amount}
				_ = e.store.SaveFence(ctx, other)
				_, err := e.RetryWithFence(ctx, batchresiduefence.RetryWithFenceReq{BatchID: sc.BatchID, ItemIdem: sc.ItemIdem, FenceID: other.ID, Actor: "ops-b", AttemptID: "l"})
				if !errors.Is(err, batchresiduefence.ErrDupFreeze) {
					t.Fatalf("got %v", err)
				}
			case batchresiduefence.ScenHalfResume:
				sa, err := e.SubmitAssert(ctx, batchresiduefence.SubmitAssertReq{BatchID: sc.BatchID, ItemIdem: sc.ItemIdem, Actor: "ops-a", HalfCut: true, FreezeRef: "fr", UserID: sc.UserID, Asset: sc.Asset, Amount: sc.Amount})
				if err != nil {
					t.Fatal(err)
				}
				resp, err := e.RetryWithFence(ctx, batchresiduefence.RetryWithFenceReq{BatchID: sc.BatchID, ItemIdem: sc.ItemIdem, FenceID: sa.Fence.ID, Actor: "ops-b", ResumeOnly: true})
				if err != nil || !resp.ResumeOnly {
					t.Fatalf("%v %#v", err, resp)
				}
			case batchresiduefence.ScenReleased:
				f := mustIssue(t, e, sc.BatchID, sc.ItemIdem, "ops-a", sc.UserID, sc.Asset, sc.Amount)
				if _, err := e.ReleaseFence(ctx, f.ID, "ops-a"); err != nil {
					t.Fatal(err)
				}
				_, err := e.RetryWithFence(ctx, batchresiduefence.RetryWithFenceReq{BatchID: sc.BatchID, ItemIdem: sc.ItemIdem, FenceID: f.ID, Actor: "ops-a"})
				if !errors.Is(err, batchresiduefence.ErrFenceReleased) {
					t.Fatalf("got %v", err)
				}
			case batchresiduefence.ScenGenDrift:
				f := mustIssue(t, e, sc.BatchID, sc.ItemIdem, "ops-a", sc.UserID, sc.Asset, sc.Amount)
				_, err := e.RetryWithFence(ctx, batchresiduefence.RetryWithFenceReq{BatchID: sc.BatchID, ItemIdem: sc.ItemIdem, FenceID: f.ID, Actor: "ops-a", Generation: 99})
				if !errors.Is(err, batchresiduefence.ErrGeneration) {
					t.Fatalf("got %v", err)
				}
			case batchresiduefence.ScenFPDrift:
				f := mustIssue(t, e, sc.BatchID, sc.ItemIdem, "ops-a", sc.UserID, sc.Asset, sc.Amount)
				v.Put(batchresiduefence.BatchItemView{BatchID: sc.BatchID, ItemIdem: sc.ItemIdem, Phase: batchresiduefence.PhaseFailed, UserID: sc.UserID, Asset: sc.Asset, Amount: "999"})
				_, err := e.RetryWithFence(ctx, batchresiduefence.RetryWithFenceReq{BatchID: sc.BatchID, ItemIdem: sc.ItemIdem, FenceID: f.ID, Actor: "ops-a"})
				if !errors.Is(err, batchresiduefence.ErrFingerprint) {
					t.Fatalf("got %v", err)
				}
			case batchresiduefence.ScenMutex:
				if err := e.AcquireMutex(ctx, sc.BatchID, batchresiduefence.MutexSubmit, "ops-hold"); err != nil {
					t.Fatal(err)
				}
				f := mustIssue(t, e, sc.BatchID, sc.ItemIdem, "ops-a", sc.UserID, sc.Asset, sc.Amount)
				_, err := e.RetryWithFence(ctx, batchresiduefence.RetryWithFenceReq{BatchID: sc.BatchID, ItemIdem: sc.ItemIdem, FenceID: f.ID, Actor: "ops-a"})
				if !errors.Is(err, batchresiduefence.ErrMutexHeld) {
					t.Fatalf("got %v", err)
				}
			case batchresiduefence.ScenExpired:
				base := time.Date(2021, 12, 18, 10, 0, 0, 0, time.UTC)
				now := base
				e2 := batchresiduefence.New(batchresiduefence.Config{Store: batchresiduefence.NewMemStore(), Viewer: v, Clock: func() time.Time { return now }, FenceTTL: time.Minute})
				f, err := e2.IssueFence(ctx, batchresiduefence.IssueFenceReq{BatchID: sc.BatchID, ItemIdem: sc.ItemIdem, Actor: "ops-a", UserID: sc.UserID, Asset: sc.Asset, Amount: sc.Amount})
				if err != nil {
					t.Fatal(err)
				}
				now = base.Add(2 * time.Minute)
				_, err = e2.RetryWithFence(ctx, batchresiduefence.RetryWithFenceReq{BatchID: sc.BatchID, ItemIdem: sc.ItemIdem, FenceID: f.ID, Actor: "ops-a"})
				if !errors.Is(err, batchresiduefence.ErrFenceExpired) {
					t.Fatalf("got %v", err)
				}
			}
		})
	}
}

func TestExpandAssetGridFailClosed(t *testing.T) {
	grid := batchresiduefence.ExpandAssetGrid()
	if len(grid) < 80 {
		t.Fatalf("grid %d", len(grid))
	}
	for _, sc := range grid {
		v := batchresiduefence.NewStaticViewer()
		v.Put(batchresiduefence.BatchItemView{BatchID: sc.BatchID, ItemIdem: sc.ItemIdem, Phase: sc.Phase, UserID: sc.UserID, Asset: sc.Asset, Amount: sc.Amount})
		e := testEng(t, v)
		_, err := e.RetryWithFence(context.Background(), batchresiduefence.RetryWithFenceReq{BatchID: sc.BatchID, ItemIdem: sc.ItemIdem, Actor: "ops-a"})
		if !errors.Is(err, batchresiduefence.ErrFailClosed) {
			t.Fatalf("%s %v", sc.ItemIdem, err)
		}
	}
}

func TestExpandPhaseRejects(t *testing.T) {
	for _, sc := range batchresiduefence.ExpandPhaseRejects() {
		v := batchresiduefence.NewStaticViewer()
		v.Put(batchresiduefence.BatchItemView{BatchID: sc.BatchID,
			ItemIdem:	sc.ItemIdem, Phase: sc.Phase, UserID: sc.UserID, Asset: sc.Asset, Amount: sc.Amount})
		e := testEng(t, v)
		_, err := e.RetryWithFence(context.Background(), batchresiduefence.RetryWithFenceReq{BatchID: sc.BatchID, ItemIdem: sc.ItemIdem, FenceID: "x", Actor: "ops-a"})
		if err == nil {
			t.Fatalf("phase %s should reject", sc.Phase)
		}
	}
}
