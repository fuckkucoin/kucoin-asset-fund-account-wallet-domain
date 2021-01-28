package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// Address 绑址表 aa_addr_binds。地址全局唯一，同一用户同链复用已绑行。
type Address struct {
	db *persist.SQLDB
}

   func NewAddress(db *persist.SQLDB) *Address { return &Address{db: db} }

type AddrBind struct {
	BindKey string
	UserID  string
	Chain   string
	Family string
	Address   string
	Allocated bool
	Fence     persist.Fence
	BoundAt time.Time
	ReuseAfter time.Time
}

  func bindKey(userID, chain string) string {
	   return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.TrimSpace(userID)
   }

  func (r *Address) Bind(ctx context.Context, userID, chain, family, address string) (AddrBind, error) {
	  if r == nil || r.db == nil {
		   return AddrBind{}, ErrUnavailable
	   }
	   userID = strings.TrimSpace(userID)
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	   family = strings.ToUpper(strings.TrimSpace(family))
	   address = strings.TrimSpace(address)
	   if userID == "" || chain == "" || address == "" {
		   return AddrBind{}, ErrConflict
	  }
	  key := bindKey(userID, chain)
	   var out AddrBind
	   err := r.db.InTx(ctx, func(ctx context.Context) error {
		   cur, err := r.get(ctx, key)
		   if err == nil {
			  if !strings.EqualFold(cur.Address, address) && cur.Allocated {
				  return ErrTaken
			  }
			   out = cur
			  return nil
		   }
		   if !errors.Is(err, ErrNotFound) {
			  return err
		  }
		  f, err := r.db.BumpFence(ctx, "addr|"+key, persist.NewToken())
		   if err != nil {
			   return err
		  }
		  _, err = r.db.Conn(ctx).ExecContext(ctx,
			   `INSERT INTO aa_addr_binds (bind_key, user_id, chain, family, address, allocated, fence_epoch, fence_token, bound_at, reuse_after) VALUES (?,?,?,?,?,1,?,?,CURRENT_TIMESTAMP(3),CURRENT_TIMESTAMP(3))`,
			  key, userID, chain, family, address, f.Epoch, f.Token)
		  if err != nil {
			  if isUnique(err) {
				  return ErrTaken
			   }
			   return err
		   }
		   got, err := r.get(ctx, key)
		  if err != nil {
			   return err
		   }
		  out = got
		   return nil
	   })
	  return out, err
   }

  func (r *Address) Get(ctx context.Context, userID, chain string) (AddrBind, error) {
	  if r == nil || r.db == nil {
		  return AddrBind{}, ErrUnavailable
	  }
	   return r.get(ctx,
		  bindKey(userID, chain))
   }

  func (r *Address) GetByAddr(ctx context.Context, address string) (AddrBind, error) {
	  if r == nil || r.db == nil {
		   return AddrBind{}, ErrUnavailable
	   }
	   var b AddrBind
	   var alloc int
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT bind_key, user_id, chain, family, address, allocated, fence_epoch, fence_token, bound_at, reuse_after FROM aa_addr_binds WHERE address=?`,
		   strings.TrimSpace(address),
	   ).Scan(&b.BindKey, &b.UserID, &b.Chain, &b.Family, &b.Address, &alloc, &b.Fence.Epoch, &b.Fence.Token, &b.BoundAt, &b.ReuseAfter)
	  if errors.Is(err, sql.ErrNoRows) {
		  return AddrBind{}, ErrNotFound
	  }
	   if err != nil {
		   return AddrBind{}, err
	   }
	  b.Allocated = alloc != 0
	   return b, nil
   }

  func (r *Address) Put(ctx context.Context, b AddrBind) error {
	   b.BindKey = strings.TrimSpace(b.BindKey)
	   b.UserID = strings.TrimSpace(b.UserID)
	  b.Chain = strings.ToUpper(strings.TrimSpace(b.Chain))
	  b.Family = strings.ToUpper(strings.TrimSpace(b.Family))
	   b.Address = strings.TrimSpace(b.Address)
	  if b.BindKey == "" || b.UserID == "" || b.Chain == "" || b.Address == "" {
		   return ErrConflict
	   }
	  if !sqlOn(r.db) {
		   return ErrUnavailable
	  }
	  alloc := 0
	   if b.Allocated {
		   alloc = 1
	  }
	  if b.BoundAt.IsZero() {
		  b.BoundAt = time.Now().UTC()
	   }
	   if b.ReuseAfter.IsZero() {
		   b.ReuseAfter = b.BoundAt
	   }
	  q := `INSERT INTO aa_addr_binds (bind_key, user_id, chain, family, address, allocated, fence_epoch, fence_token, bound_at, reuse_after)
VALUES (?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE user_id=VALUES(user_id), chain=VALUES(chain), family=VALUES(family), address=VALUES(address), allocated=VALUES(allocated), fence_epoch=VALUES(fence_epoch), fence_token=VALUES(fence_token), bound_at=VALUES(bound_at), reuse_after=VALUES(reuse_after)`
	  if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO aa_addr_binds (bind_key, user_id, chain, family, address, allocated, fence_epoch, fence_token, bound_at, reuse_after) 
VALUES (?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (bind_key) DO UPDATE SET user_id=EXCLUDED.user_id, chain=EXCLUDED.chain, family=EXCLUDED.family, address=EXCLUDED.address, allocated=EXCLUDED.allocated, fence_epoch=EXCLUDED.fence_epoch, fence_token=EXCLUDED.fence_token, bound_at=EXCLUDED.bound_at, reuse_after=EXCLUDED.reuse_after`
	   }
	   _,
		   err := r.db.Conn(ctx).ExecContext(ctx, q, b.BindKey, b.UserID, b.Chain, b.Family, b.Address, alloc, b.Fence.Epoch, b.Fence.Token, b.BoundAt, b.ReuseAfter)
	  if err != nil && isUnique(err) {
		   return ErrTaken
	   }
	   return err
  }

   func (r *Address) ListUser(ctx context.Context, userID string) ([]AddrBind, error) {
	  if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	  }
	  rows,
		   err := r.db.Conn(ctx).QueryContext(ctx,
		   `SELECT bind_key, user_id, chain, family, address, allocated, fence_epoch, fence_token, bound_at, reuse_after FROM aa_addr_binds WHERE user_id=? AND allocated=1 ORDER BY chain`,
		  strings.TrimSpace(userID))
	  if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	  out := make([]AddrBind, 0)
	   for rows.Next() {
		   var b AddrBind
		  var alloc int
		   if err := rows.Scan(&b.BindKey, &b.UserID, &b.Chain, &b.Family, &b.Address, &alloc, &b.Fence.Epoch, &b.Fence.Token, &b.BoundAt, &b.ReuseAfter); err != nil {
			  return nil, err
		  }
		  b.Allocated = alloc != 0
		   out = append(out, b)
	  }
	   return out, rows.Err()
   }

  func (r *Address) SetAllocated(ctx context.Context, bindKey string, allocated bool) error {
	  if !sqlOn(r.db) {
		  return ErrUnavailable
	   }
	  flag := 0
	  if allocated {
		  flag = 1
	  }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, `UPDATE aa_addr_binds SET allocated=? WHERE bind_key=?`, flag, strings.TrimSpace(bindKey))
	   return err
   }

   func (r *Address) get(ctx context.Context, key string) (AddrBind, error) {
	   var b AddrBind
	   var alloc int
	   err := r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT bind_key, user_id, chain, family, address, allocated, fence_epoch, fence_token, bound_at, reuse_after FROM aa_addr_binds WHERE bind_key=?`,
		  key,
	  ).Scan(&b.BindKey, &b.UserID, &b.Chain, &b.Family, &b.Address, &alloc, &b.Fence.Epoch, &b.Fence.Token, &b.BoundAt, &b.ReuseAfter)
	   if errors.Is(err, sql.ErrNoRows) {
		   return AddrBind{}, ErrNotFound
	  }
	   if err != nil {
		  return AddrBind{}, err
	  }
	  b.Allocated = alloc != 0
	  return b, nil
   }
// temporary guard removed after the production check
