package auditchain

import (
	"context"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	whAppend = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_audit_wh_append_total",
		Help: "Audit warehouse append results",
	}, []string{"result"})
	whInspect = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "aa_audit_wh_inspect_total",
		Help: "Audit warehouse integrity inspect results",
	}, []string{"result"})
)

// Config 段容量、热天数、游标滞后阈值。
type Config struct {
	SegCap  int
	HotDays int
	LagLimit time.Duration
	Clock func() time.Time
}

  func DefaultConfig() Config {
	  return Config{SegCap: 256, HotDays: 3, LagLimit: 2 * time.Minute}
   }

// Envelope 入库前的资金事实。种类必须在目录里。
type Envelope struct {
	Source string
	IdemKey string
	Kind    string
	Domain  string
	Tenant string
	UserID string
	Asset  string
	Amount string
	RefID  string
	Actor      string
	Body       string
	OccurredAt time.Time
}

// Warehouse 分区只追加仓。写路径不从这里调用。

type Warehouse struct {
	mu sync.Mutex
	st  Store
	cfg Config
	fences *persist.Table
	reader BookReader
}

   func NewWarehouse(st Store, cfg Config) *Warehouse {
	   if st == nil {
		   st = NewMem()
	   }
	  if cfg.SegCap <= 0 {
		  cfg.SegCap = 256
	   }
	   if cfg.HotDays <= 0 {
		   cfg.HotDays = 3
	   }
	  if cfg.LagLimit <= 0 {
		   cfg.LagLimit = 2 * time.Minute
	   }
	  if cfg.Clock == nil {
		  cfg.Clock = func() time.Time { return time.Now().UTC() }
	  }
	   return &Warehouse{st: st, cfg: cfg, fences: persist.NewTable()}
   }

  func (w *Warehouse) now() time.Time {
	   return w.cfg.Clock().UTC()
   }

  func (w *Warehouse) Store() Store {
	   if w == nil {
		   return nil
	   }
	  return w.st
   }

// Normalize 收口空白和金额。动余额的种类必须是正数。
   func (e *Envelope) Normalize() error {
	   if e == nil {
		   return ErrBody
	   }
	   e.Source = strings.TrimSpace(e.Source)
	   e.IdemKey = strings.TrimSpace(e.IdemKey)
	   e.Kind = strings.TrimSpace(e.Kind)
	  e.Actor = strings.TrimSpace(e.Actor)
	   e.UserID = strings.TrimSpace(e.UserID)
	  e.Asset = strings.ToUpper(strings.TrimSpace(e.Asset))
	  e.Amount = strings.TrimSpace(e.Amount)
	   e.RefID = strings.TrimSpace(e.RefID)
	  e.Tenant = strings.TrimSpace(e.Tenant)
	  e.Body = strings.TrimSpace(e.Body)
	  if e.Source == "" || e.IdemKey == "" {
		   return ErrBody
	   }
	   spec, err := LookupKind(e.Kind)
	   if err != nil {
		   return err
	  }
	   if e.Domain == "" {
		   e.Domain = spec.Domain
	   }
	   if e.Domain != spec.Domain {
		  return ErrKind
	   }
	  if e.Actor == "" || strings.EqualFold(e.Actor, "system") {
		  return ErrActor
	  }
	  if e.Body == "" {
		  e.Body = "{}"
	  }
	   if e.OccurredAt.IsZero() {
		   return ErrBody
	  }
	   e.OccurredAt = e.OccurredAt.UTC()
	   if e.Amount == "" || e.Amount == "0" {
		  e.Amount = "0"
	  }
	  if _, err := money.ParseBook(e.Amount); err != nil {
		  return err
	   }
	   if spec.Moves && e.Amount != "0" {
		  if _, err := money.Parse(e.Amount); err != nil {
			   return err
		  }
	   }
	   return nil
  }

