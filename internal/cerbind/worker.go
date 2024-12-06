package cerbind

import (
	"context"
	"errors"
	"time"
	"github.com/google/uuid"
)

// Tick 跑一轮超时、告警和完整性巡检。
  func (e *Engine) Tick(ctx context.Context) (TickReport, error) {
	   var rep TickReport
	   if err := e.ready(); err != nil {
		   return rep, err
	   }
	   if ctx == nil {
		   ctx = context.Background()
	   }
	   live, err := e.st.LiveSessions(ctx)
	   if err != nil {
		  return rep, err
	   }
	   now := e.now()
	   stuckN := 0
	   for _, ses := range live {
		   if !ses.Deadline.IsZero() && now.After(ses.Deadline) {
			  if _, err := e.Abort(ctx, ses.ID, e.timer, "timeout"); err != nil {
				   if !errors.Is(err, ErrState) {
					  return rep, err
				  }
			  } else {
				  rep.TimedOut++
			  }
			  continue
		   }
		  if e.shouldAlert(ses, now) {
			  if err := e.markStuck(ctx, ses.ID); err != nil {
				   return rep, err
			  }
			  rep.Stuck++
			   stuckN++
		   }
	  }
	  stuckGauge.Set(float64(stuckN))
	  rel, dem, fixed, err := e.patrol(ctx)
	  rep.Released = rel
	  rep.Demoted = dem
	   rep.Fixed = fixed
	  return rep, err
   }

  func (e *Engine) shouldAlert(ses Session, now time.Time) bool {
	   if e.stuckFor <= 0 || !ses.State.Live() {
		   return false
	  }
	  if ses.State == StReady || ses.State == StClosed {
		  return false
	  }
	  if now.Sub(ses.UpdatedAt) < e.stuckFor {
		  return false
	   }
	   if !ses.AlertedAt.IsZero() && now.Sub(ses.AlertedAt) < e.stuckFor {
		   return false
	   }
	   return true
   }

  func (e *Engine) markStuck(ctx context.Context, id string) error {
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   ses, err := e.load(ctx, id)
	   if err != nil {
		  return err
	  }
	  if !e.shouldAlert(ses, e.now()) {
		   return nil
	   }
	   ses.AlertedAt = e.now()
	   if err := e.st.PutSession(ctx, ses); err != nil {
		   return err
	   }
	   e.writeAudit(ctx, ses.ID, ses.BatchID, "stuck", e.timer, string(ses.State))
	  return nil
  }

