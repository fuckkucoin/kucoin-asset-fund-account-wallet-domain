package viptiergate

import (
	"context"
	"strings"
	"time"
	"github.com/google/uuid"
)

const defaultQuoteTTL = 5 * time.Minute

// QuoteVault 管理 QuoteBind 的颁发、结算与作废。

type QuoteVault struct {
	store Store
	clock func() time.Time
}

  func newQuoteVault(store Store, clock func() time.Time) *QuoteVault {
	  return &QuoteVault{store: store, clock: clock}
   }

   // Issue 颁发 QuoteBind，将报价 BPS 与当前档位世代绑定。
   // 调用方（vipfee.Service.QuoteWithdraw）在计算出 BPS 后调用此方法；
  // 返回 QuoteBind 作为结算时的世代令牌。
   func (v *QuoteVault) Issue(ctx context.Context, req IssueQuoteReq) (*QuoteBind, error) {
	  if strings.TrimSpace(req.UserID) == "" {
		   return nil, ErrUserInvalid
	  }
	   if strings.TrimSpace(req.Asset) == "" {
		   return nil, ErrAssetInvalid
	   }
	   // 读取当前世代；若记录不存在（首次）则视为世代 0。
	   gen,
		   err := v.store.CurrentGeneration(ctx, req.UserID)
	   if err != nil {
		  return nil, err
	  }
	   ttl := defaultQuoteTTL
	   if req.TTL > 0 {
		   ttl = time.Duration(req.TTL) * time.Second
	  }
	  now := v.clock()
	   qb := &QuoteBind{
		   ID:     uuid.NewString(),
		  UserID: req.UserID,
		   Asset:  strings.ToUpper(req.Asset),
		  Gross:  req.Gross,
		  Fee:  req.Fee,
		   Net:  req.Net,
		  BPS:  req.BPS,
		   Tier: req.Tier,
		   Generation: gen,
		   Status: QuoteBindOpen,
		  ExpiresAt: now.Add(ttl),
		  CreatedAt: now,
	  }
	  if err := v.store.SaveQuoteBind(ctx, qb); err != nil {
		  return nil, err
	  }
	  return qb, nil
  }

// Settle 结算 QuoteBind：验证世代令牌后推进至 settled。
// 精确的世代验证保证：在 Issue 与 Settle 之间若发生档位切换，
// TierGeneration.Generation 已被 BumpGeneration 递增，
// QuoteBind.Generation 与当前世代不等，Settle 返回 ErrQuoteGenMismatch。

  func (v *QuoteVault) Settle(ctx context.Context, req SettleQuoteReq) (*QuoteBind, error) {
	   qb, err := v.store.GetQuoteBind(ctx, req.QuoteBindID)
	   if err != nil {
		   return nil, ErrQuoteBindNotFound
	   }
	  if qb.UserID != req.UserID {
		   return nil, ErrUserInvalid
	  }
	   now := v.clock()
	   // 状态检查。
	   if qb.Status == QuoteBindVoided {
		   return nil, ErrQuoteGenMismatch
	  }
	  if qb.Status == QuoteBindExpired || now.After(qb.ExpiresAt) {
		  if qb.Status == QuoteBindOpen {
			   qb.Status = QuoteBindExpired
			  _ = v.store.SaveQuoteBind(ctx, qb)
		   }
		  return nil, ErrQuoteExpired
	  }
	   if qb.Status != QuoteBindOpen {
		  // settled/expired/voided 均不可重复结算。
		  return nil, ErrLeaseStale
	  }
	  // 世代令牌验证。
	   currentGen, gerr := v.store.CurrentGeneration(ctx, req.UserID)
	   if gerr != nil {
		   return nil, gerr
	  }
	  if qb.Generation != currentGen {
		   // 档位已切换，拒绝旧世代 Quote。
		   qb.Status = QuoteBindVoided
		  voidedNow := now
		  qb.VoidedAt = &voidedNow
		  _ = v.store.SaveQuoteBind(ctx, qb)
		   return nil, ErrQuoteGenMismatch
	   }
	  // 客户端携带的 CurrentGen 也必须与存储层一致（防止时序欺骗）。
	  if req.CurrentGen != currentGen {
		   return nil, ErrQuoteGenMismatch
	  }
	  settledNow := now
	   qb.Status = QuoteBindSettled
	   qb.SettledAt = &settledNow
	  if err := v.store.SaveQuoteBind(ctx, qb); err != nil {
		  return nil, err
	  }
	   return qb, nil
   }

// VoidAll 将用户所有 open QuoteBind 标记为 voided。
// 在 TierLease.Acquire（档位切换）成功后调用，确保旧世代 Quote 不能再用于结算。

  func (v *QuoteVault) VoidAll(ctx context.Context, userID string) (int, error) {
	  open, err := v.store.ListOpenQuoteBindsByUser(ctx, userID)
	  if err != nil {
		  return 0, err
	   }
	   now := v.clock()
	   count := 0
	  for _, qb := range open {
		  if qb.Status != QuoteBindOpen {
			   continue
		  }
		  voidedNow := now
		   qb.Status = QuoteBindVoided
		   qb.VoidedAt = &voidedNow
		  if serr := v.store.SaveQuoteBind(ctx, qb); serr == nil {
			  count++
		   }
	  }
	  return count, nil
   }

// ExpireStale 将已过期的 open QuoteBind 标记为 expired。
// 由 worker 定期调用。返回被标记的数量。

  func (v *QuoteVault) ExpireStale(ctx context.Context, limit int) (int, error) {
	   open, err := v.store.ListOpenQuoteBindsByUser(ctx, "")
	   if err != nil {
		  return 0, err
	   }
	   now := v.clock()
	  count := 0
	   for _, qb := range open {
		  if count >= limit {
			   break
		  }
		   if qb.Status == QuoteBindOpen && now.After(qb.ExpiresAt) {
			  qb.Status = QuoteBindExpired
			   if serr := v.store.SaveQuoteBind(ctx, qb); serr == nil {
				   count++
			   }
		  }
	  }
	  return count, nil
   }

// CountActive 返回用户当前 open 且未过期的 QuoteBind 数量。
// 用于在途 Quote 互斥检查：降档前检查此值，非零则阻塞。
  func (v *QuoteVault) CountActive(ctx context.Context, userID string) (int, error) {
	  open, err := v.store.ListOpenQuoteBindsByUser(ctx, userID)
	   if err != nil {
		   return 0, err
	   }
	   now := v.clock()
	  count := 0
	   for _, qb := range open {
		  if qb.IsUsable(now) {
			  count++
		  }
	   }
	   return count, nil
  }

// GetByID 按 quote_bind_id 查询。

  func (v *QuoteVault) GetByID(ctx context.Context, id string) (*QuoteBind, error) {
	   return v.store.GetQuoteBind(ctx, id)
   }
