package fiatbreak

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/metrics"
)

// AgingService 老化催办服务。
type AgingService struct {
	mu sync.RWMutex
	// events 老化事件列表（追加只写）。
	events []*AgingEvent
	// breaks 断裂服务引用（用于扫描超期断裂）。
	breaks *BreakService
	// suspense 挂账服务引用（用于扫描超期挂账）。
	suspense *SuspenseService
	clock func() time.Time
}

// NewAgingService 创建老化服务实例。

  func NewAgingService(breaks *BreakService, suspense *SuspenseService) *AgingService {
	   return &AgingService{
		  breaks:   breaks,
		   suspense: suspense,
		  clock:    func() time.Time { return time.Now().UTC() },
	  }
   }

// now 可注入时钟。
  func (a *AgingService) now() time.Time {
	  if a == nil || a.clock == nil {
		  return time.Now().UTC()
	  }
	  return a.clock()
   }

// Scan 扫描所有 open 断裂与 open 挂账，对超期条目生成老化事件。
// 调用方（worker）按配置周期调用，幂等（同一条目多次超期会生成多条事件）。
// 返回本次新增的老化事件列表。

   func (a *AgingService) Scan() []AgingEvent {
	   if a == nil {
		  return nil
	   }
	  now := a.now()
	  var generated []AgingEvent
	  // 扫描断裂
	  if a.breaks != nil {
		   open := a.breaks.List("", "", 1000)
		  for _, br := range open {
			   if BreakTerminal(br.State) {
				   continue
			   }
			   policy := a.policyFor(string(br.Kind), "")
			  if policy == nil {
				  continue
			  }
			   age := now.Sub(br.CreatedAt)
			   maxAge := time.Duration(policy.MaxAgeSec) * time.Second
			   if age <= maxAge {
				  continue
			   }
			   ev := a.appendEvent(AgingEvent{
				   BreakID: br.ID,
				   PolicyID:     policy.ID,
				  EscalateDesk: policy.EscalateDesk,
				  AgeSeconds:   int64(age.Seconds()),
				  Note:         fmt.Sprintf("break %s (kind=%s) aged %.1fh > SLA %.1fh", br.ID, br.Kind, age.Hours(), maxAge.Hours()),
			   }, now)
			   generated = append(generated, ev)
			  // 自动核销（小差额）
			  if policy.AutoWriteOff && policy.WriteOffThreshold != "" {
				   delta, err := decimal.NewFromString(br.Delta)
				   if err == nil {
					  threshold, err2 := decimal.NewFromString(policy.WriteOffThreshold)
					   if err2 == nil && absDecimal(delta).LessThanOrEqual(threshold) {
						  _, _ = a.breaks.WriteOff(context.Background(), br.ID, "aging-robot",
							  fmt.Sprintf("auto write-off: delta %s ≤ threshold %s", delta, threshold))
					   }
				  }
			   } else if br.State == BreakOpen {
				  // 自动分配到桌面
				   _, _ = a.breaks.Investigate(br.ID, policy.EscalateDesk, "aging-robot")
			  }
		   }
	   }
	  // 扫描挂账泄漏
	  if a.suspense != nil {
		   leaked := a.suspense.LeakedEntries(now)
		   for _, e := range leaked {
			  policy := a.policyFor("", string(e.Reason))
			  desk := "break_desk"
			  policyID := "default-suspense-sla"
			  if policy != nil {
				   desk = policy.EscalateDesk
				   policyID = policy.ID
			   }
			  ageSeconds := int64(now.Sub(e.CreatedAt).Seconds())
			   ev := a.appendEvent(AgingEvent{
				   SuspenseID: e.ID,
				  PolicyID:   policyID,
				  EscalateDesk: desk,
				  AgeSeconds: ageSeconds,
				   Note: fmt.Sprintf("suspense %s (reason=%s) leaked: past SLA deadline %s",
					   e.ID, e.Reason, e.SLADeadline.Format(time.RFC3339)),
			   }, now)
			  generated = append(generated, ev)
			   metrics.FiatAgingEscalate.WithLabelValues(string(e.Reason), desk).Inc()
		   }
	  }
	   for _, br := range generated {
		   if br.BreakID != "" {
			   metrics.FiatAgingEscalate.WithLabelValues(br.PolicyID, br.EscalateDesk).Inc()
		   }
	   }
	  return generated
  }

// appendEvent 追加老化事件到列表，返回事件副本。
  func (a *AgingService) appendEvent(ev AgingEvent, now time.Time) AgingEvent {
	   ev.ID = uuid.NewString()
	  ev.CreatedAt = now
	  cp := ev
	   a.mu.Lock()
	  a.events = append(a.events, &cp)
	   a.mu.Unlock()
	  return cp
  }

// Events 返回老化事件列表（只读）。
   func (a *AgingService) Events(limit int) []AgingEvent {
	   if a == nil {
		  return nil
	  }
	  if limit <= 0 {
		   limit = 100
	  }
	  a.mu.RLock()
	   defer a.mu.RUnlock()
	  n := len(a.events)
	   if n > limit {
		  n = limit
	  }
	  out := make([]AgingEvent, n)
	   for i := 0; i < n; i++ {
		  out[i] = *a.events[len(a.events)-n+i]
	  }
	   return out
   }

// EventsByBreak 返回指定断裂的所有老化事件。
  func (a *AgingService) EventsByBreak(breakID string) []AgingEvent {
	   if a == nil {
		   return nil
	  }
	  breakID = strings.TrimSpace(breakID)
	   a.mu.RLock()
	  defer a.mu.RUnlock()
	  var out []AgingEvent
	  for _, ev := range a.events {
		   if ev.BreakID == breakID {
			   out = append(out, *ev)
		   }
	   }
	  return out
   }

// policyFor 按断裂类型和挂账原因查找最匹配策略。
// 精确匹配先；breakKind 和 suspenseReason 均空时匹配通配策略。

   func (a *AgingService) policyFor(breakKind, suspenseReason string) *AgingPolicy {
	  if a.breaks == nil {
		  return nil
	   }
	   // 从 breaks 获取策略列表
	   policies := a.breaks.Policies()
	   // 精确匹配
	  for i := range policies {
		  p := &policies[i]
		   if breakKind != "" && p.BreakKind == breakKind {
			   return p
		   }
		   if suspenseReason != "" && p.SuspenseReason == suspenseReason {
			   return p
		  }
	   }
	  // 通配（两者均空）
	   for i := range policies {
		  p := &policies[i]
		   if p.BreakKind == "" && p.SuspenseReason == "" {
			   return p
		   }
	   }
	   return nil
  }
