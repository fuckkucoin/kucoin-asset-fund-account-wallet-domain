package wdrelease

import (
	"context"
	"strings"
	"time"
	"github.com/shopspring/decimal"
)

   func (d *Desk) Open(ctx context.Context, in Input) (Ticket, error) {
	  if d == nil {
		  return Ticket{}, ErrUnavailable
	   }
	  d.mu.Lock()
	   defer d.mu.Unlock()
	  t, err := d.openLocked(ctx, in)
	  if err != nil {
		   return Ticket{}, err
	   }
	  d.emitLocked()
	  return t, nil
   }

  func (d *Desk) openLocked(ctx context.Context, in Input) (Ticket, error) {
	   if d.halted != nil {
		   return Ticket{}, d.halted
	   }
	   norm,
		  amt, err := normalizeInput(in)
	   if err != nil {
		  return Ticket{}, err
	   }
	  if norm.Idem == "" {
		   return Ticket{}, WithCode(ErrConflict, CodeConflict)
	  }
	  if _, err := trimIdem(norm.Idem); err != nil {
		  return Ticket{}, err
	  }
	  applicant := norm.Applicant
	   if applicant == "" {
		   applicant = norm.UserID
	  }
	  applicant, err = trimActor(applicant)
	  if err != nil {
		  return Ticket{}, err
	  }
	   norm.Applicant = applicant
	   ver, drift, err := d.pickVersion(norm.Version)
	  if err != nil {
		   return Ticket{}, err
	  }
	   norm.Version = ver.ID
	  if id := d.byIdem[norm.Idem]; id != "" {
		  cur := d.tickets[id]
		  if cur == nil || !sameFacts(cur, norm) || (in.Version != "" && cur.PolicyVer != ver.ID) {
			  return Ticket{}, WithCode(ErrConflict, CodeConflict)
		  }
		   return cloneTicket(cur), nil
	   }
	   dec, err := decide(ver, norm, amt)
	   if err != nil {
		  return Ticket{}, err
	   }
	  dec.Drift = drift
	   dec.Reason = explain(dec)
	   if dec.Deny {
		   code := dec.Code
		   if code == "" {
			   code = CodePolicy
		   }
		   return Ticket{}, WithCode(ErrDenied, code)
	   }
	   if d.openCount(norm.UserID) >= ver.MaxOpen {
		   return Ticket{},
			   WithCode(ErrDenied, CodeOpenHold)
	   }
	   now := d.now()
	   segs, err := buildSegments(dec, now)
	   if err != nil {
		  return Ticket{}, err
	   }
	   st := StateDraft
	  if dec.Skip {
		  if err := mustChain(StateDraft, StateApproved); err != nil {
			  return Ticket{}, err
		   }
		   st = StateApproved
	   }
	   t := &Ticket{
		  ID: d.freshID(),
		   Idem: norm.Idem,
		  UserID: norm.UserID,
		  Asset:  norm.Asset,
		   Amount: norm.Amount,
		   Chain:     norm.Chain,
		  Dest:      norm.Dest,
		   DestClass: norm.DestClass,
		   Risk:      norm.Risk,
		   PolicyVer: ver.ID,
		   State: st,
		  Depth:     dec.Depth,
		  BaseDepth: dec.BaseDepth,
		   Skip: dec.Skip,
		   Timeout: dec.Timeout,
		  SLASec:    dec.SLASec,
		   Applicant: applicant,
		  Segments:  segs,
		   CreatedAt: now,
		  UpdatedAt: now,
	   }
	   if dec.SLASec > 0 {
		   t.Deadline = now.Add(timeSeconds(dec.SLASec))
	  }
	  norm.Version = ver.ID
	   t.FP = fingerprint(norm)
	   recompute(t)
	   if err := d.chargeOpen(t, dec.DailyCap, dec.DestCap); err != nil {
		   return Ticket{}, err
	  }
	   d.tickets[t.ID] = t
	  d.byIdem[t.Idem] = t.ID
	   d.seenAsset[t.Asset] = struct{}{}
	  d.appendAudit(t, StepOpen, applicant, "", now)
	   if dec.Skip {
		  d.appendAudit(t, StepApprove, "policy-skip", "", now)
	  }
	   if drift {
		  d.noteDriftLocked(t.ID)
	  }
	   if err := d.persist(ctx, t); err != nil {
		   return Ticket{}, err
	  }
	  return cloneTicket(t), nil
  }

   func timeSeconds(sec int64) time.Duration {
	   return time.Duration(sec) * time.Second
   }

  func (d *Desk) Submit(ctx context.Context, id, actor string) (Ticket, error) {
	   if d == nil {
		   return Ticket{}, ErrUnavailable
	   }
	  d.mu.Lock()
	   defer d.mu.Unlock()
	   t, err := d.submitLocked(ctx, id, actor)
	   if err != nil {
		  return Ticket{}, err
	  }
	   d.emitLocked()
	  return t, nil
   }

   func (d *Desk) submitLocked(ctx context.Context, id, actor string) (Ticket, error) {
	  t := d.tickets[strings.TrimSpace(id)]
	   if t == nil {
		   return Ticket{}, WithCode(ErrNoTicket, CodeNoTicket)
	  }
	  who, err := trimActor(actor)
	   if err != nil {
		  return Ticket{}, err
	   }
	  if !sameActor(who, t.Applicant) {
		   d.bumpPrivilege(CodeApplicant)
		   return Ticket{}, WithCode(ErrApplicant, CodeApplicant)
	  }
	  if t.State == StateApproved && t.Skip {
		   return cloneTicket(t), nil
	  }
	   if t.State != StateDraft {
		   return Ticket{}, WithCode(ErrClosed, CodeClosed)
	  }
	   if err := mustChain(t.State, StatePending); err != nil {
		  return Ticket{}, err
	   }
	   now := d.now()
	  t.State = StatePending
	   t.UpdatedAt = now
	  if t.SLASec > 0 {
		   t.Deadline = now.Add(timeSeconds(t.SLASec))
	  }
	   t.Steps = append(t.Steps, Step{Seq: len(t.Steps) + 1, Kind: StepSubmit, Actor: who, At: now})
	  d.appendAudit(t, StepSubmit, who, "", now)
	   if err := d.persist(ctx, t); err != nil {
		  return Ticket{}, err
	  }
	  return cloneTicket(t), nil
  }

