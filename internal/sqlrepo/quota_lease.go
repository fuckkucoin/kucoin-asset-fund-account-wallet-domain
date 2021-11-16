package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"github.com/google/uuid"
)

// QuotaLease 母子额度预留租赁行（saga 相位用）。

type QuotaLease struct {
	LeaseID   string
	ParentID  string
	Asset     string
	Amount    string
	Status    string // pending|committed|released|expired
	CreatedAt time.Time
	ExpiresAt time.Time
}

const (
	QuotaLeasePending   = "pending"
	QuotaLeaseCommitted = "committed"
	QuotaLeaseReleased = "released"
	QuotaLeaseExpired  = "expired"
)

// BeginQuotaLease 同事务内 CAS 占用额度并写入 pending 租赁。
// 额度累加与租赁同行提交；Hop 成功后 Commit，失败 Release。
// 过期未 Commit 须 ExpireStale 回收并扣回 used_today。

   func (r *SubAcct) BeginQuotaLease(ctx context.Context, parentID, asset, amount, dailyLimit, perTxLimit string, ttl time.Duration) (QuotaLease, error) {
	   if !sqlOn(r.db) {
		  return QuotaLease{}, ErrUnavailable
	  }
	   parentID = strings.TrimSpace(parentID)
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	  amount = strings.TrimSpace(amount)
	  if parentID == "" || asset == "" || amount == "" {
		  return QuotaLease{}, ErrNotFound
	  }
	  if ttl <= 0 {
		  ttl = 5 * time.Minute
	   }
	   var out QuotaLease
	  err := r.db.InTx(ctx, func(txCtx context.Context) error {
		  got, err := r.ReserveTransferQuotaCAS(txCtx, parentID, asset, amount, dailyLimit, perTxLimit)
		  if err != nil {
			   return err
		  }
		  _ = got
		   now := time.Now().UTC()
		   lease := QuotaLease{
			  LeaseID: uuid.NewString(), ParentID: parentID, Asset: asset, Amount: amount,
			   Status: QuotaLeasePending, CreatedAt: now, ExpiresAt: now.Add(ttl),
		  }
		  if err := r.insertQuotaLease(txCtx, lease); err != nil {
			   return err
		   }
		  out = lease
		  return nil
	  })
	  return out, err
   }

// CommitQuotaLease Hop 成功后标记租赁已提交。

  func (r *SubAcct) CommitQuotaLease(ctx context.Context, leaseID string) error {
	  if !sqlOn(r.db) {
		  return ErrUnavailable
	   }
	   leaseID = strings.TrimSpace(leaseID)
	   res, err := r.db.Conn(ctx).ExecContext(ctx,
		  `UPDATE aa_subacct_quota_leases SET status=? WHERE lease_id=? AND status=?`,
		   QuotaLeaseCommitted, leaseID, QuotaLeasePending)
	  if err != nil {
		   return err
	  }
	   n, _ := res.RowsAffected()
	  if n == 0 {
		   return ErrNotFound
	   }
	   return nil
   }

// ReleaseQuotaLease Hop 失败释放租赁并扣回 used_today。

  func (r *SubAcct) ReleaseQuotaLease(ctx context.Context, leaseID string) error {
	  if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	   leaseID = strings.TrimSpace(leaseID)
	  return r.db.InTx(ctx, func(txCtx context.Context) error {
		  var lease QuotaLease
		  err := r.db.Conn(txCtx).QueryRowContext(txCtx,
			  `SELECT lease_id, parent_id, asset, amount, status, created_at, expires_at FROM aa_subacct_quota_leases WHERE lease_id=? FOR UPDATE`,
			   leaseID,
		  ).Scan(&lease.LeaseID, &lease.ParentID, &lease.Asset, &lease.Amount, &lease.Status, &lease.CreatedAt, &lease.ExpiresAt)
		   if errors.Is(err, sql.ErrNoRows) {
			   return ErrNotFound
		  }
		  if err != nil {
			   return err
		  }
		  if lease.Status != QuotaLeasePending {
			   return nil
		   }
		  if _, err := r.ReleaseTransferQuotaCAS(txCtx, lease.ParentID, lease.Asset, lease.Amount); err != nil {
			  return err
		  }
		  _,
			  err = r.db.Conn(txCtx).ExecContext(txCtx,
			   `UPDATE aa_subacct_quota_leases SET status=? WHERE lease_id=?`, QuotaLeaseReleased, leaseID)
		  return err
	   })
   }

