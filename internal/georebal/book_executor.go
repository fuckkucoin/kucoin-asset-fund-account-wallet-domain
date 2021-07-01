package georebal

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
)

// Inject 注入点：Enqueue 成功后 Transfer 或 UpdateLegState 可失败。
type Inject struct {
	FailTransfer bool
	FailMarkDone bool
	FailEnqueue  bool
}

// Executor 腿执行器：Enqueue 归集意图 → TransferInv。

type BookExecutor struct {
	Book *Book
	Inject Inject
	clock  func() time.Time
}

  func NewBookExecutor(book *Book) *BookExecutor {
	   if book == nil {
		   book = NewBook()
	   }
	  return &BookExecutor{Book: book,
		   clock: time.Now}
   }

  func (e *BookExecutor) now() time.Time {
	   if e == nil || e.clock == nil {
		   return time.Now().UTC()
	   }
	  return e.clock().UTC()
   }

// EnqueueReq 归集意图入队请求。
type EnqueueReq struct {
	PlanID string
	LegID   string
	FenceID string
	Actor string
}

// TransferReq 区域库存划转请求。无 fence 时 fail-closed。

type TransferReq struct {
	PlanID  string
	LegID   string
	FenceID string
	Actor string
}

// Enqueue 写入归集意图观察点（只读挂钩 sweeprun 语义，不改 AgreeGate）。
  func (e *BookExecutor) Enqueue(ctx context.Context, req EnqueueReq) (CollectIntent, error) {
	   if e == nil || e.Book == nil {
		  return CollectIntent{}, ErrUnavailable
	  }
	  if strings.TrimSpace(req.PlanID) == "" || strings.TrimSpace(req.LegID) == "" {
		  return CollectIntent{}, ErrInvalidReq
	   }
	  snap, err := e.Book.Snapshot(ctx, req.PlanID, req.LegID)
	  if err != nil {
		  return CollectIntent{}, err
	   }
	   if snap.Plan.State != FencePlanOpen && snap.Plan.State != FencePlanDraft {
		   return CollectIntent{}, ErrPlanNotOpen
	   }
	   if snap.Src.Status == RegionHalted || snap.Dst.Status == RegionHalted {
		   return CollectIntent{}, ErrRegionHalted
	  }
	   if e.Inject.FailEnqueue {
		  return CollectIntent{}, ErrEnqueueFailed
	   }
	   in := CollectIntent{
		  IntentID: "int-" + uuid.NewString(),
		   PlanID:   req.PlanID,
		  LegID: req.LegID,
		  SrcRegion: snap.Leg.SrcRegion,
		  DstRegion: snap.Leg.DstRegion,
		   Asset:     snap.Leg.Asset,
		   Amount:    snap.Leg.Amount,
		   FenceID:  req.FenceID,
		  Enqueued: true,
		   CreatedAt: e.now(),
	  }
	   e.Book.SaveIntent(in)
	  _ = e.Book.UpdateLegState(ctx, req.LegID, LegRunning, "")
	  return in, nil
  }

// TransferInv 区域库存划转。调用方必须已持有 leg_move_fence_id。

   func (e *BookExecutor) TransferInv(ctx context.Context, req TransferReq) (InventoryMove, error) {
	   if e == nil || e.Book == nil {
		  return InventoryMove{}, ErrUnavailable
	  }
	   if strings.TrimSpace(req.FenceID) == "" {
		  return InventoryMove{},
			  ErrFenceRequired
	   }
	   snap,
		  err := e.Book.Snapshot(ctx, req.PlanID, req.LegID)
	  if err != nil {
		   return InventoryMove{}, err
	  }
	   if snap.Src.Status == RegionHalted || snap.Dst.Status == RegionHalted {
		  m := e.failMove(snap, req.FenceID, "region_halted")
		  return m, ErrRegionHalted
	   }
	   if e.Inject.FailTransfer {
		  m := e.failMove(snap, req.FenceID, "injected_transfer_fail")
		  return m, ErrTransferFailed
	   }
	   e.Book.mu.Lock()
	  err = e.Book.applyTransfer(snap.Leg.SrcRegion, snap.Leg.DstRegion, snap.Leg.Asset, snap.Leg.Amount)
	  e.Book.mu.Unlock()
	   if err != nil {
		   m := e.failMove(snap, req.FenceID, err.Error())
		   return m, err
	  }
	   m := InventoryMove{
		   MoveID: "mv-" + uuid.NewString(),
		   PlanID: req.PlanID,
		   LegID:  req.LegID,
		  FenceID: req.FenceID,
		  SrcRegion: snap.Leg.SrcRegion,
		  DstRegion: snap.Leg.DstRegion,
		   Asset:     snap.Leg.Asset,
		   Amount:    snap.Leg.Amount,
		  Success:   true,
		  CreatedAt: e.now(),
	  }
	  e.Book.RecordMove(m)
	   return m, nil
  }

   func (e *BookExecutor) failMove(snap LegSnapshot, fenceID, reason string) InventoryMove {
	  m := InventoryMove{
		   MoveID: "mv-" + uuid.NewString(),
		  PlanID: snap.Plan.PlanID,
		   LegID: snap.Leg.LegID,
		  FenceID: fenceID,
		  SrcRegion: snap.Leg.SrcRegion,
		  DstRegion: snap.Leg.DstRegion,
		  Asset:     snap.Leg.Asset,
		   Amount:    snap.Leg.Amount,
		  Success:   false,
		   Reason: reason,
		   CreatedAt: e.now(),
	   }
	  e.Book.RecordMove(m)
	   return m
   }

// MarkDone 将腿标 done；可注入失败以制造半截。
   func (e *BookExecutor) MarkDone(ctx context.Context, legID string) error {
	  if e.Inject.FailMarkDone {
		  return ErrStateUpdate
	  }
	   return e.Book.UpdateLegState(ctx, legID, LegDone, "")
   }

// MarkFailed 将腿标 failed（可重试）。
  func (e *BookExecutor) MarkFailed(ctx context.Context, legID, reason string) error {
	   return e.Book.UpdateLegState(ctx, legID, LegFailed, reason)
  }

// ExecLeg 无围栏裸路径，仅供；必须走 ExecLegWithFence。
  func (e *BookExecutor) ExecLeg(ctx context.Context, planID, legID, fenceID string) error {
	  if _, err := e.Enqueue(ctx, EnqueueReq{PlanID: planID, LegID: legID, FenceID: fenceID}); err != nil {
		  _ = e.MarkFailed(ctx, legID, err.Error())
		   return err
	   }
	  if _, err := e.TransferInv(ctx, TransferReq{PlanID: planID, LegID: legID, FenceID: fenceID}); err != nil {
		   _ = e.MarkFailed(ctx, legID, err.Error())
		   return err
	  }
	   if err := e.MarkDone(ctx, legID); err != nil {
		   return err
	   }
	   return nil
  }
