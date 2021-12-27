package wdrelease

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// OccDesk 出金占用台（与多级放行 Desk 并存）。

type OccDesk struct {
	store OccStore
	clock func() time.Time
}

// NewOccDesk 绑定存储。

  func NewOccDesk(store OccStore) *OccDesk {
	   return &OccDesk{store: store, clock: time.Now}
  }

// WithClock 时钟。
  func (d *OccDesk) WithClock(fn func() time.Time) *OccDesk {
	  d.clock = fn
	  return d
   }

  func (d *OccDesk) now() time.Time {
	  if d.clock != nil {
		  return d.clock().UTC()
	  }
	  return time.Now().UTC()
  }

   func (d *OccDesk) audit(ctx context.Context, ticketID, grantID, from, to, actor, note string) {
	   _ = d.store.InsertAudit(ctx, &OccAudit{
		   ID: uuid.NewString(), TicketID: ticketID, GrantID: grantID,
		   FromState: from, ToState: to, Actor: actor, Note: note, CreatedAt: d.now(),
	   })
  }

   func trim(s string) string { return strings.TrimSpace(s) }

   func parseAmt(s string) (decimal.Decimal, error) {
	   d, err := decimal.NewFromString(strings.TrimSpace(s))
	   if err != nil || !d.IsPositive() {
		   return decimal.Zero, ErrAmount
	  }
	   return d, nil
  }

// OpenTicket 开具待审票据。
   func (d *OccDesk) OpenTicket(ctx context.Context, userID, asset, amount string) (*OccTicket, error) {
	   if d == nil || d.store == nil {
		   return nil, ErrUnavailable
	  }
	   uid, ast := trim(userID), strings.ToUpper(trim(asset))
	  if uid == "" || ast == "" {
		  return nil, fmt.Errorf("wdrelease: user_id and asset required")
	   }
	  if _, err := parseAmt(amount); err != nil {
		  return nil, err
	  }
	  now := d.now()
	  t := &OccTicket{
		   ID: uuid.NewString(), UserID: uid, Asset: ast, Amount: amount,
		   State: OccTicketPending, Epoch: 1, CreatedAt: now, UpdatedAt: now,
	   }
	   if err := d.store.InsertTicket(ctx, t); err != nil {
		  return nil, err
	   }
	   d.audit(ctx, t.ID, "", "", string(OccTicketPending), "desk", "open")
	   return t, nil
  }

// Admit 准入占用轮次。
   func (d *OccDesk) Admit(ctx context.Context, ticketID string) (*OccTicket, error) {
	   if d == nil || d.store == nil {
		   return nil, ErrUnavailable
	   }
	  t, err := d.store.GetTicket(ctx, ticketID)
	   if err != nil {
		  return nil, err
	  }
	   if t.State == OccTicketAdmitted {
		  return t, nil
	   }
	   if !CanTransitOccTicket(t.State, OccTicketAdmitted) {
		   return nil, ErrInvalidTransit
	   }
	  from := t.State
	   t.State = OccTicketAdmitted
	  t.UpdatedAt = d.now()
	   if err := d.store.UpdateTicket(ctx, t); err != nil {
		  return nil, err
	   }
	   d.audit(ctx, t.ID, "", string(from), string(OccTicketAdmitted), "desk", "admit")
	  return t, nil
   }

// OccOccupyRequest BeforeFreeze 占用请求。

type OccOccupyRequest struct {
	TicketID  string
	GrantIdem string
	Amount    string
	Asset string
}

// OccOccupyResult 占用结果。

type OccOccupyResult struct {
	Ticket *OccTicket
	Grant  *OccGrant
}

