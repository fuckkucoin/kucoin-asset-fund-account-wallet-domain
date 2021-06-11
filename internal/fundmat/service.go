package fundmat

import (
	"context"
	"sync"
	"time"
	"github.com/google/uuid"
)

// Service 资金视图物化入口。账本只读源与出箱源可选，未绑定时仍可直接投递事件。
type Service struct {
	mu sync.Mutex
	st Store
	sla        time.Duration
	batch      int
	pauseUntil time.Time
	books BookSource
	moves MovementSource
}

// BookSource 账本余额只读口。重建与漂移使用，不调用 Apply。
type BookSource interface {
	Lines(ctx context.Context, userID string) ([]BookLine, error)
}

// New 装配仓储。st 为空时使用内存仓。默认追平时限 2 秒，单轮最多应用 200 条。
  func New(st Store) *Service {
	  if st == nil {
		  st = NewMem()
	   }
	  return &Service{st: st, sla: 2 * time.Second, batch: 200}
   }

// Store 返回底层仓储，供读面登记围栏。

  func (s *Service) Store() Store {
	   if s == nil {
		  return nil
	  }
	  return s.st
   }

// SetSLA 调整水位巡检使用的追平时限。
   func (s *Service) SetSLA(d time.Duration) {
	   if s == nil {
		  return
	  }
	   s.mu.Lock()
	  s.sla = d
	  s.mu.Unlock()
  }

// SetBatch 调整单轮应用条数。小于 1 时保持原值。
  func (s *Service) SetBatch(n int) {
	   if s == nil || n < 1 {
		  return
	   }
	   s.mu.Lock()
	  s.batch = n
	   s.mu.Unlock()
  }

// BindBooks 绑定账本只读源。
   func (s *Service) BindBooks(b BookSource) {
	   if s == nil {
		  return
	   }
	  s.mu.Lock()
	  s.books = b
	  s.mu.Unlock()
  }

// Pause 在给定时长内停止应用事件。已进入日志的事件仍计入水位头，强一致读会失败。
   func (s *Service) Pause(d time.Duration) {
	   if s == nil {
		   return
	   }
	  if d <= 0 {
		  d = time.Hour
	   }
	   s.mu.Lock()
	  s.pauseUntil = time.Now().Add(d)
	   s.mu.Unlock()
  }

// Resume 恢复应用。
   func (s *Service) Resume() {
	  if s == nil {
		   return
	  }
	  s.mu.Lock()
	  s.pauseUntil = time.Time{}
	  s.mu.Unlock()
   }

   func (s *Service) holding(now time.Time) bool {
	   return !s.pauseUntil.IsZero() && now.Before(s.pauseUntil)
  }