// Advance 通过或拒绝当前级数。allow 为假时走拒绝。

  func (d *Desk) Advance(ctx context.Context, id, actor, role string, allow bool) (Ticket, error) {
	  if d == nil {
		  return Ticket{}, ErrUnavailable
	  }
	  d.mu.Lock()
	   defer d.mu.Unlock()
	   var (
		   t   Ticket
		  err error
	  )
	   if !allow {
		  t, err = d.rejectLocked(ctx, id, actor, role, CodeRejected, "")
	   } else {
		   t, err = d.approveLocked(ctx, id, actor, role)
	  }
	   d.emitLocked()
	  return t, err
   }

  func (d *Desk) approveLocked(ctx context.Context, id, actor, role string) (Ticket, error) {
	  t := d.tickets[strings.TrimSpace(id)]
	  if t == nil {
		   return Ticket{}, WithCode(ErrNoTicket, CodeNoTicket)
	   }
	  who, err := trimActor(actor)
	   if err != nil {
		  return Ticket{}, err
	  }
	  role = strings.ToLower(strings.TrimSpace(role))
	   if t.State != StatePending && t.State != StatePartial {
		   return Ticket{}, WithCode(ErrClosed, CodeClosed)
	   }
	   if sameActor(who, t.Applicant) {
		   d.bumpPrivilege(CodeApplicant)
		   return cloneTicket(t),
			   WithCode(ErrApplicant, CodeApplicant)
	  }
	  for _, st := range t.Steps {
		   if st.Kind == StepApprove && sameActor(st.Actor, who) {
			   d.bumpPrivilege(CodeSame)
			  return d.samePersonLocked(ctx, t, who)
		  }
	   }
	   next := t.Approvals + 1
	  if next > t.Depth || t.Depth < 1 {
		  return Ticket{}, WithCode(ErrClosed, CodeClosed)
	  }
	   if !roleMatches(next, role) {
		   d.bumpPrivilege(CodeRole)
		  return cloneTicket(t), WithCode(ErrRole, CodeRole)
	   }
	  to := StatePartial
	  if next >= t.Depth {
		   to = StateApproved
	  }
	   if err := mustChain(t.State, to); err != nil {
		  return Ticket{}, err
	   }
	  now := d.now()
	   t.Approvals = next
	  t.State = to
	  t.UpdatedAt = now
	  t.Steps = append(t.Steps, Step{
		   Seq: len(t.Steps) + 1, Kind: StepApprove, Level: next, Actor: who, Role: role, At: now,
	  })
	   authorizeSegments(t, who, now)
	   recompute(t)
	   d.appendAudit(t, StepApprove, who, "", now)
	  if err := d.persist(ctx, t); err != nil {
		  return Ticket{}, err
	   }
	  return cloneTicket(t), nil
   }

  func (d *Desk) samePersonLocked(ctx context.Context, t *Ticket, actor string) (Ticket, error) {
	   ver,
		   ok := d.book.Get(t.PolicyVer)
	   hard := ok && ver.SamePersonHard
	  if !hard {
		   return cloneTicket(t), WithCode(ErrSame, CodeSame)
	   }
	   if err := mustChain(t.State, StateRejected); err != nil {
		   return cloneTicket(t), WithCode(ErrSame, CodeSame)
	   }
	  now := d.now()
	   t.State = StateRejected
	  t.Code = CodeSame
	  t.UpdatedAt = now
	   t.Steps = append(t.Steps, Step{
		   Seq: len(t.Steps) + 1, Kind: StepReject, Actor: actor, Code: CodeSame, At: now,
	   })
	   cancelUnused(t)
	  recompute(t)
	  d.releaseUnfrozen(t)
	  d.appendAudit(t,
		  StepReject, actor, CodeSame, now)
	   if err := d.persist(ctx, t); err != nil {
		   return Ticket{}, err
	  }
	  return cloneTicket(t), WithCode(ErrSame, CodeSame)
  }

