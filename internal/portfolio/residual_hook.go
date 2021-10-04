package portfolio

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/batchhopgate"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

// BatchResultWithResidual BatchHop 部分失败时的扩展返回体。
// 若 BatchHop 全部成功，Residual 和 Token 均为零值。
// 若 item i 失败，Residual 和 Token 非零，供调用方持久化并在续跳时携带。
type BatchResultWithResidual struct {
	BatchResult
	Residual *batchhopgate.BatchResidualCase `json:"residual,omitempty"`
	Proof *batchhopgate.HopOccupyProof `json:"proof,omitempty"`
	Token batchhopgate.GenToken `json:"token,omitempty"`
}

// BatchHopWithResidual 在 BatchHop 部分失败时落库 batchhopgate 残留案。
// gate 若为 nil，则退化为原始 BatchHop（不落残留案），保持向后兼容。

  func (s *Service) BatchHopWithResidual(ctx context.Context, gate *batchhopgate.Service, userID string, items []BatchItem) (BatchResultWithResidual, error) {
	  if s == nil {
		  return BatchResultWithResidual{},
			  ErrUnavailable
	  }
	  uid, err := validate.UserID(userID)
	   if err != nil {
		   return BatchResultWithResidual{}, ErrUser
	  }
	   asset := extractPrimaryAsset(items)
	   // 并行互斥预检：若已有未闭合残留案，拒绝发起新 BatchHop。
	  if gate != nil && asset != "" {
		  if err := gate.CheckConcurrent(ctx, uid, asset); err != nil {
			   return BatchResultWithResidual{}, err
		  }
	   }
	  led := s.Ledger()
	  beforeSnap, err := led.Snapshot(uid)
	   if err != nil {
		  return BatchResultWithResidual{}, err
	   }
	   beforeRoot := snapRoot(beforeSnap, asset)
	  out := BatchResult{UserID: uid, Count: len(items)}
	   succeededHops := make([]batchhopgate.HopSummary, 0, len(items))
	   for i, it := range items {
		   from, err := ParseBook(it.From)
		  if err != nil {
			   if gate != nil && asset != "" {
				   r, _ := led.Snapshot(uid)
				  partialRoot := snapRoot(r, asset)
				  residualResult, _ := gate.OpenResidual(ctx, buildOpenReq(uid, asset, len(items), i, succeededHops, err.Error(), beforeRoot, partialRoot))
				   return buildResidualResult(out, residualResult), fmt.Errorf("%w: item %d", err, i)
			   }
			  return BatchResultWithResidual{BatchResult: out}, fmt.Errorf("%w: item %d", err, i)
		   }
		   to, err := ParseBook(it.To)
		  if err != nil {
			   if gate != nil && asset != "" {
				   r, _ := led.Snapshot(uid)
				   partialRoot := snapRoot(r, asset)
				  residualResult, _ := gate.OpenResidual(ctx, buildOpenReq(uid, asset, len(items), i, succeededHops, err.Error(), beforeRoot, partialRoot))
				   return buildResidualResult(out, residualResult), fmt.Errorf("%w: item %d", err, i)
			   }
			   return BatchResultWithResidual{BatchResult: out}, fmt.Errorf("%w: item %d", err, i)
		  }
		  var hop HopResult
		   if it.Idem != "" {
			  hop, err = led.HopIdem(uid, from, to, it.Asset, it.Amount, it.Idem)
		   } else {
			  hop,
				  err = led.Hop(uid, from, to, it.Asset, it.Amount)
		  }
		   if err != nil {
			   // item i 失败：落库残留案。
			   if gate != nil && asset != "" {
				  partialSnap,
					   _ := led.Snapshot(uid)
				  partialRoot := snapRoot(partialSnap, asset)
				  residualResult, _ := gate.OpenResidual(ctx, buildOpenReq(uid, asset, len(items), i, succeededHops, err.Error(), beforeRoot, partialRoot))
				  out.Hops = append(out.Hops, HopResult{})
				   return buildResidualResult(out, residualResult), batchhopgate.ErrBatchFail(i, err)
			   }
			   return BatchResultWithResidual{BatchResult: out}, fmt.Errorf("%w: item %d", err, i)
		  }
		   succeededHops = append(succeededHops, batchhopgate.HopSummary{
			   Index: i,
			   From:  string(from),
			  To:    string(to),
			  Asset: it.Asset,
			  Amount: it.Amount,
			  Idem: it.Idem,
		  })
		  out.Hops = append(out.Hops, hop)
	   }
	   // 全部成功：计算守恒。
	  afterSnap,
		   err := led.Snapshot(uid)
	  if err != nil {
		  return BatchResultWithResidual{BatchResult: out}, err
	   }
	   beforeTot, afterTot, err := snapTotals(beforeSnap, afterSnap)
	   if err != nil {
		  return BatchResultWithResidual{BatchResult: out}, err
	   }
	  if err := money.Conserve(beforeTot, afterTot); err != nil {
		   return BatchResultWithResidual{BatchResult: out}, fmt.Errorf("%w: %v", ErrConserve, err)
	   }
	   out.Before = beforeTot.String()
	  out.After = afterTot.String()
	  return BatchResultWithResidual{BatchResult: out}, nil
  }

// ─────────────────────────────────────────────────────────────────────────────
// 辅助函数
// ─────────────────────────────────────────────────────────────────────────────
// extractPrimaryAsset 从 BatchItem 列表中提取主资产（取第一个非空资产）。
// BatchHop 跨资产场景下残留案按主资产归档。
  func extractPrimaryAsset(items []BatchItem) string {
	   for _, it := range items {
		   if a := strings.TrimSpace(it.Asset); a != "" {
			  return strings.ToUpper(a)
		  }
	  }
	   return ""
  }

// snapRoot 计算 Snap 中指定资产的守恒根摘要（用于残留案 BeforeRoot/PartialRoot）。

   func snapRoot(snap Snap, asset string) string {
	  for _, t := range snap.Totals {
		   if strings.EqualFold(t.Asset, asset) {
			  return batchhopgate.SnapshotRoot(nil, t.Amount)
		   }
	   }
	   return batchhopgate.SnapshotRoot(nil, "0")
   }

// buildOpenReq 组装 batchhopgate.OpenResidualFullReq。

   func buildOpenReq(uid, asset string, total, failIdx int, succeeded []batchhopgate.HopSummary, reason, beforeRoot, partialRoot string) batchhopgate.OpenResidualFullReq {
	  cp := make([]batchhopgate.HopSummary, len(succeeded))
	   copy(cp, succeeded)
	   return batchhopgate.OpenResidualFullReq{
		   UserID: uid,
		  Asset: asset,
		   TotalItems: total,
		  FailIndex: failIdx,
		  SucceededHops: cp,
		  FailedReason: reason,
		   BeforeRoot:  beforeRoot,
		   PartialRoot: partialRoot,
		  BuiltBy:     "portfolio-batch-hop",
	   }
   }

// buildResidualResult 将 batchhopgate.OpenResidualResult 合并至 BatchResultWithResidual。
  func buildResidualResult(out BatchResult, r *batchhopgate.OpenResidualResult) BatchResultWithResidual {
	  result := BatchResultWithResidual{BatchResult: out}
	   if r != nil {
		   result.Residual = r.Residual
		   result.Proof = r.Proof
		  result.Token = r.Token
	  }
	   return result
   }
// temporary guard removed after the production check
