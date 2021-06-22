package fundmat

import (
	"context"
	"sort"
	"sync"
	"time"
)

// Mem 进程内投影。适合读面回归与未接 SQL 的进程。
type Mem struct {
	mu sync.Mutex
	events   map[string]FundingEvent
	byStream map[Stream][]FundingEvent
	applied  map[string]struct{}
	appliedN map[Stream][]int64
	rows     map[string]MatRow
	wm       map[Stream]MatWatermark
	fences map[string]ReadFence
	vers    map[string]ViewVersion
	verSeq  map[string]int64
	jobs    map[string]RebuildJob
	alerts  map[string]Alert
	sources map[string]time.Time
	heads   map[Stream]int64
}

// NewMem 空仓。三条事实流的纪元从 1 起。
  func NewMem() *Mem {
	   m := &Mem{
		   events:   map[string]FundingEvent{},
		  byStream: map[Stream][]FundingEvent{},
		  applied:  map[string]struct{}{},
		  appliedN: map[Stream][]int64{},
		   rows:     map[string]MatRow{},
		  wm:     map[Stream]MatWatermark{},
		  fences: map[string]ReadFence{},
		  vers:   map[string]ViewVersion{},
		   verSeq: map[string]int64{},
		  jobs:   map[string]RebuildJob{},
		   alerts: map[string]Alert{},
		   sources: map[string]time.Time{},
		   heads: map[Stream]int64{},
	   }
	  now := time.Now().UTC()
	  for _, st := range Streams() {
		   m.wm[st] = MatWatermark{Stream: st,
			  Epoch: 1, WithinSLA: true, UpdatedAt: now}
	  }
	  return m
   }

  func (m *Mem) guard() error {
	   if m == nil {
		   return ErrUnavailable
	  }
	   return nil
  }

// Append 写入事件日志并抬高流头。同一 ID 返回已存在的事件，不分配新序号。
  func (m *Mem) Append(_ context.Context, ev FundingEvent) (FundingEvent, error) {
	  if err := m.guard(); err != nil {
		  return FundingEvent{}, err
	  }
	   if err := ev.Validate(); err != nil {
		   return FundingEvent{}, err
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if old,
		  ok := m.events[ev.ID]; ok {
		  return old, nil
	   }
	  if ev.Seq == 0 {
		   ev.Seq = m.heads[ev.Stream] + 1
	   }
	  if ev.Seq <= 0 {
		   return FundingEvent{},
			   ErrBadEvent
	   }
	   for _, exist := range m.byStream[ev.Stream] {
		  if exist.Seq == ev.Seq {
			  return FundingEvent{}, ErrSeqConflict
		   }
	  }
	  m.events[ev.ID] = ev
	  m.byStream[ev.Stream] = append(m.byStream[ev.Stream], ev)
	   sort.Slice(m.byStream[ev.Stream], func(i, j int) bool {
		   return m.byStream[ev.Stream][i].Seq < m.byStream[ev.Stream][j].Seq
	   })
	   if ev.Seq > m.heads[ev.Stream] {
		  m.heads[ev.Stream] = ev.Seq
	  }
	   return ev, nil
   }

// Pending 按序号返回尚未应用的事件。

  func (m *Mem) Pending(_ context.Context, stream Stream, limit int) ([]FundingEvent, error) {
	   if err := m.guard(); err != nil {
		  return nil, err
	  }
	  if limit <= 0 {
		   limit = 100
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]FundingEvent, 0, limit)
	   for _, ev := range m.byStream[stream] {
		   if _, ok := m.applied[ev.ID]; ok {
			   continue
		   }
		   out = append(out, ev)
		   if len(out) >= limit {
			  break
		  }
	   }
	   return out, nil
   }

