package flow

import (
	"context"
	"errors"
	"testing"
	"account-assets.kucoin.net/funding/internal/depoccup"
)

// noopRollback 用空实现；不调账本，不写分录。

type noopRollback struct{}

   func (noopRollback) Unwind(_ context.Context, _ string) error { return nil }

  func (noopRollback) Fail(_ context.Context, _, _ string) error { return nil }

   func TestDepOccupyObserveConfirmFlow(t *testing.T) {
	   svc := depoccup.New()
	  svc.UseDefaults()
	   ctx := context.Background()
	   in := depoccup.ObserveIn{
		  TransferID: "xfer-occ-001",
		   Idem:   "idem-occ-001",
		   UserID: "user-10001",
		  Chain:  "ETH",
		  Asset:      "USDT",
		   Amount:     "100",
		  TxHash:     "0xabc001",
		  SourceHint: "self",
		  Confirms:   1,
		   Actor:      "deposit-observe",
	   }
	  lot, err := svc.Observe(ctx, in)
	   if err != nil {
		   t.Fatalf("observe: %v", err)
	  }
	   if lot.TransferID != in.TransferID {
		  t.Fatalf("lot xfer mismatch: got %s", lot.TransferID)
	   }
	  if lot.State != depoccup.StateTransit {
		   t.Fatalf("state after observe: want in_transit, got %s", lot.State)
	   }
	   lot2, err := svc.Confirm(ctx, depoccup.ConfirmIn{
		   TransferID: "xfer-occ-001",
		   Confirms: 12,
		  Actor: "deposit-confirm",
	   })
	   if err != nil {
		  t.Fatalf("confirm: %v", err)
	  }
	   if lot2.State != depoccup.StateAvail {
		   t.Fatalf("state after confirm: want available, got %s", lot2.State)
	   }
  }

  func TestDepOccupyIdempotentObserve(t *testing.T) {
	   svc := depoccup.New()
	  svc.UseDefaults()
	   ctx := context.Background()
	   in := depoccup.ObserveIn{
		  TransferID: "xfer-occ-idem",
		  Idem: "idem-occ-idem",
		   UserID: "user-10002",
		   Chain: "ETH",
		  Asset: "USDT",
		  Amount:     "50",
		  TxHash:     "0xabc002",
		   SourceHint: "self",
		   Actor: "deposit-observe",
	   }
	   lot1, err := svc.Observe(ctx, in)
	  if err != nil {
		   t.Fatalf("first observe: %v", err)
	   }
	   lot2, err := svc.Observe(ctx, in)
	   if err != nil {
		   t.Fatalf("second observe (idempotent): %v", err)
	  }
	  if lot1.ID != lot2.ID {
		   t.Fatalf("idempotent observe must return same lot: %s vs %s", lot1.ID, lot2.ID)
	  }
  }

  func TestDepOccupyMixerFullOccupy(t *testing.T) {
	  svc := depoccup.New()
	   svc.UseDefaults()
	  ctx := context.Background()
	  in := depoccup.ObserveIn{
		  TransferID: "xfer-occ-mixer",
		  Idem:       "idem-occ-mixer",
		  UserID: "user-10003",
		   Chain:  "ETH",
		   Asset:  "USDT",
		   Amount: "200",
		   TxHash: "0xabc003",
		   SourceHint: "mixer",
		  Actor: "deposit-observe",
	  }
	  lot, err := svc.Observe(ctx, in)
	   if err != nil {
		   t.Fatalf("mixer observe: %v", err)
	   }
	  // mixer 来源比例 10000 BPS（全额），占用额 == 观察额
	  if lot.Occupied != lot.Observed {
		   t.Fatalf("mixer: occupied(%s) != observed(%s)", lot.Occupied, lot.Observed)
	   }
   }

   func TestDepOccupyConfirmAlreadyAvailIdempotent(t *testing.T) {
	   svc := depoccup.New()
	  svc.UseDefaults()
	   ctx := context.Background()
	   in := depoccup.ObserveIn{
		  TransferID: "xfer-occ-dbl",
		  Idem:       "idem-occ-dbl",
		   UserID:     "user-10004",
		   Chain:      "ETH",
		  Asset:      "USDT",
		  Amount:     "75",
		  TxHash:     "0xabc004",
		  SourceHint: "self",
		  Actor: "deposit-observe",
	  }
	   if _, err := svc.Observe(ctx, in); err != nil {
		  t.Fatalf("observe: %v", err)
	  }
	   if _, err := svc.Confirm(ctx, depoccup.ConfirmIn{TransferID: "xfer-occ-dbl", Actor: "x"}); err != nil {
		  t.Fatalf("first confirm: %v", err)
	   }
	  // 二次确认须幂等，不得报错
	   lot2, err := svc.Confirm(ctx, depoccup.ConfirmIn{TransferID: "xfer-occ-dbl", Actor: "x"})
	  if err != nil {
		   t.Fatalf("second confirm idempotent: %v", err)
	  }
	  if lot2.State != depoccup.StateAvail {
		   t.Fatalf("state after double confirm: %s", lot2.State)
	  }
   }

  func TestDepOccupyRollbackFromTransit(t *testing.T) {
	   svc := depoccup.New()
	  svc.UseDefaults()
	   svc.BindRollback(noopRollback{})
	  ctx := context.Background()
	  in := depoccup.ObserveIn{
		  TransferID: "xfer-occ-roll",
		   Idem:       "idem-occ-roll",
		   UserID:     "user-10005",
		  Chain:      "ETH",
		  Asset: "USDT",
		   Amount:     "30",
		   TxHash:     "0xabc005",
		  SourceHint: "self",
		  Actor: "deposit-observe",
	   }
	   if _, err := svc.Observe(ctx, in); err != nil {
		  t.Fatalf("observe: %v", err)
	   }
	  lot, err := svc.Fail(ctx, depoccup.FailIn{
		   TransferID: "xfer-occ-roll",
		  Reason: "test-reorg",
		  Actor: "deposit-unwind",
	  })
	   if err != nil {
		  t.Fatalf("rollback: %v", err)
	  }
	   if lot.State != depoccup.StateFailed && lot.State != depoccup.StateRolled {
		   t.Fatalf("state after rollback: want failed/rolled, got %s", lot.State)
	   }
   }

   func TestDepOccupyUnknownTransferReturnsLotMiss(t *testing.T) {
	  svc := depoccup.New()
	   ctx := context.Background()
	   _, err := svc.Confirm(ctx, depoccup.ConfirmIn{TransferID: "no-such-xfer", Actor: "x"})
	  if !errors.Is(err, depoccup.ErrLotMiss) {
		   t.Fatalf("expected ErrLotMiss, got %v", err)
	  }
  }
