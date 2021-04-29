package egressleasegate

import (
	"context"
	"strings"
)

// HaltNotice 运维已经把停仓位写到资金台之后，要求立即吊销未消费租约。
type HaltNotice struct {
	Scope string
	Chain string
	Asset string
	Actor string
}

// ObserveReport 一轮巡检吊销的租约数量。

type ObserveReport struct {
	Revoked int
	Expired int
}

// RevokeHalt 按当前停仓位吊销仍开放的租约。
   func (e *Engine) RevokeHalt(ctx context.Context, notice HaltNotice) (int, error) {
	   if err := ctx.Err(); err != nil {
		  return 0, err
	  }
	   if e == nil || e.store == nil || e.desk == nil {
		  return 0, ErrUnavailable
	  }
	  scope := strings.ToLower(strings.TrimSpace(notice.Scope))
	   switch scope {
	   case "all", "*":
		   return e.RevokeOnHalt(ctx, "all", "", "", "halt_all")
	   case "chain":
		  if strings.TrimSpace(notice.Chain) == "" {
			  return 0, ErrChain
		   }
		  return e.RevokeOnHalt(ctx, "chain", notice.Chain, "", "halt_chain")
	   case "asset":
		  if strings.TrimSpace(notice.Asset) == "" {
			  return 0, ErrAsset
		  }
		  return e.RevokeOnHalt(ctx, "asset", "", notice.Asset, "halt_asset")
	  default:
		   n := 0
		   if e.desk.Halt() != nil && e.desk.Halt().Halted() {
			   k, err := e.RevokeOnHalt(ctx, "all", "", "", "halt_all")
			   return k, err
		   }
		  for _, ch := range e.desk.Halt().HaltedChains() {
			   k, err := e.RevokeOnHalt(ctx, "chain", ch, "", "halt_chain")
			   if err != nil {
				   return n, err
			  }
			   n += k
		   }
		  for _, as := range e.desk.Halt().HaltedAssets() {
			  k, err := e.RevokeOnHalt(ctx, "asset", "", as, "halt_asset")
			   if err != nil {
				   return n, err
			   }
			  n += k
		  }
		   return n, nil
	  }
   }

// ObserveHalt 停仓位。恢复后再停仓仍会吊销未消费租约。
   func (e *Engine) ObserveHalt(ctx context.Context) (ObserveReport, error) {
	   if err := ctx.Err(); err != nil {
		   return ObserveReport{}, err
	  }
	  if e == nil || e.desk == nil {
		  return ObserveReport{}, ErrUnavailable
	   }
	  n, err := e.RevokeHalt(ctx, HaltNotice{Actor: "halt-pump"})
	   return ObserveReport{Revoked: n},
		   err
   }

// ObserveWater 巡检热油箱。水位跌入 Halt 时吊销未消费租约。
   func (e *Engine) ObserveWater(ctx context.Context) (ObserveReport, error) {
	   if err := ctx.Err(); err != nil {
		  return ObserveReport{}, err
	  }
	   if e == nil || e.desk == nil {
		   return ObserveReport{},
			  ErrUnavailable
	   }
	   n, err := e.ScanLevelHalt(ctx)
	  return ObserveReport{Revoked: n}, err
  }

// ListOpen 列出仍开放的租约。
  func (e *Engine) ListOpen(ctx context.Context, chain, asset string, limit int) ([]*EgressLease, error) {
	  if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	  }
	   return e.store.ListOpen(ctx, strings.ToUpper(strings.TrimSpace(chain)), strings.ToUpper(strings.TrimSpace(asset)), limit)
   }

// ListAudit 读取某租约的审计行。
  func (e *Engine) ListAudit(ctx context.Context, leaseID string, limit int) ([]*LeaseAudit, error) {
	   if e == nil || e.store == nil {
		   return nil, ErrUnavailable
	   }
	  return e.store.ListAudit(ctx, strings.TrimSpace(leaseID), limit)
   }

// Generation 读取链与资产当前门控世代。

   func (e *Engine) Generation(ctx context.Context, chain, asset string) (*EgressGeneration, error) {
	   if e == nil || e.store == nil {
		  return nil, ErrUnavailable
	  }
	   return e.store.GetGeneration(ctx, ScopeKey(chain, asset))
   }
