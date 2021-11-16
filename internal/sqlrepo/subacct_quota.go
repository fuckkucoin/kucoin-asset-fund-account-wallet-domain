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

// SubTransferQuota 母子划转额度 SQL 行。
type SubTransferQuota struct {
	ParentID string
	Asset    string
	DailyLimit string
	PerTxLimit string
	UsedToday string
	Day       string
	UpdatedAt time.Time
}

// PutTransferQuota upsert 母子划转额度。
// 日切后 used_today 与 day 一并写入；账本旁路配额权威在 SQL。
// 仅内存配额在多实例下会漂移，必须 BindSQL。
  func (r *SubAcct) PutTransferQuota(ctx context.Context, q SubTransferQuota) error {
	  if !sqlOn(r.db) {
		  return ErrUnavailable
	   }
	   if q.UpdatedAt.IsZero() {
		  q.UpdatedAt = time.Now().UTC()
	   }
	   daily := strings.TrimSpace(q.DailyLimit)
	  if daily == "" {
		   daily = "0"
	  }
	   per := strings.TrimSpace(q.PerTxLimit)
	   if per == "" {
		   per = "0"
	   }
	   used := strings.TrimSpace(q.UsedToday)
	  if used == "" {
		  used = "0"
	   }
	   stmt := `INSERT INTO aa_subacct_transfer_quotas (parent_id, asset, daily_limit, per_tx_limit, used_today, day, updated_at)
VALUES (?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE daily_limit=VALUES(daily_limit), per_tx_limit=VALUES(per_tx_limit), used_today=VALUES(used_today), day=VALUES(day), updated_at=VALUES(updated_at)`
	  if r.db.Dialect == persist.Postgres {
		   stmt = `INSERT INTO aa_subacct_transfer_quotas (parent_id, asset, daily_limit, per_tx_limit, used_today, day, updated_at)
VALUES (?,?,?,?,?,?,?)
ON CONFLICT (parent_id, asset) DO UPDATE SET daily_limit=EXCLUDED.daily_limit, per_tx_limit=EXCLUDED.per_tx_limit, used_today=EXCLUDED.used_today, day=EXCLUDED.day, updated_at=EXCLUDED.updated_at`
	   }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, stmt,
		  strings.TrimSpace(q.ParentID), strings.ToUpper(strings.TrimSpace(q.Asset)),
		   daily,
		  per, used, strings.TrimSpace(q.Day), q.UpdatedAt)
	   return err
  }

