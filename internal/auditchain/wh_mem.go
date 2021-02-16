package auditchain

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// Mem 内存实现。锁只护本结构，调用方仓库锁在外层再套一层。

type Mem struct {
	mu sync.Mutex
	segs map[string]Segment
	byPart map[string][]string
	rows map[string][]Row
	idem  map[string]string
	ckpts []Checkpoint
	curs   map[string]Cursor
	alerts []Alert
	jobs   map[string]Job
	exps map[string]ExportRec
}

  func NewMem() *Mem {
	   return &Mem{
		  segs: map[string]Segment{},
		  byPart: map[string][]string{},
		  rows:   map[string][]Row{},
		   idem:   map[string]string{},
		   curs:   map[string]Cursor{},
		   jobs:   map[string]Job{},
		   exps: map[string]ExportRec{},
	   }
  }

   func idemKey(source, key string) string {
	  return strings.TrimSpace(source) + "|" + strings.TrimSpace(key)
  }

  func (m *Mem) FindIdem(_ context.Context, source, key string) (Row, bool, error) {
	   if m == nil {
		   return Row{}, false, ErrUnavailable
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  id, ok := m.idem[idemKey(source, key)]
	  if !ok {
		   return Row{}, false, nil
	  }
	   for _, rows := range m.rows {
		   for _, r := range rows {
			  if r.ID == id {
				  return r, true, nil
			  }
		   }
	  }
	  return Row{}, false, nil
  }

   func (m *Mem) Segments(_ context.Context, partKey string) ([]Segment, error) {
	  if m == nil {
		   return nil, ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   ids := m.byPart[partKey]
	  out := make([]Segment, 0, len(ids))
	   for _, id := range ids {
		   out = append(out, m.segs[id])
	  }
	   sort.Slice(out,
		   func(i, j int) bool { return out[i].Ordinal < out[j].Ordinal })
	  return out, nil
   }

   func (m *Mem) AllSegments(_ context.Context) ([]Segment, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   out := make([]Segment, 0, len(m.segs))
	  for _, s := range m.segs {
		   out = append(out, s)
	  }
	   sort.Slice(out, func(i, j int) bool {
		  if out[i].PartKey != out[j].PartKey {
			   return out[i].PartKey < out[j].PartKey
		  }
		  return out[i].Ordinal < out[j].Ordinal
	  })
	   return out, nil
  }

  func (m *Mem) Segment(_ context.Context, id string) (Segment, bool, error) {
	   if m == nil {
		   return Segment{}, false, ErrUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  s, ok := m.segs[id]
	  return s, ok, nil
   }

   func (m *Mem) CreateSegment(_ context.Context, s Segment) error {
	  if m == nil {
		   return ErrUnavailable
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   if _, ok := m.segs[s.ID]; ok {
		   return ErrSegment
	  }
	  for _, id := range m.byPart[s.PartKey] {
		   if m.segs[id].Ordinal == s.Ordinal {
			  return ErrSegment
		   }
	  }
	  m.segs[s.ID] = s
	  m.byPart[s.PartKey] = append(m.byPart[s.PartKey], s.ID)
	   return nil
  }

   func (m *Mem) SaveSegment(_ context.Context, s Segment) error {
	   if m == nil {
		   return ErrUnavailable
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   cur, ok := m.segs[s.ID]
	  if !ok {
		   return ErrSegment
	   }
	  if err := Transit(cur.State, s.State); err != nil {
		   return err
	   }
	   m.segs[s.ID] = s
	   return nil
   }

   func (m *Mem) InsertRow(_ context.Context, r Row) error {
	   if m == nil {
		   return ErrUnavailable
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  seg, ok := m.segs[r.SegmentID]
	  if !ok {
		   return ErrSegment
	   }
	  if seg.State != SegOpen {
		  return ErrSealed
	   }
	   if _, ok := m.idem[idemKey(r.Source, r.IdemKey)]; ok {
		  return ErrImmutable
	  }
	  rows := m.rows[r.SegmentID]
	  for _, cur := range rows {
		  if cur.Seq == r.Seq {
			   return ErrImmutable
		  }
	   }
	  m.rows[r.SegmentID] = append(rows, r)
	   m.idem[idemKey(r.Source, r.IdemKey)] = r.ID
	   return nil
  }

   func (m *Mem) Rows(_ context.Context, segmentID string) ([]Row, error) {
	   if m == nil {
		   return nil, ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   src := m.rows[segmentID]
	  out := append([]Row(nil), src...)
	  sort.Slice(out, func(i, j int) bool { return out[i].Seq < out[j].Seq })
	  return out, nil
   }

   func (m *Mem) UserRows(_ context.Context, user, asset string, from, to time.Time) ([]Row, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  user = strings.TrimSpace(user)
	  asset = strings.TrimSpace(asset)
	  var out []Row
	   for _, rows := range m.rows {
		   for _, r := range rows {
			  if user != "" && r.UserID != user {
				   continue
			  }
			  if asset != "" && r.Asset != asset {
				   continue
			   }
			  if !from.IsZero() && r.OccurredAt.Before(from) {
				   continue
			  }
			   if !to.IsZero() && r.OccurredAt.After(to) {
				   continue
			   }
			  out = append(out, r)
		  }
	   }
	  sort.Slice(out, func(i, j int) bool {
		  if !out[i].OccurredAt.Equal(out[j].OccurredAt) {
			   return out[i].OccurredAt.Before(out[j].OccurredAt)
		  }
		  if out[i].SegmentID != out[j].SegmentID {
			  return out[i].SegmentID < out[j].SegmentID
		   }
		   return out[i].Seq < out[j].Seq
	   })
	   return out, nil
   }

   func (m *Mem) PutCheckpoint(_ context.Context, c Checkpoint) error {
	  if m == nil {
		   return ErrUnavailable
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   for _, cur := range m.ckpts {
		  if cur.SegmentID == c.SegmentID && cur.Seq == c.Seq && cur.Tip == c.Tip {
			  if cur.MerkleRoot != c.MerkleRoot {
				  return ErrDrift
			   }
			   return nil
		   }
	  }
	  m.ckpts = append(m.ckpts, c)
	  return nil
   }

  func (m *Mem) Checkpoints(_ context.Context, segmentID string) ([]Checkpoint, error) {
	  if m == nil {
		  return nil, ErrUnavailable
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   var out []Checkpoint
	  for _, c := range m.ckpts {
		  if c.SegmentID == segmentID {
			  out = append(out, c)
		  }
	   }
	  sort.Slice(out, func(i, j int) bool {
		  if out[i].Seq != out[j].Seq {
			  return out[i].Seq < out[j].Seq
		  }
		   if out[i].Tip != out[j].Tip {
			  return out[i].Tip
		  }
		   return out[i].At.Before(out[j].At)
	  })
	   return out, nil
   }

   func (m *Mem) AllCheckpoints(_ context.Context) ([]Checkpoint, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   out := append([]Checkpoint(nil), m.ckpts...)
	  sort.Slice(out, func(i, j int) bool {
		  if out[i].At.Equal(out[j].At) {
			  return out[i].ID < out[j].ID
		   }
		  return out[i].At.Before(out[j].At)
	   })
	  return out, nil
   }

   func (m *Mem) Cursor(_ context.Context, source string) (Cursor, bool, error) {
	   if m == nil {
		   return Cursor{}, false, ErrUnavailable
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   c, ok := m.curs[source]
	   return c, ok, nil
   }

   func (m *Mem) PutCursor(_ context.Context, c Cursor) error {
	  if m == nil {
		   return ErrUnavailable
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   cur, ok := m.curs[c.Source]
	   if ok && c.HighWater < cur.HighWater {
		  return ErrCursor
	  }
	   m.curs[c.Source] = c
	   return nil
   }

   func (m *Mem) Cursors(_ context.Context) ([]Cursor, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]Cursor, 0, len(m.curs))
	   for _, c := range m.curs {
		  out = append(out, c)
	   }
	  sort.Slice(out,
		   func(i, j int) bool { return out[i].Source < out[j].Source })
	  return out, nil
   }

   func (m *Mem) PutAlert(_ context.Context, a Alert) error {
	  if m == nil {
		   return ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  m.alerts = append(m.alerts, a)
	   return nil
  }

   func (m *Mem) Alerts(_ context.Context, openOnly bool) ([]Alert, error) {
	   if m == nil {
		  return nil, ErrUnavailable
	   }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  out := make([]Alert, 0, len(m.alerts))
	   for _, a := range m.alerts {
		   if openOnly && !a.Open {
			   continue
		   }
		   out = append(out, a)
	  }
	  return out, nil
  }

  func (m *Mem) PutJob(_ context.Context, j Job) error {
	  if m == nil {
		  return ErrUnavailable
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if cur, ok := m.jobs[j.ID]; ok {
		  if err := jobTransit(cur.State, j.State); err != nil {
			   return err
		   }
	   }
	   cp := j
	   if j.Expect != nil {
		   cp.Expect = append([]string(nil), j.Expect...)
	   }
	  m.jobs[j.ID] = cp
	   return nil
   }

   func (m *Mem) Job(_ context.Context, id string) (Job, bool, error) {
	   if m == nil {
		  return Job{}, false, ErrUnavailable
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  j, ok := m.jobs[id]
	  if ok && j.Expect != nil {
		   j.Expect = append([]string(nil), j.Expect...)
	  }
	   return j, ok, nil
   }

  func (m *Mem) Jobs(_ context.Context, state string) ([]Job, error) {
	  if m == nil {
		   return nil, ErrUnavailable
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []Job
	  for _, j := range m.jobs {
		   if state != "" && j.State != state {
			   continue
		   }
		  out = append(out, j)
	  }
	   sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.Before(out[j].CreatedAt) })
	  return out, nil
  }

   func (m *Mem) PutExport(_ context.Context, e ExportRec) error {
	  if m == nil {
		  return ErrUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  m.exps[e.ID] = e
	   return nil
  }

   func (m *Mem) Export(_ context.Context, id string) (ExportRec, bool, error) {
	   if m == nil {
		   return ExportRec{}, false, ErrUnavailable
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  e, ok := m.exps[id]
	  return e, ok, nil
  }

   func (m *Mem) CorruptBody(_ context.Context, segmentID string, seq int, body string) error {
	   if m == nil {
		   return ErrUnavailable
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   rows := m.rows[segmentID]
	  for i := range rows {
		  if rows[i].Seq == seq {
			   rows[i].Body = body
			  m.rows[segmentID] = rows
			   return nil
		  }
	  }
	   return ErrSegment
  }

   func (m *Mem) RewriteRoot(_ context.Context, segmentID, root string) error {
	  if m == nil {
		  return ErrUnavailable
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   s, ok := m.segs[segmentID]
	  if !ok {
		   return ErrSegment
	   }
	  s.MerkleRoot = root
	   m.segs[segmentID] = s
	   return nil
   }

  func jobTransit(from, to string) error {
	  if from == to {
		  return nil
	  }
	  switch from {
	  case JobQueued:
		   if to == JobRunning || to == JobFailed {
			  return nil
		  }
	   case JobRunning:
		   if to == JobDone || to == JobFailed {
			   return nil
		   }
	  }
	  return ErrReplay
   }
