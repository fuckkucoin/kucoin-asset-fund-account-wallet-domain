package cvoccup

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

const (
	kindPathOcc = "aa_cv_path_occ"
	kindOccLeg     = "aa_cv_occ_leg"
	kindBreachEvt  = "aa_cv_breach"
	kindOccSummary = "aa_cv_occ_summary"
)

// Store 路径占用存储接口。
type Store interface {
	// GetOcc 按 ID 加载 PathOcc。
	GetOcc(ctx context.Context,
		id string) (*PathOcc, error)
	// GetOccByOrder 按 orderID 加载 PathOcc。
	GetOccByOrder(ctx context.Context, orderID string) (*PathOcc, error)
	// ListOccByUser 列出用户活跃/全部 PathOcc。
	ListOccByUser(ctx context.Context,
		userID string, activeOnly bool, limit int) ([]*PathOcc, error)
	// SaveOcc 写入或更新 PathOcc（使用乐观围栏）。
	SaveOcc(ctx context.Context, occ *PathOcc) error
	// ListLegs 列出 occID 下的所有腿。
	ListLegs(ctx context.Context, occID string) ([]*OccLeg, error)
	// SaveLeg 写入或更新 OccLeg。
	SaveLeg(ctx context.Context, leg *OccLeg) error
	// SaveBreach 写入 BreachEvent。
	SaveBreach(ctx context.Context, evt *BreachEvent) error
	// GetBreach 按 ID 加载 BreachEvent。
	GetBreach(ctx context.Context, id string) (*BreachEvent, error)
	// ListBreaches 列出未解决/全部违约事件。
	ListBreaches(ctx context.Context, unresolvedOnly bool, limit int) ([]*BreachEvent, error)
	// ResolveBreach 标记违约已解决。
	ResolveBreach(ctx context.Context, id string) error
	// ListExpiredOccs 列出已过期但非终态的路径占用。
	ListExpiredOccs(ctx context.Context, before time.Time, limit int) ([]*PathOcc, error)
	// ListActiveOccsByAsset 按资产聚合活跃占用（用于可用余额校验）。
	ListActiveOccsByAsset(ctx context.Context, userID, asset string) ([]*PathOcc, error)
}

// KVStore 基于 persist.KV 的存储实现（内存/Redis/SQL 均可对接）。
type KVStore struct {
	kv persist.KV
}

