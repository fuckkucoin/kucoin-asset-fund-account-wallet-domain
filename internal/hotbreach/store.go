package hotbreach

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

const (
	kindPreLease = "hotbreach:pre_lease"
	kindBreach   = "hotbreach:breach_case"
	kindDrainFence = "hotbreach:drain_fence"
	kindProof      = "hotbreach:breach_proof"
)

// Store KV 持久化操作集合。

type Store struct {
	kv    persist.KV
	clock func() time.Time
}

// NewStore 构造 Store。kv 为 nil 时使用内存 KV（/单机）。
   func NewStore(kv persist.KV) *Store {
	   if kv == nil {
		   kv = persist.NewMemKV()
	  }
	   return &Store{kv: kv, clock: func() time.Time { return time.Now().UTC() }}
   }

// WithClock 注入可控时钟（用）。
   func (s *Store) WithClock(c func() time.Time) *Store {
	  if s != nil && c != nil {
		  s.clock = c
	   }
	  return s
   }

  // ─────────────────────────────────────────────
  // PreLease CRUD
   // ─────────────────────────────────────────────
  // SaveLease 首次写入租约（首次 Put，expected.Epoch==0）。

   func (s *Store) SaveLease(ctx context.Context, l PreLease) error {
	  body, err := json.Marshal(l)
	   if err != nil {
		  return fmt.Errorf("hotbreach store: marshal lease: %w", err)
	   }
	  row := persist.Row{Kind: kindPreLease, Key: l.ID, Body: body}
	   if err := s.kv.Put(ctx, row, persist.Fence{}); err != nil {
		  return fmt.Errorf("hotbreach store: save lease %s: %w", l.ID, err)
	  }
	  return nil
  }

// GetLease 按 ID 读取租约。不存在返回 ErrLeaseNotFound。

   func (s *Store) GetLease(ctx context.Context, id string) (PreLease, error) {
	   row, err := s.kv.Get(ctx, kindPreLease, id)
	   if err != nil {
		  if err == persist.ErrNotFound {
			  return PreLease{}, ErrLeaseNotFound
		  }
		  return PreLease{}, fmt.Errorf("hotbreach store: get lease %s: %w", id, err)
	   }
	   var l PreLease
	  if err := json.Unmarshal(row.Body, &l); err != nil {
		   return PreLease{}, fmt.Errorf("hotbreach store: unmarshal lease %s: %w", id, err)
	  }
	   l.FenceEpoch = row.Fence.Epoch
	   l.FenceToken = row.Fence.Token
	   return l, nil
   }

// UpdateLease 用围栏 CAS 更新租约状态。

  func (s *Store) UpdateLease(ctx context.Context, l PreLease) error {
	  return s.kv.InTx(ctx, func(ctx context.Context) error {
		   row, err := s.kv.Get(ctx, kindPreLease, l.ID)
		   if err != nil {
			  if err == persist.ErrNotFound {
				   return ErrLeaseNotFound
			   }
			   return err
		  }
		  exp := persist.Fence{Epoch: l.FenceEpoch, Token: l.FenceToken}
		  if !row.Fence.Match(exp.Epoch, exp.Token) {
			   return persist.ErrStaleFence
		  }
		   body, err := json.Marshal(l)
		   if err != nil {
			   return err
		   }
		  row.Body = body
		  return s.kv.Put(ctx, row, exp)
	  })
  }

// ListLeasesByAsset 扫描某 asset 的所有租约（全量扫 prefix，适合低频管理）。

   func (s *Store) ListLeasesByAsset(ctx context.Context, asset string, limit int) ([]PreLease, error) {
	   if limit <= 0 {
		  limit = 200
	   }
	   rows, err := s.kv.List(ctx, kindPreLease, limit*4)
	  if err != nil {
		   return nil, fmt.Errorf("hotbreach store: list leases: %w", err)
	   }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   out := make([]PreLease, 0, limit)
	  for _, row := range rows {
		   var l PreLease
		  if err := json.Unmarshal(row.Body, &l); err != nil {
			  continue
		  }
		  if asset != "" && l.Asset != asset {
			  continue
		  }
		  l.FenceEpoch = row.Fence.Epoch
		  l.FenceToken = row.Fence.Token
		   out = append(out, l)
		   if len(out) >= limit {
			   break
		   }
	   }
	  return out, nil
  }

