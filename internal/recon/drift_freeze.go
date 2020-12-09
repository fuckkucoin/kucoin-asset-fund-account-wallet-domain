package recon

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

const KindDriftFreeze = "aa_recon_drift_freeze"

// DriftFreeze 用户漂移挡冻快照。

type DriftFreeze struct {
	UserID string `json:"user_id"`
	Frozen bool   `json:"frozen"`
	OpenCount int       `json:"open_count"`
	Reason    string    `json:"reason,omitempty"`
	At        time.Time `json:"at"`
}

// DriftFreezeStore 漂移挡冻 KV 仓储。
type DriftFreezeStore struct {
	kv persist.KV
}

  func NewDriftFreezeStore(kv persist.KV) *DriftFreezeStore {
	   return &DriftFreezeStore{kv: kv}
   }

// Mark 记用户因对账差异挡冻；写失败返回错误。
   func (s *DriftFreezeStore) Mark(ctx context.Context, userID, reason string, openCount int) error {
	  if s == nil || s.kv == nil {
		   return ErrStore
	   }
	   userID = strings.TrimSpace(userID)
	  if userID == "" {
		  return ErrActor
	  }
	  row := DriftFreeze{
		   UserID: userID, Frozen: true, OpenCount: openCount,
		  Reason: strings.TrimSpace(reason), At: time.Now().UTC(),
	  }
	   body, _ := json.Marshal(row)
	   cur, err := s.kv.Get(ctx, KindDriftFreeze, userID)
	   exp := persist.Fence{}
	  if err == nil {
		  exp = cur.Fence
	  } else if err != persist.ErrNotFound {
		  // 兼容：部分实现用 errors.Is；此处仅区分未找到。
		  if !isNotFound(err) {
			  return err
		   }
	  }
	  return s.kv.Put(ctx, persist.Row{Kind: KindDriftFreeze, Key: userID, Body: body}, exp)
   }

// Clear 差异结清后解除挡冻标记；仅人工/结案路径调用。
   func (s *DriftFreezeStore) Clear(ctx context.Context, userID string) error {
	   if s == nil || s.kv == nil {
		  return ErrStore
	   }
	  userID = strings.TrimSpace(userID)
	   if userID == "" {
		  return ErrActor
	  }
	  row := DriftFreeze{UserID: userID, Frozen: false, At: time.Now().UTC()}
	   body, _ := json.Marshal(row)
	  cur, err := s.kv.Get(ctx, KindDriftFreeze, userID)
	  exp := persist.Fence{}
	  if err == nil {
		  exp = cur.Fence
	  } else if !isNotFound(err) {
		  return err
	   }
	  return s.kv.Put(ctx, persist.Row{Kind: KindDriftFreeze, Key: userID, Body: body}, exp)
   }

// Get 读用户挡冻快照；无记录返回 Frozen=false。

   func (s *DriftFreezeStore) Get(ctx context.Context, userID string) (DriftFreeze, error) {
	  if s == nil || s.kv == nil {
		   return DriftFreeze{UserID: userID}, ErrStore
	   }
	   userID = strings.TrimSpace(userID)
	  row, err := s.kv.Get(ctx, KindDriftFreeze, userID)
	   if err != nil {
		  if isNotFound(err) {
			  return DriftFreeze{UserID: userID}, nil
		  }
		  return DriftFreeze{UserID: userID},
			  err
	   }
	  var out DriftFreeze
	   _ = json.Unmarshal(row.Body, &out)
	   out.UserID = userID
	   return out, nil
  }

// ListFrozen 列出仍标记 Frozen 的用户（上限 limit）。
  func (s *DriftFreezeStore) ListFrozen(ctx context.Context, limit int) []DriftFreeze {
	  if s == nil || s.kv == nil {
		   return nil
	   }
	  if limit <= 0 || limit > 200 {
		  limit = 50
	  }
	   rows, err := s.kv.List(ctx, KindDriftFreeze, limit*2)
	   if err != nil {
		  return nil
	   }
	   out := make([]DriftFreeze, 0, len(rows))
	   for _, r := range rows {
		  var d DriftFreeze
		   if json.Unmarshal(r.Body, &d) != nil || !d.Frozen {
			   continue
		  }
		   if d.UserID == "" {
			  d.UserID = r.Key
		  }
		  out = append(out, d)
		   if len(out) >= limit {
			  break
		  }
	   }
	  return out
  }

  func isNotFound(err error) bool {
	  if err == nil {
		  return false
	  }
	  if err == persist.ErrNotFound {
		  return true
	  }
	  return strings.Contains(err.Error(), "not found")
   }

// SyncDriftFreeze 按未关差异数刷新挡冻标记；open==0 则 Clear。

   func (w *Workflow) SyncDriftFreeze(ctx context.Context, store *DriftFreezeStore, userID string) (DriftFreeze, error) {
	   st := w.DriftOf(userID)
	  if store == nil {
		  return DriftFreeze{UserID: userID, Frozen: st.Blocked, OpenCount: st.OpenCount, Reason: st.Note, At: time.Now().UTC()}, nil
	  }
	  if st.Blocked {
		   if err := store.Mark(ctx, userID, st.Note, st.OpenCount); err != nil {
			  return DriftFreeze{}, err
		  }
	   } else {
		  if err := store.Clear(ctx, userID); err != nil {
			   return DriftFreeze{}, err
		   }
	   }
	  return store.Get(ctx, userID)
  }

// DriftStatusFull 读面：内存未关数 ∪ 持久化挡冻标记。
type DriftStatusFull struct {
	DriftStatus
	Persisted *DriftFreeze `json:"persisted,omitempty"`
}

// DriftOfFull 组合内存与持久化挡冻读面。

  func (w *Workflow) DriftOfFull(ctx context.Context, store *DriftFreezeStore, userID string) DriftStatusFull {
	  base := w.DriftOf(userID)
	   out := DriftStatusFull{DriftStatus: base}
	  if store == nil || strings.TrimSpace(userID) == "" {
		  return out
	  }
	  got, err := store.Get(ctx, userID)
	  if err != nil {
		  return out
	   }
	   out.Persisted = &got
	  if got.Frozen {
		   out.Blocked = true
		   if out.Note == "" {
			   out.Note = got.Reason
		  }
	   }
	   return out
  }
