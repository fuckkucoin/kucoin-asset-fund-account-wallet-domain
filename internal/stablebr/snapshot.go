package stablebr

import (
	"time"
	"github.com/google/uuid"
)

// AttestSnapshot 是某一 kind/asset 组合在特定时刻的证明快照。
// 快照不可变；所有字段在创建后不得修改。
type AttestSnapshot struct {
	ID    string `json:"id"`
	Kind  Kind   `json:"kind"`
	Asset string `json:"asset"`
	Checks []Check `json:"checks"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// Valid 报告快照在给定时刻是否仍在有效窗口内。
// 快照 ExpiresAt 为零表示无过期（不应出现在路径）。
  func (s AttestSnapshot) Valid(now time.Time) bool {
	  if s.ExpiresAt.IsZero() {
		  return false // 防御：零时间视为已过期
	   }
	  return s.ExpiresAt.After(now)
  }

// SnapshotAttest 将当前 kind/asset 所有证明项捕获为不可变快照。
// 任意证明项缺失或已过期时 fail-closed，返回 ErrAttest 或 ErrExpired。
// TTL 边：若所有项均有效但最快到期项已到达 TTL 临界，快照仍有效——
// 调用方必须在 ExpiresAt 前完成 Mint/Redeem，否则快照失效。

  func (s *Service) SnapshotAttest(kindS, asset string) (AttestSnapshot, error) {
	   if s == nil {
		  return AttestSnapshot{}, ErrUnavailable
	  }
	  kind, err := ParseKind(kindS)
	   if err != nil {
		  return AttestSnapshot{}, err
	  }
	  as, err := allowedAsset(asset)
	  if err != nil {
		   return AttestSnapshot{}, err
	   }
	  now := s.now()
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  items := required(kind)
	   checks := make([]Check, 0, len(items))
	  minExpiry := time.Time{}
	  for _, item := range items {
		  c,
			  ok := s.checks[checkKey(kind, as, item)]
		   if !ok {
			  return AttestSnapshot{}, ErrAttest
		  }
		   // TTL 边：过期即 fail-closed
		  if !c.Expiry.IsZero() && !c.Expiry.After(now) {
			  return AttestSnapshot{},
				   ErrExpired
		   }
		  checks = append(checks, c.Copy())
		  if minExpiry.IsZero() || (!c.Expiry.IsZero() && c.Expiry.Before(minExpiry)) {
			   minExpiry = c.Expiry
		  }
	   }
	   if minExpiry.IsZero() {
		  // 所有证明均无 Expiry 字段——防御性拒绝
		  return AttestSnapshot{},
			   ErrExpired
	   }
	   snap := AttestSnapshot{
		  ID:   uuid.NewString(),
		  Kind: kind,
		  Asset: as,
		  Checks:    checks,
		  CreatedAt: now,
		  ExpiresAt: minExpiry,
	   }
	  return snap, nil
   }

// VerifySnapshot 验证快照在当前时刻是否仍然有效，并与操作的 kind/asset 匹配。
// 用于 Mint/Redeem 路径在提交账本分录前的最终门控检查。
   func (s *Service) VerifySnapshot(snap AttestSnapshot, kind Kind, asset string) error {
	   if s == nil {
		  return ErrUnavailable
	  }
	  if snap.Kind != kind {
		  return ErrKind
	  }
	  if snap.Asset != asset {
		   return ErrAsset
	   }
	   if !snap.Valid(s.now()) {
		   return ErrExpired
	   }
	   // 双重校验：快照内证明项数量必须与当前 required 一致
	  if len(snap.Checks) != len(required(kind)) {
		   return ErrAttest
	   }
	   return nil
   }
