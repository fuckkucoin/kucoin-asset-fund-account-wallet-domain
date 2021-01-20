package deposit

import (
	"context"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

// BookTable 进程内 aa_deposits 行语义夹具。自然键 (chain, tx_hash, output_index)。

type BookTable struct {
	mu   sync.Mutex
	rows map[string]sqlrepo.DepositClaim
	failList bool
}

   func NewBookTable() *BookTable {
	   return &BookTable{rows: map[string]sqlrepo.DepositClaim{}}
  }

   func OpenBookTable(rows map[string]sqlrepo.DepositClaim) *BookTable {
	   if rows == nil {
		   rows = map[string]sqlrepo.DepositClaim{}
	   }
	   return &BookTable{rows: rows}
  }

   func (t *BookTable) Rows() map[string]sqlrepo.DepositClaim {
	   if t == nil {
		   return nil
	   }
	   return t.rows
  }

  func (t *BookTable) FailNextList() { t.failList = true }

   func claimKey(chain, hash string, vout int) string {
	   return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.TrimSpace(hash) + "|" + itoaOut(vout)
   }

   func (t *BookTable) PersistRun(_ context.Context, c sqlrepo.DepositClaim) error {
	   if t == nil || t.rows == nil {
		   return sqlrepo.ErrUnavailable
	  }
	   t.mu.Lock()
	  defer t.mu.Unlock()
	   k := claimKey(c.Chain, c.TxHash, c.OutputIndex)
	   cur, ok := t.rows[k]
	   if ok {
		  if c.TransferID != "" {
			  cur.TransferID = c.TransferID
		   }
		  if c.Step != "" {
			   cur.Step = c.Step
		  }
		   if c.FromAddress != "" {
			  cur.FromAddress = c.FromAddress
		  }
		  cur.LastError = c.LastError
		   if c.RunID != "" {
			   cur.RunID = c.RunID
		   }
		   if c.Amount != "" {
			   cur.Amount = c.Amount
		  }
		   if c.UserID != "" && cur.UserID == "" {
			  cur.UserID = c.UserID
		   }
		   if c.Asset != "" && cur.Asset == "" {
			  cur.Asset = c.Asset
		  }
		  cur.UpdatedAt = time.Now().UTC()
		  t.rows[k] = cur
		   return nil
	  }
	  c.Chain = strings.ToUpper(strings.TrimSpace(c.Chain))
	  c.TxHash = strings.TrimSpace(c.TxHash)
	  c.Asset = strings.ToUpper(strings.TrimSpace(c.Asset))
	  if c.UpdatedAt.IsZero() {
		  c.UpdatedAt = time.Now().UTC()
	   }
	   if c.CreatedAt.IsZero() {
		  c.CreatedAt = c.UpdatedAt
	   }
	  t.rows[k] = c
	   return nil
  }

   func (t *BookTable) Get(_ context.Context, chain, hash string, outputIndex int) (sqlrepo.DepositClaim, error) {
	  if t == nil {
		  return sqlrepo.DepositClaim{}, sqlrepo.ErrUnavailable
	   }
	  t.mu.Lock()
	  defer t.mu.Unlock()
	   c,
		  ok := t.rows[claimKey(chain, hash, outputIndex)]
	  if !ok {
		   return sqlrepo.DepositClaim{}, sqlrepo.ErrNotFound
	  }
	  return c, nil
  }

   func (t *BookTable) ListByHash(_ context.Context, chain, hash string) ([]sqlrepo.DepositClaim, error) {
	  if t == nil {
		   return nil, sqlrepo.ErrUnavailable
	   }
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   hash = strings.TrimSpace(hash)
	  t.mu.Lock()
	  defer t.mu.Unlock()
	  out := make([]sqlrepo.DepositClaim, 0)
	  for _, c := range t.rows {
		   if c.Chain == chain && c.TxHash == hash {
			  out = append(out, c)
		   }
	  }
	   return out, nil
   }

  func (t *BookTable) ListAll(_ context.Context, limit int) ([]sqlrepo.DepositClaim, error) {
	  if t == nil {
		   return nil, sqlrepo.ErrUnavailable
	  }
	  t.mu.Lock()
	  defer t.mu.Unlock()
	  if t.failList {
		   t.failList = false
		  return nil, sqlrepo.ErrUnavailable
	   }
	  out := make([]sqlrepo.DepositClaim, 0, len(t.rows))
	  for _, c := range t.rows {
		   out = append(out, c)
		   if limit > 0 && len(out) >= limit {
			   break
		   }
	  }
	  return out, nil
  }

// Claim 同键同用户幂等；异用户冲突 ErrTaken。多 vout 分列。
  func (t *BookTable) Claim(_ context.Context, chain, hash string, outputIndex int, userID, asset, amount, transferID string) (sqlrepo.DepositClaim, error) {
	  if t == nil || t.rows == nil {
		   return sqlrepo.DepositClaim{}, sqlrepo.ErrUnavailable
	   }
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	   hash = strings.TrimSpace(hash)
	  userID = strings.TrimSpace(userID)
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	   if chain == "" || hash == "" || userID == "" || outputIndex < 0 {
		   return sqlrepo.DepositClaim{}, sqlrepo.ErrConflict
	   }
	  t.mu.Lock()
	   defer t.mu.Unlock()
	  k := claimKey(chain,
		  hash, outputIndex)
	   if cur, ok := t.rows[k]; ok {
		  if cur.UserID != userID || cur.Asset != asset {
			  return sqlrepo.DepositClaim{}, sqlrepo.ErrTaken
		  }
		  return cur, nil
	   }
	   now := time.Now().UTC()
	  row := sqlrepo.DepositClaim{
		  Chain: chain, TxHash: hash, OutputIndex: outputIndex,
		  UserID: userID, Asset: asset, Amount: amount, TransferID: transferID,
		  CreatedAt: now, UpdatedAt: now,
	  }
	  t.rows[k] = row
	  return row, nil
   }