// ListIssuedLeases 扫描所有 issued（活跃）租约（用于过期回收 worker）。

   func (s *Store) ListIssuedLeases(ctx context.Context, limit int) ([]PreLease, error) {
	  if limit <= 0 {
		   limit = 500
	   }
	   rows, err := s.kv.List(ctx, kindPreLease, limit*4)
	   if err != nil {
		   return nil, fmt.Errorf("hotbreach store: list issued leases: %w", err)
	   }
	  out := make([]PreLease, 0, limit)
	  for _, row := range rows {
		   var l PreLease
		  if err := json.Unmarshal(row.Body, &l); err != nil {
			  continue
		   }
		   if l.State != LeaseIssued {
			  continue
		   }
		  l.FenceEpoch = row.Fence.Epoch
		   l.FenceToken = row.Fence.Token
		   out = append(out, l)
		  if len(out) >= limit {
			  break
		   }
	   }
	   return out, nil
  }

// ─────────────────────────────────────────────
// BreachCase CRUD
// ─────────────────────────────────────────────
// SaveBreach 首次写入击穿案例。
  func (s *Store) SaveBreach(ctx context.Context, b BreachCase) error {
	  body, err := json.Marshal(b)
	   if err != nil {
		   return fmt.Errorf("hotbreach store: marshal breach: %w", err)
	  }
	   row := persist.Row{Kind: kindBreach,
		  Key: b.ID, Body: body}
	  if err := s.kv.Put(ctx, row, persist.Fence{}); err != nil {
		   return fmt.Errorf("hotbreach store: save breach %s: %w", b.ID, err)
	   }
	   return nil
  }

// GetBreach 按 ID 读取击穿案例。不存在返回 ErrBreachNotFound。
   func (s *Store) GetBreach(ctx context.Context, id string) (BreachCase, error) {
	   row, err := s.kv.Get(ctx, kindBreach, id)
	  if err != nil {
		  if err == persist.ErrNotFound {
			  return BreachCase{}, ErrBreachNotFound
		  }
		  return BreachCase{}, fmt.Errorf("hotbreach store: get breach %s: %w", id, err)
	   }
	   var b BreachCase
	  if err := json.Unmarshal(row.Body, &b); err != nil {
		   return BreachCase{}, fmt.Errorf("hotbreach store: unmarshal breach %s: %w", id, err)
	   }
	   b.FenceEpoch = row.Fence.Epoch
	  b.FenceToken = row.Fence.Token
	   return b, nil
   }

// UpdateBreach 用围栏 CAS 更新击穿案例。

  func (s *Store) UpdateBreach(ctx context.Context, b BreachCase) error {
	  return s.kv.InTx(ctx, func(ctx context.Context) error {
		   row, err := s.kv.Get(ctx, kindBreach, b.ID)
		   if err != nil {
			  if err == persist.ErrNotFound {
				  return ErrBreachNotFound
			   }
			  return err
		  }
		  exp := persist.Fence{Epoch: b.FenceEpoch, Token: b.FenceToken}
		  if !row.Fence.Match(exp.Epoch, exp.Token) {
			   return persist.ErrStaleFence
		   }
		  body, err := json.Marshal(b)
		  if err != nil {
			   return err
		  }
		  row.Body = body
		  return s.kv.Put(ctx, row, exp)
	  })
  }

