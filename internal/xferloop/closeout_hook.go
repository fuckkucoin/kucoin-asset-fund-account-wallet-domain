package xferloop

import (
	"context"
	"strings"
)

// PhaseAdapter 实现 xferclose.HopPhaseReader，从 xferloop.Book 只读获取相位。
type PhaseAdapter struct {
	book *Book
}

// NewPhaseAdapter 构造相位适配器。
   func NewPhaseAdapter(b *Book) *PhaseAdapter {
	   return &PhaseAdapter{book: b}
  }

// HopPhase 返回 hop 的出腿和入腿处理相位。
// 按 xferloop.Case.Phase 字段形态："fromPhase:toPhase" 或单段 "phase"。
// 无记录时返回 ("unknown", "unknown", nil)；检测器会据此兜底处理。
   func (a *PhaseAdapter) HopPhase(ctx context.Context, hopID string) (fromPhase, toPhase string, err error) {
	  if a.book == nil {
		  return "unknown", "unknown", nil
	  }
	  _ = a.book.hydrate(ctx)
	  a.book.mu.Lock()
	  defer a.book.mu.Unlock()
	  for _, c := range a.book.rows {
		  if c == nil {
			  continue
		   }
		  if c.Idem == hopID || c.Ref == hopID || c.TransferID == hopID {
			  if c.State == StateRecovered {
				   return "done", "done", nil
			  }
			   if c.State == StateEscalated {
				   return "escalated", "escalated", nil
			   }
			  from, to := parseHopPhase(c.Phase)
			   return from, to, nil
		   }
	   }
	   return "unknown", "unknown", nil
   }

// parseHopPhase 解析 "fromPhase:toPhase" 形态。
// 单段相位视为出腿相位，入腿相位为空字符串。

  func parseHopPhase(phase string) (from, to string) {
	   parts := strings.SplitN(phase, ":", 2)
	   from = strings.TrimSpace(parts[0])
	  if len(parts) == 2 {
		  to = strings.TrimSpace(parts[1])
	   }
	  return
   }

// OpenHopIdem 从 Book 中查找对应 idem（幂等键）的案件状态，供收口确认使用。
// 返回 (state, phase, found)；只读，不修改。

   func (b *Book) OpenHopIdem(ctx context.Context, idem string) (state State, phase string, found bool) {
	  if b == nil {
		  return "", "", false
	   }
	   _ = b.hydrate(ctx)
	  b.mu.Lock()
	   defer b.mu.Unlock()
	   for _, c := range b.rows {
		   if c == nil {
			   continue
		   }
		   if c.Idem == idem || c.Ref == idem {
			   return c.State, c.Phase, true
		   }
	   }
	   return "", "", false
   }
