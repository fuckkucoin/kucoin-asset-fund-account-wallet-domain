package reclaim

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

const (
	kindReclaimCase = "reclaim_case"
	kindReclaimTimeline = "reclaim_timeline"
	kindReclaimSweep     = "reclaim_sweep"
	kindReclaimOccupancy = "reclaim_occupancy"
)

// Store 围绕 persist.KV 提供 ReclaimCase 的读写能力。

type Store struct {
	kv persist.KV
}

// NewStore 构造持久化层；传入 nil 时使用内存 KV（用）。
   func NewStore(kv persist.KV) *Store {
	   if kv == nil {
		  kv = persist.NewMemKV()
	   }
	  return &Store{kv: kv}
  }

// ─── 主案件 ───────────────────────────────────────────────────────────────────
// SaveCase 写入主案件（围栏 CAS）；若 fence 过期返回 ErrFence。

   func (s *Store) SaveCase(ctx context.Context, c ReclaimCase) (ReclaimCase, error) {
	  if s == nil || s.kv == nil {
		   return ReclaimCase{}, ErrUnavailable
	   }
	  body, err := json.Marshal(c)
	  if err != nil {
		  return ReclaimCase{}, fmt.Errorf("reclaim: marshal case: %w", err)
	  }
	   werr := s.kv.InTx(ctx, func(ctx context.Context) error {
		  cur, ferr := s.kv.Get(ctx, kindReclaimCase, c.ID)
		  exp := persist.Fence{}
		  if ferr == nil {
			  exp = cur.Fence
		   } else if !errors.Is(ferr, persist.ErrNotFound) {
			  return ferr
		   }
		  return s.kv.Put(ctx, persist.Row{Kind: kindReclaimCase, Key: c.ID, Body: body}, exp)
	   })
	   if werr != nil {
		   if errors.Is(werr, persist.ErrStaleFence) {
			   return ReclaimCase{}, ErrFence
		  }
		   return ReclaimCase{}, werr
	   }
	  saved, err := s.GetCase(ctx, c.ID)
	  if err != nil {
		   return c, nil
	  }
	  return saved, nil
  }

// GetCase 读取单个案件；不存在时返回 ErrNotFound。

  func (s *Store) GetCase(ctx context.Context, id string) (ReclaimCase, error) {
	  if s == nil || s.kv == nil {
		  return ReclaimCase{}, ErrUnavailable
	  }
	   row, err := s.kv.Get(ctx, kindReclaimCase, id)
	   if err != nil {
		  if errors.Is(err, persist.ErrNotFound) {
			   return ReclaimCase{}, ErrNotFound
		  }
		  return ReclaimCase{}, err
	  }
	   var c ReclaimCase
	  if err := json.Unmarshal(row.Body, &c); err != nil {
		   return ReclaimCase{}, fmt.Errorf("reclaim: unmarshal case: %w", err)
	   }
	  return c, nil
   }

// ListCases 按游标翻页返回所有案件（仅用于启动时 Hydrate 和；应使用分页 SQL）。

   func (s *Store) ListCases(ctx context.Context) ([]ReclaimCase, error) {
	   if s == nil || s.kv == nil {
		  return nil, ErrUnavailable
	   }
	  var out []ReclaimCase
	  after := ""
	   for {
		   rows, err := s.kv.ListAfter(ctx, kindReclaimCase, after, 200)
		   if err != nil {
			  return nil, err
		   }
		   for _, row := range rows {
			  var c ReclaimCase
			   if err := json.Unmarshal(row.Body, &c); err != nil {
				   continue
			  }
			  out = append(out, c)
			  after = row.Key
		  }
		  if len(rows) < 200 {
			   break
		   }
	  }
	  return out, nil
   }

// ─── 时间轴 ───────────────────────────────────────────────────────────────────
// AppendTimeline 追加时间轴事件。
  func (s *Store) AppendTimeline(ctx context.Context, ev TimelineEvent) error {
	   if s == nil || s.kv == nil {
		  return ErrUnavailable
	  }
	  body, err := json.Marshal(ev)
	   if err != nil {
		   return fmt.Errorf("reclaim: marshal timeline: %w", err)
	  }
	   key := fmt.Sprintf("%s:%06d", ev.CaseID, ev.Seq)
	   return s.kv.Put(ctx, persist.Row{Kind: kindReclaimTimeline, Key: key, Body: body}, persist.Fence{})
  }