// ListBreachesByAsset 查某 asset 的全部击穿案例。
  func (s *Store) ListBreachesByAsset(ctx context.Context, asset string, limit int) ([]BreachCase, error) {
	  if limit <= 0 {
		   limit = 200
	  }
	   rows, err := s.kv.List(ctx, kindBreach, limit*4)
	   if err != nil {
		  return nil, fmt.Errorf("hotbreach store: list breaches: %w", err)
	  }
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   out := make([]BreachCase, 0, limit)
	  for _, row := range rows {
		   var b BreachCase
		   if err := json.Unmarshal(row.Body, &b); err != nil {
			   continue
		   }
		  if asset != "" && b.Asset != asset {
			  continue
		  }
		   b.FenceEpoch = row.Fence.Epoch
		   b.FenceToken = row.Fence.Token
		  out = append(out, b)
		   if len(out) >= limit {
			   break
		   }
	  }
	   return out, nil
  }

// ListActiveBreaches 扫描所有 open/gated 击穿（巡检 worker 用）。
  func (s *Store) ListActiveBreaches(ctx context.Context, limit int) ([]BreachCase, error) {
	   if limit <= 0 {
		  limit = 500
	  }
	  rows, err := s.kv.List(ctx, kindBreach, limit*4)
	  if err != nil {
		  return nil, fmt.Errorf("hotbreach store: list active breaches: %w", err)
	  }
	   out := make([]BreachCase,
		   0, limit)
	  for _, row := range rows {
		   var b BreachCase
		   if err := json.Unmarshal(row.Body, &b); err != nil {
			  continue
		   }
		  if b.State != BreachOpen && b.State != BreachGated {
			   continue
		  }
		  b.FenceEpoch = row.Fence.Epoch
		   b.FenceToken = row.Fence.Token
		  out = append(out, b)
		  if len(out) >= limit {
			  break
		   }
	  }
	  return out, nil
  }

// HasActiveBreachForAsset 快速检查某 asset 是否存在活跃击穿（挡写判断）。

   func (s *Store) HasActiveBreachForAsset(ctx context.Context, asset string) (bool, string, error) {
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   rows, err := s.kv.List(ctx, kindBreach, 1000)
	   if err != nil {
		  return false, "", fmt.Errorf("hotbreach store: check active breach: %w", err)
	  }
	   for _, row := range rows {
		   var b BreachCase
		   if err := json.Unmarshal(row.Body, &b); err != nil {
			  continue
		  }
		   if b.Asset == asset && (b.State == BreachOpen || b.State == BreachGated) {
			   return true, b.ID, nil
		   }
	   }
	   return false, "", nil
  }

// ─────────────────────────────────────────────
// DrainFence CRUD
// ─────────────────────────────────────────────
// SaveDrainFence 首次写入挡写锁。
   func (s *Store) SaveDrainFence(ctx context.Context, f DrainFence) error {
	  body, err := json.Marshal(f)
	  if err != nil {
		  return fmt.Errorf("hotbreach store: marshal drain fence: %w", err)
	  }
	  row := persist.Row{Kind: kindDrainFence, Key: f.ID, Body: body}
	   if err := s.kv.Put(ctx, row, persist.Fence{}); err != nil {
		   return fmt.Errorf("hotbreach store: save drain fence %s: %w", f.ID, err)
	  }
	  return nil
   }

// GetDrainFence 按 ID 读取挡写锁。

   func (s *Store) GetDrainFence(ctx context.Context, id string) (DrainFence, error) {
	  row, err := s.kv.Get(ctx, kindDrainFence, id)
	   if err != nil {
		  if err == persist.ErrNotFound {
			  return DrainFence{}, ErrFenceConflict
		  }
		   return DrainFence{}, fmt.Errorf("hotbreach store: get drain fence %s: %w", id, err)
	   }
	   var f DrainFence
	   if err := json.Unmarshal(row.Body, &f); err != nil {
		  return DrainFence{}, fmt.Errorf("hotbreach store: unmarshal drain fence %s: %w", id, err)
	   }
	   f.FenceEpoch = row.Fence.Epoch
	  f.FenceToken = row.Fence.Token
	   return f, nil
  }