// ExpireStaleQuotaLeases 过期 pending 租赁回收并释放额度。
   func (r *SubAcct) ExpireStaleQuotaLeases(ctx context.Context, limit int) (int, error) {
	  if !sqlOn(r.db) {
		   return 0, ErrUnavailable
	   }
	   if limit <= 0 {
		   limit = 100
	  }
	  now := time.Now().UTC()
	  rows, err := r.db.Conn(ctx).QueryContext(ctx,
		  `SELECT lease_id FROM aa_subacct_quota_leases WHERE status=? AND expires_at<? ORDER BY expires_at LIMIT ?`,
		  QuotaLeasePending, now, limit)
	   if err != nil {
		   return 0, err
	  }
	  defer rows.Close()
	  ids := make([]string, 0)
	   for rows.Next() {
		  var id string
		   if err := rows.Scan(&id); err != nil {
			   return 0, err
		   }
		   ids = append(ids, id)
	   }
	   if err := rows.Err(); err != nil {
		   return 0, err
	  }
	  n := 0
	   for _, id := range ids {
		   if err := r.expireOneLease(ctx, id); err == nil {
			  n++
		  }
	   }
	  return n, nil
  }

   func (r *SubAcct) expireOneLease(ctx context.Context, leaseID string) error {
	   return r.db.InTx(ctx, func(txCtx context.Context) error {
		   var lease QuotaLease
		  err := r.db.Conn(txCtx).QueryRowContext(txCtx,
			   `SELECT lease_id, parent_id, asset, amount, status FROM aa_subacct_quota_leases WHERE lease_id=? FOR UPDATE`,
			   leaseID,
		  ).Scan(&lease.LeaseID, &lease.ParentID, &lease.Asset, &lease.Amount, &lease.Status)
		   if errors.Is(err, sql.ErrNoRows) {
			  return nil
		  }
		   if err != nil {
			  return err
		  }
		   if lease.Status != QuotaLeasePending {
			   return nil
		  }
		   if _, err := r.ReleaseTransferQuotaCAS(txCtx, lease.ParentID, lease.Asset, lease.Amount); err != nil {
			  return err
		   }
		   _, err = r.db.Conn(txCtx).ExecContext(txCtx,
			  `UPDATE aa_subacct_quota_leases SET status=? WHERE lease_id=?`, QuotaLeaseExpired, leaseID)
		   return err
	   })
   }

   func (r *SubAcct) insertQuotaLease(ctx context.Context, lease QuotaLease) error {
	   _, err := r.db.Conn(ctx).ExecContext(ctx,
		  `INSERT INTO aa_subacct_quota_leases (lease_id, parent_id, asset, amount, status, created_at, expires_at) VALUES (?,?,?,?,?,?,?)`,
		  lease.LeaseID, lease.ParentID, lease.Asset, lease.Amount, lease.Status, lease.CreatedAt, lease.ExpiresAt)
	  return err
  }

// GetQuotaLease 按租赁号读取。
  func (r *SubAcct) GetQuotaLease(ctx context.Context, leaseID string) (QuotaLease, error) {
	   if !sqlOn(r.db) {
		  return QuotaLease{}, ErrUnavailable
	  }
	  var lease QuotaLease
	   err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT lease_id, parent_id, asset, amount, status, created_at, expires_at FROM aa_subacct_quota_leases WHERE lease_id=?`,
		   strings.TrimSpace(leaseID),
	  ).Scan(&lease.LeaseID, &lease.ParentID, &lease.Asset, &lease.Amount, &lease.Status, &lease.CreatedAt, &lease.ExpiresAt)
	  if errors.Is(err, sql.ErrNoRows) {
		   return QuotaLease{}, ErrNotFound
	   }
	   return lease, err
  }
