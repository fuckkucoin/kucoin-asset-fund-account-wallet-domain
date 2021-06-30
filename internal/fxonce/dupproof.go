package fxonce

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// ────────────────────────────────────────────────────────────────────────────
// DupProofService
// ────────────────────────────────────────────────────────────────────────────
// DupProofService 提供双花证明的高层操作。

type DupProofService struct {
	store       DupProofStore
	effectStore EffectStore
	retentionDays int // 保留窗口天数（默认 90）
	clock         func() time.Time
}

// NewDupProofService 构造 DupProofService。
  func NewDupProofService(store Store) *DupProofService {
	   return &DupProofService{
		   store:       store,
		  effectStore: store,
		  retentionDays: 90,
		   clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 替换时钟（用）。

  func (s *DupProofService) WithClock(c func() time.Time) *DupProofService {
	  if s != nil && c != nil {
		  s.clock = c
	  }
	   return s
   }

  // WithRetentionDays 设置保留窗口天数（默认 90）。
  func (s *DupProofService) WithRetentionDays(days int) *DupProofService {
	   if s != nil && days > 0 {
		  s.retentionDays = days
	   }
	   return s
  }

// ────────────────────────────────────────────────────────────────────────────
// 查询接口
// ────────────────────────────────────────────────────────────────────────────
// Get 按 ID 查询 DupProof。超过保留窗口返回 ErrWindowExpired（仍返回数据）。
   func (s *DupProofService) Get(ctx context.Context, id string) (*DupProof, error) {
	  d, err := s.store.GetDupProof(ctx, id)
	  if err != nil {
		   return nil, fmt.Errorf("fxonce dupproof get: %w", err)
	  }
	  threshold := s.clock().Add(-time.Duration(s.retentionDays) * 24 * time.Hour)
	   if d.CreatedAt.Before(threshold) {
		  return d, ErrWindowExpired
	  }
	   return d, nil
   }

// ListForBusiness 按 business_key 查询双花证明列表（最多 limit 条，按 created_at 降序）。
  func (s *DupProofService) ListForBusiness(ctx context.Context, businessKey string, limit int) ([]*DupProof, error) {
	   if businessKey == "" {
		  return nil, ErrBadBusinessKey
	   }
	  if limit <= 0 || limit > 200 {
		  limit = 50
	   }
	   return s.store.ListDupProofsForBusiness(ctx,
		  businessKey, limit)
  }

// ListUnreviewed 查询未审阅的 DupProof（供 desk 日常巡查）。

   func (s *DupProofService) ListUnreviewed(ctx context.Context, limit int) ([]*DupProof, error) {
	  if limit <= 0 || limit > 500 {
		   limit = 50
	  }
	   return s.store.ListUnreviewed(ctx, limit)
  }

// MarkReviewed desk 标记已审阅。
   func (s *DupProofService) MarkReviewed(ctx context.Context, id string, reviewer string) error {
	   if id == "" {
		  return ErrEffectNotFound
	  }
	   if reviewer == "" {
		  return fmt.Errorf("fxonce: reviewer must not be empty")
	  }
	   return s.store.MarkReviewed(ctx, id, reviewer, s.clock())
  }

// ────────────────────────────────────────────────────────────────────────────
// 聚合统计
// ────────────────────────────────────────────────────────────────────────────
// DupSummary 双花事故摘要，供 desk 仪表盘使用。
type DupSummary struct {
	// BusinessKey 业务键。
	BusinessKey string `json:"business_key"`
	// TotalDups 该 business_key 产生的双花总次数。
	TotalDups int `json:"total_dups"`
	// UnreviewedDups 未审阅次数。
	UnreviewedDups int `json:"unreviewed_dups"`
	// LatestDupAt 最近一次双花时刻。
	LatestDupAt *time.Time `json:"latest_dup_at,omitempty"`
	// MaxWindowMs 最大双花时间窗口（毫秒），反映分布式时钟偏差上界。
	MaxWindowMs int64 `json:"max_window_ms"`
	// RejectCodes 各拒绝码计数。
	RejectCodes map[string]int `json:"reject_codes"`
}

// Summarize 对指定 business_key 的所有 DupProof 做聚合。
  func (s *DupProofService) Summarize(ctx context.Context, businessKey string) (*DupSummary, error) {
	   dups, err := s.store.ListDupProofsForBusiness(ctx, businessKey, 0)
	  if err != nil {
		   return nil, fmt.Errorf("fxonce dupproof summarize: %w", err)
	   }
	  if len(dups) == 0 {
		   return &DupSummary{BusinessKey: businessKey, RejectCodes: map[string]int{}}, nil
	  }
	   // 按 created_at 排序（降序）
	  sort.Slice(dups, func(i, j int) bool { return dups[i].CreatedAt.After(dups[j].CreatedAt) })
	   summary := &DupSummary{
		  BusinessKey: businessKey,
		   TotalDups: len(dups),
		   RejectCodes: map[string]int{},
	   }
	  for _, d := range dups {
		  if !d.Reviewed {
			  summary.UnreviewedDups++
		  }
		   if summary.LatestDupAt == nil || d.CreatedAt.After(*summary.LatestDupAt) {
			  t := d.CreatedAt
			   summary.LatestDupAt = &t
		  }
		  if d.WindowMs > summary.MaxWindowMs {
			  summary.MaxWindowMs = d.WindowMs
		  }
		  summary.RejectCodes[string(d.RejectCode)]++
	  }
	  return summary, nil
   }

// ────────────────────────────────────────────────────────────────────────────
// 重放验证（供 desk 人工放行前核查）
// ────────────────────────────────────────────────────────────────────────────
// ReplayCheckResult 重放前校验结果。
type ReplayCheckResult struct {
	// EffectID 目标 FundEffect.ID。
	EffectID string `json:"effect_id"`
	// CurrentState 当前 effect 状态。
	CurrentState EffectState `json:"current_state"`
	// CanForceReplay 是否可以强制重放（必须是 poisoned 状态）。
	CanForceReplay bool `json:"can_force_replay"`
	// BlockingReason 不可重放时的原因描述。
	BlockingReason string `json:"blocking_reason,omitempty"`
	// RelatedDupCount 该 business_key 关联的双花证明数量（供 desk 参考）。
	RelatedDupCount int `json:"related_dup_count"`
}

// CheckForceReplay 在 ForceReplay 前做前置校验（不写库）。

  func (s *DupProofService) CheckForceReplay(ctx context.Context, effectID string) (*ReplayCheckResult, error) {
	  e, err := s.effectStore.GetEffectByID(ctx, effectID)
	  if err != nil {
		   return nil, fmt.Errorf("fxonce: check force replay get effect: %w", err)
	   }
	  result := &ReplayCheckResult{
		  EffectID:     effectID,
		  CurrentState: e.State,
	   }
	  if e.State != EffectPoisoned {
		   result.BlockingReason = fmt.Sprintf("effect state is %s, must be poisoned for force-replay", e.State)
		   return result, nil
	  }
	   result.CanForceReplay = true
	  // 查关联 dup 数
	   dups, _ := s.store.ListDupProofsForBusiness(ctx, e.BusinessKey, 0)
	   result.RelatedDupCount = len(dups)
	  return result, nil
  }
