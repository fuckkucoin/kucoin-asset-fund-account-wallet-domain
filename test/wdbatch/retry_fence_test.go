package wdbatch_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/batchresiduefence"
	"account-assets.kucoin.net/funding/internal/wdbatch"
)

func TestServiceViewAndRetryFence(t *testing.T) {
	s := wdbatch.New()
	row, err := s.Open(wdbatch.OpenReq{
		Actor:	"ops-alice", IdempotencyKey: "open-rf-1",
		Items: []wdbatch.Item{{
			UserID:	"u-2001", Asset: "USDT", Amount: "12",
			Chain:	"ETH", ToAddress: "0x9858EfFD232B4033E47d90003D41EC34EcaEda94",
			IdempotencyKey:	"item-rf-1",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.rows[row.ID].Items[0].Status = wdbatch.ItemFailed
	s.mu.Unlock()
	view, err := s.ViewItem(context.Background(), row.ID, "item-rf-1")
	if err != nil || view.Phase != batchresiduefence.PhaseFailed {
		t.Fatalf("%v %#v", err, view)
	}
	f, err := s.IssueResidueFence(context.Background(), batchresiduefence.IssueFenceReq{
		BatchID:	row.ID, ItemIdem: "item-rf-1", Actor: "ops-alice",
		UserID:	"u-2001", Asset: "USDT", Amount: "12",
	})
	if err != nil {
		t.Fatal(err)
	}
	resp, err := s.RetryWithFence(context.Background(), batchresiduefence.RetryWithFenceReq{
		BatchID:	row.ID, ItemIdem: "item-rf-1", FenceID: f.ID, Actor: "ops-alice",
	})
	if err != nil || !resp.Allowed {
		t.Fatalf("%v %#v", err, resp)
	}
}

func TestServiceSubmittedViewRejects(t *testing.T) {
	s := wdbatch.New()
	row, err := s.Open(wdbatch.OpenReq{
		Actor:	"ops-bob", IdempotencyKey: "open-rf-2",
		Items: []wdbatch.Item{{
			UserID:	"u-2002", Asset: "USDT", Amount: "1",
			Chain:	"ETH", ToAddress: "0x9858EfFD232B4033E47d90003D41EC34EcaEda94",
			IdempotencyKey:	"item-rf-2",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	s.mu.Lock()
	s.rows[row.ID].Items[0].Status = wdbatch.ItemSubmitted
	s.mu.Unlock()
	_, err = s.RetryWithFence(context.Background(), batchresiduefence.RetryWithFenceReq{
		BatchID:	row.ID, ItemIdem: "item-rf-2", FenceID: "any", Actor: "ops-bob",
	})
	if err == nil {
		t.Fatal("submitted must reject")
	}
}
