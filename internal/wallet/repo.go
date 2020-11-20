// 托管钱包落库。私钥在 HSM，这里只记 key_id 和地址。
// 作者: Likeo
// 日期: 2020.11.19
package wallet

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/cryptoenc"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

type Repo interface {
	Insert(ctx context.Context,
		w *Wallet, mnemonicEnc []byte) error
	List(ctx context.Context, userID string) ([]Wallet, error)
	Get(ctx context.Context, id string) (*Wallet, error)
	Address(ctx context.Context,
		userID, chain string) (string, bool)
	ListUserIDs(ctx context.Context) ([]string, error)
}

type SQLRepo struct {
	db *sql.DB
	enc     *cryptoenc.Envelope
	dialect persist.Dialect
}

  func NewSQLRepo(db *sql.DB, enc *cryptoenc.Envelope) *SQLRepo {
	  return NewSQLRepoDialect(db, enc, persist.MySQL)
   }

  func NewSQLRepoDialect(db *sql.DB, enc *cryptoenc.Envelope, d persist.Dialect) *SQLRepo {
	   return &SQLRepo{db: db, enc: enc, dialect: d}
  }

   func (r *SQLRepo) conn() persist.Queryer {
	   if r == nil {
		   return nil
	   }
	  return persist.WrapQueryer(r.db, r.dialect)
   }

   func (r *SQLRepo) wrap(inner persist.Queryer) persist.Queryer {
	   if r == nil {
		   return inner
	  }
	   return persist.WrapQueryer(inner, r.dialect)
   }

// Insert 钱包行和多链地址同一事务写入；助记词只存密文。
  func (r *SQLRepo) Insert(ctx context.Context, w *Wallet, mnemonicEnc []byte) error {
	   tx, err := r.db.BeginTx(ctx, nil)
	  if err != nil {
		   return err
	  }
	  defer func() { _ = tx.Rollback() }()
	  q := r.wrap(tx)
	  if _, err := q.ExecContext(ctx, `INSERT INTO custody_wallets (id, user_id, label, source, mnemonic_enc) VALUES (?,?,?,?,?)`,
		   w.ID, w.UserID, w.Label, w.Source, mnemonicEnc); err != nil {
		   return err
	  }
	  allow := `INSERT IGNORE INTO address_allowlist (user_id, chain, address, label) VALUES (?,?,?,?)`
	  if r.dialect == persist.Postgres {
		  allow = `INSERT INTO address_allowlist (user_id, chain, address, label) VALUES (?,?,?,?) ON CONFLICT (user_id, chain, address) DO NOTHING`
	  }
	   for chain, addr := range w.Addresses {
		  path,
			   idx := DerivationPath(chain)
		  if _, err := q.ExecContext(ctx, `INSERT INTO custody_addresses (wallet_id, user_id, chain, address, derivation_path, hd_index) VALUES (?,?,?,?,?,?)`,
			   w.ID, w.UserID, chain, addr, path, idx); err != nil {
			  return err
		   }
		  _, _ = q.ExecContext(ctx, allow, w.UserID, chain, addr, w.Label)
	   }
	  return tx.Commit()
  }

// List 该用户已创建/导入的钱包。

   func (r *SQLRepo) List(ctx context.Context, userID string) ([]Wallet, error) {
	   rows, err := r.conn().QueryContext(ctx, `SELECT id, user_id, label, source, created_at FROM custody_wallets WHERE user_id=? ORDER BY created_at`, userID)
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   var out []Wallet
	   for rows.Next() {
		   var w Wallet
		   if err := rows.Scan(&w.ID, &w.UserID, &w.Label, &w.Source, &w.CreatedAt); err != nil {
			   return nil, err
		  }
		  w.Addresses = map[string]string{}
		  out = append(out, w)
	  }
	  for i := range out {
		   addrs, err := r.addrs(ctx, out[i].ID)
		   if err != nil {
			  return nil, err
		   }
		   out[i].Addresses = addrs
	  }
	  return out, rows.Err()
   }

  func (r *SQLRepo) Get(ctx context.Context, id string) (*Wallet, error) {
	   w := &Wallet{Addresses: map[string]string{}}
	   err := r.conn().QueryRowContext(ctx, `SELECT id, user_id, label, source, created_at FROM custody_wallets WHERE id=?`, id).
		   Scan(&w.ID,
			  &w.UserID, &w.Label, &w.Source, &w.CreatedAt)
	  if errors.Is(err, sql.ErrNoRows) {
		   return nil, fmt.Errorf("wallet not found")
	   }
	   if err != nil {
		   return nil, err
	  }
	   w.Addresses, err = r.addrs(ctx, id)
	   return w, err
  }

   func (r *SQLRepo) addrs(ctx context.Context, walletID string) (map[string]string, error) {
	   rows, err := r.conn().QueryContext(ctx, `SELECT chain, address FROM custody_addresses WHERE wallet_id=?`, walletID)
	  if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	  out := map[string]string{}
	   for rows.Next() {
		   var c, a string
		  if err := rows.Scan(&c, &a); err != nil {
			  return nil, err
		  }
		   out[c] = a
	   }
	   return out, rows.Err()
   }

