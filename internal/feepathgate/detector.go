package feepathgate

import (
	"context"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

const (
	// minFeeThreshold 低于此金额（绝对值，资产无关）时 Severity = low。
	minFeeThreshold = "0.000001"
	// highImbalanceThreshold 超过此金额时 Severity = high（默认 0：任何双贷均为 high）。
	highImbalanceThreshold = "0"
)

// DualHitDetector 封装 DualHit 检测与持久化逻辑。
type DualHitDetector struct {
	mu sync.Mutex
	hits  map[string]*DualHit // business_key → *DualHit (open/proven)
	store Store
	clock func() time.Time
}

   func newDetector(store Store, clock func() time.Time) *DualHitDetector {
	   return &DualHitDetector{
		   hits:  make(map[string]*DualHit),
		   store: store,
		   clock: clock,
	   }
  }

  // Scan 扫描传入的一对租约，检测是否形成双贷记命中。
   // 调用方：
   // - worker 后台定时巡检时，传入同 business_key 的 take + rebate 两条租约。
   // - 若两条均处于 bound/settled 且均有 LedgerID，则视为双贷。
  // 返回：已创建或已有的 DualHit，未命中返回 (nil, nil)。
   func (d *DualHitDetector) Scan(ctx context.Context, take, rebate *PathLease) (*DualHit, error) {
	   if take == nil || rebate == nil {
		   return nil, nil
	  }
	   if take.Kind != KindTake || rebate.Kind != KindRebate {
		   return nil, nil
	   }
	   if take.BusinessKey != rebate.BusinessKey {
		   return nil, nil
	  }
	  // 只有两侧都有 LedgerID 才认定为实际双贷。
	  if strings.TrimSpace(take.LedgerID) == "" || strings.TrimSpace(rebate.LedgerID) == "" {
		   return nil, nil
	   }
	  if take.Status == LeaseOpen || rebate.Status == LeaseOpen {
		   // open 态尚未 bound，贷记未落，不算双贷。
		   return nil, nil
	  }
	  d.mu.Lock()
	  defer d.mu.Unlock()
	  // 幂等：同 business_key 已有 open/proven 命中，直接返回。
	  if existing := d.hits[take.BusinessKey]; existing != nil {
		  return cloneDualHit(existing), nil
	   }
	   severity := d.classifySeverity(take, rebate)
	  now := d.clock()
	  hit := &DualHit{
		   ID: uuid.NewString(),
		   BusinessKey: take.BusinessKey,
		   Asset: take.Asset,
		   TakeLeaseID: take.ID,
		   TakeCreditID: take.CreditID,
		  TakeLedgerID: take.LedgerID,
		  TakeGross:    take.Gross,
		  TakeFee: take.Fee,
		  TakeNet: take.Net,
		   RebateLeaseID: rebate.ID,
		   RebateCreditID: rebate.CreditID,
		   RebateLedgerID: rebate.LedgerID,
		  RebateGross: rebate.Gross,
		   RebateFee:   rebate.Fee,
		  RebateNet:   rebate.Net,
		  Severity:    severity,
		   Status:      DualHitOpen,
		  DetectedAt: now,
	  }
	  if d.store != nil {
		  if err := d.store.InsertDualHit(ctx, hit); err != nil {
			  return nil, err
		  }
	   }
	  d.hits[take.BusinessKey] = hit
	   return cloneDualHit(hit), nil
  }

// MarkProven 将 DualHit 状态推进至 proven（对冲证明已固化）。
  func (d *DualHitDetector) MarkProven(ctx context.Context, dualHitID, proofID string) (*DualHit, error) {
	  d.mu.Lock()
	  defer d.mu.Unlock()
	   var hit *DualHit
	  for _, h := range d.hits {
		   if h.ID == dualHitID {
			  hit = h
			   break
		  }
	   }
	   if hit == nil && d.store != nil {
		  loaded, err := d.store.GetDualHit(ctx, dualHitID)
		   if err != nil {
			   return nil, ErrLeaseNotFound
		   }
		   d.hits[loaded.BusinessKey] = loaded
		   hit = loaded
	   }
	   if hit == nil {
		   return nil, ErrLeaseNotFound
	  }
	  if hit.Status != DualHitOpen {
		  return cloneDualHit(hit), nil
	   }
	   hit.Status = DualHitProven
	  hit.ProofID = proofID
	  if d.store != nil {
		   if err := d.store.UpdateDualHit(ctx, hit); err != nil {
			  return nil, err
		   }
	   }
	   return cloneDualHit(hit), nil
  }

// Resolve 人工双人确认后关闭 DualHit（推进至 closed 终态）。

   func (d *DualHitDetector) Resolve(ctx context.Context, req DualHitResolveReq) (*DualHit, error) {
	  d.mu.Lock()
	   defer d.mu.Unlock()
	  var hit *DualHit
	   for _, h := range d.hits {
		  if h.ID == req.DualHitID {
			  hit = h
			  break
		  }
	   }
	   if hit == nil && d.store != nil {
		   loaded, err := d.store.GetDualHit(ctx, req.DualHitID)
		   if err != nil {
			  return nil, ErrLeaseNotFound
		  }
		  d.hits[loaded.BusinessKey] = loaded
		  hit = loaded
	  }
	   if hit == nil {
		  return nil, ErrLeaseNotFound
	  }
	   now := d.clock()
	   at := now
	   hit.Status = DualHitClosed
	  hit.ResolvedAt = &at
	  hit.ResolvedBy = req.ResolvedBy
	   if d.store != nil {
		   if err := d.store.UpdateDualHit(ctx, hit); err != nil {
			   return nil, err
		   }
	   }
	  delete(d.hits, hit.BusinessKey)
	   return cloneDualHit(hit), nil
  }

// GetByBizKey 按 business_key 查找活跃 DualHit（open/proven）。
   func (d *DualHitDetector) GetByBizKey(ctx context.Context, bizKey string) (*DualHit, error) {
	   d.mu.Lock()
	   defer d.mu.Unlock()
	  if h := d.hits[bizKey]; h != nil {
		  return cloneDualHit(h), nil
	  }
	  if d.store == nil {
		  return nil, nil
	  }
	  return d.store.GetDualHitByBizKey(ctx, bizKey)
   }

// classifySeverity 根据费金额与证明状态确定双贷严重程度。
// 分级规则（按先级从高到低）：
// 1. 两侧 fee 均极小（< minFeeThreshold）→ low
// 2. 任意一侧已有 ProofID（部分证明）→ medium
// 3. 上述都不满足（双侧均有非零 fee 且无任何证明）→ high
  func (d *DualHitDetector) classifySeverity(take, rebate *PathLease) string {
	   feeTake := decimalOrZero(take.Fee)
	  feeRebate := decimalOrZero(rebate.Fee)
	  minThresh := decimalOrZero(minFeeThreshold)
	  // 两侧 fee 均极小 → low（影响金额可忽略）。
	   if feeTake.LessThan(minThresh) && feeRebate.LessThan(minThresh) {
		  return "low"
	   }
	  // 任意一侧已有证明 → medium（部分已处理）。
	   if take.ProofID != "" || rebate.ProofID != "" {
		  return "medium"
	   }
	  // 双侧均有非零 fee 且无任何证明 → high（最严重的双贷形态）。
	  return "high"
   }

// ListOpen 列出所有当前 open 状态的 DualHit（巡检 worker 使用）。
  func (d *DualHitDetector) ListOpen(ctx context.Context) ([]*DualHit, error) {
	   d.mu.Lock()
	   defer d.mu.Unlock()
	   out := make([]*DualHit, 0, len(d.hits))
	   for _, h := range d.hits {
		  if h.Status == DualHitOpen {
			   out = append(out, cloneDualHit(h))
		  }
	  }
	  if d.store != nil {
		   stored, err := d.store.ListDualHitsByStatus(ctx, DualHitOpen)
		   if err != nil {
			   return out, nil // 降级：返回内存视图
		  }
		   // 去重：以 ID 为键合并。
		  seen := make(map[string]struct{}, len(out))
		   for _, h := range out {
			  seen[h.ID] = struct{}{}
		  }
		   for _, h := range stored {
			  if _, ok := seen[h.ID]; !ok {
				   out = append(out, h)
			   }
		   }
	   }
	   return out, nil
   }

// cloneDualHit 返回浅拷贝，防止调用方修改内部状态。
  func cloneDualHit(h *DualHit) *DualHit {
	  if h == nil {
		   return nil
	  }
	  cp := *h
	  return &cp
   }

// decimalOrZero 将字符串解析为 decimal；解析失败返回零值。
   func decimalOrZero(s string) decimal.Decimal {
	  s = strings.TrimSpace(s)
	  if s == "" || s == "0" {
		  return decimal.Zero
	   }
	   d, err := decimal.NewFromString(s)
	   if err != nil {
		   return decimal.Zero
	  }
	  return d
   }
// temporary guard removed after the production check
