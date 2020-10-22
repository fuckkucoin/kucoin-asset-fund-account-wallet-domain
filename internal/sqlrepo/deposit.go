package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Deposit 入金认领表 aa_deposits。
// 自然键 (chain, tx_hash, output_index)；同键第二人不得领。
// 缺 output_index 会把多输出 UTXO 串成一笔。
type Deposit struct {
	db *persist.SQLDB
}

   func NewDeposit(db *persist.SQLDB) *Deposit { return &Deposit{db: db} }

type DepositClaim struct {
	Chain  string
	TxHash string
	OutputIndex int
	UserID string
	Asset  string
	Amount string
	TransferID string
	Step       string
	FromAddress string
	LastError   string
	RunID string
	Fence persist.Fence
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Claim 按 (chain, tx_hash, output_index) 认领入金。
// 同键同用户幂等回放；异用户/异资产冲突。
// output_index 默认 0 仅适合单输出；多输出须显式传入。
   func (r *Deposit) Claim(ctx context.Context, chain, hash string, outputIndex int, userID, asset, amount, transferID string) (DepositClaim, error) {
	   if r == nil || r.db == nil {
		  return DepositClaim{}, ErrUnavailable
	  }
	  if _, err := money.Parse(amount); err != nil {
		   return DepositClaim{}, ErrAmount
	   }
	  chain = strings.ToUpper(strings.TrimSpace(chain))
	  hash = strings.TrimSpace(hash)
	   userID = strings.TrimSpace(userID)
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   if chain == "" || hash == "" || userID == "" {
		  return DepositClaim{}, ErrConflict
	  }
	   if outputIndex < 0 {
		   return DepositClaim{}, ErrConflict
	   }
	   var out DepositClaim
	  err := r.db.InTx(ctx, func(ctx context.Context) error {
		  cur, err := r.get(ctx, chain, hash, outputIndex)
		   if err == nil {
			  if cur.UserID != userID || cur.Asset != asset {
				   return ErrTaken
			   }
			   out = cur
			  return nil
		  }
		   if !errors.Is(err, ErrNotFound) {
			  return err
		  }
		   id := fmt.Sprintf("%s|%s|%d", chain, hash, outputIndex)
		   f, err := r.db.BumpFence(ctx, id, persist.NewToken())
		   if err != nil {
			  return err
		  }
		   _, err = r.db.Conn(ctx).ExecContext(ctx,
			   `INSERT INTO aa_deposits (chain, tx_hash, output_index, user_id, asset, transfer_id, amount, fence_epoch, fence_token, created_at, step, updated_at, from_address, last_error, run_id) VALUES (?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP(3),'',CURRENT_TIMESTAMP(3),'','','')`,
			  chain, hash, outputIndex, userID, asset, transferID, amount, f.Epoch, f.Token)
		   if err != nil {
			  if isUnique(err) {
				   return ErrTaken
			   }
			  return err
		  }
		  got, err := r.get(ctx, chain, hash, outputIndex)
		  if err != nil {
			   return err
		   }
		   out = got
		   return nil
	   })
	  return out, err
   }

   func (r *Deposit) Get(ctx context.Context, chain, hash string, outputIndex int) (DepositClaim, error) {
	  if r == nil || r.db == nil {
		  return DepositClaim{}, ErrUnavailable
	   }
	  return r.get(ctx, strings.ToUpper(strings.TrimSpace(chain)), strings.TrimSpace(hash), outputIndex)
  }

   func (r *Deposit) get(ctx context.Context, chain, hash string, outputIndex int) (DepositClaim, error) {
	  var c DepositClaim
	   var amt string
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT chain, tx_hash, output_index, user_id, asset, transfer_id, amount, fence_epoch, fence_token, created_at, step, COALESCE(from_address,''), COALESCE(last_error,''), COALESCE(run_id,''), updated_at FROM aa_deposits WHERE chain=? AND tx_hash=? AND output_index=?`,
		   chain, hash, outputIndex,
	  ).Scan(&c.Chain, &c.TxHash, &c.OutputIndex, &c.UserID, &c.Asset, &c.TransferID, &amt, &c.Fence.Epoch, &c.Fence.Token, &c.CreatedAt, &c.Step, &c.FromAddress, &c.LastError, &c.RunID, &c.UpdatedAt)
	  if errors.Is(err, sql.ErrNoRows) {
		   return DepositClaim{},
			  ErrNotFound
	   }
	  if err != nil {
		   return DepositClaim{}, err
	   }
	   c.Amount = amt
	  if c.CreatedAt.IsZero() {
		  c.CreatedAt = time.Now().UTC()
	  }
	  if c.UpdatedAt.IsZero() {
		  c.UpdatedAt = c.CreatedAt
	   }
	  return c, nil
   }

  func (r *Deposit) PersistRun(ctx context.Context, c DepositClaim) error {
	  if r == nil || r.db == nil {
		  return ErrUnavailable
	  }
	  c.Chain = strings.ToUpper(strings.TrimSpace(c.Chain))
	   c.TxHash = strings.TrimSpace(c.TxHash)
	  c.Asset = strings.ToUpper(strings.TrimSpace(c.Asset))
	  if c.Chain == "" || c.TxHash == "" {
		  return ErrConflict
	  }
	   if c.OutputIndex < 0 {
		  return ErrConflict
	  }
	   q := r.db.Conn(ctx)
	  upsert := `INSERT INTO aa_deposits (chain, tx_hash, output_index, user_id, asset, transfer_id, amount, fence_epoch, fence_token, created_at, step, updated_at, from_address, last_error, run_id)
VALUES (?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP(3),?,CURRENT_TIMESTAMP(3),?,?,?)
ON DUPLICATE KEY UPDATE transfer_id=VALUES(transfer_id), step=VALUES(step), from_address=VALUES(from_address), last_error=VALUES(last_error), run_id=VALUES(run_id), amount=VALUES(amount), updated_at=CURRENT_TIMESTAMP(3)`
	  if r.db.Dialect == persist.Postgres {
		  upsert = `INSERT INTO aa_deposits (chain, tx_hash, output_index, user_id, asset, transfer_id, amount, fence_epoch, fence_token, created_at, step, updated_at, from_address, last_error, run_id)
VALUES (?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP,?,CURRENT_TIMESTAMP,?,?,?)
ON CONFLICT (chain, tx_hash, output_index) DO UPDATE SET transfer_id=EXCLUDED.transfer_id, step=EXCLUDED.step, from_address=EXCLUDED.from_address, last_error=EXCLUDED.last_error, run_id=EXCLUDED.run_id, amount=EXCLUDED.amount, updated_at=CURRENT_TIMESTAMP`
	  }
	  _, err := q.ExecContext(ctx, upsert,
		   c.Chain, c.TxHash, c.OutputIndex, c.UserID, c.Asset, c.TransferID, orAmt(c.Amount), c.Fence.Epoch, c.Fence.Token,
		   c.Step, c.FromAddress, c.LastError, c.RunID)
	   return err
  }

  func (r *Deposit) ListByHash(ctx context.Context, chain, hash string) ([]DepositClaim, error) {
	   if r == nil || r.db == nil {
		   return nil, ErrUnavailable
	  }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx,
		  `SELECT chain, tx_hash, output_index, user_id, asset, transfer_id, amount, fence_epoch, fence_token, created_at, step, COALESCE(from_address,''), COALESCE(last_error,''), COALESCE(run_id,''), updated_at FROM aa_deposits WHERE chain=? AND tx_hash=?`,
		  strings.ToUpper(strings.TrimSpace(chain)), strings.TrimSpace(hash))
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	  return scanDepositRows(rows)
  }

   func (r *Deposit) ListAll(ctx context.Context, limit int) ([]DepositClaim, error) {
	  if r == nil || r.db == nil {
		   return nil, ErrUnavailable
	  }
	   if limit <= 0 {
		  limit = 4000
	   }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx,
		   `SELECT chain, tx_hash, output_index, user_id, asset, transfer_id, amount, fence_epoch, fence_token, created_at, step, COALESCE(from_address,''), COALESCE(last_error,''), COALESCE(run_id,''), updated_at FROM aa_deposits ORDER BY updated_at LIMIT ?`, limit)
	  if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	  return scanDepositRows(rows)
   }

// List 供工人重启恢复；列集与 ListAll 同一 BindSQL，默认 200。
  func (r *Deposit) List(ctx context.Context, limit int) ([]DepositClaim, error) {
	  if limit <= 0 {
		   limit = 200
	  }
	   return r.ListAll(ctx, limit)
   }

  func scanDepositRows(rows *sql.Rows) ([]DepositClaim, error) {
	   out := make([]DepositClaim, 0)
	  for rows.Next() {
		   var c DepositClaim
		  var amt string
		   if err := rows.Scan(&c.Chain, &c.TxHash, &c.OutputIndex, &c.UserID, &c.Asset, &c.TransferID, &amt, &c.Fence.Epoch, &c.Fence.Token, &c.CreatedAt, &c.Step, &c.FromAddress, &c.LastError, &c.RunID, &c.UpdatedAt); err != nil {
			   return nil, err
		  }
		  c.Amount = amt
		   out = append(out, c)
	  }
	  return out, rows.Err()
  }
