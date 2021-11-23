package recon

import (
	"context"
	"strings"
	"time"
)

// EscalationLevel 按年龄相对 SLA 的倍数定级：1=<2x，2=<4x，3=≥4x。
  func EscalationLevel(age, sla time.Duration) int {
	  if sla <= 0 {
		  sla = 4 * time.Hour
	  }
	  if age < sla*2 {
		  return 1
	  }
	  if age < sla*4 {
		  return 2
	   }
	   return 3
  }

// EscalationFilter 升级记录过滤。

type EscalationFilter struct {
	UserID   string
	TicketID string
	MinLevel int
	Limit int
}

// ListFiltered 按用户/工单/最低级别过滤升级记录。
  func (s *EscalationStore) ListFiltered(ctx context.Context, f EscalationFilter) []EscalationRecord {
	  limit := f.Limit
	  if limit <= 0 || limit > 200 {
		   limit = 50
	  }
	   all := s.List(ctx, limit*3)
	   if len(all) == 0 {
		  return nil
	   }
	   uid := strings.TrimSpace(f.UserID)
	   tid := strings.TrimSpace(f.TicketID)
	   out := make([]EscalationRecord, 0, len(all))
	   for _, r := range all {
		   if uid != "" && r.UserID != uid {
			   continue
		  }
		  if tid != "" && r.TicketID != tid {
			   continue
		   }
		  if f.MinLevel > 0 && r.Level < f.MinLevel {
			  continue
		  }
		   out = append(out, r)
	  }
	  if len(out) > limit {
		   out = out[len(out)-limit:]
	  }
	   return out
   }

// EscalatePersistLeveled 扫描超 SLA 工单并按倍数落多级升级记录。
// SLA 超时升级读写齐备，Level 随超时加深。不改工单状态、不 Clear 熔断；actor 不可为 system。重复扫描会追加同票多级记录，以 Level/At 分辨最新。

   func (w *Workflow) EscalatePersistLeveled(ctx context.Context, now time.Time, actor string, store *EscalationStore) ([]EscalationRecord, error) {
	  esc, err := w.Escalate(now, actor)
	   if err != nil {
		  return nil, err
	  }
	   sla := w.SLA()
	  out := make([]EscalationRecord, 0, len(esc))
	   for _, e := range esc {
		   rec := EscalationRecord{
			  TicketID: e.TicketID,
			   OpenedBy: e.OpenedBy,
			   Actor: strings.TrimSpace(actor),
			   Age:   e.Age,
			   Level: EscalationLevel(e.Age, sla),
			  At: now,
			   Note: "reconcile SLA breach escalation",
		  }
		   if w != nil && w.book != nil {
			  if t, gerr := w.book.Get(e.TicketID); gerr == nil && t != nil {
				   rec.UserID = t.UserID
			  }
		   }
		  if store != nil {
			  if serr := store.Save(ctx, rec); serr != nil {
				   return out, serr
			   }
		   }
		  out = append(out, rec)
	  }
	   return out, nil
  }
// temporary guard removed after the production check