// Address 用户在该链的充值地址；EVM 家族共用派生结果。
  func (r *SQLRepo) Address(ctx context.Context, userID, chain string) (string, bool) {
	  var addr string
	   err := r.conn().QueryRowContext(ctx, `SELECT address FROM custody_addresses WHERE user_id=? AND chain=? ORDER BY wallet_id LIMIT 1`, userID, chain).Scan(&addr)
	  if err == nil && addr != "" {
		  return addr, true
	   }
	   fam := chain
	  if IsEVM(chain) {
		  fam = "EVM"
	  }
	   err = r.conn().QueryRowContext(ctx, `SELECT address FROM aa_addr_binds WHERE user_id=? AND allocated=1 AND (chain=? OR family=?) ORDER BY bound_at LIMIT 1`, userID, chain, fam).Scan(&addr)
	   if err != nil || addr == "" {
		   return "", false
	  }
	   return addr, true
   }

  func (r *SQLRepo) ListUserIDs(ctx context.Context) ([]string, error) {
	   rows, err := r.conn().QueryContext(ctx, `SELECT DISTINCT user_id FROM custody_addresses`)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  var out []string
	   for rows.Next() {
		  var u string
		  if err := rows.Scan(&u); err != nil {
			  return nil, err
		  }
		   out = append(out, u)
	  }
	  return out, rows.Err()
  }

type MemRepo struct {
	mu      sync.Mutex
	wallets map[string]*Wallet
}

  func NewMemRepo() *MemRepo { return &MemRepo{wallets: map[string]*Wallet{}} }

   func (m *MemRepo) Insert(_ context.Context, w *Wallet, _ []byte) error {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  cp := *w
	   addr := map[string]string{}
	  for k, v := range w.Addresses {
		  addr[k] = v
	  }
	   cp.Addresses = addr
	   cp.Mnemonic = ""
	   m.wallets[w.ID] = &cp
	  return nil
   }

  func (m *MemRepo) List(_ context.Context, userID string) ([]Wallet, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   var out []Wallet
	   for _, w := range m.wallets {
		   if w.UserID == userID {
			   cp := *w
			   out = append(out, cp)
		  }
	  }
	   return out, nil
   }

  func (m *MemRepo) Get(_ context.Context, id string) (*Wallet, error) {
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   w, ok := m.wallets[id]
	  if !ok {
		   return nil, fmt.Errorf("wallet not found")
	   }
	   cp := *w
	   return &cp, nil
   }

  func (m *MemRepo) Address(_ context.Context, userID, chain string) (string, bool) {
	  m.mu.Lock()
	   defer m.mu.Unlock()
	   for _, w := range m.wallets {
		  if w.UserID == userID {
			   if a, ok := w.Addresses[chain]; ok {
				   return a, true
			  }
		  }
	  }
	  return "", false
   }

   func (m *MemRepo) ListUserIDs(_ context.Context) ([]string, error) {
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  seen := map[string]struct{}{}
	   for _, w := range m.wallets {
		  seen[w.UserID] = struct{}{}
	  }
	  out := make([]string, 0, len(seen))
	   for u := range seen {
		  out = append(out, u)
	   }
	  return out, nil
  }

  func NewWalletRecord(userID, label, source, secret string) *Wallet {
	   return &Wallet{
		  ID: uuid.NewString(), UserID: userID, Label: label, Source: source,
		   Addresses: DeriveAll(secret), CreatedAt: time.Now().UTC(),
	  }
   }
