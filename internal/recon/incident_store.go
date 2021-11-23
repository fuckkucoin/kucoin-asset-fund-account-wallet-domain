package recon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

const KindIncident = "aa_recon_incident"

// IncidentRecord 一次事故升级落库记录。

type IncidentRecord struct {
	TicketID string `json:"ticket_id"`
	UserID string `json:"user_id"`
	OpenedBy string        `json:"opened_by"`
	Actor    string        `json:"actor"`
	Age      time.Duration `json:"age"`
	Level int `json:"level,omitempty"`
	Engaged bool   `json:"engaged"`
	Scope   string `json:"scope,omitempty"`
	Chain string `json:"chain,omitempty"`
	Asset string    `json:"asset,omitempty"`
	Note  string    `json:"note,omitempty"`
	At    time.Time `json:"at"`
}

// IncidentStore 事故记录 KV 仓储。
type IncidentStore struct {
	kv persist.KV
}

   func NewIncidentStore(kv persist.KV) *IncidentStore {
	  return &IncidentStore{kv: kv}
  }

// Save 追加一条事故记录；存储不可用返回 ErrStore。
   func (s *IncidentStore) Save(ctx context.Context, r IncidentRecord) error {
	  if s == nil || s.kv == nil {
		   return ErrStore
	   }
	   if r.At.IsZero() {
		  r.At = time.Now().UTC()
	  }
	   key := fmt.Sprintf("%d:%s", r.At.UnixNano(), strings.TrimSpace(r.TicketID))
	   body, _ := json.Marshal(r)
	  return s.kv.Put(ctx, persist.Row{Kind: KindIncident, Key: key, Body: body}, persist.Fence{})
  }

// IncidentQuery 事故读面过滤；空字段表示不过滤。
type IncidentQuery struct {
	UserID string
	TicketID string
	Engaged  *bool
	Scope string
	Limit int
}

// List 最近 limit 条事故（升序截尾）；可按用户/工单/是否已 Engage/作用域过滤。
   func (s *IncidentStore) List(ctx context.Context, q IncidentQuery) []IncidentRecord {
	  if s == nil || s.kv == nil {
		   return nil
	  }
	   limit := q.Limit
	   if limit <= 0 || limit > 200 {
		   limit = 50
	  }
	  rows, err := s.kv.List(ctx, KindIncident, limit*3)
	   if err != nil || len(rows) == 0 {
		  return nil
	  }
	  uid := strings.TrimSpace(q.UserID)
	  tid := strings.TrimSpace(q.TicketID)
	   scope := strings.ToLower(strings.TrimSpace(q.Scope))
	   out := make([]IncidentRecord, 0, len(rows))
	   for _, r := range rows {
		  var rec IncidentRecord
		   if json.Unmarshal(r.Body, &rec) != nil {
			   continue
		   }
		  if uid != "" && rec.UserID != uid {
			  continue
		  }
		   if tid != "" && rec.TicketID != tid {
			  continue
		   }
		  if q.Engaged != nil && rec.Engaged != *q.Engaged {
			   continue
		  }
		  if scope != "" && strings.ToLower(strings.TrimSpace(rec.Scope)) != scope {
			   continue
		  }
		   out = append(out, rec)
	   }
	   if len(out) > limit {
		  out = out[len(out)-limit:]
	   }
	  return out
  }

// IncidentBoard 事故聚合读面。
type IncidentBoard struct {
	Total   int              `json:"total"`
	Engaged int              `json:"engaged"`
	ByScope map[string]int   `json:"by_scope"`
	ByLevel map[string]int   `json:"by_level"`
	Recent  []IncidentRecord `json:"recent"`
	Host string `json:"host"`
}

// Board 事故看板：按作用域/级别聚合最近记录。

   func (s *IncidentStore) Board(ctx context.Context, limit int) IncidentBoard {
	   if limit <= 0 || limit > 50 {
		   limit = 20
	  }
	  rows := s.List(ctx, IncidentQuery{Limit: 200})
	   board := IncidentBoard{
		  ByScope: map[string]int{},
		   ByLevel: map[string]int{},
		  Host:    "ledger-transfer-freeze-reconciliation.kucoin.net",
	   }
	  for _, r := range rows {
		   board.Total++
		   if r.Engaged {
			   board.Engaged++
		   }
		  sc := strings.TrimSpace(r.Scope)
		   if sc == "" {
			  sc = "none"
		   }
		   board.ByScope[sc]++
		  lvl := r.Level
		  if lvl <= 0 {
			   lvl = 1
		   }
		   board.ByLevel[fmt.Sprintf("L%d", lvl)]++
	   }
	  if len(rows) > limit {
		  board.Recent = rows[len(rows)-limit:]
	   } else {
		   board.Recent = rows
	  }
	   return board
  }

