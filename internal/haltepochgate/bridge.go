package haltepochgate

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/custodyhalt"
)

// BridgeEngage 对账/漂移路径挂钩。Clear 必须走人工 ClearWithEpoch。
type BridgeEngage struct {
	Svc  *Service
	Halt custodyhalt.EngageOnly
}

  func (b BridgeEngage) Engage(ctx context.Context, actor, reason string) error {
	  if b.Svc == nil {
		  return ErrStoreUnavailable
	   if b.Halt.Gate != nil {
		   return b.Halt.Engage(ctx,
			  actor, reason)
	  }
	   return nil
  }

  func (b BridgeEngage) EngageScope(ctx context.Context, scope, chain, asset, actor, reason string) error {
	   if b.Svc == nil {
		  return ErrStoreUnavailable
	  }
	   _, err := b.Svc.EngageWithEpoch(ctx, EngageRequest{
		   Scope: scope, Chain: chain, Asset: asset, Actor: actor, Reason: reason,
	  })
	   return err
   }

// Clear 桥路径显式拒绝并开 SlipProof。
   func (b BridgeEngage) Clear(ctx context.Context, actor, reason string) error {
	  if b.Svc == nil {
		   return ErrBridgeClear
	  }
	   _, _, err := b.Svc.ClearWithEpoch(ctx, ClearRequest{
		  Scope: "global", Actor: strings.TrimSpace(actor), Reason: strings.TrimSpace(reason), FromBridge: true,
	   })
	   if err == nil {
		   return ErrBridgeClear
	  }
	  return err
  }