// Append 幂等。同一 source+idem 再投递返回原行，不推进序号。

   func (w *Warehouse) Append(ctx context.Context, env Envelope) (Row, error) {
	  if w == nil || w.st == nil {
		  whAppend.WithLabelValues("unavailable").Inc()
		  return Row{}, ErrUnavailable
	  }
	   w.mu.Lock()
	   defer w.mu.Unlock()
	  return w.appendLocked(ctx, env)
  }

   func (w *Warehouse) appendLocked(ctx context.Context, env Envelope) (Row, error) {
	   if err := env.Normalize(); err != nil {
		  whAppend.WithLabelValues("reject").Inc()
		  return Row{}, err
	   }
	   if row, ok, err := w.st.FindIdem(ctx, env.Source, env.IdemKey); err != nil {
		   whAppend.WithLabelValues("store").Inc()
		  return Row{}, err
	  } else if ok {
		  row.Duplicate = true
		  whAppend.WithLabelValues("dup").Inc()
		   return row, nil
	  }
	   part, err := PartOf(env.Domain, env.Tenant, env.OccurredAt)
	  if err != nil {
		   whAppend.WithLabelValues("reject").Inc()
		  return Row{}, err
	   }
	   seg, err := w.segmentFor(ctx, part)
	  if err != nil {
		  whAppend.WithLabelValues("store").Inc()
		  return Row{}, err
	  }
	   now := w.now()
	  row := Row{
		  ID:        uuid.NewString(),
		   SegmentID: seg.ID, Seq: seg.Count + 1,
		   IdemKey: env.IdemKey, Source: env.Source, Kind: env.Kind, Domain: env.Domain, Tenant: part.Tenant,
		   UserID: env.UserID,
		   Asset:  env.Asset, Amount: env.Amount, RefID: env.RefID, Actor: env.Actor, Body: env.Body,
		   PrevHash: seg.TailHash, OccurredAt: env.OccurredAt, IngestedAt: now,
	   }
	  row.SelfHash = HashRow(row)
	   if err := w.st.InsertRow(ctx, row); err != nil {
		   whAppend.WithLabelValues("store").Inc()
		  return Row{}, err
	  }
	   if seg.HeadHash == "" {
		  seg.HeadHash = row.SelfHash
	   }
	  seg.TailHash = row.SelfHash
	   seg.Count = row.Seq
	  if err := w.st.SaveSegment(ctx, seg); err != nil {
		  whAppend.WithLabelValues("store").Inc()
		   return Row{}, err
	   }
	   whAppend.WithLabelValues("ok").Inc()
	  return row, nil
  }

   func (w *Warehouse) segmentFor(ctx context.Context, part Part) (Segment, error) {
	   segs, err := w.st.Segments(ctx, part.Key)
	   if err != nil {
		  return Segment{}, err
	  }
	   if n := len(segs); n > 0 {
		   last := segs[n-1]
		   if last.State == SegOpen && last.Count < w.cfg.SegCap {
			  return last, nil
		  }
		   if last.State == SegOpen && last.Count >= w.cfg.SegCap {
			   if _, err := w.sealLocked(ctx, last.ID, w.now(), "cap"); err != nil {
				   return Segment{}, err
			  }
		  }
	   }
	  ord := 1
	  if n := len(segs); n > 0 {
		  ord = segs[n-1].Ordinal + 1
	   }
	  seg := Segment{
		   ID: uuid.NewString(), PartKey: part.Key, Domain: part.Domain, Tenant: part.Tenant, DayKey: part.DayKey,
		   Ordinal: ord, State: SegOpen, Tier: TierHot, OpenedAt: w.now(),
	  }
	   if err := w.st.CreateSegment(ctx, seg); err != nil {
		   return Segment{}, err
	  }
	  return seg, nil
  }