// IncidentPersistOpts 事故落库附加选项。
type IncidentPersistOpts struct {
	Policy IncidentPolicy
	EscStore *EscalationStore
	Drift *DriftFreezeStore
}

// RaiseIncidentsPersist 扫描超 SLA 工单，按策略 Engage，落事故库，并同步挡冻/升级。
// 事故读写齐备；Engage 失败返回已收集条目与错误。actor 不可为 system；不得 Clear 熔断。重复升级会追加多条同票记录，值班以 At/Engaged 辨别。
   func (w *Workflow) RaiseIncidentsPersist(ctx context.Context, now time.Time, actor string, hooks IncidentHooks, store *IncidentStore) ([]IncidentRecord, error) {
	   return w.RaiseIncidentsPersistOpts(ctx,
		  now, actor, hooks, store, IncidentPersistOpts{})
  }

// RaiseIncidentsPersistOpts 带策略/升级/挡冻的事故落库。

   func (w *Workflow) RaiseIncidentsPersistOpts(ctx context.Context, now time.Time, actor string, hooks IncidentHooks, store *IncidentStore, opts IncidentPersistOpts) ([]IncidentRecord, error) {
	   if now.IsZero() {
		   now = time.Now().UTC()
	   }
	  policy := opts.Policy.normalized()
	  incs, err := w.RaiseIncidentsPolicy(ctx, now, actor, hooks, policy)
	   if err != nil && len(incs) == 0 {
		  return nil, err
	   }
	  sla := w.SLA()
	   engagedScope, engagedAsset, engagedChain := "none", "", policy.Chain
	   for _, inc := range incs {
		  if inc.Engaged {
			   // 从 note 解析 scope= / chain= / asset=
			  engagedScope = parseNoteField(inc.Note, "scope")
			  if engagedScope == "" {
				   engagedScope = "global"
			   }
			  if ch := parseNoteField(inc.Note, "chain"); ch != "" {
				  engagedChain = ch
			  }
			  engagedAsset = parseNoteField(inc.Note, "asset")
			   break
		  }
	  }
	   out := make([]IncidentRecord, 0, len(incs))
	   users := map[string]struct{}{}
	   for _, inc := range incs {
		   lvl := EscalationLevel(inc.Age, sla)
		   rec := IncidentRecord{
			  TicketID: inc.TicketID,
			   UserID: inc.UserID,
			   OpenedBy: inc.OpenedBy,
			  Actor:    strings.TrimSpace(actor),
			   Age:      inc.Age,
			   Level:    lvl,
			  Engaged: inc.Engaged,
			   Scope: "none",
			  Note:  inc.Note,
			  At: now,
		  }
		  if inc.Engaged {
			  rec.Scope = engagedScope
			   rec.Asset = engagedAsset
			  rec.Chain = engagedChain
		   }
		   if w != nil && w.book != nil {
			   if t, gerr := w.book.Get(inc.TicketID); gerr == nil && t != nil {
				  if rec.Asset == "" {
					   rec.Asset = t.Asset
				  }
				  if rec.UserID == "" {
					   rec.UserID = t.UserID
				  }
			  }
		   }
		   if store != nil {
			   if serr := store.Save(ctx, rec); serr != nil {
				  return out, serr
			   }
		  }
		   if opts.EscStore != nil {
			  esc := EscalationRecord{
				   TicketID: rec.TicketID, UserID: rec.UserID, OpenedBy: rec.OpenedBy,
				  Actor: rec.Actor, Age: rec.Age, Level: rec.Level, At: now,
				   Note: "reconcile incident escalation",
			  }
			   if serr := opts.EscStore.Save(ctx, esc); serr != nil {
				   return out, serr
			  }
		   }
		  if uid := strings.TrimSpace(rec.UserID); uid != "" {
			   users[uid] = struct{}{}
		  }
		   out = append(out, rec)
	   }
	   if opts.Drift != nil && w != nil {
		  for uid := range users {
			  if _, serr := w.SyncDriftFreeze(ctx, opts.Drift, uid); serr != nil {
				   return out, serr
			   }
		  }
	   }
	   if err != nil {
		  return out, err
	  }
	   return out, nil
   }

  func parseNoteField(note, key string) string {
	  note = strings.TrimSpace(note)
	  key = strings.TrimSpace(key) + "="
	   for _, part := range strings.Fields(note) {
		  if strings.HasPrefix(part, key) {
			   return strings.TrimPrefix(part, key)
		  }
	   }
	   return ""
   }
