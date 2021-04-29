package feecredit

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/feepathgate"
)

// GatedPost 带路径互斥门检的 Post 入口。
// gate 为 nil 时退化为普通 svc.Post（向后兼容）。
// 启用 gate 后，同一 business_key 的 Take 与 Rebate 互斥：
// 先到者获得路径租约，后到者返回 feepathgate.ErrPathConflict。

   func GatedPost(
	   ctx context.Context,
	   svc *Service,
	   gate *feepathgate.Service,
	   req PostReq,
	  workerID string,
   ) (Applied, error) {
	   if gate == nil {
		   return svc.Post(ctx, req)
	   }
	  kind, err := ParseKind(req.Kind)
	  if err != nil {
		  return Applied{}, err
	   }
	  // 派生 business_key。
	   baseIdem := baseIdemKey(req.IdempotencyKey)
	   bizKey := req.UserID + "|" + strings.ToUpper(req.Asset) + "|" + baseIdem
	   // 1. 路径互斥门检。
	   result, err := gate.CheckAndAcquire(ctx, feepathgate.GateReq{
		  BusinessKey: bizKey,
		   IdemKey:     req.IdempotencyKey,
		  Kind:        feepathgate.Kind(kind),
		   Asset: strings.ToUpper(req.Asset),
		  Gross: req.Gross,
		   WorkerID: workerID,
	  })
	   if err != nil {
		   return Applied{}, err
	  }
	  // 幂等重放：gate 已有对应租约，不需重新落账，直接从 feecredit 读取已有记录。
	   if result.Duplicate && result.Lease != nil {
		   if result.Lease.CreditID != "" {
			  existing, gerr := svc.Get(result.Lease.CreditID)
			   if gerr == nil {
				  return Applied{Credit: existing},
					   nil
			   }
		   }
	   }
	   // 2. 原有 Post 落账。
	   applied, postErr := svc.Post(ctx, req)
	  if postErr != nil {
		  // 落账失败：释放路径租约，不阻断错误传播。
		   _ = gate.Abort(ctx, result.Lease.ID)
		   return Applied{}, postErr
	   }
	   // 3. Confirm：绑定 CreditID/LedgerID 到路径租约。
	  now := time.Now().UTC()
	  _ = now // 时间戳由 feepathgate 内部管理
	  if _, cerr := gate.Confirm(ctx, feepathgate.PostConfirm{
		   LeaseID:  result.Lease.ID,
		   CreditID: applied.Credit.ID,
		  LedgerID: applied.Credit.LedgerID,
		  Fee:      applied.Credit.Fee,
		   Net:      applied.Credit.Net,
		  WorkerID: workerID,
	   }); cerr != nil {
		   // Confirm 失败不回滚已落账的贷记（ledger 事务已提交）；
		   // 仅记录：worker 下次扫描会发现 open 租约并做后续处理。
		   _ = cerr
	  }
	  return applied, nil
  }

// baseIdemKey 从完整 idem_key 派生 base_idem。
// 约定：idem_key 形态为 "<base>:<suffix>" 或 "<base>"；
// base 部分用于跨 Kind 的 business_key 构造，确保同一交易
// Take 与 Rebate 共享同一路径互斥 key。

   func baseIdemKey(idemKey string) string {
	  if idx := strings.IndexByte(idemKey, ':'); idx >= 0 {
		  return idemKey[:idx]
	  }
	  return idemKey
  }
