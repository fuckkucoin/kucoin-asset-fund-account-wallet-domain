package auditchain

import (
	"context"
	"encoding/json"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/auditchain/replay"
	"account-assets.kucoin.net/funding/internal/dualctl"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

// BookReader 只读账面。实现里不得调用账本 Apply。

type BookReader interface {
	Read(ctx context.Context, userID, asset string) (replay.Snap, error)
}

   func (w *Warehouse) SetReader(r BookReader) {
	   if w == nil {
		   return
	   }
	  w.mu.Lock()
	  w.reader = r
	  w.mu.Unlock()
  }

type expectPack struct {
	Expect []string `json:"expect,omitempty"`
}

// SubmitReplay 入队。needDual 时先记下双控单号，执行前由调用方保证达票。
  func (w *Warehouse) SubmitReplay(ctx context.Context, user, asset, actor string, from, to time.Time, expect []string, dualID string) (Job, error) {
	  if w == nil {
		   return Job{}, ErrUnavailable
	   }
	  user = strings.TrimSpace(user)
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   actor = strings.TrimSpace(actor)
	  if user == "" || asset == "" || actor == "" || strings.EqualFold(actor, "system") {
		  return Job{}, ErrReplay
	   }
	   if from.IsZero() || to.IsZero() || to.Before(from) {
		   return Job{}, ErrReplay
	   }
	  now := w.now()
	  raw, err := json.Marshal(expectPack{Expect: expect})
	  if err != nil {
		   return Job{}, err
	  }
	  j := Job{
		   ID: uuid.NewString(), UserID: user, Asset: asset, From: from.UTC(), To: to.UTC(),
		   State: JobQueued, DualID: strings.TrimSpace(dualID), Actor: actor, Expect: append([]string(nil), expect...),
		  ReportJSON: string(raw),
		   CreatedAt:  now, UpdatedAt: now,
	  }
	  if err := w.st.PutJob(ctx, j); err != nil {
		  return Job{}, err
	  }
	  return j, nil
  }

  func (w *Warehouse) loadJob(ctx context.Context, id string) (Job, error) {
	  j, ok, err := w.st.Job(ctx, id)
	  if err != nil {
		   return Job{}, err
	  }
	  if !ok {
		   return Job{}, ErrReplay
	  }
	   if len(j.Expect) == 0 && j.ReportJSON != "" {
		  var p expectPack
		   if json.Unmarshal([]byte(j.ReportJSON), &p) == nil {
			   j.Expect = p.Expect
		  }
	   }
	  return j, nil
   }

// RunReplay 把作业从 queued 推进到 done 或 failed。围栏不前进则失败。

   func (w *Warehouse) RunReplay(ctx context.Context) (Job, error) {
	   return w.RunReplayID(ctx, "")
   }

// RunReplayID 指定作业。id 空则跑最早一条排队。

   func (w *Warehouse) RunReplayID(ctx context.Context, id string) (Job, error) {
	  if w == nil {
		   return Job{}, ErrUnavailable
	   }
	  w.mu.Lock()
	  defer w.mu.Unlock()
	   j, err := w.pickJob(ctx, id)
	  if err != nil {
		   return Job{}, err
	   }
	  before, _ := w.fences.Get(j.ID)
	   after, err := w.fences.Bump(j.ID, persist.NewToken())
	  if err != nil {
		  return w.failJob(ctx, j, err)
	  }
	   if err := dualctl.GuardReplayAdvance(before, after); err != nil {
		   return w.failJob(ctx, j, err)
	  }
	   j.State = JobRunning
	  j.FenceEpoch = after.Epoch
	   j.FenceToken = after.Token
	   j.UpdatedAt = w.now()
	  if err := w.st.PutJob(ctx, j); err != nil {
		   return Job{}, err
	  }
	   rep, err := w.replayLocked(ctx, j)
	   if err != nil {
		  return w.failJob(ctx, j, err)
	  }
	   raw, err := json.Marshal(rep)
	  if err != nil {
		  return w.failJob(ctx, j, err)
	  }
	  j.State = JobDone
	   j.ReportJSON = string(raw)
	  j.UpdatedAt = w.now()
	   j.ErrorText = ""
	   if err := w.st.PutJob(ctx, j); err != nil {
		  return Job{}, err
	  }
	  if rep.CaseOpened {
		   if err := w.raise(ctx, AlertReplay, "", dualctl.ReplaySubject(j.ID)); err != nil {
			  return j, err
		   }
	  }
	  return j, nil
   }

   func (w *Warehouse) pickJob(ctx context.Context, id string) (Job, error) {
	   if id != "" {
		  j, err := w.loadJob(ctx, id)
		   if err != nil {
			   return Job{}, err
		  }
		   if j.State != JobQueued {
			  return Job{}, ErrReplay
		   }
		  return j, nil
	   }
	   jobs, err := w.st.Jobs(ctx, JobQueued)
	  if err != nil {
		  return Job{}, err
	  }
	   if len(jobs) == 0 {
		   return Job{}, ErrReplay
	   }
	  return w.loadJob(ctx,
		  jobs[0].ID)
  }

   func (w *Warehouse) failJob(ctx context.Context, j Job, cause error) (Job, error) {
	   j.State = JobFailed
	   if cause != nil {
		  j.ErrorText = cause.Error()
	  }
	   j.UpdatedAt = w.now()
	   if err := w.st.PutJob(ctx, j); err != nil {
		   return Job{}, err
	  }
	  return j, cause
  }

  func (w *Warehouse) replayLocked(ctx context.Context, j Job) (replay.Report, error) {
	  rows, err := w.st.UserRows(ctx, j.UserID, j.Asset, j.From, j.To)
	   if err != nil {
		   return replay.Report{}, err
	   }
	  evs := make([]replay.Event,
		  0, len(rows))
	   for _, r := range rows {
		   eff, err := EffectOf(r.Kind)
		   if err != nil {
			  return replay.Report{}, err
		  }
		  evs = append(evs, replay.Event{
			  Seq: r.Seq, Kind: r.Kind, Effect: eff, UserID: r.UserID, Asset: r.Asset,
			  Amount: r.Amount, RefID: r.RefID, IdemKey: r.IdemKey, OccurredAt: r.OccurredAt,
		  })
	  }
	   evs = replay.SameSubject(replay.Window(evs, j.From, j.To), j.UserID, j.Asset)
	  var snap *replay.Snap
	  if w.reader != nil {
		   got, err := w.reader.Read(ctx, j.UserID, j.Asset)
		  if err != nil {
			  return replay.Report{}, err
		   }
		  snap = &got
	   }
	  return replay.Run(replay.Input{
		  UserID: j.UserID, Asset: j.Asset, From: j.From, To: j.To,
		   Events: evs, Snap: snap, Expect: j.Expect,
	  })
   }

// Timeline 只读重放，不开差异单，也不推进围栏。
  func (w *Warehouse) Timeline(ctx context.Context, user, asset string, from, to time.Time) (replay.Report, error) {
	  if w == nil {
		  return replay.Report{}, ErrUnavailable
	  }
	   w.mu.Lock()
	   defer w.mu.Unlock()
	   j := Job{UserID: strings.TrimSpace(user), Asset: strings.ToUpper(strings.TrimSpace(asset)), From: from, To: to}
	  return w.replayLocked(ctx, j)
  }

// EntryCount 全部段的条目数，给摄取幂等演练对账。

   func (w *Warehouse) EntryCount(ctx context.Context) (int, error) {
	   if w == nil {
		   return 0, ErrUnavailable
	  }
	   segs, err := w.st.AllSegments(ctx)
	   if err != nil {
		  return 0, err
	  }
	   n := 0
	  for _, seg := range segs {
		   n += seg.Count
	   }
	  return n, nil
  }

// GetReplay 读作业。报告还在 expect 包装里时不当成最终报告。
  func (w *Warehouse) GetReplay(ctx context.Context, id string) (Job, error) {
	  if w == nil {
		  return Job{}, ErrUnavailable
	   }
	  return w.loadJob(ctx, id)
  }

// StaticBook 演练和只读用的固定快照，没有写方法。
type StaticBook struct {
	Available string
	Frozen    string
	Reads     int
}

  func (b *StaticBook) Read(_ context.Context, userID, asset string) (replay.Snap, error) {
	  if b == nil {
		   return replay.Snap{}, ErrUnavailable
	   }
	   b.Reads++
	  return replay.Snap{UserID: userID, Asset: asset, Available: b.Available, Frozen: b.Frozen}, nil
  }