// NewKVStore 构造 KV 存储。

   func NewKVStore(kv persist.KV) *KVStore {
	  return &KVStore{kv: kv}
   }

   func (s *KVStore) GetOcc(ctx context.Context, id string) (*PathOcc, error) {
	   if s == nil || s.kv == nil {
		   return nil, ErrOccStore
	   }
	  row, err := s.kv.Get(ctx, kindPathOcc, id)
	  if err != nil {
		   if errors.Is(err, persist.ErrNotFound) {
			   return nil, ErrOccNotFound
		  }
		   return nil, err
	  }
	  var o PathOcc
	  if err := json.Unmarshal(row.Body, &o); err != nil || o.ID == "" {
		   return nil, ErrOccNotFound
	   }
	   return &o, nil
   }

   func (s *KVStore) GetOccByOrder(ctx context.Context, orderID string) (*PathOcc, error) {
	  if s == nil || s.kv == nil {
		  return nil, ErrOccStore
	   }
	   // 使用二级索引 idx|orderID → occID
	  idx, err := s.kv.Get(ctx, kindPathOcc, "idx|order|"+orderID)
	  if err != nil {
		  if errors.Is(err, persist.ErrNotFound) {
			   return nil, ErrOccNotFound
		   }
		   return nil, err
	  }
	   return s.GetOcc(ctx,
		   string(idx.Body))
  }

   func (s *KVStore) ListOccByUser(ctx context.Context, userID string, activeOnly bool, limit int) ([]*PathOcc, error) {
	   if s == nil || s.kv == nil {
		   return nil, ErrOccStore
	  }
	  if limit <= 0 {
		  limit = 50
	   }
	   scan := limit * 4
	  if scan < 200 {
		  scan = 200
	   }
	   rows, err := s.kv.List(ctx, kindPathOcc, scan)
	  if err != nil {
		  return nil, err
	  }
	  out := make([]*PathOcc, 0, limit)
	  for _, r := range rows {
		   // 跳过二级索引行
		   if len(r.Body) < 10 {
			   continue
		   }
		   var o PathOcc
		  if json.Unmarshal(r.Body, &o) != nil || o.ID == "" {
			  continue
		  }
		  if userID != "" && o.UserID != userID {
			  continue
		  }
		  if activeOnly && o.IsTerminal() {
			  continue
		  }
		  out = append(out, &o)
		   if len(out) >= limit {
			   break
		   }
	   }
	  return out, nil
  }

  func (s *KVStore) SaveOcc(ctx context.Context, occ *PathOcc) error {
	   if s == nil || s.kv == nil {
		  return ErrOccStore
	   }
	  occ.UpdatedAt = time.Now().UTC()
	  body, err := json.Marshal(occ)
	  if err != nil {
		   return err
	  }
	  exp := persist.Fence{}
	  if cur, err := s.kv.Get(ctx, kindPathOcc, occ.ID); err == nil {
		  exp = cur.Fence
	   }
	   if err := s.kv.Put(ctx, persist.Row{Kind: kindPathOcc, Key: occ.ID, Body: body}, exp); err != nil {
		   return err
	  }
	   // 写二级索引
	  if occ.OrderID != "" {
		   _ = s.kv.Put(ctx, persist.Row{Kind: kindPathOcc, Key: "idx|order|" + occ.OrderID, Body: []byte(occ.ID)}, persist.Fence{})
	  }
	   return nil
  }

  func (s *KVStore) ListLegs(ctx context.Context, occID string) ([]*OccLeg, error) {
	  if s == nil || s.kv == nil {
		  return nil, ErrOccStore
	   }
	   rows, err := s.kv.List(ctx, kindOccLeg, 50)
	  if err != nil {
		   return nil, err
	  }
	  out := make([]*OccLeg, 0)
	   for _, r := range rows {
		  var l OccLeg
		   if json.Unmarshal(r.Body, &l) != nil || l.ID == "" {
			   continue
		   }
		  if l.OccID == occID {
			  out = append(out, &l)
		   }
	  }
	  return out, nil
  }
	   if s == nil || s.kv == nil {
		  return ErrOccStore
	   }
	   body, err := json.Marshal(evt)
	   if err != nil {
		   return err
	  }
	   return s.kv.Put(ctx, persist.Row{Kind: kindBreachEvt, Key: evt.ID, Body: body}, persist.Fence{})
   }

  func (s *KVStore) GetBreach(ctx context.Context, id string) (*BreachEvent, error) {
	   if s == nil || s.kv == nil {
		   return nil, ErrOccStore
	   }
	  row, err := s.kv.Get(ctx, kindBreachEvt, id)
	  if err != nil {
		  if errors.Is(err, persist.ErrNotFound) {
			  return nil, ErrOccNotFound
		   }
		  return nil, err
	  }
	   var e BreachEvent
	  if json.Unmarshal(row.Body, &e) != nil || e.ID == "" {
		  return nil, ErrOccNotFound
	  }
	  return &e, nil
  }

  func (s *KVStore) ListBreaches(ctx context.Context, unresolvedOnly bool, limit int) ([]*BreachEvent, error) {
	   if s == nil || s.kv == nil {
		   return nil, ErrOccStore
	   }
	  if limit <= 0 {
		  limit = 50
	   }
	   rows, err := s.kv.List(ctx, kindBreachEvt, limit*3)
	  if err != nil {
		   return nil, err
	   }
	   out := make([]*BreachEvent, 0, limit)
	   for _, r := range rows {
		   var e BreachEvent
		  if json.Unmarshal(r.Body, &e) != nil || e.ID == "" {
			  continue
		  }
		   if unresolvedOnly && e.Resolved {
			   continue
		  }
		  out = append(out, &e)
		   if len(out) >= limit {
			  break
		   }
	   }
	   return out, nil
  }

   func (s *KVStore) ResolveBreach(ctx context.Context, id string) error {
	  evt, err := s.GetBreach(ctx, id)
	  if err != nil {
		   return err
	   }
	   if evt.Resolved {
		  return nil
	   }
	   now := time.Now().UTC()
	  evt.Resolved = true
	   evt.ResolvedAt = &now
	  return s.SaveBreach(ctx, evt)
   }

   func (s *KVStore) ListExpiredOccs(ctx context.Context, before time.Time, limit int) ([]*PathOcc, error) {
	  if s == nil || s.kv == nil {
		  return nil, ErrOccStore
	   }
	   if limit <= 0 {
		   limit = 100
	   }
	   rows, err := s.kv.List(ctx, kindPathOcc, limit*4)
	   if err != nil {
		   return nil, err
	   }
	   out := make([]*PathOcc, 0)
	  for _, r := range rows {
		   if len(r.Body) < 10 {
			  continue
		   }
		  var o PathOcc
		   if json.Unmarshal(r.Body, &o) != nil || o.ID == "" {
			  continue
		   }
		  if o.IsTerminal() {
			  continue
		  }
		  if o.ExpireAt.IsZero() || !o.ExpireAt.Before(before) {
			   continue
		   }
		  out = append(out, &o)
		  if len(out) >= limit {
			  break
		  }
	  }
	  return out, nil
  }

  func (s *KVStore) ListActiveOccsByAsset(ctx context.Context, userID, asset string) ([]*PathOcc, error) {
	   if s == nil || s.kv == nil {
		   return nil, ErrOccStore
	  }
	  rows, err := s.kv.List(ctx, kindPathOcc, 400)
	   if err != nil {
		  return nil, err
	   }
	  asset = NormalizeAsset(asset)
	   out := make([]*PathOcc, 0)
	   for _, r := range rows {
		   if len(r.Body) < 10 {
			   continue
		   }
		   var o PathOcc
		  if json.Unmarshal(r.Body, &o) != nil || o.ID == "" {
			  continue
		  }
		  if userID != "" && o.UserID != userID {
			  continue
		  }
		  if NormalizeAsset(o.FromAsset) != asset {
			  continue
		   }
		  if o.IsTerminal() {
			   continue
		   }
		  out = append(out, &o)
	  }
	   return out, nil
   }

