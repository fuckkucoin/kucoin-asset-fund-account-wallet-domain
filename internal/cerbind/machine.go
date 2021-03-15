package cerbind

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"
	"unicode"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

// Open 把 planned 推进到 ceremony_open，并调用已挂上的执行器。
// 已经打开的会话按幂等返回，不重复调用执行器。
   func (e *Engine) Open(ctx context.Context, id, actor string, epoch int64, token string) (Session, error) {
	   if err := e.ready(); err != nil {
		  return Session{}, err
	   }
	   actor, err := requireActor(actor)
	   if err != nil {
		  return Session{}, err
	  }
	   e.mu.Lock()
	  ses, err := e.load(ctx, id)
	   if err != nil {
		   e.mu.Unlock()
		   return Session{}, err
	   }
	  if ses.State != StPlanned {
		  e.mu.Unlock()
		   if ses.State.Live() || ses.State == StClosed {
			   return ses, nil
		  }
		  return Session{}, ErrState
	   }
	  if !ses.FenceMatch(epoch, token) {
		  e.mu.Unlock()
		   return Session{}, ErrFence
	  }
	   exec := e.exec
	   e.mu.Unlock()
	   ids, note, err := runExec(exec, ses, actor)
	   if err != nil {
		  e.mu.Lock()
		   e.writeAudit(ctx, ses.ID, ses.BatchID, "exec_fail", actor, err.Error())
		   e.mu.Unlock()
		   return Session{}, err
	  }
	  e.mu.Lock()
	  defer e.mu.Unlock()
	  cur, err := e.load(ctx, ses.ID)
	   if err != nil {
		   return Session{}, err
	  }
	  if cur.State != StPlanned {
		  if cur.State.Live() {
			   return cur, nil
		  }
		  return Session{}, ErrState
	  }
	  if !cur.FenceMatch(epoch, token) {
		  return Session{}, ErrFence
	   }
	   now := e.now()
	  cur.State = StOpen
	  cur.AirgapID = ids.Air
	  cur.KeycerTicket = ids.Key
	   cur.CustodyID = ids.Cus
	  if note != "" {
		   cur.Note = clip(note, 240)
	   } else {
		   cur.Note = "ceremony open"
	  }
	  cur.LastActor = actor
	  cur.UpdatedAt = now
	  cur.FenceEpoch++
	   cur.FenceToken = persist.NewToken()
	  if err := e.st.PutSession(ctx, cur); err != nil {
		  return Session{}, err
	   }
	  e.writeAudit(ctx, cur.ID, cur.BatchID, "open", actor, cur.Note)
	   countTo(StOpen)
	   return cur, nil
   }

