// 入金 Saga 簿 SQL 附着与拒 Mem。

// Author: Likeo
// Date: 2020.10.13

package deposit

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/authn"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

// ErrMemBook 档 Book 仍为纯 Mem 或未 BindSQL。错误码稳定，禁止静默降级。

var ErrMemBook = errors.New("refuse production: deposit book bound to in-process mem")

type runStore interface {
	PersistRun(ctx context.Context, c sqlrepo.DepositClaim) error
	Get(ctx context.Context, chain, hash string, outputIndex int) (sqlrepo.DepositClaim, error)
	ListByHash(ctx context.Context, chain, hash string) ([]sqlrepo.DepositClaim, error)
	ListAll(ctx context.Context, limit int) ([]sqlrepo.DepositClaim, error)
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
		  b.store = sqlrepo.NewDeposit(db)
	  }
	  return b
   }

// BindSQLTable 进程内表夹具附着，语义一致 BindSQL（与门控 TableRegister 同模式）。
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

  func (b *Book) sqlOn() bool {
	   return b != nil && b.sql != nil && b.sql.DB != nil
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
	   if r.UpdatedAt.IsZero() {
		  r.UpdatedAt = time.Now().UTC()
	   }
	  b.mu.Lock()
	  b.putMemLocked(r)
	   store := b.store
	   b.mu.Unlock()
	   if store == nil {
		   return nil
	   }
	   return store.PersistRun(context.Background(), runToClaim(r))
   }

  func (b *Book) putMemLocked(r Run) {
	  if b.byID == nil {
		   b.byID = map[string]*Run{}
	   }
	   if b.byTx == nil {
		   b.byTx = map[string]string{}
	   }
	  if b.byOut == nil {
		  b.byOut = map[string]string{}
	  }
	  cp := r
	  if r.ID != "" {
		   b.byID[r.ID] = &cp
	   }
	   if r.TxHash != "" {
		  b.byTx[r.Chain+"|"+r.TxHash] = r.ID
		  b.byOut[outKey(r.Chain, r.TxHash, r.OutputIndex)] = r.ID
	  }
   }

// Hydrate 自 aa_deposits（或表夹具）装回 Run。终态行可 Get，不入 Pending。
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
	   for _, c := range rows {
		  r := claimToRun(c)
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

// BookHealth 与 DeskHealth 并列，并入既有探活，不另起健康框架。
type BookHealth struct {
	SQLBound bool `json:"sql_bound"`
	HydratedN  int       `json:"hydrated_n"`
	HydratedAt time.Time `json:"hydrated_at,omitempty"`
	HydrateErr string    `json:"hydrate_err,omitempty"`
	AttachErr  string    `json:"attach_err,omitempty"`
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
		  AttachErr:  b.attachErr, MemoryN: len(b.byID),
	  }
	   if b.prod {
		  h.OK = h.SQLBound && h.HydrateErr == "" && h.AttachErr == ""
	   } else {
		  h.OK = h.HydrateErr == ""
	  }
	   return h
   }

  func itoaOut(n int) string { return strconv.Itoa(n) }

   func runToClaim(r Run) sqlrepo.DepositClaim {
	  return sqlrepo.DepositClaim{
		  Chain: r.Chain, TxHash: r.TxHash, OutputIndex: r.OutputIndex,
		  UserID: r.UserID, Asset: r.Asset, Amount: r.Amount, TransferID: r.TransferID,
		   Step:        string(r.Step),
		  FromAddress: r.FromAddress, LastError: r.Err,
		   RunID: r.ID, UpdatedAt: r.UpdatedAt, CreatedAt: r.UpdatedAt,
	   }
   }

  func claimToRun(c sqlrepo.DepositClaim) Run {
	   id := strings.TrimSpace(c.RunID)
	   if id == "" {
		  id = c.Chain + "|" + c.TxHash + "|" + itoaOut(c.OutputIndex)
	  }
	  return Run{
		  ID: id, TransferID: c.TransferID, Chain: c.Chain, TxHash: c.TxHash,
		   OutputIndex: c.OutputIndex,
		  UserID:      c.UserID, Asset: c.Asset,
		   FromAddress: c.FromAddress, Step: Step(c.Step), Amount: c.Amount,
		   Err: c.LastError, UpdatedAt: c.UpdatedAt,
	   }
   }
