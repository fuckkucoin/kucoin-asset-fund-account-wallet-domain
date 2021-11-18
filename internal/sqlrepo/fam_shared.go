package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// FamSharedAddr FamMEMO 链平台共享充值地址。
type FamSharedAddr struct {
	Chain     string
	Address   string
	Family    string
	UpdatedAt time.Time
}

// FamShared aa_fam_shared_addrs 读写。
type FamShared struct{ db *persist.SQLDB }

   func NewFamShared(db *persist.SQLDB) *FamShared { return &FamShared{db: db} }

// Put 登记/更新某链共享充值地址。
// 地址全局唯一；同地址不得绑两链。
// 空地址拒；不得用用户托管地址冒充共享。
  func (r *FamShared) Put(ctx context.Context, row FamSharedAddr) error {
	   if !sqlOn(r.db) {
		   return ErrUnavailable
	   }
	   row.Chain = strings.ToUpper(strings.TrimSpace(row.Chain))
	  row.Address = strings.TrimSpace(row.Address)
	  row.Family = strings.ToUpper(strings.TrimSpace(row.Family))
	   if row.Family == "" {
		  row.Family = "MEMO"
	  }
	   if row.Chain == "" || row.Address == "" {
		  return ErrConflict
	   }
	   if row.UpdatedAt.IsZero() {
		   row.UpdatedAt = time.Now().UTC()
	  }
	  q := `INSERT INTO aa_fam_shared_addrs (chain, address, family, updated_at) VALUES (?,?,?,?)
ON DUPLICATE KEY UPDATE address=VALUES(address), family=VALUES(family), updated_at=VALUES(updated_at)`
	  if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO aa_fam_shared_addrs (chain, address, family, updated_at) VALUES (?,?,?,?)
ON CONFLICT (chain) DO UPDATE SET address=EXCLUDED.address, family=EXCLUDED.family, updated_at=EXCLUDED.updated_at`
	  }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, q, row.Chain, row.Address, row.Family, row.UpdatedAt)
	   if err != nil && isUnique(err) {
		  return ErrTaken
	   }
	   return err
   }

// Get 按链取共享地址。
  func (r *FamShared) Get(ctx context.Context, chain string) (FamSharedAddr, error) {
	  if !sqlOn(r.db) {
		   return FamSharedAddr{}, ErrUnavailable
	  }
	  var out FamSharedAddr
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT chain, address, family, updated_at FROM aa_fam_shared_addrs WHERE chain=?`,
		   strings.ToUpper(strings.TrimSpace(chain)),
	   ).Scan(&out.Chain, &out.Address, &out.Family, &out.UpdatedAt)
	   if errors.Is(err, sql.ErrNoRows) {
		   return FamSharedAddr{},
			  ErrNotFound
	   }
	   return out, err
   }

// List 全量共享地址（扫链用）。

   func (r *FamShared) List(ctx context.Context) ([]FamSharedAddr, error) {
	   if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	   }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx,
		   `SELECT chain, address, family, updated_at FROM aa_fam_shared_addrs`)
	  if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	   out := make([]FamSharedAddr, 0)
	  for rows.Next() {
		   var row FamSharedAddr
		  if err := rows.Scan(&row.Chain, &row.Address, &row.Family, &row.UpdatedAt); err != nil {
			   return nil, err
		  }
		  out = append(out, row)
	   }
	  return out, rows.Err()
  }

// DepAttrib 入金 memo/tag 归因行。
type DepAttrib struct {
	AttribKey string
	UserID    string
	Chain string
	Kind    string
	MemoKey string
	BoundAt time.Time
}

// DepAttribs aa_deposit_attribs 读写。
type DepAttribs struct{ db *persist.SQLDB }

   func NewDepAttribs(db *persist.SQLDB) *DepAttribs { return &DepAttribs{db: db} }

// Put 唯一绑定 chain+kind+key → user。
// 覆盖等于改户入金；已绑他户拒。
  func (r *DepAttribs) Put(ctx context.Context, row DepAttrib) error {
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	   }
	   row.UserID = strings.TrimSpace(row.UserID)
	   row.Chain = strings.ToUpper(strings.TrimSpace(row.Chain))
	  row.Kind = strings.ToLower(strings.TrimSpace(row.Kind))
	   row.MemoKey = strings.TrimSpace(row.MemoKey)
	   if row.UserID == "" || row.Chain == "" || row.Kind == "" || row.MemoKey == "" {
		  return ErrConflict
	   }
	   if row.AttribKey == "" {
		   row.AttribKey = row.Chain + "|" + row.Kind + "|" + row.MemoKey
	   }
	   if row.BoundAt.IsZero() {
		   row.BoundAt = time.Now().UTC()
	   }
	  // 已存在且他户 → Taken
	  cur, err := r.Get(ctx, row.Chain, row.Kind, row.MemoKey)
	  if err == nil && cur.UserID != row.UserID {
		   return ErrTaken
	  }
	   if err != nil && !errors.Is(err, ErrNotFound) {
		  return err
	  }
	   q := `INSERT INTO aa_deposit_attribs (attrib_key, user_id, chain, kind, memo_key, bound_at) VALUES (?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE user_id=VALUES(user_id), bound_at=VALUES(bound_at)`
	   if r.db.Dialect == persist.Postgres {
		  q = `INSERT INTO aa_deposit_attribs (attrib_key, user_id, chain, kind, memo_key, bound_at) VALUES (?,?,?,?,?,?)
ON CONFLICT (attrib_key) DO UPDATE SET user_id=EXCLUDED.user_id, bound_at=EXCLUDED.bound_at`
	   }
	   _, err = r.db.Conn(ctx).ExecContext(ctx, q, row.AttribKey, row.UserID, row.Chain, row.Kind, row.MemoKey, row.BoundAt)
	   if err != nil && isUnique(err) {
		   return ErrTaken
	   }
	   return err
   }

// Get 按 chain+kind+key 查归因。

   func (r *DepAttribs) Get(ctx context.Context, chain, kind, key string) (DepAttrib, error) {
	   if !sqlOn(r.db) {
		   return DepAttrib{}, ErrUnavailable
	   }
	  ak := strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.ToLower(strings.TrimSpace(kind)) + "|" + strings.TrimSpace(key)
	  var out DepAttrib
	   err := r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT attrib_key, user_id, chain, kind, memo_key, bound_at FROM aa_deposit_attribs WHERE attrib_key=?`, ak,
	   ).Scan(&out.AttribKey, &out.UserID, &out.Chain, &out.Kind, &out.MemoKey, &out.BoundAt)
	   if errors.Is(err, sql.ErrNoRows) {
		   return DepAttrib{}, ErrNotFound
	  }
	  return out, err
   }

// List Hydrate 全量归因。
   func (r *DepAttribs) List(ctx context.Context) ([]DepAttrib, error) {
	   if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	   }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx,
		  `SELECT attrib_key, user_id, chain, kind, memo_key, bound_at FROM aa_deposit_attribs`)
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	   out := make([]DepAttrib, 0)
	  for rows.Next() {
		   var row DepAttrib
		   if err := rows.Scan(&row.AttribKey, &row.UserID, &row.Chain, &row.Kind, &row.MemoKey, &row.BoundAt); err != nil {
			   return nil, err
		  }
		  out = append(out, row)
	   }
	   return out, rows.Err()
  }