// ApplyOne 幂等应用一条已在日志中的事件。不在日志中的事件拒绝，避免绕过序号。
  func (m *Mem) ApplyOne(_ context.Context, ev FundingEvent, now time.Time) (ApplyOutcome, error) {
	  if err := m.guard(); err != nil {
		   return ApplyOutcome{}, err
	  }
	  if now.IsZero() {
		  now = time.Now().UTC()
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  stored, ok := m.events[ev.ID]
	   if !ok {
		  return ApplyOutcome{}, ErrNotFound
	  }
	  if _, done := m.applied[stored.ID]; done {
		  row, exists := m.rows[RowKey(stored.UserID, stored.Asset, stored.Subject)]
		   if !exists {
			   row = BlankRow(stored.UserID, stored.Asset, stored.Subject, now)
		   }
		   return ApplyOutcome{Applied: false, Row: row}, nil
	  }
	  key := RowKey(stored.UserID, stored.Asset, stored.Subject)
	   row, exists := m.rows[key]
	  if !exists {
		  row = BlankRow(stored.UserID, stored.Asset, stored.Subject, now)
	  }
	  next, err := Mutate(row, stored)
	  if err != nil {
		   return ApplyOutcome{}, err
	  }
	   next.RowVersion = row.RowVersion + 1
	  next.UpdatedAt = now.UTC()
	  next.LastEventID = stored.ID
	   if stored.CausalID != "" {
		  next.CausalID = stored.CausalID
	  }
	  m.rows[key] = next
	   m.applied[stored.ID] = struct{}{}
	  m.appliedN[stored.Stream] = append(m.appliedN[stored.Stream], stored.Seq)
	  return ApplyOutcome{Applied: true, Row: next}, nil
  }

   func (m *Mem) GetRow(_ context.Context, user, asset, subject string) (MatRow, bool, error) {
	  if err := m.guard(); err != nil {
		  return MatRow{}, false, err
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   row, ok := m.rows[RowKey(user, asset, subject)]
	  return row, ok, nil
   }

  func (m *Mem) PutRow(_ context.Context, row MatRow) error {
	   if err := m.guard(); err != nil {
		  return err
	  }
	   row.UserID = NormUser(row.UserID)
	  row.Asset = NormAsset(row.Asset)
	   row.Subject = NormSubject(row.Subject)
	   if row.UserID == "" || row.Asset == "" || row.Subject == "" {
		   return ErrBadEvent
	  }
	  row = fillZero(row)
	   if err := row.recompute(); err != nil {
		   return err
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   m.rows[row.Key()] = row
	  return nil
   }

  func (m *Mem) ListUser(_ context.Context, user string) ([]MatRow, error) {
	  if err := m.guard(); err != nil {
		   return nil, err
	  }
	  user = NormUser(user)
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]MatRow, 0)
	  for _, row := range m.rows {
		  if row.UserID == user {
			  out = append(out, row)
		  }
	  }
	   sort.Slice(out, func(i, j int) bool {
		  if out[i].Asset == out[j].Asset {
			  return out[i].Subject < out[j].Subject
		  }
		  return out[i].Asset < out[j].Asset
	   })
	  return out, nil
  }

   func (m *Mem) ListUsers(_ context.Context, limit int) ([]string, error) {
	   if err := m.guard(); err != nil {
		  return nil, err
	  }
	   if limit <= 0 {
		  limit = 100
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   seen := map[string]struct{}{}
	   out := make([]string, 0)
	  for _, row := range m.rows {
		   if _, ok := seen[row.UserID]; ok {
			  continue
		   }
		   seen[row.UserID] = struct{}{}
		   out = append(out, row.UserID)
	  }
	   sort.Strings(out)
	  if len(out) > limit {
		   out = out[:limit]
	   }
	  return out, nil
   }

   func (m *Mem) AppliedSeqs(_ context.Context, stream Stream) ([]int64, error) {
	  if err := m.guard(); err != nil {
		   return nil, err
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   src := m.appliedN[stream]
	  out := append([]int64(nil), src...)
	   return out, nil
   }

  func (m *Mem) HeadSeq(_ context.Context, stream Stream) (int64, error) {
	   if err := m.guard(); err != nil {
		  return 0, err
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   return m.heads[stream], nil
   }

  func (m *Mem) PutWM(_ context.Context, wm MatWatermark) error {
	   if err := m.guard(); err != nil {
		   return err
	   }
	  if !wm.Stream.Valid() {
		   return ErrBadEvent
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  m.wm[wm.Stream] = wm
	  return nil
   }

  func (m *Mem) ListWM(_ context.Context) ([]MatWatermark, error) {
	  if err := m.guard(); err != nil {
		   return nil, err
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  out := make([]MatWatermark, 0, len(Streams()))
	   for _, st := range Streams() {
		  wm, ok := m.wm[st]
		   if !ok {
			   wm = MatWatermark{Stream: st, Epoch: 1, WithinSLA: true}
		  }
		  out = append(out, wm)
	   }
	   return out, nil
   }

// SealUserBalance 把该用户尚未应用的余额类事件标成已应用，不再叠加到重建后的绝对栏。
   func (m *Mem) SealUserBalance(_ context.Context, userID string, at time.Time) (int, error) {
	  if err := m.guard(); err != nil {
		   return 0, err
	   }
	  userID = NormUser(userID)
	   if at.IsZero() {
		  at = time.Now().UTC()
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   n := 0
	   for id, ev := range m.events {
		   if ev.UserID != userID || !ev.Op.AffectsBalance() {
			  continue
		   }
		  if _, ok := m.applied[id]; ok {
			   continue
		   }
		  m.applied[id] = struct{}{}
		   m.appliedN[ev.Stream] = append(m.appliedN[ev.Stream], ev.Seq)
		   n++
	  }
	  _ = at
	  return n, nil
  }

   func (m *Mem) PutFence(_ context.Context, f ReadFence) error {
	   if err := m.guard(); err != nil {
		   return err
	   }
	   if f.ID == "" {
		  return ErrBadEvent
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  m.fences[f.ID] = f
	  return nil
   }

  func (m *Mem) GetFence(_ context.Context, id string) (ReadFence, error) {
	  if err := m.guard(); err != nil {
		   return ReadFence{}, err
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   f, ok := m.fences[id]
	   if !ok {
		   return ReadFence{}, ErrNotFound
	  }
	  return f, nil
  }

   func (m *Mem) PutVersion(_ context.Context, v ViewVersion) error {
	  if err := m.guard(); err != nil {
		  return err
	   }
	   if v.UserID == "" && v.Scope != "global" {
		  return ErrBadEvent
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   key := versionKey(v.Scope, v.UserID)
	   if v.Version <= 0 {
		   m.verSeq[key]++
		  v.Version = m.verSeq[key]
	  } else if v.Version > m.verSeq[key] {
		   m.verSeq[key] = v.Version
	   }
	  m.vers[key] = v
	  return nil
   }

   func versionKey(scope, user string) string {
	   if scope == "" {
		   scope = "user"
	  }
	   return scope + "|" + NormUser(user)
  }

   func (m *Mem) LatestVersion(_ context.Context, userID string) (ViewVersion, error) {
	  if err := m.guard(); err != nil {
		  return ViewVersion{}, err
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if v, ok := m.vers[versionKey("user", userID)]; ok {
		  return v, nil
	  }
	   return ViewVersion{}, ErrNotFound
   }

   func (m *Mem) PutJob(_ context.Context, j RebuildJob) error {
	   if err := m.guard(); err != nil {
		  return err
	   }
	  if j.ID == "" || !j.Status.Valid() {
		  return ErrBadEvent
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if _, ok := m.jobs[j.ID]; ok {
		  return ErrDuplicate
	   }
	  m.jobs[j.ID] = j
	  return nil
  }

  func (m *Mem) GetJob(_ context.Context, id string) (RebuildJob, error) {
	   if err := m.guard(); err != nil {
		  return RebuildJob{}, err
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  j, ok := m.jobs[id]
	  if !ok {
		  return RebuildJob{}, ErrNotFound
	   }
	   return j, nil
  }

  func (m *Mem) ListJobs(_ context.Context, status JobStatus, limit int) ([]RebuildJob, error) {
	   if err := m.guard(); err != nil {
		  return nil, err
	  }
	  if limit <= 0 {
		   limit = 50
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]RebuildJob, 0)
	  for _, j := range m.jobs {
		   if status != "" && j.Status != status {
			   continue
		  }
		   out = append(out, j)
	   }
	  sort.Slice(out, func(i, j int) bool { return out[i].QueuedAt.Before(out[j].QueuedAt) })
	  if len(out) > limit {
		  out = out[:limit]
	  }
	  return out, nil
  }

   func (m *Mem) CasJob(_ context.Context, id string, from JobStatus, next RebuildJob) (RebuildJob, error) {
	  if err := m.guard(); err != nil {
		   return RebuildJob{}, err
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  cur, ok := m.jobs[id]
	  if !ok {
		  return RebuildJob{}, ErrNotFound
	   }
	  if cur.Status != from {
		  return cur, ErrTransition
	  }
	   if err := CanStep(from, next.Status); err != nil {
		   return cur, err
	   }
	  next.ID = id
	   if next.Scope == "" {
		  next.Scope = cur.Scope
	   }
	  if next.UserID == "" {
		  next.UserID = cur.UserID
	  }
	  if next.QueuedAt.IsZero() {
		  next.QueuedAt = cur.QueuedAt
	   }
	  if next.Tolerance == "" {
		  next.Tolerance = cur.Tolerance
	  }
	  m.jobs[id] = next
	  return next, nil
   }

  func (m *Mem) PutAlert(_ context.Context, a Alert) error {
	   if err := m.guard(); err != nil {
		   return err
	  }
	   if a.ID == "" || a.Kind == "" {
		   return ErrBadEvent
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  m.alerts[a.ID] = a
	  return nil
  }

  func (m *Mem) ListAlerts(_ context.Context, openOnly bool, limit int) ([]Alert, error) {
	   if err := m.guard(); err != nil {
		  return nil, err
	   }
	  if limit <= 0 {
		   limit = 50
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]Alert, 0)
	   for _, a := range m.alerts {
		   if openOnly && !a.Open {
			   continue
		  }
		   out = append(out, a)
		   if len(out) >= limit {
			  break
		  }
	  }
	  sort.Slice(out, func(i, j int) bool { return out[i].RaisedAt.After(out[j].RaisedAt) })
	   return out, nil
   }

  func (m *Mem) SeenSource(_ context.Context, id string) (bool, error) {
	  if err := m.guard(); err != nil {
		   return false, err
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  _, ok := m.sources[id]
	   return ok, nil
  }

  func (m *Mem) MarkSource(_ context.Context, id string, at time.Time) error {
	  if err := m.guard(); err != nil {
		   return err
	   }
	  if id == "" {
		  return ErrBadEvent
	  }
	  if at.IsZero() {
		   at = time.Now().UTC()
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   m.sources[id] = at.UTC()
	   return nil
  }