// Paused 当前是否停止应用。
   func (s *Service) Paused() bool {
	   if s == nil {
		  return false
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   return s.holding(time.Now())
   }

// Ingest 把事件写入日志。不在此处改投影行，需再调用 Tick。
   func (s *Service) Ingest(ctx context.Context, ev FundingEvent) (FundingEvent, error) {
	   if s == nil || s.st == nil {
		  return FundingEvent{}, ErrUnavailable
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   stored, err := s.st.Append(ctx, ev)
	   if err != nil {
		   return FundingEvent{}, err
	   }
	   if _, err := s.refreshLocked(ctx, stored.Stream, time.Now().UTC()); err != nil {
		  return stored, err
	   }
	   noteIngest(string(stored.Stream))
	  return stored, nil
  }

// Tick 应用一批未物化事件。暂停时只刷新滞后，不改分栏。
   func (s *Service) Tick(ctx context.Context) (ApplyStats, error) {
	  if s == nil || s.st == nil {
		   return ApplyStats{}, ErrUnavailable
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  now := time.Now().UTC()
	  if s.holding(now) {
		   for _, st := range Streams() {
			   if _, err := s.refreshLocked(ctx, st, now); err != nil {
				  return ApplyStats{Paused: true}, err
			   }
		   }
		   notePause()
		  return ApplyStats{Paused: true}, nil
	  }
	   var stats ApplyStats
	   touched := map[string]struct{}{}
	  // 多轮是让「先入账再冻结」和「先冻结再消耗冻结」在同一轮泵里都能落到连续水位。
	  for pass := 0; pass < 4; pass++ {
		   progress := 0
		   for _, st := range Streams() {
			   pending, err := s.st.Pending(ctx, st, s.batch)
			  if err != nil {
				  return stats, err
			   }
			  for _, ev := range pending {
				  out, err := s.st.ApplyOne(ctx, ev, now)
				   if err != nil {
					  if err == ErrNegativeColumn || err == ErrBadEvent {
						  stats.Rejected++
						   noteReject(string(ev.Op))
						  continue
					  }
					  return stats, err
				   }
				   if out.Applied {
					   stats.Applied++
					   progress++
					  touched[out.Row.UserID] = struct{}{}
					  noteApply(string(ev.Stream))
				  } else {
					  stats.Skipped++
					   noteDup()
				   }
			   }
		  }
		  if progress == 0 {
			   break
		   }
	  }
	   for _, st := range Streams() {
		  if _, err := s.refreshLocked(ctx, st, now); err != nil {
			  return stats, err
		  }
	   }
	  for user := range touched {
		  if err := s.bumpUserLocked(ctx, user, now); err != nil {
			  return stats, err
		  }
	  }
	   return stats, nil
  }

// Rows 读取某用户投影，可按资产与科目过滤。
  func (s *Service) Rows(ctx context.Context, user, asset, subject string) ([]MatRow, error) {
	   if s == nil || s.st == nil {
		   return nil, ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   rows, err := s.st.ListUser(ctx, NormUser(user))
	  if err != nil {
		   return nil, err
	  }
	  asset = NormAsset(asset)
	   subject = NormSubject(subject)
	  if asset == "" && subject == "" {
		   return rows, nil
	  }
	   out := make([]MatRow, 0, len(rows))
	  for _, row := range rows {
		  if asset != "" && row.Asset != asset {
			  continue
		  }
		  if subject != "" && row.Subject != subject {
			  continue
		  }
		  out = append(out, row)
	  }
	   return out, nil
   }

// Lag 返回各流追平报告。调用前会按当前日志重算水位。
   func (s *Service) Lag(ctx context.Context) (LagReport, error) {
	   if s == nil || s.st == nil {
		  return LagReport{}, ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   now := time.Now().UTC()
	   paused := s.holding(now)
	  for _, st := range Streams() {
		  if _, err := s.refreshLocked(ctx, st, now); err != nil {
			  return LagReport{}, err
		  }
	   }
	  wms, err := s.st.ListWM(ctx)
	   if err != nil {
		   return LagReport{}, err
	  }
	   open, err := s.st.ListAlerts(ctx, true, 500)
	   if err != nil {
		   return LagReport{}, err
	  }
	  return Report(wms,
		  paused, len(open), s.sla), nil
   }

// Version 返回用户视图的最近代次。尚无物化时返回 ErrNotFound。
  func (s *Service) Version(ctx context.Context, userID string) (ViewVersion, error) {
	  if s == nil || s.st == nil {
		  return ViewVersion{},
			   ErrUnavailable
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  return s.st.LatestVersion(ctx, NormUser(userID))
  }

// PutFence 保存读围栏。

  func (s *Service) PutFence(ctx context.Context, f ReadFence) error {
	  if s == nil || s.st == nil {
		   return ErrUnavailable
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  return s.st.PutFence(ctx, f)
  }

// Fence 按标识读取围栏。
   func (s *Service) Fence(ctx context.Context, id string) (ReadFence, error) {
	  if s == nil || s.st == nil {
		   return ReadFence{}, ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   return s.st.GetFence(ctx, id)
  }

// Job 读取重建任务。
  func (s *Service) Job(ctx context.Context, id string) (RebuildJob, error) {
	  if s == nil || s.st == nil {
		   return RebuildJob{},
			   ErrUnavailable
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   return s.st.GetJob(ctx, id)
  }

  func (s *Service) refreshLocked(ctx context.Context, stream Stream, now time.Time) (MatWatermark, error) {
	  head, err := s.st.HeadSeq(ctx, stream)
	   if err != nil {
		   return MatWatermark{}, err
	   }
	   applied, err := s.st.AppliedSeqs(ctx, stream)
	   if err != nil {
		  return MatWatermark{}, err
	   }
	   pending, err := s.st.Pending(ctx, stream, 1000)
	   if err != nil {
		  return MatWatermark{}, err
	  }
	  epoch := int64(1)
	  if wms, err := s.st.ListWM(ctx); err == nil {
		   for _, wm := range wms {
			  if wm.Stream == stream && wm.Epoch > 0 {
				   epoch = wm.Epoch
			  }
		  }
	  }
	   wm := Measure(stream, epoch, head, applied, OldestUnapplied(pending), now, s.sla)
	  if err := s.st.PutWM(ctx, wm); err != nil {
		  return MatWatermark{}, err
	  }
	  noteLag(wm)
	   return wm, nil
  }

   func (s *Service) bumpEpochLocked(ctx context.Context, now time.Time) error {
	   for _, st := range Streams() {
		   wm, err := s.refreshLocked(ctx, st, now)
		  if err != nil {
			   return err
		   }
		   wm.Epoch++
		  wm.UpdatedAt = now.UTC()
		  if err := s.st.PutWM(ctx, wm); err != nil {
			   return err
		   }
	   }
	  return nil
   }

   func (s *Service) bumpUserLocked(ctx context.Context, user string, now time.Time) error {
	   user = NormUser(user)
	   if user == "" {
		   return nil
	  }
	  rows, err := s.st.ListUser(ctx, user)
	  if err != nil {
		   return err
	   }
	   var wmSeq int64
	   epoch := int64(1)
	   if wms, err := s.st.ListWM(ctx); err == nil {
		  for _, wm := range wms {
			  if wm.ContiguousSeq > wmSeq {
				   wmSeq = wm.ContiguousSeq
			   }
			  if wm.Epoch > epoch {
				   epoch = wm.Epoch
			   }
		  }
	   }
	  ver := int64(1)
	   if prev, err := s.st.LatestVersion(ctx, user); err == nil && prev.Version > 0 {
		   ver = prev.Version + 1
	   }
	   v := ViewVersion{
		  ID: uuid.NewString(), Scope: "user", UserID: user, Version: ver,
		  Epoch: epoch, WMSeq: wmSeq, Digest: DigestRows(user, rows),
		   RowCount: len(rows), BuiltAt: now.UTC(),
	   }
	   return s.st.PutVersion(ctx, v)
  }
