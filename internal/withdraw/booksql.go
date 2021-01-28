package withdraw

import (
	"context"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/authn"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

// ErrMemBook 档 Book 仍为纯 Mem 或未 BindSQL。错误码稳定，禁止静默降级。
var ErrMemBook = errors.New("refuse production: withdraw outbound book bound to in-process mem")

// ErrFailSealed 已失败封闭，禁止再记成功或重复扣款。

var ErrFailSealed = errors.New("withdraw outbound fail-sealed")

// ErrIdemTaken 同一幂等键已绑定其他 transfer，禁止再冻。
var ErrIdemTaken = errors.New("withdraw outbound idem already bound")

type runStore interface {
	PersistRun(ctx context.Context, row sqlrepo.WDOutboundRun) error
	Get(ctx context.Context, transferID string) (sqlrepo.WDOutboundRun, error)
	GetByIdem(ctx context.Context, idem string) (sqlrepo.WDOutboundRun, error)
	ListAll(ctx context.Context, limit int) ([]sqlrepo.WDOutboundRun, error)
}

// RefuseMemBook +（Runtime.Mem 或未 SQL 附着）失败即关；非允许纯 Mem。

   func RefuseMemBook(production, memRuntime, sqlBound bool) error {
	  if !production {
		  return nil
	  }
	   if memRuntime || !sqlBound {
		  return ErrMemBook
	   }
	  return nil
  }

   func RefuseMemBookCfg(kind, profile string, memRuntime, sqlBound bool) error {
	   return RefuseMemBook(authn.ProductionProfile(kind, profile), memRuntime, sqlBound)
   }

   func (b *Book) BindSQL(db *persist.SQLDB) *Book {
	   if b == nil {
		   return b
	  }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   b.sql = db
	  if db != nil && db.DB != nil {
		  b.store = sqlrepo.NewWDOutbound(db)
	   }
	   return b
   }

  func (b *Book) BindSQLTable(t *BookTable) *Book {
	   if b == nil {
		   return b
	   }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   if t != nil {
		   b.store = t
	  }
	   return b
   }

   func (b *Book) MarkProduction(on bool) *Book {
	   if b != nil {
		  b.prod = on
	  }
	  return b
  }

  func (b *Book) SQLBound() bool {
	  if b == nil {
		  return false
	  }
	   b.mu.Lock()
	   defer b.mu.Unlock()
	  return b.store != nil
   }

   func (b *Book) writable() error {
	  if b == nil {
		   return ErrMemBook
	  }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   if b.prod && b.store == nil {
		  return ErrMemBook
	  }
	  return nil
   }

   func (b *Book) PutErr(r Run) error {
	   if b == nil {
		   return ErrMemBook
	   }
	  if err := b.writable(); err != nil {
		  b.mu.Lock()
		   b.attachErr = err.Error()
		   b.mu.Unlock()
		  return err
	  }
	   if r.FailSealed && !terminalFail(r.Step) {
		   return ErrFailSealed
	   }
	   if r.Idem == "" {
		  r.Idem = r.IdemKey
	   }
	  if r.IdemKey == "" {
		   r.IdemKey = r.Idem
	   }
	  if r.UpdatedAt.IsZero() {
		  r.UpdatedAt = time.Now().UTC()
	  }
	  b.mu.Lock()
	  if r.Idem != "" {
		  if rid, ok := b.byIdem[r.Idem]; ok {
			  if cur, ok := b.rows[rid]; ok && cur.TransferID != "" && r.TransferID != "" && cur.TransferID != r.TransferID {
				   b.mu.Unlock()
				   return ErrIdemTaken
			  }
		  }
	  }
	  if r.FailSealed || (r.TransferID != "" && b.xferFailSealedLocked(r.TransferID)) {
		   if r.Step == StepConfirmed || r.Step == StepSubmitted {
			   b.mu.Unlock()
			  return ErrFailSealed
		  }
	   }
	   b.putMemLocked(r)
	  store := b.store
	  b.mu.Unlock()
	   if store == nil {
		   return nil
	  }
	  if strings.TrimSpace(r.TransferID) == "" {
		  return nil
	   }
	  return store.PersistRun(context.Background(), runToRow(r))
  }

   func (b *Book) xferFailSealedLocked(xfer string) bool {
	   if b.byXfer == nil {
		   return false
	  }
	   rid, ok := b.byXfer[xfer]
	  if !ok {
		  return false
	  }
	   cur, ok := b.rows[rid]
	   return ok && cur.FailSealed
  }

  func (b *Book) putMemLocked(r Run) {
	   if b.rows == nil {
		  b.rows = map[string]*Run{}
	  }
	  if b.byXfer == nil {
		  b.byXfer = map[string]string{}
	   }
	  if b.byIdem == nil {
		   b.byIdem = map[string]string{}
	  }
	  cp := r
	   if r.ID != "" {
		  b.rows[r.ID] = &cp
	   }
	   if r.TransferID != "" {
		  b.byXfer[r.TransferID] = r.ID
	   }
	   if r.Idem != "" {
		  b.byIdem[r.Idem] = r.ID
	  }
  }

  func (b *Book) ResumeByIdem(idem string) (Run, bool) {
	   if b == nil {
		   return Run{}, false
	  }
	  idem = strings.TrimSpace(idem)
	  if idem == "" {
		   return Run{}, false
	   }
	   b.mu.Lock()
	  if rid, ok := b.byIdem[idem]; ok {
		  if r, ok := b.rows[rid]; ok {
			   cp := *r
			  b.mu.Unlock()
			   return cp, true
		   }
	  }
	   store := b.store
	   b.mu.Unlock()
	   if store == nil {
		   return Run{}, false
	  }
	   row, err := store.GetByIdem(context.Background(), idem)
	  if err != nil {
		   return Run{}, false
	  }
	  r := rowToRun(row)
	   b.mu.Lock()
	   b.putMemLocked(r)
	   b.mu.Unlock()
	  return r, true
   }

   func (b *Book) MarkDebitOnce(r Run) error {
	  r.DebitOnce = true
	  if r.Step == StepRequested {
		   r.Step = StepFrozen
	  }
	   return b.PutErr(r)
  }

   func (b *Book) FailSeal(r Run, reason string) error {
	  if strings.TrimSpace(reason) == "" {
		   reason = "fail_seal"
	  }
	  if r.Step == StepConfirmed {
		  return errors.New("cannot fail-seal confirmed outbound as success")
	   }
	  wasSubmitted := r.Step == StepSubmitted
	   debit := r.DebitOnce
	   r.Step = StepFailed
	  r.FailSealed = true
	  r.Err = reason
	   if wasSubmitted {
		   r.Compensate = "submitted_live"
	  } else if debit && r.Compensate == "" {
		  r.Compensate = "unfreeze_required"
	   }
	   return b.PutErr(r)
   }

   func (b *Book) Compensate(r Run, note string) error {
	  if !r.FailSealed {
		  return errors.New("compensate requires fail-seal")
	  }
	   if r.Compensate == "submitted_live" {
		  return errors.New("submitted live cannot silent-unfreeze")
	  }
	  if strings.TrimSpace(note) == "" {
		   note = "compensated"
	   }
	  r.Compensate = note
	  r.Step = StepFailed
	  r.FailSealed = true
	   return b.PutErr(r)
  }

   func (b *Book) AdvancePersist(id string, next Step) error {
	   cur, ok := b.Get(id)
	   if !ok {
		  return errors.New("outbound run not found")
	  }
	  if cur.FailSealed {
		  return ErrFailSealed
	  }
	   if err := Advance(cur.Step, next); err != nil {
		  return err
	   }
	  cur.Step = next
	   if next == StepFrozen || next == StepReview || next == StepSubmitted || next == StepConfirmed {
		   cur.DebitOnce = true
	   }
	   if next == StepFailed {
		  return b.FailSeal(cur, "advance_failed")
	   }
	  return b.PutErr(cur)
   }

   func (b *Book) Pending(limit int) []Run {
	   b.mu.Lock()
	  defer b.mu.Unlock()
	  out := make([]Run, 0)
	  for _, r := range b.rows {
		   if !pendingOutbound(r.Step) || r.FailSealed {
			  continue
		   }
		  out = append(out, *r)
		  if limit > 0 && len(out) >= limit {
			   break
		  }
	  }
	  return out
  }

   func pendingOutbound(s Step) bool {
	  switch s {
	  case StepFrozen,
		   StepAllowlisted, StepDecrypted, StepKYT, StepReview, StepSigned, StepSubmitted:
		  return true
	   default:
		   return false
	   }
   }

  func terminalFail(s Step) bool {
	  return s == StepFailed
   }

  func (b *Book) Hydrate(ctx context.Context) error {
	  if b == nil {
		   return nil
	  }
	   if ctx == nil {
		  ctx = context.Background()
	  }
	   b.mu.Lock()
	  store := b.store
	  b.mu.Unlock()
	  if store == nil {
		  return nil
	   }
	   rows, err := store.ListAll(ctx, 4000)
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   if err != nil {
		  b.hydrateEr = err.Error()
		  return err
	  }
	  n := 0
	  for _, row := range rows {
		  r := rowToRun(row)
		   b.putMemLocked(r)
		   n++
	  }
	   b.hydrateN = n
	  b.hydrateAt = time.Now().UTC()
	   b.hydrateEr = ""
	  return nil
  }

   func (b *Book) NoteAttachErr(err error) {
	  if b == nil || err == nil {
		   return
	   }
	   b.mu.Lock()
	  b.attachErr = err.Error()
	  b.mu.Unlock()
   }

  func (b *Book) AttachError() error {
	  if b == nil {
		   return nil
	  }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   if strings.TrimSpace(b.attachErr) == "" {
		  return nil
	   }
	   if strings.Contains(b.attachErr, ErrMemBook.Error()) {
		   return ErrMemBook
	  }
	   return errors.New(b.attachErr)
  }

type BookHealth struct {
	SQLBound bool `json:"sql_bound"`
	HydratedN  int       `json:"hydrated_n"`
	HydratedAt time.Time `json:"hydrated_at,omitempty"`
	HydrateErr string    `json:"hydrate_err,omitempty"`
	AttachErr string `json:"attach_err,omitempty"`
	MemoryN int `json:"memory_n"`
	OK bool `json:"ok"`
}

   func (b *Book) Health() BookHealth {
	   if b == nil {
		  return BookHealth{OK: true}
	   }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  h := BookHealth{
		  SQLBound: b.store != nil, HydratedN: b.hydrateN, HydratedAt: b.hydrateAt,
		   HydrateErr: b.hydrateEr,
		  AttachErr:  b.attachErr, MemoryN: len(b.rows),
	  }
	   if b.prod {
		  h.OK = h.SQLBound && h.HydrateErr == "" && h.AttachErr == ""
	   } else {
		  h.OK = h.HydrateErr == ""
	  }
	  return h
  }

  func runToRow(r Run) sqlrepo.WDOutboundRun {
	   return sqlrepo.WDOutboundRun{
		  TransferID: r.TransferID, RunID: r.ID, UserID: r.UserID, Asset: r.Asset,
		   Amount: r.Amount, Chain: r.Chain, Idem: r.Idem, Step: string(r.Step),
		  KYT: r.KYT, PolicyVer: r.PolicyVer, LastError: r.Err,
		   DebitOnce: r.DebitOnce, FailSealed: r.FailSealed, Compensate: r.Compensate,
		  UpdatedAt: r.UpdatedAt, CreatedAt: r.UpdatedAt,
	   }
  }

  func rowToRun(row sqlrepo.WDOutboundRun) Run {
	  id := strings.TrimSpace(row.RunID)
	   if id == "" {
		   id = row.TransferID
	  }
	   return Run{
		  ID: id, TransferID: row.TransferID, UserID: row.UserID, Step: Step(row.Step),
		   KYT: row.KYT, PolicyVer: row.PolicyVer, Err: row.LastError,
		   UpdatedAt: row.UpdatedAt, Idem: row.Idem, Asset: row.Asset, Amount: row.Amount,
		  Chain: row.Chain, DebitOnce: row.DebitOnce, FailSealed: row.FailSealed,
		   Compensate: row.Compensate,
	   }
  }
