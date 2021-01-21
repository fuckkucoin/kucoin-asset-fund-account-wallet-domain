package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/google/uuid"
)

// KYT 命中落 aa_kyt_hits。决策必须是 allow/review/deny，缺决策当不可用。

type KYT struct {
	db *persist.SQLDB
}

  func NewKYT(db *persist.SQLDB) *KYT { return &KYT{db: db} }

type KYTHit struct {
	ID     string
	UserID string
	MovementID string
	Chain      string
	Dest string
	Asset string
	Amount string
	Decision string
	Reason   string
	Pack    string
	PackVer int
	Mode    string
	Fence persist.Fence
	CreatedAt time.Time
}

   func (r *KYT) Record(ctx context.Context, hit KYTHit) (KYTHit, error) {
	  if r == nil || r.db == nil {
		   return KYTHit{}, ErrUnavailable
	  }
	   if _, err := money.Parse(hit.Amount); err != nil {
		  return KYTHit{}, ErrAmount
	  }
	   hit.UserID = strings.TrimSpace(hit.UserID)
	   hit.Chain = strings.ToUpper(strings.TrimSpace(hit.Chain))
	   hit.Asset = strings.ToUpper(strings.TrimSpace(hit.Asset))
	  hit.Dest = strings.TrimSpace(hit.Dest)
	  hit.Decision = strings.ToLower(strings.TrimSpace(hit.Decision))
	  hit.Mode = strings.ToLower(strings.TrimSpace(hit.Mode))
	   if hit.Mode == "" {
		   hit.Mode = "enforce"
	  }
	   switch hit.Decision {
	  case "allow", "review", "deny":
	  default:
		   return KYTHit{}, ErrConflict
	  }
	  if hit.UserID == "" || hit.Chain == "" {
		   return KYTHit{}, ErrConflict
	   }
	  if hit.ID == "" {
		  hit.ID = uuid.NewString()
	  }
	  err := r.db.InTx(ctx, func(ctx context.Context) error {
		   f, err := r.db.BumpFence(ctx, "kyt|"+hit.ID, persist.NewToken())
		   if err != nil {
			   return err
		   }
		   hit.Fence = f
		  _,
			   err = r.db.Conn(ctx).ExecContext(ctx,
			  `INSERT INTO aa_kyt_hits (id, user_id, movement_id, chain, dest, asset, amount, decision, reason, pack, pack_ver, mode, fence_epoch, fence_token, created_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP(3))`,
			  hit.ID, hit.UserID, hit.MovementID, hit.Chain, hit.Dest, hit.Asset, hit.Amount, hit.Decision, hit.Reason, hit.Pack, hit.PackVer, hit.Mode, f.Epoch, f.Token)
		   return err
	   })
	  if err != nil {
		   return KYTHit{}, err
	   }
	  return r.Get(ctx, hit.ID)
  }

  func (r *KYT) Get(ctx context.Context, id string) (KYTHit, error) {
	   if r == nil || r.db == nil {
		  return KYTHit{}, ErrUnavailable
	   }
	  var h KYTHit
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT id, user_id, movement_id, chain, dest, asset, amount, decision, reason, pack, pack_ver, mode, fence_epoch, fence_token, created_at FROM aa_kyt_hits WHERE id=?`,
		  strings.TrimSpace(id),
	   ).Scan(&h.ID, &h.UserID, &h.MovementID, &h.Chain, &h.Dest, &h.Asset, &h.Amount, &h.Decision, &h.Reason, &h.Pack, &h.PackVer, &h.Mode, &h.Fence.Epoch, &h.Fence.Token, &h.CreatedAt)
	   if errors.Is(err, sql.ErrNoRows) {
		  return KYTHit{}, ErrNotFound
	   }
	  return h, err
  }

  func (r *KYT) ListByUser(ctx context.Context, userID string, limit int) ([]KYTHit, error) {
	  if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	   }
	   if limit <= 0 {
		  limit = 50
	   }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx,
		   `SELECT id, user_id, movement_id, chain, dest, asset, amount, decision, reason, pack, pack_ver, mode, fence_epoch, fence_token, created_at FROM aa_kyt_hits WHERE user_id=? ORDER BY created_at DESC LIMIT ?`,
		  strings.TrimSpace(userID), limit)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   out := make([]KYTHit, 0)
	   for rows.Next() {
		   var h KYTHit
		  if err := rows.Scan(&h.ID, &h.UserID, &h.MovementID, &h.Chain, &h.Dest, &h.Asset, &h.Amount, &h.Decision, &h.Reason, &h.Pack, &h.PackVer, &h.Mode, &h.Fence.Epoch, &h.Fence.Token, &h.CreatedAt); err != nil {
			  return nil, err
		  }
		   out = append(out, h)
	   }
	  return out, rows.Err()
   }

  func (r *KYT) PutEvidence(ctx context.Context, hitID, movementID, userID, body string) error {
	  if !sqlOn(r.db) {
		   return ErrUnavailable
	   }
	  _, err := r.db.Conn(ctx).ExecContext(ctx,
		   `INSERT INTO aa_kyt_evidence (id, hit_id, movement_id, user_id, body, created_at) VALUES (?,?,?,?,?,CURRENT_TIMESTAMP(3))`,
		   uuid.NewString(),
		   hitID, movementID, userID, body)
	   return err
   }