// ListTimeline 返回案件所有时间轴事件（按 seq 顺序）。
   func (s *Store) ListTimeline(ctx context.Context, caseID string) ([]TimelineEvent, error) {
	  if s == nil || s.kv == nil {
		   return nil, ErrUnavailable
	  }
	   prefix := caseID + ":"
	   after := ""
	  var out []TimelineEvent
	  for {
		  rows, err := s.kv.ListAfter(ctx, kindReclaimTimeline, after, 200)
		  if err != nil {
			  return nil, err
		  }
		  for _, row := range rows {
			  if !strings.HasPrefix(row.Key, prefix) {
				   continue
			  }
			   var ev TimelineEvent
			  if err := json.Unmarshal(row.Body, &ev); err != nil {
				  continue
			   }
			   out = append(out, ev)
			  after = row.Key
		  }
		  if len(rows) < 200 {
			  break
		  }
	   }
	   return out, nil
   }

// ─── Sweep leg 目录 ───────────────────────────────────────────────────────────
// SaveSweepGroup 写入 SweepLegGroup。

  func (s *Store) SaveSweepGroup(ctx context.Context, g SweepLegGroup) error {
	  if s == nil || s.kv == nil {
		  return ErrUnavailable
	  }
	   body, err := json.Marshal(g)
	   if err != nil {
		   return fmt.Errorf("reclaim: marshal sweep group: %w", err)
	   }
	  return s.kv.Put(ctx, persist.Row{Kind: kindReclaimSweep, Key: g.CaseID, Body: body}, persist.Fence{})
  }

// GetSweepGroup 读取案件的 Sweep leg 目录。
   func (s *Store) GetSweepGroup(ctx context.Context, caseID string) (SweepLegGroup, error) {
	  if s == nil || s.kv == nil {
		   return SweepLegGroup{}, ErrUnavailable
	  }
	  row,
		   err := s.kv.Get(ctx, kindReclaimSweep, caseID)
	  if err != nil {
		  if errors.Is(err, persist.ErrNotFound) {
			   return SweepLegGroup{}, ErrNotFound
		  }
		   return SweepLegGroup{}, err
	  }
	  var g SweepLegGroup
	   if err := json.Unmarshal(row.Body, &g); err != nil {
		  return SweepLegGroup{}, fmt.Errorf("reclaim: unmarshal sweep group: %w", err)
	   }
	  return g, nil
   }

// ─── Occupancy 持久化（进程重启恢复用） ──────────────────────────────────────
// SaveOccupancy 持久化单条占用记录，供重启后恢复内存状态用。
   func (s *Store) SaveOccupancy(ctx context.Context, rec occupyRecord) error {
	  if s == nil || s.kv == nil {
		  return ErrUnavailable
	   }
	   body, err := json.Marshal(rec)
	  if err != nil {
		   return fmt.Errorf("reclaim: marshal occupancy: %w", err)
	   }
	  return s.kv.Put(ctx,
		  persist.Row{Kind: kindReclaimOccupancy, Key: rec.OutputKey, Body: body}, persist.Fence{})
  }

// DeleteOccupancy 删除占用记录（案件终态后清理）。
   func (s *Store) DeleteOccupancy(ctx context.Context, outputKey string) error {
	   if s == nil || s.kv == nil {
		   return ErrUnavailable
	  }
	   row,
		  err := s.kv.Get(ctx, kindReclaimOccupancy, outputKey)
	   if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			   return nil
		  }
		  return err
	  }
	  return s.kv.Delete(ctx, kindReclaimOccupancy, outputKey, row.Fence)
   }

// ListOccupancies 列出所有占用记录（重启后恢复内存用）。
   func (s *Store) ListOccupancies(ctx context.Context) ([]occupyRecord, error) {
	   if s == nil || s.kv == nil {
		  return nil, ErrUnavailable
	  }
	  var out []occupyRecord
	  after := ""
	  for {
		   rows, err := s.kv.ListAfter(ctx, kindReclaimOccupancy, after, 200)
		   if err != nil {
			  return nil, err
		   }
		   for _, row := range rows {
			   var rec occupyRecord
			   if err := json.Unmarshal(row.Body, &rec); err != nil {
				   continue
			   }
			   out = append(out, rec)
			  after = row.Key
		   }
		  if len(rows) < 200 {
			  break
		  }
	   }
	   return out, nil
  }
