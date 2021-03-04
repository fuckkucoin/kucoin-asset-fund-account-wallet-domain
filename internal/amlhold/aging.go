package amlhold

import (
	"fmt"
	"time"
)

// AgingScanner 定期扫描活跃 hold，按老化配置驱动 AdvanceAging。

type AgingScanner struct {
	book *Book
	cfg   AgingConfig
	clock func() time.Time
	onWarn func(h QuarantineHold, ev AgingEvent) // 可选钩子
	onEsc func(h QuarantineHold, ev AgingEvent) // 可选钩子
}

// NewAgingScanner 构造老化扫描器。

  func NewAgingScanner(book *Book, cfg AgingConfig) *AgingScanner {
	   if cfg.WarnAfter <= 0 {
		   cfg = DefaultAgingConfig
	   }
	  s := &AgingScanner{
		  book: book,
		  cfg:  cfg,
		   clock: func() time.Time { return time.Now().UTC() },
	  }
	   return s
  }

// WithClock 注入时钟。

   func (s *AgingScanner) WithClock(fn func() time.Time) *AgingScanner {
	   if s != nil && fn != nil {
		  s.clock = fn
		  if s.book != nil {
			  s.book.WithClock(fn)
		   }
	   }
	  return s
   }

  // OnWarn 注册警告阶段钩子（通知/告警）。

   func (s *AgingScanner) OnWarn(fn func(QuarantineHold, AgingEvent)) *AgingScanner {
	  if s != nil {
		  s.onWarn = fn
	  }
	   return s
  }

   // OnEscalate 注册升级阶段钩子（通知/告警）。

   func (s *AgingScanner) OnEscalate(fn func(QuarantineHold, AgingEvent)) *AgingScanner {
	   if s != nil {
		  s.onEsc = fn
	  }
	   return s
   }

  // ScanResult 一次扫描的结果汇总。

  type ScanResult struct {
	  Scanned int
	   Advanced int
	   Unchanged int
	  Errors []string
	  Events []AgingEvent
  }

  // Scan 执行一次老化扫描。
   // 遍历所有活跃 hold，按老化配置判断是否需要推进，若需要则调用 AdvanceAging。
  // 关键约束：SLA 超时不得自动将 Case 置为 Resolved（本函数不碰 Case 状态）。
  func (s *AgingScanner) Scan() ScanResult {
	  if s == nil || s.book == nil {
		   return ScanResult{}
	   }
	   now := s.clock()
	   holds := s.book.AllOpenHolds()
	  res := ScanResult{}
	  for _, h := range holds {
		   res.Scanned++
		  target := s.targetTier(h, now)
		  if target == "" {
			  res.Unchanged++
			   continue
		  }
		  updated, ev, err := s.book.AdvanceAging(h.ID, "system:aging-scanner", target, h.FenceEpoch, h.FenceToken)
		   if err != nil {
			  res.Errors = append(res.Errors, fmt.Sprintf("hold=%s: %v", h.ID, err))
			   continue
		  }
		   res.Advanced++
		   res.Events = append(res.Events, ev)
		   if s.onWarn != nil && (updated.AgingTier == AgingTierWarn || updated.AgingTier == AgingTierForceHold) {
			   s.onWarn(updated, ev)
		   }
		  if s.onEsc != nil && (updated.Status == HoldStatusEscalated || updated.Status == HoldStatusCompelReview) {
			  s.onEsc(updated, ev)
		   }
	   }
	   return res
  }

// targetTier 判断 hold 应当推进到的下一级别的原因字符串；空字符串表示不需要推进。

  func (s *AgingScanner) targetTier(h QuarantineHold, now time.Time) string {
	  age := int64(now.Sub(h.OpenedAt).Seconds())
	   switch h.Status {
	  case HoldStatusOpen:
		   if age >= s.cfg.WarnAfter {
			  return fmt.Sprintf("sla_warn: age=%ds threshold=%ds", age, s.cfg.WarnAfter)
		   }
	   case HoldStatusAging:
		   switch h.AgingTier {
		   case AgingTierWarn:
			  if age >= s.cfg.ForceHoldAfter {
				  return fmt.Sprintf("force_hold: age=%ds threshold=%ds", age, s.cfg.ForceHoldAfter)
			   }
		   case AgingTierForceHold:
			   if age >= s.cfg.EscalateAfter {
				   return fmt.Sprintf("escalate: age=%ds threshold=%ds", age, s.cfg.EscalateAfter)
			  }
		   }
	   case HoldStatusEscalated:
		   if age >= s.cfg.CompelReviewAfter {
			  return fmt.Sprintf("compel_review: age=%ds threshold=%ds", age, s.cfg.CompelReviewAfter)
		  }
	   }
	   return ""
  }

// DueHolds 返回当前时刻需要升级的 hold 列表（不执行，只查询，供 worker 日志/监控）。

   func (s *AgingScanner) DueHolds(now time.Time) []QuarantineHold {
	  if s == nil || s.book == nil {
		   return nil
	  }
	   all := s.book.AllOpenHolds()
	   out := make([]QuarantineHold, 0)
	   for _, h := range all {
		   if s.targetTier(h, now) != "" {
			   out = append(out, h)
		  }
	  }
	  return out
   }

// SLAReport SLA 状态报告（供 ops API 读取，不触发升级）。

type SLAReport struct {
	HoldID string `json:"hold_id"`
	CaseID string `json:"case_id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Status HoldStatus `json:"status"`
	AgingTier  AgingTier `json:"aging_tier"`
	AgeSeconds int64     `json:"age_seconds"`
	Overdue    bool      `json:"overdue"`
	NextTier string `json:"next_tier,omitempty"`
}

// SLAScan 只读扫描，返回所有活跃 hold 的 SLA 报告。
   func (s *AgingScanner) SLAScan(now time.Time) []SLAReport {
	  if s == nil || s.book == nil {
		  return nil
	  }
	   holds := s.book.AllOpenHolds()
	  out := make([]SLAReport, 0, len(holds))
	  for _, h := range holds {
		   age := int64(now.Sub(h.OpenedAt).Seconds())
		   overdue := age >= s.cfg.WarnAfter
		   nextTier := s.targetTier(h, now)
		  out = append(out, SLAReport{
			  HoldID: h.ID,
			  CaseID: h.CaseID,
			   UserID: h.UserID,
			  Asset:     h.Asset,
			  Status:    h.Status,
			  AgingTier: h.AgingTier,
			   AgeSeconds: age,
			  Overdue:  overdue,
			   NextTier: nextTier,
		  })
	   }
	   return out
  }
