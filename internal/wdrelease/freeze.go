package wdrelease

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/withdraw"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/shopspring/decimal"
)

// BeforeFreeze 在 ledger.Create 之前调用。没有票据或已授权额度不足则失败关闭。
  func (d *Desk) BeforeFreeze(ctx context.Context, ask withdraw.ReleaseAsk) (withdraw.ReleaseGrant, error) {
	  if d == nil {
		  return withdraw.ReleaseGrant{}, ErrUnavailable
	   }
	   d.mu.Lock()
	  defer d.mu.Unlock()
	   g, err := d.beforeLocked(ctx, ask)
	  d.emitLocked()
	   if err != nil {
		  return withdraw.ReleaseGrant{}, err
	  }
	   return withdraw.ReleaseGrant{TicketID: g.TicketID, Idem: g.Idem, Amount: g.Amount}, nil
   }

// CommitFreeze 账本已冻之后把占用段写成 frozen。重复调用保持原样。

   func (d *Desk) CommitFreeze(ctx context.Context, grant withdraw.ReleaseGrant) error {
	  if d == nil {
		  return ErrUnavailable
	  }
	   d.mu.Lock()
	   defer d.mu.Unlock()
	   err := d.commitLocked(ctx, grant.Idem)
	   d.emitLocked()
	  return err
  }

// AbortFreeze 账本没有冻成时退回占用。已经 frozen 的段不退，避免在本域解冻。
   func (d *Desk) AbortFreeze(ctx context.Context, grant withdraw.ReleaseGrant) error {
	   if d == nil {
		  return ErrUnavailable
	  }
	   d.mu.Lock()
	  defer d.mu.Unlock()
	  err := d.abortLocked(ctx, grant.Idem)
	  d.emitLocked()
	  return err
  }

// AdmitPath 出金路径预检。只读额度，不占用。小额跳过放行，大额没有票据则失败关闭。

  func (d *Desk) AdmitPath(ctx context.Context, userID, asset, amount, chain, dest, idem, ticketID string, risk int, destClass string) error {
	   if d == nil {
		  return ErrUnavailable
	   }
	  _ = ctx
	   d.mu.Lock()
	  defer d.mu.Unlock()
	  err := d.admitLocked(userID,
		   asset, amount, chain, dest, idem, ticketID, risk, destClass)
	  d.emitLocked()
	  return err
   }