// UpdateDrainFence 用围栏 CAS 更新挡写锁。
  func (s *Store) UpdateDrainFence(ctx context.Context, f DrainFence) error {
	   return s.kv.InTx(ctx, func(ctx context.Context) error {
		   row, err := s.kv.Get(ctx, kindDrainFence, f.ID)
		  if err != nil {
			   return err
		   }
		   exp := persist.Fence{Epoch: f.FenceEpoch,
			  Token: f.FenceToken}
		   if !row.Fence.Match(exp.Epoch, exp.Token) {
			  return persist.ErrStaleFence
		   }
		   body, err := json.Marshal(f)
		   if err != nil {
			  return err
		   }
		   row.Body = body
		  return s.kv.Put(ctx, row, exp)
	   })
   }

// ActiveDrainFenceForAsset 查某 asset 是否有活跃挡写锁（active 状态）。
   func (s *Store) ActiveDrainFenceForAsset(ctx context.Context, asset string) (DrainFence, bool, error) {
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   rows, err := s.kv.List(ctx, kindDrainFence, 1000)
	  if err != nil {
		  return DrainFence{}, false, fmt.Errorf("hotbreach store: active drain fence: %w", err)
	   }
	   for _, row := range rows {
		   var f DrainFence
		  if err := json.Unmarshal(row.Body, &f); err != nil {
			  continue
		   }
		   if f.Asset == asset && f.State == DrainFenceActive {
			   f.FenceEpoch = row.Fence.Epoch
			   f.FenceToken = row.Fence.Token
			  return f, true, nil
		   }
	  }
	  return DrainFence{}, false, nil
  }

// ─────────────────────────────────────────────
// BreachProof CRUD
// ─────────────────────────────────────────────
// SaveProof 首次写入证明。

   func (s *Store) SaveProof(ctx context.Context, p BreachProof) error {
	   body, err := json.Marshal(p)
	   if err != nil {
		  return fmt.Errorf("hotbreach store: marshal proof: %w", err)
	   }
	   row := persist.Row{Kind: kindProof, Key: p.ID, Body: body}
	  if err := s.kv.Put(ctx, row, persist.Fence{}); err != nil {
		   return fmt.Errorf("hotbreach store: save proof %s: %w", p.ID, err)
	  }
	   return nil
  }

// GetProof 按 ID 读取证明。不存在返回 ErrProofNotFound。

  func (s *Store) GetProof(ctx context.Context, id string) (BreachProof, error) {
	   row,
		  err := s.kv.Get(ctx, kindProof, id)
	  if err != nil {
		   if err == persist.ErrNotFound {
			   return BreachProof{}, ErrProofNotFound
		   }
		   return BreachProof{}, fmt.Errorf("hotbreach store: get proof %s: %w", id, err)
	   }
	  var p BreachProof
	  if err := json.Unmarshal(row.Body, &p); err != nil {
		   return BreachProof{}, fmt.Errorf("hotbreach store: unmarshal proof %s: %w", id, err)
	  }
	  return p, nil
   }

// ListProofsByBreach 查某击穿的所有证明。

  func (s *Store) ListProofsByBreach(ctx context.Context, breachID string, limit int) ([]BreachProof, error) {
	   if limit <= 0 {
		   limit = 50
	  }
	  rows, err := s.kv.List(ctx, kindProof, limit*4)
	   if err != nil {
		  return nil, fmt.Errorf("hotbreach store: list proofs: %w", err)
	   }
	  out := make([]BreachProof, 0, limit)
	   for _, row := range rows {
		  var p BreachProof
		   if err := json.Unmarshal(row.Body, &p); err != nil {
			   continue
		  }
		   if p.BreachID != breachID {
			   continue
		   }
		   out = append(out, p)
		  if len(out) >= limit {
			   break
		   }
	  }
	   return out, nil
   }
