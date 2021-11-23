package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// Nonce EVM 出金 nonce 槽。同一 from 必须串行占号，回放原哈希，禁止双播。

type Nonce struct {
	db *persist.SQLDB
}

   func NewNonce(db *persist.SQLDB) *Nonce { return &Nonce{db: db} }

type NonceSlot struct {
	Key string
	Chain string
	From string
	Next int64
	LastHash string
	Fence     persist.Fence
	UpdatedAt time.Time
}

   func nonceSlotKey(chain, from string) string {
	  return strings.ToUpper(strings.TrimSpace(chain)) + "|" + strings.ToLower(strings.TrimSpace(from))
   }

   func (r *Nonce) Reserve(ctx context.Context, chain, from, hash string) (NonceSlot, error) {
	   if r == nil || r.db == nil {
		  return NonceSlot{}, ErrUnavailable
	  }
	   chain = strings.ToUpper(strings.TrimSpace(chain))
	  from = strings.TrimSpace(from)
	   hash = strings.TrimSpace(hash)
	   if chain == "" || from == "" || hash == "" {
		  return NonceSlot{}, ErrConflict
	  }
	   key := nonceSlotKey(chain, from)
	   var out NonceSlot
	   err := r.db.InTx(ctx, func(ctx context.Context) error {
		   cur, err := r.get(ctx, key)
		  if err != nil && !errors.Is(err, ErrNotFound) {
			  return err
		  }
		   next := int64(0)
		  missing := errors.Is(err, ErrNotFound)
		   if !missing {
			   next = cur.Next
		   }
		   f, err := r.db.BumpFence(ctx, "nonce|"+key, persist.NewToken())
		  if err != nil {
			   return err
		   }
		  if missing {
			  _, err = r.db.Conn(ctx).ExecContext(ctx,
				  `INSERT INTO aa_nonce_slots (slot_key, chain, from_addr, next_nonce, last_hash, fence_epoch, fence_token, updated_at) VALUES (?,?,?,?,?,?,?,CURRENT_TIMESTAMP(3))`,
				  key, chain, from, next+1, hash, f.Epoch, f.Token)
		  } else {
			   _, err = r.db.Conn(ctx).ExecContext(ctx,
				  `UPDATE aa_nonce_slots SET next_nonce=?, last_hash=?, fence_epoch=?, fence_token=?, updated_at=CURRENT_TIMESTAMP(3) WHERE slot_key=?`,
				   next+1, hash, f.Epoch, f.Token, key)
		  }
		   if err != nil {
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

  func (r *Nonce) Get(ctx context.Context, chain, from string) (NonceSlot, error) {
	  if r == nil || r.db == nil {
		  return NonceSlot{}, ErrUnavailable
	   }
	   return r.get(ctx, nonceSlotKey(chain, from))
   }

   func (r *Nonce) get(ctx context.Context, key string) (NonceSlot, error) {
	  var s NonceSlot
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT slot_key, chain, from_addr, next_nonce, last_hash, fence_epoch, fence_token, updated_at FROM aa_nonce_slots WHERE slot_key=?`,
		  key,
	   ).Scan(&s.Key, &s.Chain, &s.From, &s.Next, &s.LastHash, &s.Fence.Epoch, &s.Fence.Token, &s.UpdatedAt)
	  if errors.Is(err, sql.ErrNoRows) {
		   return NonceSlot{}, ErrNotFound
	   }
	   return s, err
  }