// RefuseLiveUnfreeze 放行域不提供解冻。无论 hash 是否仍在链上，都拒绝。
  func RefuseLiveUnfreeze(liveHash string) error {
	   _ = strings.TrimSpace(liveHash)
	  return WithCode(ErrLive, CodeLive)
   }

  func (d *Desk) beforeLocked(ctx context.Context, ask withdraw.ReleaseAsk) (Grant, error) {
	  if d.halted != nil {
		   return Grant{}, d.halted
	  }
	  idem, err := trimIdem(ask.Idem)
	  if err != nil {
		  return Grant{}, err
	  }
	   amt, err := money.Parse(ask.Amount)
	   if err != nil {
		   return Grant{}, WithCode(ErrAmount, CodeAmount)
	   }
	   if g := d.grants[idem]; g != nil && (g.Phase == PhaseReserved || g.Phase == PhaseFrozen) {
		   if g.Amount != amt.String() {
			   return Grant{}, WithCode(ErrConflict, CodeConflict)
		   }
		  return *g, nil
	   }
	   in := Input{
		   UserID: ask.UserID, Asset: ask.Asset, Amount: ask.Amount, Chain: ask.Chain,
		  Dest: ask.Dest, DestClass: ask.DestClass, Risk: ask.Risk, Idem: idem, Applicant: ask.UserID,
	  }
	   norm, _, err := normalizeInput(in)
	  if err != nil {
		   return Grant{}, err
	   }
	   origClass := strings.TrimSpace(ask.DestClass)
	  t,
		  err := d.locate(norm, strings.TrimSpace(ask.TicketID), origClass, ask.Risk)
	   if err != nil {
		  return Grant{}, err
	  }
	  if t == nil {
		  dec, _, err := d.evalFor(norm)
		  if err != nil {
			   return Grant{}, err
		   }
		  if dec.Deny {
			  code := dec.Code
			  if code == "" {
				  code = CodePolicy
			   }
			   return Grant{}, WithCode(ErrDenied, code)
		  }
		  if !dec.Skip {
			   return Grant{},
				   WithCode(ErrNoTicket, CodeNoTicket)
		   }
		  opened, err := d.openLocked(ctx, norm)
		  if err != nil {
			  return Grant{}, err
		  }
		  t = d.tickets[opened.ID]
	  }
	   if t == nil {
		   return Grant{},
			   WithCode(ErrNoTicket, CodeNoTicket)
	   }
	   seqs, err := takeAuthorized(t, amt.Dec())
	  if err != nil {
		   return Grant{}, err
	   }
	  now := d.now()
	  if err := markSeq(t, seqs, SegReserved, "", now); err != nil {
		   return Grant{}, err
	   }
	  recompute(t)
	  t.UpdatedAt = now
	  g := &Grant{Idem: idem, TicketID: t.ID, Amount: amt.String(), Seqs: seqs, Phase: PhaseReserved}
	   d.grants[idem] = g
	  d.appendAudit(t,
		  StepFreeze, t.UserID, PhaseReserved, now)
	  if err := d.persist(ctx, t); err != nil {
		   return Grant{}, err
	   }
	   if err := d.persistGrant(ctx, g); err != nil {
		   return Grant{}, err
	  }
	   return *g, nil
   }

   func (d *Desk) locate(in Input, ticketID, origClass string, origRisk int) (*Ticket, error) {
	  var t *Ticket
	   if ticketID != "" {
		   t = d.tickets[ticketID]
		  if t == nil {
			  return nil,
				   WithCode(ErrNoTicket, CodeNoTicket)
		  }
	  } else if id := d.byIdem[in.Idem]; id != "" {
		  t = d.tickets[id]
	   }
	  if t == nil {
		   return nil, nil
	  }
	  if t.UserID != in.UserID || t.Asset != in.Asset || t.Chain != in.Chain || !strings.EqualFold(t.Dest, in.Dest) {
		   return nil, WithCode(ErrFingerprint, CodeFingerprint)
	  }
	  if origClass != "" && !strings.EqualFold(t.DestClass, origClass) {
		  return nil, WithCode(ErrFingerprint, CodeFingerprint)
	  }
	   if origRisk != 0 && t.Risk != origRisk {
		  return nil, WithCode(ErrFingerprint, CodeFingerprint)
	  }
	  return t, nil
   }

   func (d *Desk) evalFor(in Input) (Decision, bool, error) {
	  ver,
		   drift, err := d.pickVersion(in.Version)
	   if err != nil {
		   return Decision{}, false, err
	   }
	   amt, err := decimal.NewFromString(in.Amount)
	  if err != nil {
		  return Decision{}, false, WithCode(ErrAmount, CodeAmount)
	   }
	  dec, err := decide(ver, in, amt)
	  if err != nil {
		  return Decision{}, false, err
	   }
	   return dec, drift, nil
  }

   func (d *Desk) commitLocked(ctx context.Context, idem string) error {
	   g := d.grants[strings.TrimSpace(idem)]
	  if g == nil {
		  return WithCode(ErrNoTicket, CodeNoTicket)
	  }
	  if g.Phase == PhaseFrozen {
		  return nil
	  }
	  if g.Phase != PhaseReserved {
		  return WithCode(ErrClosed,
			  CodeClosed)
	  }
	  t := d.tickets[g.TicketID]
	  if t == nil {
		   return WithCode(ErrNoTicket, CodeNoTicket)
	  }
	  now := d.now()
	   if err := markSeq(t, g.Seqs, SegFrozen, "", now); err != nil {
		  return err
	   }
	  part, err := decimal.NewFromString(g.Amount)
	   if err != nil {
		   return WithCode(ErrAmount, CodeAmount)
	  }
	   recompute(t)
	  d.moveFrozen(t, part)
	   t.UpdatedAt = now
	   g.Phase = PhaseFrozen
	  d.appendAudit(t, StepFreeze, t.UserID, PhaseFrozen, now)
	  if err := d.persist(ctx, t); err != nil {
		  return err
	   }
	  return d.persistGrant(ctx, g)
  }

  func (d *Desk) abortLocked(ctx context.Context, idem string) error {
	   g := d.grants[strings.TrimSpace(idem)]
	   if g == nil {
		  return nil
	   }
	  if g.Phase == PhaseFrozen {
		   return WithCode(ErrFrozen,
			  CodeLive)
	   }
	  if g.Phase == PhaseAborted {
		   return nil
	   }
	  t := d.tickets[g.TicketID]
	   if t == nil {
		  return WithCode(ErrNoTicket, CodeNoTicket)
	   }
	   now := d.now()
	   if err := markSeq(t, g.Seqs, SegAuthorized, "", now); err != nil {
		  return err
	  }
	   recompute(t)
	  t.UpdatedAt = now
	   g.Phase = PhaseAborted
	  d.appendAudit(t, StepAbort, t.UserID, PhaseAborted, now)
	   if err := d.persist(ctx, t); err != nil {
		  return err
	   }
	  return d.persistGrant(ctx, g)
  }

  func (d *Desk) admitLocked(userID, asset, amount, chain, dest, idem, ticketID string, risk int, destClass string) error {
	   if d.halted != nil {
		  return d.halted
	   }
	   in := Input{
		   UserID: userID, Asset: asset, Amount: amount, Chain: chain, Dest: dest,
		   DestClass: destClass, Risk: risk, Idem: idem, Applicant: userID,
	   }
	  norm, amt, err := normalizeInput(in)
	   if err != nil {
		  return err
	  }
	   if idem != "" {
		   if _, err := trimIdem(idem); err != nil {
			  return err
		   }
		   norm.Idem = idem
	   }
	   dec, _, err := d.evalFor(norm)
	   if err != nil {
		  return err
	   }
	  if dec.Deny {
		  code := dec.Code
		   if code == "" {
			  code = CodePolicy
		  }
		  return WithCode(ErrDenied,
			  code)
	  }
	  if dec.Skip {
		  return nil
	  }
	   t, err := d.locate(norm, strings.TrimSpace(ticketID), strings.TrimSpace(destClass), risk)
	   if err != nil {
		   return err
	   }
	  if t == nil {
		  return WithCode(ErrNoTicket, CodeNoTicket)
	   }
	   if _, err := takeAuthorized(t, amt); err != nil {
		  return err
	  }
	   return nil
  }