// Reject 复核拒绝或申请人撤回草稿。未冻结段取消，已冻结段不动。
  func (d *Desk) Reject(ctx context.Context, id, actor, role, code, note string) (Ticket, error) {
	  if d == nil {
		   return Ticket{}, ErrUnavailable
	  }
	  d.mu.Lock()
	  defer d.mu.Unlock()
	   t,
		   err := d.rejectLocked(ctx, id, actor, role, code, note)
	  d.emitLocked()
	   return t, err
  }

  func (d *Desk) rejectLocked(ctx context.Context, id, actor, role, code, note string) (Ticket, error) {
	  t := d.tickets[strings.TrimSpace(id)]
	  if t == nil {
		   return Ticket{}, WithCode(ErrNoTicket, CodeNoTicket)
	  }
	   who, err := trimActor(actor)
	   if err != nil {
		  return Ticket{}, err
	   }
	  if code == "" {
		   code = CodeRejected
	  }
	  if !knownCode(code) && code != CodeApplicantCancel {
		   code = CodeRejected
	   }
	  now := d.now()
	  if sameActor(who, t.Applicant) {
		  if t.State != StateDraft {
			  d.bumpPrivilege(CodeApplicant)
			   return cloneTicket(t), WithCode(ErrApplicant, CodeApplicant)
		   }
		  if err := mustChain(t.State, StateRejected); err != nil {
			  return Ticket{}, err
		  }
		   t.State = StateRejected
		  t.Code = CodeApplicantCancel
		  t.Note = strings.TrimSpace(note)
		  t.UpdatedAt = now
		  t.Steps = append(t.Steps, Step{Seq: len(t.Steps) + 1, Kind: StepReject, Actor: who, Code: t.Code, Note: t.Note, At: now})
		  cancelUnused(t)
		   recompute(t)
		   d.releaseUnfrozen(t)
		   d.appendAudit(t,
			  StepReject, who, t.Code, now)
		  if err := d.persist(ctx, t); err != nil {
			   return Ticket{}, err
		  }
		   return cloneTicket(t), nil
	  }
	   role = strings.ToLower(strings.TrimSpace(role))
	  if !KnownRole(role) {
		  d.bumpPrivilege(CodeRole)
		  return cloneTicket(t), WithCode(ErrRole, CodeRole)
	  }
	   if !openStates(t.State) || t.State == StateDraft {
		   return Ticket{}, WithCode(ErrClosed, CodeClosed)
	   }
	   if err := mustChain(t.State, StateRejected); err != nil {
		   return Ticket{}, err
	   }
	   t.State = StateRejected
	  t.Code = code
	   t.Note = strings.TrimSpace(note)
	   t.UpdatedAt = now
	  t.Steps = append(t.Steps, Step{Seq: len(t.Steps) + 1, Kind: StepReject, Actor: who, Role: role, Code: code, Note: t.Note, At: now})
	   cancelUnused(t)
	  recompute(t)
	  d.releaseUnfrozen(t)
	   d.appendAudit(t,
		   StepReject, who, code, now)
	  if err := d.persist(ctx, t); err != nil {
		   return Ticket{}, err
	  }
	   return cloneTicket(t), nil
  }

