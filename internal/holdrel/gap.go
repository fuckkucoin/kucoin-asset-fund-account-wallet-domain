package holdrel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

const (
	kindGap    = "aa_hold_gap"
	kindGapIdx = "aa_hold_gap_idx" // tokenID+kind → gapID
)

// GapStore 缺口单存储。
type GapStore struct {
	kv persist.KV
}

// NewGapStore 创建 GapStore。

   func NewGapStore(kv persist.KV) *GapStore {
	  return &GapStore{kv: kv}
   }

// Open 创建缺口单；同 tokenID+kind 已有 open/acked 则幂等返回。

   func (gs *GapStore) Open(ctx context.Context, gap HoldGap) (HoldGap, error) {
	   if gs == nil || gs.kv == nil {
		  return HoldGap{}, ErrStore
	   }
	   gap.UserID = strings.TrimSpace(gap.UserID)
	  gap.Asset = strings.ToUpper(strings.TrimSpace(gap.Asset))
	   if gap.UserID == "" || gap.Asset == "" {
		   return HoldGap{}, errors.New("holdrel: gap user_id and asset required")
	  }
	   if gap.Kind == "" {
		  return HoldGap{}, errors.New("holdrel: gap kind required")
	   }
	  idxKey := gapIndexKey(gap.TokenID,
		   string(gap.Kind))
	   if idxKey != "" {
		   if existing, err := gs.byIndex(ctx, idxKey); err == nil &&
			  (existing.State == GapStateOpen || existing.State == GapStateAcked) {
			   return existing, nil
		   }
	   }
	  now := time.Now().UTC()
	  gap.GapID = uuid.NewString()
	  gap.State = GapStateOpen
	   gap.OpenedAt = now
	   gap.AckedAt = nil
	  gap.ClosedAt = nil
	   if idxKey != "" {
		   if err := gs.kv.Put(ctx,
			   persist.Row{Kind: kindGapIdx, Key: idxKey, Body: []byte(gap.GapID)},
			  persist.Fence{},
		   ); err != nil {
			  // 并发竞争：读已有
			  if existing, e2 := gs.byIndex(ctx, idxKey); e2 == nil {
				  return existing, nil
			  }
			  return HoldGap{}, err
		   }
	  }
	   body, err := json.Marshal(gap)
	  if err != nil {
		  return HoldGap{}, err
	   }
	   if err := gs.kv.Put(ctx,
		  persist.Row{Kind: kindGap,
			  Key: gap.GapID, Body: body},
		   persist.Fence{},
	   ); err != nil {
		   return HoldGap{}, err
	  }
	   return gap, nil
   }

// Ack 确认缺口单（open → acked）。
   func (gs *GapStore) Ack(ctx context.Context, gapID, ackedBy string) (HoldGap, error) {
	   gap, stored, err := gs.load(ctx, gapID)
	  if err != nil {
		  return HoldGap{}, err
	   }
	   if gap.State == GapStateAcked {
		   return gap, nil // 幂等
	   }
	   if gap.State != GapStateOpen {
		   return HoldGap{},
			   ErrGapStateInvalid
	  }
	  now := time.Now().UTC()
	  gap.State = GapStateAcked
	   gap.AckedBy = ackedBy
	  gap.AckedAt = &now
	  return gs.save(ctx, gap, stored.Fence)
  }

// Close 关闭缺口单（open/acked → closed）。
   func (gs *GapStore) Close(ctx context.Context, gapID, closedBy, note string) (HoldGap, error) {
	   gap, stored, err := gs.load(ctx, gapID)
	   if err != nil {
		   return HoldGap{}, err
	   }
	   if gap.State == GapStateClosed {
		  return gap, nil // 幂等
	  }
	  now := time.Now().UTC()
	  gap.State = GapStateClosed
	   gap.ClosedBy = closedBy
	  gap.ClosedAt = &now
	   if note != "" {
		  gap.Note = appendGapNote(gap.Note, note)
	  }
	  return gs.save(ctx, gap, stored.Fence)
   }

// Get 按 GapID 查询缺口单。
   func (gs *GapStore) Get(ctx context.Context, gapID string) (HoldGap, error) {
	  gap, _, err := gs.load(ctx, gapID)
	   return gap, err
   }

// List 按 userID/state 列出缺口单；state="" 返回所有状态。
  func (gs *GapStore) List(ctx context.Context, userID string, state GapState, limit int) ([]HoldGap, error) {
	   if gs == nil || gs.kv == nil {
		   return nil, nil
	   }
	   if limit <= 0 || limit > 1000 {
		   limit = 200
	  }
	  rows, err := gs.kv.List(ctx, kindGap, limit)
	  if err != nil {
		  return nil, err
	  }
	   out := make([]HoldGap, 0)
	  for _, r := range rows {
		   var g HoldGap
		   if json.Unmarshal(r.Body, &g) != nil || g.GapID == "" {
			   continue
		   }
		  if userID != "" && g.UserID != userID {
			  continue
		  }
		   if state != "" && g.State != state {
			   continue
		  }
		   out = append(out, g)
	   }
	   return out, nil
   }

// OpenCount 返回 open 状态缺口单数（监控指标用）。

  func (gs *GapStore) OpenCount(ctx context.Context) (int, error) {
	   gaps, err := gs.List(ctx, "", GapStateOpen, 1000)
	   if err != nil {
		  return 0, err
	   }
	   return len(gaps), nil
   }

  func (gs *GapStore) byIndex(ctx context.Context, idxKey string) (HoldGap, error) {
	   r, err := gs.kv.Get(ctx, kindGapIdx, idxKey)
	   if err != nil {
		   return HoldGap{}, err
	  }
	   gap, _, err := gs.load(ctx, string(r.Body))
	  return gap, err
  }

  func (gs *GapStore) load(ctx context.Context, gapID string) (HoldGap, persist.Row, error) {
	   if gs == nil || gs.kv == nil {
		   return HoldGap{}, persist.Row{}, ErrStore
	  }
	   stored, err := gs.kv.Get(ctx, kindGap, strings.TrimSpace(gapID))
	   if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			   return HoldGap{}, persist.Row{}, ErrGapNotFound
		   }
		   return HoldGap{}, persist.Row{}, err
	  }
	  var g HoldGap
	   if json.Unmarshal(stored.Body, &g) != nil || g.GapID == "" {
		  return HoldGap{}, persist.Row{}, ErrGapNotFound
	   }
	   return g, stored, nil
   }

   func (gs *GapStore) save(ctx context.Context, gap HoldGap, fence persist.Fence) (HoldGap, error) {
	   body, err := json.Marshal(gap)
	   if err != nil {
		   return HoldGap{}, err
	   }
	  if err := gs.kv.Put(ctx,
		  persist.Row{Kind: kindGap, Key: gap.GapID, Body: body},
		  fence,
	  ); err != nil {
		   return HoldGap{}, err
	  }
	  return gap, nil
  }

  func gapIndexKey(tokenID, kind string) string {
	  if tokenID == "" {
		   return ""
	  }
	  return tokenID + "|" + kind
  }

  func appendGapNote(existing, extra string) string {
	   if existing == "" {
		   return extra
	  }
	   return existing + "; " + extra
  }
// temporary guard removed after the production check