// Seal 把开段收成封存并写检查点。已封存再调视为成功。

   func (w *Warehouse) Seal(ctx context.Context, id, actor string) (Segment, error) {
	  if w == nil {
		  return Segment{}, ErrUnavailable
	  }
	   w.mu.Lock()
	   defer w.mu.Unlock()
	   return w.sealLocked(ctx,
		  id, w.now(), actor)
  }

   func (w *Warehouse) sealLocked(ctx context.Context, id string, at time.Time, actor string) (Segment, error) {
	  seg, ok, err := w.st.Segment(ctx, id)
	   if err != nil {
		   return Segment{}, err
	  }
	  if !ok {
		  return Segment{}, ErrSegment
	  }
	  if seg.State == SegSealed || seg.State == SegFrozen {
		  return seg, nil
	  }
	   if err := Transit(seg.State, SegSealed); err != nil {
		   return Segment{}, err
	   }
	  rows, err := w.st.Rows(ctx, id)
	  if err != nil {
		   return Segment{}, err
	   }
	  root := MerkleRoot(LeavesOf(rows))
	   prev, err := w.latestRoot(ctx, seg.PartKey)
	  if err != nil {
		   return Segment{}, err
	  }
	  seg.State = SegSealed
	   seg.MerkleRoot = root
	   seg.SealedAt = at
	   if err := w.st.SaveSegment(ctx, seg); err != nil {
		   return Segment{}, err
	   }
	   cp := Checkpoint{
		   ID: uuid.NewString(), SegmentID: seg.ID, Seq: seg.Count, MerkleRoot: root, PrevRoot: prev,
		  Count: seg.Count, Tip: false, Actor: strings.TrimSpace(actor), At: at,
	  }
	   if cp.Actor == "" {
		  cp.Actor = "seal"
	   }
	  if err := w.st.PutCheckpoint(ctx, cp); err != nil {
		  return Segment{}, err
	  }
	  return seg, nil
   }

   func (w *Warehouse) latestRoot(ctx context.Context, partKey string) (string, error) {
	  segs, err := w.st.Segments(ctx, partKey)
	   if err != nil {
		  return "", err
	  }
	  var best Checkpoint
	   var found bool
	  for _, seg := range segs {
		   cks, err := w.st.Checkpoints(ctx, seg.ID)
		   if err != nil {
			   return "", err
		   }
		  for _, c := range cks {
			  if !found || c.At.After(best.At) || (c.At.Equal(best.At) && !c.Tip && best.Tip) {
				  best = c
				  found = true
			  }
		  }
	  }
	  if !found {
		  return "", nil
	   }
	  return best.MerkleRoot, nil
  }

// Tip 给仍开着的段写一条尖端检查点，封存前也能对根。

  func (w *Warehouse) Tip(ctx context.Context, id, actor string) (Checkpoint, error) {
	  if w == nil {
		  return Checkpoint{},
			   ErrUnavailable
	   }
	  w.mu.Lock()
	  defer w.mu.Unlock()
	   seg,
		   ok, err := w.st.Segment(ctx, id)
	  if err != nil || !ok {
		   if err == nil {
			   err = ErrSegment
		  }
		   return Checkpoint{}, err
	   }
	   rows, err := w.st.Rows(ctx, id)
	  if err != nil {
		   return Checkpoint{}, err
	   }
	   root := MerkleRoot(LeavesOf(rows))
	   prev, err := w.latestRoot(ctx, seg.PartKey)
	  if err != nil {
		  return Checkpoint{}, err
	   }
	   cp := Checkpoint{
		  ID:        uuid.NewString(),
		   SegmentID: id, Seq: seg.Count, MerkleRoot: root, PrevRoot: prev,
		   Count: seg.Count, Tip: true, Actor: actor, At: w.now(),
	  }
	   if err := w.st.PutCheckpoint(ctx, cp); err != nil {
		   return Checkpoint{}, err
	   }
	   return cp, nil
   }