// BeforeFreeze 占用：签发 Grant 并将票据标 reserved。
// 不变量：同一票据同一 Epoch 至多一张活跃 Grant。
  func (d *OccDesk) BeforeFreeze(ctx context.Context, req OccOccupyRequest) (*OccOccupyResult, error) {
	   if d == nil || d.store == nil {
		   return nil, ErrUnavailable
	  }
	   t, err := d.store.GetTicket(ctx, trim(req.TicketID))
	  if err != nil {
		  return nil, err
	   }
	   if t.State == OccTicketExpired {
		  return nil, ErrTicketExpired
	  }
	   if t.State != OccTicketAdmitted && t.State != OccTicketReserved {
		  return nil, ErrAdmitRequired
	   }
	  idem := trim(req.GrantIdem)
	   if idem == "" {
		  return nil, ErrIdemConflict
	   }
	  if existing, err := d.store.GetGrantByIdem(ctx, idem); err == nil {
		   if existing.TicketID != t.ID || existing.Epoch != t.Epoch {
			   return nil, ErrIdemConflict
		  }
		  tt, _ := d.store.GetTicket(ctx, t.ID)
		  return &OccOccupyResult{Ticket: tt, Grant: existing}, nil
	   } else if !errors.Is(err, ErrNotFound) {
		  return nil, err
	  }
	   if t.State == OccTicketReserved {
		   return nil, ErrDoubleOccupancy
	  }
	   amt := trim(req.Amount)
	  if amt == "" {
		   amt = t.Amount
	  }
	   if _, err := parseAmt(amt); err != nil {
		   return nil, err
	  }
	  ast := strings.ToUpper(trim(req.Asset))
	   if ast == "" {
		  ast = t.Asset
	   }
	  if ast != t.Asset {
		  return nil, ErrAsset
	   }
	  now := d.now()
	   g := &OccGrant{
		  ID: uuid.NewString(), TicketID: t.ID, UserID: t.UserID, GrantIdem: idem,
		  Asset: ast, Amount: amt, State: OccGrantReserved, Epoch: t.Epoch,
		   LedgerTag: "reserved", CreatedAt: now, UpdatedAt: now,
	  }
	   if err := d.store.InsertGrant(ctx, g); err != nil {
		   return nil, err
	   }
	   from := t.State
	   t.State = OccTicketReserved
	  t.GrantID = g.ID
	   t.GrantIdem = idem
	  t.UpdatedAt = now
	   if err := d.store.UpdateTicket(ctx, t); err != nil {
		  return nil, err
	  }
	  d.audit(ctx, t.ID, g.ID, string(from), string(OccTicketReserved), "desk", "before_freeze")
	   return &OccOccupyResult{Ticket: t, Grant: g}, nil
   }

// CommitFreeze 将占用段标为 frozen。
// 失败后果：若 Grant 已 frozen，幂等返回成功；若已 aborted/expired，失败。
  func (d *OccDesk) CommitFreeze(ctx context.Context, grantIdem string) (*OccGrant, error) {
	  if d == nil || d.store == nil {
		   return nil, ErrUnavailable
	  }
	  g,
		  err := d.store.GetGrantByIdem(ctx, trim(grantIdem))
	   if err != nil {
		   return nil, err
	   }
	  if g.State == OccGrantFrozen {
		   return g, nil
	   }
	  if g.State == OccGrantExpired || g.State == OccGrantRevoked {
		   return nil, ErrGrantClosed
	  }
	   if g.State == OccGrantAborted {
		  return nil, ErrGrantClosed
	   }
	  if !CanTransitOccGrant(g.State, OccGrantFrozen) {
		   return nil, ErrInvalidTransit
	   }
	  t, err := d.store.GetTicket(ctx, g.TicketID)
	   if err != nil {
		  return nil, err
	   }
	   if t.Epoch != g.Epoch {
		   return nil, ErrEpochStale
	  }
	   if t.State == OccTicketExpired {
		  return nil, ErrTicketExpired
	  }
	   now := d.now()
	  fromG, fromT := g.State, t.State
	  g.State = OccGrantFrozen
	   g.LedgerTag = "frozen"
	   g.UpdatedAt = now
	  if !CanTransitOccTicket(t.State, OccTicketFrozen) && t.State != OccTicketFrozen {
		  return nil, ErrInvalidTransit
	  }
	  t.State = OccTicketFrozen
	  t.UpdatedAt = now
	   if err := d.store.UpdateGrant(ctx, g); err != nil {
		   return nil, err
	  }
	  if err := d.store.UpdateTicket(ctx, t); err != nil {
		  return nil, err
	   }
	   d.audit(ctx, t.ID, g.ID, string(fromT)+"/"+string(fromG), string(OccTicketFrozen), "desk", "commit_freeze")
	  return g, nil
  }

