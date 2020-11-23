// 已广播的链上流水落库和查询。
// 作者: Likeo
// 日期: 2020.11.17
package wallet

import (
	"context"
	"database/sql"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/shopspring/decimal"
)

// HistoryStore 已广播交易的落库。提交成功就要写下 hash。

type HistoryStore interface {
	Record(ctx context.Context,
		userID string, tx Tx) error
	List(ctx context.Context, userID, chain, address string, limit int) ([]Tx, error)
	RememberSubmit(ctx context.Context, nonce, hash string) error
	LookupSubmit(ctx context.Context, nonce string) (string, bool)
}

// MemHistory 进程内充提流水。

type MemHistory struct {
	mu     sync.Mutex
	rows   []histRow
	nonces map[string]string
}

type histRow struct {
	UserID string
	Tx     Tx
}

   func NewMemHistory() *MemHistory { return &MemHistory{nonces: map[string]string{}} }

   func (m *MemHistory) RememberSubmit(_ context.Context, nonce, hash string) error {
	  if nonce == "" || hash == "" {
		   return nil
	   }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   if m.nonces == nil {
		   m.nonces = map[string]string{}
	  }
	  if _, ok := m.nonces[nonce]; !ok {
		   m.nonces[nonce] = hash
	   }
	  return nil
  }

  func (m *MemHistory) LookupSubmit(_ context.Context, nonce string) (string, bool) {
	  if nonce == "" {
		   return "", false
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   h, ok := m.nonces[nonce]
	  return h, ok && h != ""
   }

   func (m *MemHistory) Record(_ context.Context, userID string, tx Tx) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  if tx.At.IsZero() {
		  tx.At = time.Now().UTC()
	  }
	   for i, r := range m.rows {
		   if r.Tx.Chain == tx.Chain && r.Tx.Hash == tx.Hash {
			   m.rows[i] = histRow{UserID: userID, Tx: tx}
			  return nil
		  }
	  }
	   m.rows = append(m.rows, histRow{UserID: userID, Tx: tx})
	   return nil
  }

   func (m *MemHistory) List(_ context.Context, userID, chain, address string, limit int) ([]Tx, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   if limit <= 0 {
		  limit = 20
	   }
	  var out []Tx
	  for i := len(m.rows) - 1; i >= 0; i-- {
		  r := m.rows[i]
		   if userID != "" && r.UserID != userID {
			   continue
		  }
		  if chain != "" && r.Tx.Chain != chain {
			   continue
		  }
		   if address != "" && r.Tx.Address != address {
			   continue
		   }
		  out = append(out, r.Tx)
		   if len(out) >= limit {
			  break
		   }
	  }
	  return out, nil
   }

// SQLHistory 把已广播的链上交易写入 chain_tx_history。
type SQLHistory struct {
	db      *sql.DB
	dialect persist.Dialect
}

   func NewSQLHistory(db *sql.DB) *SQLHistory {
	   return NewSQLHistoryDialect(db, persist.MySQL)
   }

  func NewSQLHistoryDialect(db *sql.DB, d persist.Dialect) *SQLHistory {
	   s := &SQLHistory{db: db, dialect: d}
	  if db != nil {
		  ddl := `CREATE TABLE IF NOT EXISTS submit_nonces (
			nonce VARCHAR(128) PRIMARY KEY,
			tx_hash VARCHAR(128) NOT NULL,
			created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`
		  if d == persist.Postgres {
			   ddl = persist.Rewrite(ddl, persist.Postgres)
		  }
		  _, _ = db.Exec(ddl)
	  }
	  return s
   }

  func (s *SQLHistory) q() persist.Queryer {
	   if s == nil {
		  return nil
	   }
	   return persist.WrapQueryer(s.db, s.dialect)
  }

   func (s *SQLHistory) RememberSubmit(ctx context.Context, nonce, hash string) error {
	  if s == nil || s.db == nil || nonce == "" || hash == "" {
		   return nil
	   }
	   ins := `INSERT INTO submit_nonces (nonce, tx_hash) VALUES (?,?) ON DUPLICATE KEY UPDATE tx_hash=tx_hash`
	   if s.dialect == persist.Postgres {
		   ins = `INSERT INTO submit_nonces (nonce, tx_hash) VALUES (?,?) ON CONFLICT (nonce) DO NOTHING`
	  }
	  _, err := s.q().ExecContext(ctx, ins, nonce, hash)
	  return err
   }

   func (s *SQLHistory) LookupSubmit(ctx context.Context, nonce string) (string, bool) {
	  if s == nil || s.db == nil || nonce == "" {
		   return "", false
	   }
	   var hash string
	   err := s.q().QueryRowContext(ctx, `SELECT tx_hash FROM submit_nonces WHERE nonce=?`, nonce).Scan(&hash)
	  if err != nil || hash == "" {
		  return "", false
	   }
	   return hash, true
  }

   func (s *SQLHistory) Record(ctx context.Context, userID string, tx Tx) error {
	   if s == nil || s.db == nil {
		   return nil
	   }
	   if tx.At.IsZero() {
		  tx.At = time.Now().UTC()
	   }
	  status := string(tx.Status)
	   if status == "" {
		  status = string(TxPending)
	   }
	  ins := `INSERT INTO chain_tx_history (user_id, chain, address, tx_hash, asset, amount, direction, status, confirmed_at) VALUES (?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE user_id=VALUES(user_id), address=VALUES(address), status=VALUES(status), amount=VALUES(amount)`
	  if s.dialect == persist.Postgres {
		  ins = `INSERT INTO chain_tx_history (user_id, chain, address, tx_hash, asset, amount, direction, status, confirmed_at) VALUES (?,?,?,?,?,?,?,?,?)
ON CONFLICT (chain, tx_hash) DO UPDATE SET user_id=EXCLUDED.user_id, address=EXCLUDED.address, status=EXCLUDED.status, amount=EXCLUDED.amount`
	  }
	  _, err := s.q().ExecContext(ctx, ins,
		   userID,
		  tx.Chain, tx.Address, tx.Hash, tx.Asset, tx.Amount.StringFixed(18), tx.Direction, status, confirmedAt(tx))
	   return err
   }

   func confirmedAt(tx Tx) any {
	  if tx.Status == TxConfirmed {
		  return tx.At
	  }
	   return nil
  }

   func (s *SQLHistory) List(ctx context.Context, userID, chain, address string, limit int) ([]Tx, error) {
	   if s == nil || s.db == nil {
		  return nil, nil
	   }
	   if limit <= 0 {
		   limit = 20
	   }
	  rows, err := s.q().QueryContext(ctx, `SELECT chain, address, tx_hash, asset, amount, direction, status, created_at
FROM chain_tx_history
WHERE (?='' OR user_id=?) AND (?='' OR chain=?) AND (?='' OR address=?)
ORDER BY id DESC LIMIT ?`, userID, userID, chain, chain, address, address, limit)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	  var out []Tx
	   for rows.Next() {
		  var t Tx
		  var amt, status string
		  if err := rows.Scan(&t.Chain, &t.Address, &t.Hash, &t.Asset, &amt, &t.Direction, &status, &t.At); err != nil {
			  return nil, err
		   }
		   d, err := decimal.NewFromString(amt)
		  if err != nil {
			   return nil, err
		   }
		   t.Amount = d
		   t.Status = TxStatus(status)
		  out = append(out, t)
	  }
	  return out, rows.Err()
  }
