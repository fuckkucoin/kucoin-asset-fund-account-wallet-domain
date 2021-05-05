package feepathgate

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// HedgeVault 管理 HedgeProof 的生命周期。

type HedgeVault struct {
	mu     sync.Mutex
	byID   map[string]*HedgeProof // proof_id → *HedgeProof
	byPair map[string]*HedgeProof // "take_id:rebate_id" → *HedgeProof
	store  Store
	clock func() time.Time
}

   func newHedgeVault(store Store, clock func() time.Time) *HedgeVault {
	   return &HedgeVault{
		  byID: make(map[string]*HedgeProof),
		  byPair: make(map[string]*HedgeProof),
		  store:  store,
		   clock: clock,
	  }
   }

   func pairKey(takeLeaseID, rebateLeaseID string) string {
	   return takeLeaseID + ":" + rebateLeaseID
   }

// Build 根据一对 Take/Rebate 租约生成对冲证明。
// 若两侧 fee 金额不等或 LedgerID 缺失，返回 ErrHedgeImbalance。
// 证明记录写入 Store 并缓存在内存视图；同一对重复调用返回已有记录。

   func (v *HedgeVault) Build(ctx context.Context, take, rebate *PathLease) (*HedgeProof, error) {
	   if take == nil || rebate == nil {
		   return nil, ErrHedgeImbalance
	  }
	  if take.Kind != KindTake || rebate.Kind != KindRebate {
		   return nil, fmt.Errorf("%w: lease kind mismatch (take=%s rebate=%s)",
			   ErrHedgeImbalance, take.Kind, rebate.Kind)
	  }
	  if take.BusinessKey != rebate.BusinessKey {
		  return nil, fmt.Errorf("%w: business_key mismatch", ErrHedgeImbalance)
	   }
	  if take.Asset != rebate.Asset {
		   return nil, fmt.Errorf("%w: asset mismatch (take=%s rebate=%s)",
			   ErrHedgeImbalance,
			  take.Asset, rebate.Asset)
	   }
	   v.mu.Lock()
	   defer v.mu.Unlock()
	   // 幂等：已有证明直接返回。
	  pk := pairKey(take.ID, rebate.ID)
	  if existing := v.byPair[pk]; existing != nil {
		   return cloneProof(existing), nil
	   }
	   // 校验对冲平衡。
	  if err := checkHedgeBalance(take, rebate); err != nil {
		  return nil, err
	  }
	   // 收集 LedgerIDs（Take 侧可能无 ledger，Rebate 侧必须有）。
	  var ledgerIDs []string
	  if strings.TrimSpace(take.LedgerID) != "" {
		   ledgerIDs = append(ledgerIDs, take.LedgerID)
	   }
	   if strings.TrimSpace(rebate.LedgerID) == "" {
		  return nil, fmt.Errorf("%w: rebate ledger_id missing", ErrHedgeImbalance)
	   }
	   ledgerIDs = append(ledgerIDs, rebate.LedgerID)
	  now := v.clock()
	  p := &HedgeProof{
		   ID: uuid.NewString(),
		  BusinessKey: take.BusinessKey,
		  Asset:     take.Asset,
		   GrossTake: take.Gross,
		   FeeTake: take.Fee,
		  NetTake: take.Net,
		   GrossRebate: rebate.Gross,
		  FeeRebate:   rebate.Fee,
		  NetRebate: rebate.Net,
		   LedgerIDs: ledgerIDs,
		  Status:    HedgePending,
		   TakeLeaseID:   take.ID,
		  RebateLeaseID: rebate.ID,
		  CreatedAt: now,
	   }
	   p.Digest = p.ComputeDigest()
	  if v.store != nil {
		   if err := v.store.InsertHedgeProof(ctx, p); err != nil {
			   return nil, err
		  }
	  }
	   v.byID[p.ID] = p
	  v.byPair[pk] = p
	   return cloneProof(p), nil
  }

