package feebumpgate

import (
	"context"
	"strings"
)

// MaterialBind 签章材料绑定面。

type MaterialBind struct {
	TransferID string
	Canon string
	SigHex         string
	DestinationTag string
	NewFee         string
	PrevHash       string
}

// BindDigest 材料绑定摘要。
   func BindDigest(b MaterialBind) string {
	   return BroadcastHash(
		  strings.TrimSpace(b.TransferID),
		   CanonDigest(b.Canon),
		  strings.TrimSpace(b.SigHex),
		  strings.TrimSpace(b.DestinationTag),
		  strings.TrimSpace(b.NewFee),
		  strings.TrimSpace(b.PrevHash),
	   )
   }

// AssertMaterial 广播前材料复验。
  func (e *Engine) AssertMaterial(ctx context.Context, genID string, b MaterialBind) error {
	   if e == nil || e.store == nil {
		  return ErrUnavailable
	   }
	   gen, err := e.store.GetGen(ctx, genID)
	   if err != nil {
		   return err
	  }
	   if gen.TransferID != strings.TrimSpace(b.TransferID) {
		  return ErrTransfer
	  }
	   if b.Canon != "" && CanonDigest(b.Canon) != gen.CanonDigest {
		  e.metric.IncFailClosed()
		  return ErrPrevHash
	  }
	   if b.NewFee != "" && strings.TrimSpace(b.NewFee) != gen.NewFee {
		   e.metric.IncFailClosed()
		  return ErrFee
	  }
	  if b.DestinationTag != "" && strings.TrimSpace(b.DestinationTag) != gen.DestinationTag {
		  e.metric.IncFailClosed()
		   return ErrDest
	  }
	  if b.PrevHash != "" && !SameHash(b.PrevHash, gen.PrevBroadcastHash) && gen.Generation > 1 {
		  e.metric.IncFailClosed()
		  return ErrPrevHash
	   }
	  latest, err := e.store.LatestGen(ctx, gen.TransferID)
	   if err != nil {
		   return err
	   }
	  if latest.GenID != gen.GenID {
		   e.metric.IncFailClosed()
		   return ErrGenStale
	   }
	  return nil
  }

// BindAndBroadcast 材料复验后替换广播。
  func (e *Engine) BindAndBroadcast(ctx context.Context, genID, actor string, b MaterialBind) (BroadcastResp, error) {
	   if err := e.AssertMaterial(ctx, genID, b); err != nil {
		   return BroadcastResp{}, err
	  }
	   hash := ""
	  if gen, err := e.store.GetGen(ctx, genID); err == nil {
		  hash = gen.NewBroadcastHash
	  }
	  return e.BroadcastReplace(ctx, BroadcastReq{
		  GenID: genID, TransferID: b.TransferID, BroadcastHash: hash, Actor: actor,
	   })
   }
