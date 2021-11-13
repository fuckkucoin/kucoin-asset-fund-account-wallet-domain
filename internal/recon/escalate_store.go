package recon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

const KindEscalation = "aa_recon_escalation"

// EscalationRecord 一次 SLA 升级落库记录。

type EscalationRecord struct {
	TicketID string `json:"ticket_id"`
	UserID   string `json:"user_id"`
	OpenedBy string `json:"opened_by"`
	Actor string `json:"actor"`
	Age   time.Duration `json:"age"`
	Level int           `json:"level"`
	At    time.Time     `json:"at"`
	Note string `json:"note,omitempty"`
}

// EscalationStore 升级记录 KV 仓储。

type EscalationStore struct {
	kv persist.KV
}

   func NewEscalationStore(kv persist.KV) *EscalationStore {
	   return &EscalationStore{kv: kv}
  }

// Save 追加一条升级记录；存储不可用返回 ErrStore。
   func (s *EscalationStore) Save(ctx context.Context, r EscalationRecord) error {
	  if s == nil || s.kv == nil {
		  return ErrStore
	   }
	  if r.At.IsZero() {
		   r.At = time.Now().UTC()
	   }
	   if r.Level <= 0 {
		  r.Level = 1
	  }
	   key := fmt.Sprintf("%d:%s", r.At.UnixNano(), strings.TrimSpace(r.TicketID))
	   body, _ := json.Marshal(r)
	   return s.kv.Put(ctx, persist.Row{Kind: KindEscalation, Key: key, Body: body}, persist.Fence{})
  }

// List 最近 limit 条升级记录（升序，截尾）。

  func (s *EscalationStore) List(ctx context.Context, limit int) []EscalationRecord {
	   if s == nil || s.kv == nil {
		  return nil
	  }
	  if limit <= 0 || limit > 200 {
		   limit = 50
	  }
	   rows, err := s.kv.List(ctx, KindEscalation, limit*2)
	  if err != nil || len(rows) == 0 {
		   return nil
	   }
	  out := make([]EscalationRecord, 0, len(rows))
	  for _, r := range rows {
		  var rec EscalationRecord
		   if json.Unmarshal(r.Body, &rec) != nil {
			   continue
		  }
		   out = append(out, rec)
	   }
	   if len(out) > limit {
		   out = out[len(out)-limit:]
	  }
	   return out
  }

// EscalatePersist 扫描超 SLA 工单并落升级记录；不改工单状态、不 Clear 熔断。
// SLA 超时升级的读写齐备。actor 不可为 system；写失败返回已收集条目与错误。重复调用会追加多条同票升级，值班以 Level/At 辨别。

  func (w *Workflow) EscalatePersist(ctx context.Context, now time.Time, actor string, store *EscalationStore) ([]EscalationRecord, error) {
	   esc, err := w.Escalate(now, actor)
	   if err != nil {
		   return nil, err
	  }
	   out := make([]EscalationRecord, 0, len(esc))
	  for _, e := range esc {
		  rec := EscalationRecord{
			   TicketID: e.TicketID,
			   OpenedBy: e.OpenedBy,
			   Actor: strings.TrimSpace(actor),
			   Age: e.Age,
			   Level: EscalationLevel(e.Age,
				   w.SLA()),
			  At:   now,
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