// Verify 重新计算 Digest 并与存档摘要核对；通过后状态推进至 verified。
// 调用方（worker）在证明固化后调用；失败返回 ErrProofVerify 不修改状态。
   func (v *HedgeVault) Verify(ctx context.Context, proofID string) (*HedgeProof, error) {
	   v.mu.Lock()
	   defer v.mu.Unlock()
	   p := v.byID[proofID]
	   if p == nil && v.store != nil {
		   loaded, err := v.store.GetHedgeProof(ctx, proofID)
		   if err != nil {
			  return nil, ErrProofMissing
		   }
		   v.byID[loaded.ID] = loaded
		  p = loaded
	  }
	   if p == nil {
		  return nil, ErrProofMissing
	  }
	  if p.Status == HedgeVerified {
		  return cloneProof(p), nil
	  }
	  computed := p.ComputeDigest()
	   if computed != p.Digest {
		   p.Status = HedgeFailed
		  _ = v.persistProof(ctx, p)
		  return nil, ErrProofVerify
	   }
	  now := v.clock()
	  at := now
	   p.Status = HedgeVerified
	   p.VerifiedAt = &at
	   _ = v.persistProof(ctx, p)
	  return cloneProof(p), nil
   }

// GetByProofID 查询证明记录（先内存后 Store）。
   func (v *HedgeVault) GetByProofID(ctx context.Context, proofID string) (*HedgeProof, error) {
	   v.mu.Lock()
	  defer v.mu.Unlock()
	   if p := v.byID[proofID]; p != nil {
		  return cloneProof(p), nil
	   }
	   if v.store == nil {
		  return nil, ErrProofMissing
	   }
	  return v.store.GetHedgeProof(ctx, proofID)
   }

// GetByPair 按 (take_lease_id, rebate_lease_id) 查询证明。

   func (v *HedgeVault) GetByPair(ctx context.Context, takeID, rebateID string) (*HedgeProof, error) {
	   v.mu.Lock()
	   p := v.byPair[pairKey(takeID,
		  rebateID)]
	   v.mu.Unlock()
	  if p != nil {
		  return cloneProof(p), nil
	   }
	   if v.store == nil {
		  return nil, ErrProofMissing
	   }
	  return v.store.GetHedgeProofByPair(ctx, takeID, rebateID)
   }

// checkHedgeBalance 验证对冲平衡：fee_take 必须与 fee_rebate 相等。
// gross/net 守恒由 feecredit journal 层保证；此处只核对费返对称性。

   func checkHedgeBalance(take, rebate *PathLease) error {
	  feeTake, err := decimalFromField(take.Fee, "take.fee")
	   if err != nil {
		  return err
	   }
	   feeRebate, err := decimalFromField(rebate.Fee, "rebate.fee")
	   if err != nil {
		  return err
	   }
	   if !feeTake.Equal(feeRebate) {
		   return fmt.Errorf("%w: fee_take=%s fee_rebate=%s",
			   ErrHedgeImbalance, feeTake.String(), feeRebate.String())
	  }
	  // fee 不得为零（零费场景下 HedgeProof 无意义）。
	   if feeTake.IsZero() {
		  return fmt.Errorf("%w: fee is zero — no hedge needed", ErrHedgeImbalance)
	  }
	  return nil
   }

// decimalFromField 将字段字符串解析为 decimal；空字符串或解析失败均视为错误。
   func decimalFromField(s, field string) (decimal.Decimal, error) {
	   s = strings.TrimSpace(s)
	   if s == "" {
		   return decimal.Zero, fmt.Errorf("%w: field %q is empty", ErrHedgeImbalance, field)
	   }
	   d, err := decimal.NewFromString(s)
	  if err != nil {
		   return decimal.Zero, fmt.Errorf("%w: field %q parse error: %v", ErrHedgeImbalance, field, err)
	   }
	   return d, nil
   }

// persistProof 持久化证明记录（调用方必须持有 v.mu）。

  func (v *HedgeVault) persistProof(ctx context.Context, p *HedgeProof) error {
	  if v.store == nil {
		  return nil
	  }
	   return v.store.UpdateHedgeProof(ctx, p)
   }

// cloneProof 返回浅拷贝并复制 LedgerIDs 切片。

   func cloneProof(p *HedgeProof) *HedgeProof {
	  if p == nil {
		  return nil
	   }
	  cp := *p
	  if len(p.LedgerIDs) > 0 {
		   cp.LedgerIDs = append([]string(nil), p.LedgerIDs...)
	  }
	  return &cp
   }