// GetTransferQuota 按母户+资产读取划转额度。
   func (r *SubAcct) GetTransferQuota(ctx context.Context, parentID, asset string) (SubTransferQuota, error) {
	  if !sqlOn(r.db) {
		   return SubTransferQuota{}, ErrUnavailable
	  }
	   var q SubTransferQuota
	   err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT parent_id, asset, daily_limit, per_tx_limit, used_today, day, updated_at FROM aa_subacct_transfer_quotas WHERE parent_id=? AND asset=?`,
		  strings.TrimSpace(parentID), strings.ToUpper(strings.TrimSpace(asset)),
	  ).Scan(&q.ParentID, &q.Asset, &q.DailyLimit, &q.PerTxLimit, &q.UsedToday, &q.Day, &q.UpdatedAt)
	   if errors.Is(err, sql.ErrNoRows) {
		   return SubTransferQuota{},
			  ErrNotFound
	  }
	  return q, err
   }

// ListTransferQuotas 列出母户下全部资产划转额度（重建内存表）。
  func (r *SubAcct) ListTransferQuotas(ctx context.Context, parentID string) ([]SubTransferQuota, error) {
	  if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	   }
	  parentID = strings.TrimSpace(parentID)
	   var rows *sql.Rows
	  var err error
	   if parentID == "" {
		   rows, err = r.db.Conn(ctx).QueryContext(ctx,
			   `SELECT parent_id, asset, daily_limit, per_tx_limit, used_today, day, updated_at FROM aa_subacct_transfer_quotas`)
	  } else {
		   rows, err = r.db.Conn(ctx).QueryContext(ctx,
			   `SELECT parent_id, asset, daily_limit, per_tx_limit, used_today, day, updated_at FROM aa_subacct_transfer_quotas WHERE parent_id=?`, parentID)
// 无行时若 daily/per 未配置则放行（与内存路径一致）；超额返回 ErrConflict。

   func (r *SubAcct) ReserveTransferQuotaCAS(ctx context.Context, parentID, asset, amount, dailyLimit, perTxLimit string) (SubTransferQuota, error) {
	  if !sqlOn(r.db) {
		  return SubTransferQuota{}, ErrUnavailable
	   }
	   parentID = strings.TrimSpace(parentID)
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   amount = strings.TrimSpace(amount)
	   if parentID == "" || asset == "" || amount == "" {
		   return SubTransferQuota{}, ErrNotFound
	   }
	   var out SubTransferQuota
	  err := r.db.InTx(ctx, func(txCtx context.Context) error {
		  day := time.Now().UTC().Format("2006-01-02")
		  var q SubTransferQuota
		  err := r.db.Conn(txCtx).QueryRowContext(txCtx,
			  `SELECT parent_id, asset, daily_limit, per_tx_limit, used_today, day, updated_at FROM aa_subacct_transfer_quotas WHERE parent_id=? AND asset=? FOR UPDATE`,
			  parentID, asset,
		  ).Scan(&q.ParentID, &q.Asset, &q.DailyLimit, &q.PerTxLimit, &q.UsedToday, &q.Day, &q.UpdatedAt)
		   if errors.Is(err, sql.ErrNoRows) {
			   // 无持久化额度配置：不占用，保持与内存「无配置放行」一致
			   if strings.TrimSpace(dailyLimit) == "" && strings.TrimSpace(perTxLimit) == "" {
				   out = SubTransferQuota{ParentID: parentID, Asset: asset, DailyLimit: "0", PerTxLimit: "0", UsedToday: "0", Day: day}
				   return nil
			   }
			   q = SubTransferQuota{
				  ParentID: parentID, Asset: asset,
				  DailyLimit: nz(dailyLimit, "0"), PerTxLimit: nz(perTxLimit, "0"),
				   UsedToday: "0", Day: day, UpdatedAt: time.Now().UTC(),
			   }
			   if err := r.insertQuotaRow(txCtx, q); err != nil {
				  return err
			   }
		   } else if err != nil {
			   return err
		   }
		   if q.Day != day {
			   q.Day = day
			   q.UsedToday = "0"
		  }
		  if lim := strings.TrimSpace(dailyLimit); lim != "" {
			   q.DailyLimit = lim
		  }
		  if lim := strings.TrimSpace(perTxLimit); lim != "" {
			  q.PerTxLimit = lim
		   }
		  nextUsed, err := addQuotaUsed(q.UsedToday, amount, q.PerTxLimit, q.DailyLimit)
		  if err != nil {
			  return err
		  }
		   q.UsedToday = nextUsed
		  q.UpdatedAt = time.Now().UTC()
		  if err := r.updateQuotaRowLocked(txCtx, q); err != nil {
			  return err
		   }
		   out = q
		   return nil
	  })
	  return out, err
  }

// ReleaseTransferQuotaCAS 跨实例额度释放——行锁后扣减 used_today（不小于 0）。
   func (r *SubAcct) ReleaseTransferQuotaCAS(ctx context.Context, parentID, asset, amount string) (SubTransferQuota, error) {
	   if !sqlOn(r.db) {
		  return SubTransferQuota{}, ErrUnavailable
	   }
	   parentID = strings.TrimSpace(parentID)
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  amount = strings.TrimSpace(amount)
	  var out SubTransferQuota
	   err := r.db.InTx(ctx, func(txCtx context.Context) error {
		   day := time.Now().UTC().Format("2006-01-02")
		   var q SubTransferQuota
		   err := r.db.Conn(txCtx).QueryRowContext(txCtx,
			  `SELECT parent_id, asset, daily_limit, per_tx_limit, used_today, day, updated_at FROM aa_subacct_transfer_quotas WHERE parent_id=? AND asset=? FOR UPDATE`,
			  parentID, asset,
		  ).Scan(&q.ParentID, &q.Asset, &q.DailyLimit, &q.PerTxLimit, &q.UsedToday, &q.Day, &q.UpdatedAt)
		  if errors.Is(err, sql.ErrNoRows) {
			  return nil
		   }
		  if err != nil {
			  return err
		   }
		  if q.Day != day {
			   q.Day = day
			  q.UsedToday = "0"
		  } else {
			   q.UsedToday = subQuotaUsed(q.UsedToday, amount)
		  }
		   q.UpdatedAt = time.Now().UTC()
		   if err := r.updateQuotaRowLocked(txCtx, q); err != nil {
			   return err
		   }
		   out = q
		   return nil
	   })
	  return out, err
   }

  func nz(s, def string) string {
	   s = strings.TrimSpace(s)
	  if s == "" {
		   return def
	  }
	  return s
   }

  func (r *SubAcct) insertQuotaRow(ctx context.Context, q SubTransferQuota) error {
	   stmt := `INSERT INTO aa_subacct_transfer_quotas (parent_id, asset, daily_limit, per_tx_limit, used_today, day, updated_at) VALUES (?,?,?,?,?,?,?)`
	   _, err := r.db.Conn(ctx).ExecContext(ctx, stmt,
		  q.ParentID, q.Asset, q.DailyLimit, q.PerTxLimit, q.UsedToday, q.Day, q.UpdatedAt)
	  return err
  }

   func (r *SubAcct) updateQuotaRowLocked(ctx context.Context, q SubTransferQuota) error {
	  _, err := r.db.Conn(ctx).ExecContext(ctx,
		   `UPDATE aa_subacct_transfer_quotas SET daily_limit=?, per_tx_limit=?, used_today=?, day=?, updated_at=? WHERE parent_id=? AND asset=?`,
		   q.DailyLimit, q.PerTxLimit, q.UsedToday, q.Day, q.UpdatedAt, q.ParentID, q.Asset)
	   return err
   }

  func addQuotaUsed(used, amount, perTx, daily string) (string, error) {
	  amt, err := money.Parse(amount)
	   if err != nil {
		  return "", ErrAmount
	   }
	  if per := strings.TrimSpace(perTx); per != "" {
		  lim, err := money.Parse(per)
		   if err == nil && lim.IsPositive() && amt.Cmp(lim) > 0 {
			   return "", fmt.Errorf("%w: per-tx limit %s", ErrConflict, per)
		  }
	   }
	   dailyLim, err := money.ParseBook(strings.TrimSpace(daily))
	   if err != nil || !dailyLim.IsPositive() {
		  // 日额未配：不累加占用
		   return used, nil
	   }
	   cur, err := money.ParseBook(used)
	   if err != nil {
		  cur = money.Zero()
	   }
	  next, err := cur.Add(amt)
	  if err != nil {
		  return "", err
	   }
	   if next.Cmp(dailyLim) > 0 {
		  return "", fmt.Errorf("%w: daily transfer quota exceeded", ErrConflict)
	   }
	   return next.String(), nil
  }

   func subQuotaUsed(used, amount string) string {
	  amt, err := money.Parse(amount)
	  if err != nil {
		   return used
	  }
	   cur, err := money.ParseBook(used)
	  if err != nil {
		   return "0"
	   }
	   next, err := cur.Sub(amt)
	   if err != nil || next.IsNegative() {
		   return "0"
	  }
	   return next.String()
   }