// MemStore 进程内存 Store（用）。
type MemStore struct {
	mu   sync.RWMutex
	occs map[string]*PathOcc
	legs map[string]*OccLeg
	breaches map[string]*BreachEvent
	orderIdx map[string]string // orderID → occID
}

// NewMemStore 构造内存 Store。

   func NewMemStore() *MemStore {
	   return &MemStore{
		  occs: map[string]*PathOcc{},
		   legs: map[string]*OccLeg{},
		  breaches: map[string]*BreachEvent{},
		   orderIdx: map[string]string{},
	   }
   }

   func (m *MemStore) GetOcc(_ context.Context, id string) (*PathOcc, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   o, ok := m.occs[id]
	   if !ok {
		  return nil, ErrOccNotFound
	   }
	  cp := *o
	  return &cp, nil
  }

  func (m *MemStore) GetOccByOrder(_ context.Context, orderID string) (*PathOcc, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   id, ok := m.orderIdx[orderID]
	  if !ok {
		  return nil, ErrOccNotFound
	   }
	   o, ok := m.occs[id]
	   if !ok {
		   return nil, ErrOccNotFound
	  }
	  cp := *o
	   return &cp, nil
   }

   func (m *MemStore) ListOccByUser(_ context.Context, userID string, activeOnly bool, limit int) ([]*PathOcc, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   if limit <= 0 {
		   limit = 50
	  }
	   out := make([]*PathOcc, 0)
	  for _, o := range m.occs {
		   if userID != "" && o.UserID != userID {
			   continue
		  }
		  if activeOnly && o.IsTerminal() {
			  continue
		  }
		  cp := *o
		  out = append(out, &cp)
		   if len(out) >= limit {
			  break
		   }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	   return out, nil
   }

   func (m *MemStore) SaveOcc(_ context.Context, occ *PathOcc) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   occ.UpdatedAt = time.Now().UTC()
	  cp := *occ
	   m.occs[occ.ID] = &cp
	  if occ.OrderID != "" {
		   m.orderIdx[occ.OrderID] = occ.ID
	   }
	  return nil
   }

  func (m *MemStore) ListLegs(_ context.Context, occID string) ([]*OccLeg, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	  out := make([]*OccLeg, 0)
	  for _, l := range m.legs {
		  if l.OccID == occID {
			   cp := *l
			   out = append(out, &cp)
		  }
	  }
	   return out, nil
  }

   func (m *MemStore) SaveLeg(_ context.Context, leg *OccLeg) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   leg.UpdatedAt = time.Now().UTC()
	  cp := *leg
	   m.legs[leg.ID] = &cp
	   return nil
  }

  func (m *MemStore) SaveBreach(_ context.Context, evt *BreachEvent) error {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   cp := *evt
	   m.breaches[evt.ID] = &cp
	  return nil
   }

   func (m *MemStore) GetBreach(_ context.Context, id string) (*BreachEvent, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	  e, ok := m.breaches[id]
	   if !ok {
		  return nil, ErrOccNotFound
	  }
	  cp := *e
	  return &cp, nil
  }

   func (m *MemStore) ListBreaches(_ context.Context, unresolvedOnly bool, limit int) ([]*BreachEvent, error) {
	  m.mu.RLock()
	   defer m.mu.RUnlock()
	   if limit <= 0 {
		   limit = 50
	   }
	   out := make([]*BreachEvent, 0)
	   for _, e := range m.breaches {
		   if unresolvedOnly && e.Resolved {
			   continue
		  }
		   cp := *e
		  out = append(out, &cp)
		   if len(out) >= limit {
			  break
		   }
	   }
	   return out, nil
  }

  func (m *MemStore) ResolveBreach(_ context.Context, id string) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   e, ok := m.breaches[id]
	   if !ok {
		   return ErrOccNotFound
	  }
	  now := time.Now().UTC()
	  e.Resolved = true
	   e.ResolvedAt = &now
	  return nil
   }

   func (m *MemStore) ListExpiredOccs(_ context.Context, before time.Time, limit int) ([]*PathOcc, error) {
	  m.mu.RLock()
	  defer m.mu.RUnlock()
	   if limit <= 0 {
		  limit = 100
	   }
	   out := make([]*PathOcc, 0)
	  for _, o := range m.occs {
		   if o.IsTerminal() {
			  continue
		   }
		   if o.ExpireAt.IsZero() || !o.ExpireAt.Before(before) {
			  continue
		   }
		   cp := *o
		   out = append(out, &cp)
		  if len(out) >= limit {
			  break
		  }
	   }
	  return out, nil
  }

   func (m *MemStore) ListActiveOccsByAsset(_ context.Context, userID, asset string) ([]*PathOcc, error) {
	   m.mu.RLock()
	  defer m.mu.RUnlock()
	   asset = NormalizeAsset(asset)
	  out := make([]*PathOcc, 0)
	   for _, o := range m.occs {
		  if userID != "" && o.UserID != userID {
			  continue
		  }
		  if NormalizeAsset(o.FromAsset) != asset {
			  continue
		  }
		   if o.IsTerminal() {
			  continue
		   }
		   cp := *o
		  out = append(out, &cp)
	  }
	  return out, nil
   }