// AbortFreeze 对 reserved 段退占用；对 frozen 段必须失败。
// 不变量：已 frozen 不得静默成功，不得把 LedgerTag 改回可用。
  func (d *OccDesk) AbortFreeze(ctx context.Context, grantIdem string) (*OccGrant, error) {
	  if d == nil || d.store == nil {
		   return nil, ErrUnavailable
	   }
	   g, err := d.store.GetGrantByIdem(ctx, trim(grantIdem))
	  if err != nil {
		   return nil, err
	   }
	  if g.State == OccGrantFrozen || g.FrozenOnLedger() {
		  return nil, ErrAlreadyFrozen
	   }
	  if g.State == OccGrantAborted {
		  return g, nil
	  }
	   if g.State == OccGrantExpired || g.State == OccGrantRevoked {
		   return nil, ErrGrantClosed
	   }
	   if !g.State.Abortable() {
		  return nil, ErrInvalidTransit
	   }
	  t, err := d.store.GetTicket(ctx, g.TicketID)
	   if err != nil {
		  return nil, err
	   }
	  if t.Epoch != g.Epoch {
		   return nil, ErrEpochStale
	   }
	   if t.State == OccTicketExpired {
		  return nil, ErrTicketExpired
	  }
	   now := d.now()
	   fromG, fromT := g.State, t.State
	  g.State = OccGrantAborted
	  g.LedgerTag = "aborted"
	   g.UpdatedAt = now
	   if !CanTransitOccTicket(t.State, OccTicketAborted) && t.State != OccTicketAborted {
		   return nil, ErrInvalidTransit
	  }
	   t.State = OccTicketAborted
	  t.UpdatedAt = now
	   if err := d.store.UpdateGrant(ctx, g); err != nil {
		   return nil, err
	   }
	   if err := d.store.UpdateTicket(ctx, t); err != nil {
		  return nil, err
	  }
	   d.audit(ctx, t.ID, g.ID, string(fromT)+"/"+string(fromG), string(OccTicketAborted), "desk", "abort_freeze")
	  return g, nil
  }

// Expire 将票据切到 expired，并吊销本 Epoch 全部 Grant。
   func (d *OccDesk) Expire(ctx context.Context, ticketID string, note string) (*OccTicket, error) {
	  if d == nil || d.store == nil {
		  return nil, ErrUnavailable
	   }
	   t, err := d.store.GetTicket(ctx, trim(ticketID))
	   if err != nil {
		   return nil, err
	   }
	  if t.State == OccTicketExpired {
		  return t, nil
	  }
	   if !CanTransitOccTicket(t.State, OccTicketExpired) {
		  return nil, ErrInvalidTransit
	   }
	  now := d.now()
	  from := t.State
	   t.State = OccTicketExpired
	  t.Note = trim(note)
	  t.UpdatedAt = now
	   grants, err := d.store.ListGrantsByTicket(ctx, t.ID, 256)
	   if err != nil {
		   return nil, err
	   }
	  for _, g := range grants {
		   if g.Epoch != t.Epoch {
			   continue
		   }
		   if g.State == OccGrantExpired || g.State == OccGrantRevoked {
			   continue
		  }
		   if CanTransitOccGrant(g.State, OccGrantExpired) {
			   g.State = OccGrantExpired
			  g.UpdatedAt = now
			  _ = d.store.UpdateGrant(ctx, g)
		   }
	  }
	  if err := d.store.UpdateTicket(ctx, t); err != nil {
		   return nil, err
	  }
	   d.audit(ctx, t.ID, t.GrantID, string(from), string(OccTicketExpired), "desk", "expire")
	   return t, nil
   }

// Reopen 将 expired 票据切回 pending，Epoch +1。

   func (d *OccDesk) Reopen(ctx context.Context, ticketID string) (*OccTicket, error) {
	  if d == nil || d.store == nil {
		  return nil, ErrUnavailable
	  }
	  t, err := d.store.GetTicket(ctx, trim(ticketID))
	   if err != nil {
		  return nil, err
	   }
	  if t.State != OccTicketExpired {
		   return nil, ErrReopenDenied
	   }
	  if !CanTransitOccTicket(t.State, OccTicketPending) {
		  return nil, ErrInvalidTransit
	  }
	   now := d.now()
	  t.State = OccTicketPending
	  t.Epoch++
	   t.GrantID = ""
	   t.GrantIdem = ""
	  t.UpdatedAt = now
	   if err := d.store.UpdateTicket(ctx, t); err != nil {
		  return nil, err
	  }
	   d.audit(ctx, t.ID, "", string(OccTicketExpired), string(OccTicketPending), "desk", "reopen")
	  return t, nil
   }

// GetTicket 查询。

  func (d *OccDesk) GetTicket(ctx context.Context, id string) (*OccTicket, error) {
	  return d.store.GetTicket(ctx, id)
   }

// GetGrantByIdem 查询。
  func (d *OccDesk) GetGrantByIdem(ctx context.Context, idem string) (*OccGrant, error) {
	   return d.store.GetGrantByIdem(ctx, idem)
  }

// ListAudit 审计。

   func (d *OccDesk) ListAudit(ctx context.Context, ticketID string, limit int) ([]*OccAudit, error) {
	  return d.store.ListAuditByTicket(ctx,
		   ticketID, limit)
  }

// TryLiveUnfreeze 活链解冻旁路：恒拒绝。
  func (d *OccDesk) TryLiveUnfreeze(_ context.Context, _ string) error {
	  if LiveUnfreezeAllowed() {
		  return ErrLiveUnfreeze
	   }
	  return RefuseOccLiveUnfreeze()
   }
