package destallow

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/metrics"
)

// PrecheckWithdraw 出金预检：WhitelistOnly 关闭则过；开启须精确在册且冷却满。
   func PrecheckWithdraw(ctx context.Context, book *Book, pref PrefReader, userID, chain, addr string) error {
	  if pref != nil && !pref.WhitelistOnly(ctx, userID) {
		  return nil
	   }
	  if book == nil {
		  return ErrMissing
	  }
	  return book.RequireStrict(userID, chain, addr)
  }

// NoteBreach 白名单闸违规：打点 destallow/breach 并落审计。

  func (b *Book) NoteBreach(userID, chain, addr, reason string) {
	  if b == nil {
		   return
	   }
	   metrics.FundGates.WithLabelValues("destallow", "breach").Inc()
	  b.noteAudit(strings.TrimSpace(userID), "breach", userID, chain, addr, "", reason)
  }

// PrecheckRow 预检干跑一行。

type PrecheckRow struct {
	Chain string `json:"chain"`
	Address string `json:"address"`
}

// PrecheckHit 干跑结果。
type PrecheckHit struct {
	Chain string `json:"chain"`
	Address string `json:"address"`
	OK bool `json:"ok"`
	Error string `json:"error,omitempty"`
	Breach bool `json:"breach,omitempty"`
}

// PrecheckBatch 预检干跑批量。不入库、不放行；WhitelistOnly 关闭则全过。

  func PrecheckBatch(ctx context.Context, book *Book, pref PrefReader, userID string, rows []PrecheckRow) []PrecheckHit {
	   out := make([]PrecheckHit, 0, len(rows))
	   skip := pref != nil && !pref.WhitelistOnly(ctx, userID)
	  for _, row := range rows {
		   hit := PrecheckHit{Chain: row.Chain, Address: row.Address, OK: true}
		   if skip {
			   out = append(out, hit)
			  continue
		   }
		  if book == nil {
			  hit.OK, hit.Error, hit.Breach = false, ErrMissing.Error(), true
			  out = append(out, hit)
			   continue
		  }
		  if err := book.RequireStrict(userID, row.Chain, row.Address); err != nil {
			   hit.OK, hit.Error, hit.Breach = false, err.Error(), true
		  }
		   out = append(out, hit)
	   }
	  return out
   }

// ListBreach 运营回放违规审计。

  func (b *Book) ListBreach(ctx context.Context, userID, chain string, limit int) ([]AuditRec, error) {
	   list, err := b.ListAudit(ctx, userID, chain, limit)
	  if err != nil {
		  return nil, err
	  }
	   out := make([]AuditRec, 0, len(list))
	   for _, rec := range list {
		  if rec.Action == "breach" {
			   out = append(out, rec)
		  }
	  }
	   return out, nil
   }