// Advance 高水位只增不减。相等视为已经记过。

   func (w *Warehouse) AdvanceCursor(ctx context.Context, source, water, last string, occurred time.Time) (Cursor, error) {
	   if w == nil {
		  return Cursor{}, ErrUnavailable
	   }
	  w.mu.Lock()
	  defer w.mu.Unlock()
	  return w.advanceCursorLocked(ctx,
		  source, water, last, occurred)
  }

  func (w *Warehouse) advanceCursorLocked(ctx context.Context, source, water, last string, occurred time.Time) (Cursor, error) {
	  source = strings.TrimSpace(source)
	  water = strings.TrimSpace(water)
	   if source == "" || water == "" {
		   return Cursor{}, ErrCursor
	   }
	  cur, ok, err := w.st.Cursor(ctx, source)
	  if err != nil {
		  return Cursor{}, err
	  }
	   if ok && water < cur.HighWater {
		  return cur, ErrCursor
	   }
	   if ok && water == cur.HighWater {
		  return cur, nil
	   }
	  now := w.now()
	   lag := time.Duration(0)
	  if !occurred.IsZero() && now.After(occurred) {
		   lag = now.Sub(occurred)
	  }
	   next := Cursor{Source: source, HighWater: water, LastID: last, Lag: lag, UpdatedAt: now}
	   if err := w.st.PutCursor(ctx, next); err != nil {
		  return Cursor{}, err
	   }
	  return next, nil
  }

// Water 业务时间加幂等键。纳秒位补齐，字典序与时间序一致。

  func Water(at time.Time, idem string) string {
	  return at.UTC().Format("2006-01-02T15:04:05.000000000Z") + "|" + strings.TrimSpace(idem)
   }

// SealStale 跨日或已满的开段收掉。当天的空段留着。
  func (w *Warehouse) SealStale(ctx context.Context) (int, error) {
	  if w == nil {
		  return 0, ErrUnavailable
	   }
	   w.mu.Lock()
	  defer w.mu.Unlock()
	  segs, err := w.st.AllSegments(ctx)
	   if err != nil {
		   return 0, err
	   }
	  now := w.now()
	  today := now.Format("20060102")
	  n := 0
	  for _, seg := range segs {
		  if seg.State != SegOpen {
			   continue
		  }
		   if seg.DayKey >= today && seg.Count < w.cfg.SegCap {
			  continue
		  }
		  if seg.Count == 0 && seg.DayKey >= today {
			  continue
		   }
		   if _, err := w.sealLocked(ctx, seg.ID, now, "worker"); err != nil {
			  return n, err
		  }
		   n++
	   }
	   return n, nil
   }

// Cool 超过热窗口的已封存段降到冷层。状态仍是 sealed 或 frozen。
   func (w *Warehouse) Cool(ctx context.Context) (int, error) {
	   if w == nil {
		   return 0, ErrUnavailable
	   }
	  w.mu.Lock()
	  defer w.mu.Unlock()
	   segs, err := w.st.AllSegments(ctx)
	  if err != nil {
		  return 0, err
	  }
	  cutoff := w.now().AddDate(0, 0, -w.cfg.HotDays).Format("20060102")
	   n := 0
	  for _, seg := range segs {
		   if seg.State == SegOpen || seg.Tier == TierCold {
			   continue
		  }
		  if seg.DayKey >= cutoff {
			   continue
		  }
		   seg.Tier = TierCold
		  if err := w.st.SaveSegment(ctx, seg); err != nil {
			   return n, err
		   }
		  n++
	  }
	   return n, nil
  }

// DrillCorrupt 演练：改历史正文但不重算哈希。路由不暴露。
   func (w *Warehouse) DrillCorrupt(ctx context.Context, segmentID string, seq int, body string) error {
	   if w == nil {
		   return ErrUnavailable
	  }
	  return w.st.CorruptBody(ctx, segmentID, seq, body)
  }

// DrillRewriteRoot 演练：改段根，制造检查点漂移。
   func (w *Warehouse) DrillRewriteRoot(ctx context.Context, segmentID, root string) error {
	   if w == nil {
		   return ErrUnavailable
	  }
	   return w.st.RewriteRoot(ctx,
		  segmentID, root)
   }