// AuthorizeSegment 按已通过的级数确认一段。不能跳过复核，也不能带覆盖标记。

   func (d *Desk) AuthorizeSegment(ctx context.Context, id, actor string, seq int, override bool) (Ticket, error) {
	  if d == nil {
		  return Ticket{}, ErrUnavailable
	  }
	  d.mu.Lock()
	  defer d.mu.Unlock()
	   t := d.tickets[strings.TrimSpace(id)]
	   if t == nil {
		   return Ticket{}, WithCode(ErrNoTicket, CodeNoTicket)
	   }
	   if override {
		   d.bumpPrivilege(CodePrivilege)
		  d.emitLocked()
		  return cloneTicket(t), WithCode(ErrDenied, CodePrivilege)
	   }
	  who, err := trimActor(actor)
	  if err != nil {
		  d.emitLocked()
		   return Ticket{}, err
	  }
	   if sameActor(who, t.Applicant) {
		  d.bumpPrivilege(CodeApplicant)
		   d.emitLocked()
		  return cloneTicket(t), WithCode(ErrApplicant, CodeApplicant)
	  }
	   var seg *Segment
	   for i := range t.Segments {
		  if t.Segments[i].Seq == seq {
			   seg = &t.Segments[i]
			  break
		  }
	  }
	   if seg == nil {
		  d.emitLocked()
		  return Ticket{}, WithCode(ErrSegment, CodeQuota)
	  }
	  if seg.State == SegAuthorized || seg.State == SegReserved || seg.State == SegFrozen {
		  d.emitLocked()
		  return cloneTicket(t), nil
	   }
	   if seg.State != SegPlanned {
		   d.emitLocked()
		  return cloneTicket(t), WithCode(ErrSegment, CodeClosed)
	  }
	   need := seg.Need
	   if seq == len(t.Segments) && t.Depth > need {
		   need = t.Depth
	  }
	  if t.Approvals < need {
		  d.bumpPrivilege(CodePrivilege)
		  d.emitLocked()
		  return cloneTicket(t), WithCode(ErrDenied, CodePrivilege)
	  }
	   now := d.now()
	   seg.State = SegAuthorized
	  seg.By = who
	  seg.At = now
	   t.UpdatedAt = now
	   recompute(t)
	   d.appendAudit(t, "authorize", who, "", now)
	  if err := d.persist(ctx, t); err != nil {
		   d.emitLocked()
		   return Ticket{}, err
	   }
	   d.emitLocked()
	  return cloneTicket(t), nil
   }

  func frozenDecimal(t *Ticket) decimal.Decimal {
	   if t == nil || t.Frozen == "" {
		  return decimal.Zero
	   }
	  v, err := decimal.NewFromString(t.Frozen)
	   if err != nil {
		  return decimal.Zero
	   }
	   return v
   }

// FrozenAmount 已冻结段合计，拒绝后不得下降。

  func FrozenAmount(t Ticket) decimal.Decimal {
	  return frozenDecimal(&t)
   }
