package destlife

import (
	"context"
	"fmt"
	"math"
	"time"
	"github.com/google/uuid"
)

const (
	// DefaultDriftThreshold 默认允许的最大时钟漂移（秒）。
	DefaultDriftThreshold = 30 * time.Second
	// ReasonClockDrift 时钟漂移绕过原因标签。
	ReasonClockDrift = "clock_drift"
	// ReasonGenMismatch 世代不一致绕过原因标签。
	ReasonGenMismatch = "gen_mismatch"
	// ReasonCooldownSkip 冷却跳过绕过原因标签（闸逻辑 bug）。
	ReasonCooldownSkip = "cooldown_skip"
)

// BypassStore 绕过证明持久化接口（append-only）。

type BypassStore interface {
	// InsertProof 写入绕过证明（proof_id 唯一；相同 entry_life_id+reason 幂等）。
	InsertProof(ctx context.Context, proof BypassProof) error
	// BlockProof 标记证明已被挡写（blocked=true）。
	BlockProof(ctx context.Context, proofID, blockedBy string, at time.Time) error
	// ListByEntry 按条目列出绕过证明。
	ListByEntry(ctx context.Context, entryLifeID string, limit int) ([]BypassProof, error)
	// ListUnblocked 列出所有未挡写的证明（巡检 Worker 调用）。
	ListUnblocked(ctx context.Context,
		limit int) ([]BypassProof, error)
}

// BypassGuard 冷却绕过巡检与证明挡写器。
type BypassGuard struct {
	store    BypassStore
	calendar *Calendar
	clock          func() time.Time
	driftThreshold time.Duration
}

// NewBypassGuard 构造 BypassGuard。driftThreshold <= 0 时使用 DefaultDriftThreshold。
  func NewBypassGuard(store BypassStore, cal *Calendar, clock func() time.Time, driftThreshold time.Duration) *BypassGuard {
	   if clock == nil {
		  clock = func() time.Time { return time.Now().UTC() }
	}
	if driftThreshold <= 0 {
		driftThreshold = DefaultDriftThreshold
	}
	return &BypassGuard{store: store, calendar: cal, clock: clock, driftThreshold: driftThreshold}
}

// CheckAndProve 出金闸放行前调用：若条目仍在冷却中却通过了放行，写入 BypassProof 并返回 ErrBypassBlocked。
// 正常情况（冷却已满）：返回 nil。
// 使用场景：出金预检最后一道屏障，捕获闸逻辑 bug 或时钟漂移。

   func (g *BypassGuard) CheckAndProve(ctx context.Context, entryLifeID, userID, chain, addr string, gatePassedAt time.Time) error {
	  if g == nil {
		  return nil
	  }
	  if g.calendar == nil {
		   return nil
	   }
	   e,
		   err := g.calendar.GetLifeByID(ctx, entryLifeID)
	   if err != nil {
		   return nil
	   }
	   now := g.clock()
	   window := g.calendar.resolveWindow(ctx, userID)
	  deadline := e.CooldownDeadline(window)
	   // 冷却已满：无绕过，放行。
	  if !now.Before(deadline) {
		  return nil
	   }
	  // 检测时钟漂移：gatePassedAt 与服务器当前时间偏差超过阈值。
	  drift := gatePassedAt.Sub(now)
	  if drift < 0 {
		   drift = -drift
	  }
	  driftMs := drift.Milliseconds()
	   proof := BypassProof{
		   ProofID:     uuid.NewString(),
		   EntryLifeID: entryLifeID,
		  UserID:      userID,
		   Chain:   chain,
		   Address: addr,
		   Reason:  ReasonCooldownSkip,
		   ClockDrift: driftMs,
		  Blocked: true,
		  BlockedBy: "bypass_guard",
		  DetectedAt: now,
		  BlockedAt: now,
	  }
	  if drift >= g.driftThreshold {
		   proof.Reason = ReasonClockDrift
	  }
	   if g.store != nil {
		   _ = g.store.InsertProof(ctx, proof)
	  }
	   return ErrBypassBlocked
   }

// ProveGenMismatch 世代不一致时写入 BypassProof 并返回 ErrBypassBlocked。
// 与 Strict 闸一致：生命周期世代 != Book 世代时必须记录证明并阻断。
   func (g *BypassGuard) ProveGenMismatch(ctx context.Context, mm *GenMismatch, userID, chain, addr string) error {
	  if g == nil || mm == nil {
		   return ErrBypassBlocked
	  }
	   now := g.clock()
	  proof := BypassProof{
		  ProofID:     uuid.NewString(),
		  EntryLifeID: mm.EntryLifeID,
		  UserID:      userID,
		   Chain:       chain,
		  Address:     addr,
		  Reason:      ReasonGenMismatch,
		  ClockDrift: 0,
		  Blocked:    true,
		  BlockedBy:  "gen_cas",
		  DetectedAt: now,
		  BlockedAt:  now,
	  }
	   if g.store != nil {
		   _ = g.store.InsertProof(ctx, proof)
	   }
	  return ErrBypassBlocked
  }

// PatrolUnblocked 巡检所有未挡写证明（Worker 定时调用）。
// 对每条证明尝试补写 blocked=true；返回处理数与错误。
   func (g *BypassGuard) PatrolUnblocked(ctx context.Context, limit int) (processed int, errs []error) {
	   if g == nil || g.store == nil {
		   return 0, nil
	   }
	  proofs, err := g.store.ListUnblocked(ctx, limit)
	  if err != nil {
		  return 0, []error{fmt.Errorf("destlife bypass patrol: list: %w", err)}
	   }
	   now := g.clock()
	  for _, p := range proofs {
		  if err := g.store.BlockProof(ctx, p.ProofID, "patrol_worker", now); err != nil {
			   errs = append(errs, fmt.Errorf("destlife bypass patrol: block proof %s: %w", p.ProofID, err))
			   continue
		  }
		   processed++
	   }
	  return processed, errs
  }

// ListProofs 列出条目所有绕过证明。

   func (g *BypassGuard) ListProofs(ctx context.Context, entryLifeID string, limit int) ([]BypassProof, error) {
	  if g == nil || g.store == nil {
		  return nil, nil
	  }
	   return g.store.ListByEntry(ctx, entryLifeID, limit)
   }

// DriftReport 时钟漂移报告（供运营 API 返回）。

type DriftReport struct {
	ProofID string `json:"proof_id"`
	EntryLife string `json:"entry_life_id"`
	DriftMs  int64   `json:"drift_ms"`
	DriftSec float64 `json:"drift_sec"`
	ThresholdS float64 `json:"threshold_sec"`
	Exceeded   bool    `json:"exceeded"`
	At string `json:"detected_at"`
}

// BuildDriftReport 将 BypassProof 转换为可读漂移报告。

  func (g *BypassGuard) BuildDriftReport(p BypassProof) DriftReport {
	  driftSec := float64(p.ClockDrift) / 1000.0
	  threshSec := g.driftThreshold.Seconds()
	   return DriftReport{
		   ProofID:   p.ProofID,
		   EntryLife: p.EntryLifeID,
		   DriftMs:   p.ClockDrift,
		   DriftSec: math.Round(driftSec*1000) / 1000,
		  ThresholdS: threshSec,
		   Exceeded: float64(p.ClockDrift) >= threshSec*1000,
		   At:       p.DetectedAt.UTC().Format(time.RFC3339),
	   }
  }