// Start 按间隔巡检，直到 ctx 取消。单次失败不退出循环。
   func (e *Engine) Start(ctx context.Context) {
	  if e == nil {
		   return
	   }
	  if ctx == nil {
		  ctx = context.Background()
	   }
	  go func() {
		  t := time.NewTicker(30 * time.Second)
		   defer t.Stop()
		   for {
			   select {
			   case <-ctx.Done():
				   return
			  case <-t.C:
				   _, _ = e.Tick(ctx)
			   }
		  }
	   }()
  }

  func (e *Engine) patrol(ctx context.Context) (released, demoted, fixed int, err error) {
	   e.mu.Lock()
	   defer e.mu.Unlock()
	  binds, err := e.st.ActiveBindings(ctx)
	   if err != nil {
		  return 0, 0, 0, err
	  }
	  byBatch := map[string][]Binding{}
	  bySes := map[string][]Binding{}
	   for _, b := range binds {
		   byBatch[b.BatchID] = append(byBatch[b.BatchID], b)
		  bySes[b.SessionID] = append(bySes[b.SessionID], b)
	   }
	   now := e.now()
	  for batch, rows := range byBatch {
		   if len(rows) < 2 {
			   continue
		   }
		   for _, extra := range rows[1:] {
			   extra.State = BindReleased
			  extra.ReleasedAt = now
			   if err := e.st.PutBinding(ctx, extra); err != nil {
				   return released, demoted, fixed, err
			   }
			  released++
			  e.writeAudit(ctx, extra.SessionID, batch, "integrity", e.timer, "dup-batch")
			   integrityTotal.WithLabelValues("dup_batch").Inc()
		  }
	   }
	  for sesID, rows := range bySes {
		  ses, ok, err := e.st.Session(ctx, sesID)
		  if err != nil {
			  return released, demoted, fixed, err
		   }
		  if !ok || ses.State == StAborted {
			   for _, b := range rows {
				  if b.State != BindActive {
					   continue
				   }
				  b.State = BindReleased
				   b.ReleasedAt = now
				   if err := e.st.PutBinding(ctx, b); err != nil {
					  return released, demoted, fixed, err
				  }
				  released++
				  e.writeAudit(ctx, sesID, b.BatchID, "integrity", e.timer, "orphan")
				   integrityTotal.WithLabelValues("orphan").Inc()
			  }
			   continue
		  }
		   if ses.State == StClosed {
			  for _, b := range rows {
				   b.State = BindConsumed
				   b.ReleasedAt = now
				  if err := e.st.PutBinding(ctx, b); err != nil {
					   return released, demoted, fixed, err
				  }
				  fixed++
			  }
			  continue
		   }
		  if ses.BindMode == ModeOne && len(rows) > 1 {
			  for _, extra := range rows[1:] {
				   extra.State = BindReleased
				   extra.ReleasedAt = now
				   if err := e.st.PutBinding(ctx, extra); err != nil {
					   return released, demoted, fixed, err
				   }
				   released++
				  e.writeAudit(ctx, ses.ID, extra.BatchID, "integrity", e.timer, "mode-one")
				   integrityTotal.WithLabelValues("mode_one").Inc()
			  }
			  rows = rows[:1]
		   }
		  demote := false
		  reason := ""
		   rcs, err := e.st.Receipts(ctx, ses.ID)
		  if err != nil {
			  return released, demoted, fixed, err
		  }
		  n := len(distinctActors(rcs))
		   if ses.ReceiptN != n {
			  ses.ReceiptN = n
			   fixed++
		   }
		  if n < ses.Threshold && (ses.State == StReady || ses.State == StBound || ses.State == StQuorum) {
			  demote = true
			  reason = "quorum"
		   }
		   for _, b := range rows {
			  if b.State != BindActive {
				   continue
			   }
			  snap, ok, err := e.st.Snap(ctx, b.BatchID)
			  if err != nil {
				  return released, demoted, fixed, err
			  }
			  if !ok || snap.Digest != b.Digest {
				  b.State = BindStale
				  b.ReleasedAt = now
				   if err := e.st.PutBinding(ctx, b); err != nil {
					   return released, demoted, fixed, err
				   }
				  released++
				   demote = true
				   reason = "digest"
				   e.writeAudit(ctx, ses.ID, b.BatchID, "integrity", e.timer, "digest")
				  integrityTotal.WithLabelValues("digest").Inc()
			   }
		  }
		   if ses.State == StReady || ses.State == StBound {
			   left, err := e.st.BindingsBySession(ctx, ses.ID)
			  if err != nil {
				  return released, demoted, fixed, err
			   }
			  if len(activeBinds(left)) == 0 {
				   demote = true
				   if reason == "" {
					   reason = "unbound"
				   }
			  }
		   }
		   if demote && (ses.State == StReady || ses.State == StBound || ses.State == StQuorum) {
			   next := StOpen
			  if n >= ses.Threshold && reason != "quorum" {
				  next = StQuorum
			   }
			  ses.State = next
			  ses.Note = "integrity:" + reason
			  ses.LastActor = e.timer
			   ses.UpdatedAt = now
			   ses.FenceEpoch++
			   ses.FenceToken = uuid.NewString()
			  demoted++
			   e.writeAudit(ctx, ses.ID, ses.BatchID, "integrity", e.timer, "demote:"+reason)
			  integrityTotal.WithLabelValues("demote").Inc()
			  countTo(next)
		  }
		   if err := e.st.PutSession(ctx, ses); err != nil {
			   return released, demoted, fixed, err
		  }
	  }
	   live, err := e.st.LiveSessions(ctx)
	   if err != nil {
		  return released, demoted, fixed, err
	  }
	  for _, ses := range live {
		   if ses.State != StReady && ses.State != StBound {
			   continue
		  }
		   left, err := e.st.BindingsBySession(ctx, ses.ID)
		   if err != nil {
			  return released, demoted, fixed, err
		   }
		   if len(activeBinds(left)) > 0 {
			  continue
		   }
		  rcs, err := e.st.Receipts(ctx, ses.ID)
		  if err != nil {
			   return released, demoted, fixed, err
		  }
		  n := len(distinctActors(rcs))
		  next := StOpen
		   if n >= ses.Threshold {
			  next = StQuorum
		   }
		   ses.State = next
		   ses.ReceiptN = n
		   ses.Note = "integrity:unbound"
		   ses.LastActor = e.timer
		  ses.UpdatedAt = now
		   ses.FenceEpoch++
		   ses.FenceToken = uuid.NewString()
		   if err := e.st.PutSession(ctx, ses); err != nil {
			  return released,
				  demoted, fixed, err
		   }
		   demoted++
		  e.writeAudit(ctx, ses.ID, ses.BatchID, "integrity", e.timer, "demote:unbound")
		   integrityTotal.WithLabelValues("unbound").Inc()
		  countTo(next)
	   }
	  return released, demoted, fixed, nil
  }

// Alerts 返回最近的超时、卡死和完整性审计。

  func (e *Engine) Alerts(ctx context.Context, limit int) ([]Audit, error) {
	   rows, err := e.AuditsOf(ctx, "", "", 200)
	   if err != nil {
		   return nil, err
	  }
	  out := make([]Audit, 0)
	   for _, a := range rows {
		   switch a.Kind {
		   case "timeout", "stuck", "integrity", "abort", "gate_deny":
			  out = append(out, a)
		  }
		  if len(out) >= clampLimit(limit) {
			   break
		  }
	  }
	   return out, nil
   }