// Receipt 提交一份份额回执。达到阈值时进入 quorum_met。
// 同一人重复提交且槽位相同则返回原回执。

  func (e *Engine) Receipt(ctx context.Context, id, actor, slot, note string, epoch int64, token string) (Session, error) {
	  if err := e.ready(); err != nil {
		   return Session{}, err
	   }
	  actor, err := requireActor(actor)
	   if err != nil {
		   return Session{}, err
	  }
	   slot = strings.TrimSpace(slot)
	  if err := guardLabel(slot); err != nil {
		   return Session{}, err
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  ses, err := e.load(ctx, id)
	  if err != nil {
		  return Session{}, err
	   }
	   if ses.State != StOpen && ses.State != StQuorum {
		   return Session{}, ErrState
	   }
	  rcs, err := e.st.Receipts(ctx, ses.ID)
	  if err != nil {
		   return Session{}, err
	   }
	   for _, rc := range rcs {
		   if strings.EqualFold(rc.Actor, actor) {
			  if rc.SlotLabel == slot {
				  return ses, nil
			   }
			   return Session{}, ErrShare
		  }
	  }
	   if len(rcs) >= ses.Shares {
		   return Session{}, ErrQuorum
	  }
	   if !ses.FenceMatch(epoch, token) {
		  return Session{}, ErrFence
	  }
	   now := e.now()
	   rc := Receipt{
		  ID: uuid.NewString(), SessionID: ses.ID, Actor: actor, SlotLabel: slot,
		   ShareRef: shareRef(ses.ID,
			  actor, slot), Note: clip(note, 240), At: now,
	  }
	   if err := e.st.PutReceipt(ctx, rc); err != nil {
		  return Session{}, err
	   }
	   rcs = append(rcs, rc)
	  ses.ReceiptN = len(distinctActors(rcs))
	   if ses.State == StOpen && ses.ReceiptN >= ses.Threshold {
		   ses.State = StQuorum
		  countTo(StQuorum)
	   }
	   ses.LastActor = actor
	   ses.UpdatedAt = now
	   ses.FenceEpoch++
	  ses.FenceToken = persist.NewToken()
	   ses.Note = "receipt"
	  if err := e.st.PutSession(ctx, ses); err != nil {
		   return Session{}, err
	   }
	  e.writeAudit(ctx, ses.ID, ses.BatchID, "receipt", actor, slot)
	  if ses.State == StQuorum {
		  e.writeAudit(ctx, ses.ID, ses.BatchID, "quorum", actor, "met")
	  }
	   return ses, nil
  }

// Bind 把批次接到已达 quorum 的会话。one 档已有有效绑定时拒绝第二批。
  func (e *Engine) Bind(ctx context.Context, id, batch, actor string, epoch int64, token string) (Binding, error) {
	  if err := e.ready(); err != nil {
		   return Binding{}, err
	   }
	   actor, err := requireActor(actor)
	   if err != nil {
		  return Binding{}, err
	  }
	  batch, err = batchIDOf(batch)
	  if err != nil {
		  return Binding{}, err
	   }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   ses, err := e.load(ctx, id)
	  if err != nil {
		  return Binding{}, err
	   }
	   rcs, err := e.st.Receipts(ctx, ses.ID)
	   if err != nil {
		   return Binding{}, err
	   }
	  if len(distinctActors(rcs)) < ses.Threshold {
		  return Binding{}, ErrQuorum
	  }
	  if !actorIn(rcs, actor) {
		   return Binding{}, ErrActor
	  }
	   snap, ok, err := e.st.Snap(ctx, batch)
	  if err != nil {
		   return Binding{}, err
	  }
	  if !ok {
		  return Binding{}, ErrNotFound
	  }
	  if snap.Digest != ses.Digest && batch == ses.BatchID {
		   return Binding{}, ErrDigest
	  }
	  if ses.BindMode == ModeOne && batch != ses.BatchID {
		  return Binding{}, ErrBindMode
	   }
	   if ses.BindMode == ModeMany && batch != ses.BatchID && snap.Digest == "" {
		   return Binding{}, ErrDigest
	  }
	   existing, ok, err := e.st.ActiveByBatch(ctx, batch)
	  if err != nil {
		   return Binding{}, err
	   }
	  if ok {
		  if existing.SessionID == ses.ID && existing.Digest == snap.Digest {
			  return existing, nil
		   }
		  return Binding{}, ErrBindMode
	  }
	  binds, err := e.st.BindingsBySession(ctx, ses.ID)
	  if err != nil {
		   return Binding{}, err
	   }
	   active := activeBinds(binds)
	  if ses.BindMode == ModeOne && len(active) > 0 {
		  return Binding{}, ErrBindMode
	  }
	  if ses.State != StQuorum && !(ses.BindMode == ModeMany && ses.State == StBound) {
		   return Binding{}, ErrState
	   }
	   if !ses.FenceMatch(epoch, token) && len(active) == 0 {
		   return Binding{}, ErrFence
	  }
	  if ses.BindMode == ModeMany && ses.State == StBound && !ses.FenceMatch(epoch, token) {
		  return Binding{}, ErrFence
	   }
	   if batch == ses.BatchID && snap.Digest != ses.Digest {
		  return Binding{}, ErrDigest
	  }
	   if batch != ses.BatchID {
		  ps, err := e.st.Policies(ctx)
		  if err != nil {
			   return Binding{}, err
		  }
		  dec, err := e.DecideSnap(ps, snap)
		   if err != nil {
			  return Binding{}, err
		  }
		  if !dec.Required {
			  return Binding{}, ErrExempt
		   }
		   if dec.Threshold > ses.Threshold {
			  return Binding{}, ErrQuorum
		  }
	   }
	   now := e.now()
	  row := Binding{
		   ID:      uuid.NewString(),
		   BatchID: batch, SessionID: ses.ID, Digest: snap.Digest,
		   State: BindActive, BindMode: ses.BindMode, Actor: actor, BoundAt: now,
	   }
	  if err := e.st.PutBinding(ctx, row); err != nil {
		  return Binding{}, err
	   }
	   if ses.State == StQuorum {
		  ses.State = StBound
		  countTo(StBound)
	   }
	   ses.LastActor = actor
	  ses.UpdatedAt = now
	   ses.FenceEpoch++
	   ses.FenceToken = persist.NewToken()
	   ses.Note = "bound"
	  if err := e.st.PutSession(ctx, ses); err != nil {
		  return Binding{}, err
	   }
	   e.writeAudit(ctx, ses.ID, batch, "bind", actor, snap.Digest)
	  return row, nil
  }

// MarkReady 在已绑定且份额仍够时进入 sign_ready。

  func (e *Engine) MarkReady(ctx context.Context, id, actor string, epoch int64, token string) (Session, error) {
	   if err := e.ready(); err != nil {
		  return Session{}, err
	   }
	  actor, err := requireActor(actor)
	  if err != nil {
		   return Session{}, err
	  }
	   e.mu.Lock()
	  defer e.mu.Unlock()
	   ses, err := e.load(ctx, id)
	  if err != nil {
		   return Session{}, err
	  }
	   if ses.State == StReady {
		  return ses, nil
	   }
	   if ses.State != StBound {
		  return Session{}, ErrState
	  }
	   rcs,
		   err := e.st.Receipts(ctx, ses.ID)
	  if err != nil {
		  return Session{}, err
	  }
	   if len(distinctActors(rcs)) < ses.Threshold {
		  return Session{}, ErrQuorum
	   }
	  if !actorIn(rcs, actor) {
		  return Session{}, ErrActor
	   }
	   binds,
		   err := e.st.BindingsBySession(ctx, ses.ID)
	  if err != nil {
		  return Session{}, err
	  }
	  active := activeBinds(binds)
	   if len(active) == 0 {
		  return Session{}, ErrDenied
	  }
	  if ses.BindMode == ModeOne && len(active) != 1 {
		  return Session{}, ErrBindMode
	  }
	  for _, b := range active {
		   snap, ok, err := e.st.Snap(ctx, b.BatchID)
		  if err != nil {
			  return Session{}, err
		  }
		   if !ok || snap.Digest != b.Digest {
			   return Session{}, ErrDigest
		   }
		  if b.BatchID == ses.BatchID && snap.Digest != ses.Digest {
			   return Session{}, ErrDigest
		  }
	   }
	  if !ses.FenceMatch(epoch, token) {
		  return Session{}, ErrFence
	   }
	  if !ses.Deadline.IsZero() && e.now().After(ses.Deadline) {
		  _, _ = e.abortLocked(ctx, ses, e.timer, "timeout")
		   return Session{}, ErrTimeout
	   }
	   now := e.now()
	   ses.State = StReady
	   ses.LastActor = actor
	   ses.UpdatedAt = now
	   ses.Note = "sign ready"
	  ses.FenceEpoch++
	   ses.FenceToken = persist.NewToken()
	   if err := e.st.PutSession(ctx, ses); err != nil {
		   return Session{}, err
	  }
	   e.writeAudit(ctx, ses.ID, ses.BatchID, "sign_ready", actor, "ready")
	  countTo(StReady)
	   return ses, nil
   }

// Close 签播完成后关闭会话，绑定改为 consumed，不能再当放行凭据。
  func (e *Engine) Close(ctx context.Context, id, actor string, epoch int64, token string) (Session, error) {
	   if err := e.ready(); err != nil {
		   return Session{}, err
	  }
	  actor, err := requireActor(actor)
	  if err != nil {
		  return Session{}, err
	   }
	  e.mu.Lock()
	   defer e.mu.Unlock()
	  ses, err := e.load(ctx, id)
	  if err != nil {
		  return Session{}, err
	   }
	  if ses.State == StClosed {
		  return ses, nil
	   }
	  if ses.State != StReady {
		   return Session{}, ErrState
	  }
	   rcs,
		  err := e.st.Receipts(ctx, ses.ID)
	   if err != nil {
		   return Session{}, err
	  }
	  if !actorIn(rcs, actor) {
		  return Session{}, ErrActor
	  }
	   if strings.EqualFold(actor, ses.LastActor) {
		   return Session{}, ErrActor
	   }
	   if !ses.FenceMatch(epoch, token) {
		   return Session{}, ErrFence
	   }
	   now := e.now()
	   if err := e.consumeLocked(ctx, ses.ID, now); err != nil {
		   return Session{}, err
	   }
	  ses.State = StClosed
	  ses.LastActor = actor
	  ses.UpdatedAt = now
	   ses.Note = "closed"
	   ses.FenceEpoch++
	  ses.FenceToken = persist.NewToken()
	   if err := e.st.PutSession(ctx, ses); err != nil {
		   return Session{}, err
	   }
	   e.writeAudit(ctx, ses.ID, ses.BatchID, "close", actor, "closed")
	   countTo(StClosed)
	   return ses, nil
  }

// Abort 中止会话并释放全部有效绑定。已中止的调用按幂等返回。
// 不变量：返回后该批次没有 active 绑定，签播闸必须拒绝。

   func (e *Engine) Abort(ctx context.Context, id, actor, reason string) (Session, error) {
	  if err := e.ready(); err != nil {
		   return Session{}, err
	  }
	   actor, err := requireActor(actor)
	   if err != nil {
		   return Session{}, err
	   }
	   reason = clip(reason, 240)
	  if reason == "" {
		  reason = "abort"
	  }
	   e.mu.Lock()
	   defer e.mu.Unlock()
	   ses, err := e.load(ctx, id)
	   if err != nil {
		   return Session{}, err
	  }
	   return e.abortLocked(ctx, ses, actor, reason)
  }

  func (e *Engine) abortLocked(ctx context.Context, ses Session, actor, reason string) (Session, error) {
	  if ses.State == StAborted {
		  return ses, nil
	  }
	   if ses.State == StClosed {
		   return Session{}, ErrState
	  }
	  now := e.now()
	   n, err := e.releaseLocked(ctx, ses.ID, now)
	  if err != nil {
		   return Session{}, err
	   }
	   ses.State = StAborted
	  ses.AbortReason = reason
	  ses.LastActor = actor
	  ses.UpdatedAt = now
	  ses.Note = reason
	  ses.FenceEpoch++
	   ses.FenceToken = persist.NewToken()
	   if err := e.st.PutSession(ctx, ses); err != nil {
		  return Session{}, err
	  }
	  e.writeAudit(ctx, ses.ID, ses.BatchID, "abort", actor, reason)
	  e.writeAudit(ctx, ses.ID, ses.BatchID, "release", actor, "n="+itoa(n))
	   countTo(StAborted)
	   if reason == "timeout" {
		  timeoutTotal.Inc()
	  }
	   return ses, nil
  }

   func (e *Engine) releaseLocked(ctx context.Context, sessionID string, now time.Time) (int, error) {
	  binds,
		  err := e.st.BindingsBySession(ctx, sessionID)
	  if err != nil {
		   return 0, err
	   }
	   n := 0
	   for _, b := range binds {
		   if b.State != BindActive {
			   continue
		  }
		  b.State = BindReleased
		   b.ReleasedAt = now
		   if err := e.st.PutBinding(ctx, b); err != nil {
			  return n, err
		   }
		   n++
	   }
	   return n, nil
   }

   func (e *Engine) consumeLocked(ctx context.Context, sessionID string, now time.Time) error {
	   binds, err := e.st.BindingsBySession(ctx, sessionID)
	   if err != nil {
		   return err
	  }
	   for _, b := range binds {
		  if b.State != BindActive {
			   continue
		   }
		  b.State = BindConsumed
		  b.ReleasedAt = now
		   if err := e.st.PutBinding(ctx, b); err != nil {
			   return err
		   }
	   }
	   return nil
  }

  func distinctActors(in []Receipt) []string {
	   seen := map[string]struct{}{}
	   out := make([]string, 0, len(in))
	   for _, rc := range in {
		  k := strings.ToLower(rc.Actor)
		  if _, ok := seen[k]; ok {
			   continue
		   }
		   seen[k] = struct{}{}
		  out = append(out, rc.Actor)
	   }
	  return out
  }

  func actorIn(in []Receipt, actor string) bool {
	  for _, rc := range in {
		  if strings.EqualFold(rc.Actor, actor) {
			   return true
		  }
	  }
	  return false
   }

   func activeBinds(in []Binding) []Binding {
	   out := make([]Binding, 0)
	  for _, b := range in {
		   if b.State == BindActive {
			   out = append(out, b)
		   }
	  }
	  return out
   }

  func shareRef(sessionID, actor, label string) string {
	   sum := sha256.Sum256([]byte(sessionID + "|" + strings.ToLower(actor) + "|" + label))
	   return hex.EncodeToString(sum[:])
   }

   func guardLabel(label string) error {
	  if label == "" || len(label) > 64 {
		   return ErrShare
	  }
	  for _, r := range label {
		   if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			  continue
		  }
		   return ErrShare
	  }
	   low := strings.ToLower(label)
	  for _, bad := range []string{"mnemonic", "seed", "xprv", "privkey", "secret"} {
		   if strings.Contains(low, bad) {
			  return ErrShare
		   }
	  }
	  return nil
  }

  func itoa(n int) string {
	  if n == 0 {
		  return "0"
	  }
	   var b [16]byte
	   i := len(b)
	   neg := n < 0
	   if neg {
		   n = -n
	  }
	   for n > 0 {
		  i--
		  b[i] = byte('0' + n%10)
		  n /= 10
	  }
	   if neg {
		  i--
		  b[i] = '-'
	  }
	  return string(b[i:])
   }
