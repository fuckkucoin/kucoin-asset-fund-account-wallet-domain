package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

// EarnPosition 理财仓位 SQL 行（跨实例主存）。

type EarnPosition struct {
	ID        string
	UserID    string
	ProductID string
	Asset string
	Principal string
	Accrued   string
	Claimed   string
	Penalty   string
	OpenedAt time.Time
	UnlockAt time.Time
	LastAccrue    time.Time
	Status        string
	TransferID    string
	Idempotency   string
	AutoSubscribe bool
	UpdatedAt     time.Time
}

// EarnProduct 理财产品目录 SQL 行（含 EarlyBPS）。

type EarnProduct struct {
	ID string
	Asset        string
	Category     string
	DailyBPS     int
	LockDays     int
	MinSubscribe string
	MaxPosition  string
	EarlyBPS      int
	Active        bool
	AutoSubscribe bool
	UpdatedAt time.Time
}

// EarnStore 理财仓位与产品目录 SQL 面。
type EarnStore struct{ db *persist.SQLDB }

   func NewEarn(db *persist.SQLDB) *EarnStore { return &EarnStore{db: db} }

// PutPosition upsert 仓位；主存。
// 幂等键可空；空 unlock/last_accrue 写 NULL。
// 未 BindSQL 时调用方仍走内存/KV，多实例会漂移。

   func (r *EarnStore) PutPosition(ctx context.Context, row EarnPosition) error {
	   if !sqlOn(r.db) {
		   return ErrUnavailable
	   }
	   row.ID = strings.TrimSpace(row.ID)
	   if row.ID == "" {
		  return ErrNotFound
	  }
	   if row.UpdatedAt.IsZero() {
		  row.UpdatedAt = time.Now().UTC()
	  }
	   if row.OpenedAt.IsZero() {
		  row.OpenedAt = row.UpdatedAt
	   }
	   auto := 0
	  if row.AutoSubscribe {
		   auto = 1
	   }
	   var unlock, last any
	   if !row.UnlockAt.IsZero() {
		   unlock = row.UnlockAt
	  }
	  if !row.LastAccrue.IsZero() {
		  last = row.LastAccrue
	   }
	   stmt := `INSERT INTO aa_earn_positions
 (id, user_id, product_id, asset, principal, accrued, claimed, penalty, opened_at, unlock_at, last_accrue,
  status, transfer_id, idempotency_key, auto_subscribe, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE user_id=VALUES(user_id), product_id=VALUES(product_id), asset=VALUES(asset),
 principal=VALUES(principal), accrued=VALUES(accrued), claimed=VALUES(claimed), penalty=VALUES(penalty),
 opened_at=VALUES(opened_at), unlock_at=VALUES(unlock_at), last_accrue=VALUES(last_accrue),
 status=VALUES(status), transfer_id=VALUES(transfer_id), idempotency_key=VALUES(idempotency_key),
 auto_subscribe=VALUES(auto_subscribe), updated_at=VALUES(updated_at)`
	  if r.db.Dialect == persist.Postgres {
		   stmt = `INSERT INTO aa_earn_positions
 (id, user_id, product_id, asset, principal, accrued, claimed, penalty, opened_at, unlock_at, last_accrue,
  status, transfer_id, idempotency_key, auto_subscribe, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET user_id=EXCLUDED.user_id, product_id=EXCLUDED.product_id, asset=EXCLUDED.asset,
 principal=EXCLUDED.principal, accrued=EXCLUDED.accrued, claimed=EXCLUDED.claimed, penalty=EXCLUDED.penalty,
 opened_at=EXCLUDED.opened_at, unlock_at=EXCLUDED.unlock_at, last_accrue=EXCLUDED.last_accrue,
 status=EXCLUDED.status, transfer_id=EXCLUDED.transfer_id, idempotency_key=EXCLUDED.idempotency_key,
 auto_subscribe=EXCLUDED.auto_subscribe, updated_at=EXCLUDED.updated_at`
	  }
	   _,
		  err := r.db.Conn(ctx).ExecContext(ctx, stmt,
		  row.ID, strings.TrimSpace(row.UserID), strings.TrimSpace(row.ProductID), strings.ToUpper(strings.TrimSpace(row.Asset)),
		  nz(row.Principal, "0"), nz(row.Accrued, "0"), nz(row.Claimed, "0"), row.Penalty,
		   row.OpenedAt, unlock, last, strings.TrimSpace(row.Status), row.TransferID, strings.TrimSpace(row.Idempotency),
		  auto, row.UpdatedAt)
	  return err
   }

// GetPosition 按仓位 id 读取。
   func (r *EarnStore) GetPosition(ctx context.Context, id string) (EarnPosition, error) {
	   if !sqlOn(r.db) {
		   return EarnPosition{}, ErrUnavailable
	   }
	   id = strings.TrimSpace(id)
	   row, err := r.scanPosition(r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT id, user_id, product_id, asset, principal, accrued, claimed, penalty, opened_at, unlock_at, last_accrue,
 status, transfer_id, idempotency_key, auto_subscribe, updated_at FROM aa_earn_positions WHERE id=?`, id))
	  if errors.Is(err, sql.ErrNoRows) {
		  return EarnPosition{}, ErrNotFound
	  }
	  return row, err
   }

// GetPositionByIdem 按幂等键读取仓位。
   func (r *EarnStore) GetPositionByIdem(ctx context.Context, key string) (EarnPosition, error) {
	   if !sqlOn(r.db) {
		   return EarnPosition{},
			   ErrUnavailable
	  }
	  key = strings.TrimSpace(key)
	  if key == "" {
		   return EarnPosition{}, ErrNotFound
	  }
	   row, err := r.scanPosition(r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT id, user_id, product_id, asset, principal, accrued, claimed, penalty, opened_at, unlock_at, last_accrue,
 status, transfer_id, idempotency_key, auto_subscribe, updated_at FROM aa_earn_positions WHERE idempotency_key=? LIMIT 1`, key))
	   if errors.Is(err, sql.ErrNoRows) {
		  return EarnPosition{}, ErrNotFound
	  }
	   return row, err
   }

// ListPositions 按用户列仓位；userID 空则全量（Hydrate）。

   func (r *EarnStore) ListPositions(ctx context.Context, userID string) ([]EarnPosition, error) {
	   if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	  }
	  userID = strings.TrimSpace(userID)
	   var rows *sql.Rows
	  var err error
	  q := `SELECT id, user_id, product_id, asset, principal, accrued, claimed, penalty, opened_at, unlock_at, last_accrue,  
 status, transfer_id, idempotency_key, auto_subscribe, updated_at FROM aa_earn_positions`
	  if userID == "" {
		  rows, err = r.db.Conn(ctx).QueryContext(ctx, q+` ORDER BY opened_at DESC`)
	   } else {
		   rows, err = r.db.Conn(ctx).QueryContext(ctx, q+` WHERE user_id=? ORDER BY opened_at DESC`, userID)
	  }
	   if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	   out := make([]EarnPosition, 0)
	  for rows.Next() {
		   row, err := r.scanPosition(rows)
		   if err != nil {
			  return nil, err
		   }
		   out = append(out, row)
	  }
	  return out, rows.Err()
   }

type positionScanner interface {
	Scan(dest ...any) error
}

   func (r *EarnStore) scanPosition(sc positionScanner) (EarnPosition, error) {
	   var row EarnPosition
	  var unlock, last sql.NullTime
	   var auto int
	   err := sc.Scan(&row.ID, &row.UserID, &row.ProductID, &row.Asset, &row.Principal, &row.Accrued, &row.Claimed, &row.Penalty,
		  &row.OpenedAt, &unlock, &last, &row.Status, &row.TransferID, &row.Idempotency, &auto, &row.UpdatedAt)
	  if err != nil {
		  return EarnPosition{}, err
	  }
	  if unlock.Valid {
		  row.UnlockAt = unlock.Time
	   }
	   if last.Valid {
		  row.LastAccrue = last.Time
	   }
	   row.AutoSubscribe = auto != 0
	   return row, nil
  }

// PutProduct upsert 产品目录行（含 EarlyBPS）。
// 运营 PUT 先写库再热更新内存。
// 损坏 BPS/限额不得绕过申购闸。

   func (r *EarnStore) PutProduct(ctx context.Context, row EarnProduct) error {
	  if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	   row.ID = strings.TrimSpace(row.ID)
	  if row.ID == "" {
		  return ErrNotFound
	  }
	  if row.UpdatedAt.IsZero() {
		   row.UpdatedAt = time.Now().UTC()
	   }
	   active, auto := 0, 0
	  if row.Active {
		   active = 1
	  }
	  if row.AutoSubscribe {
		  auto = 1
	   }
	  stmt := `INSERT INTO aa_earn_products
 (id, asset, category, daily_bps, lock_days, min_subscribe, max_position, early_bps, active, auto_subscribe, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE asset=VALUES(asset), category=VALUES(category), daily_bps=VALUES(daily_bps),
 lock_days=VALUES(lock_days), min_subscribe=VALUES(min_subscribe), max_position=VALUES(max_position),
 early_bps=VALUES(early_bps), active=VALUES(active), auto_subscribe=VALUES(auto_subscribe), updated_at=VALUES(updated_at),
 deleted_at=NULL`
	  if r.db.Dialect == persist.Postgres {
		   stmt = `INSERT INTO aa_earn_products 
 (id, asset, category, daily_bps, lock_days, min_subscribe, max_position, early_bps, active, auto_subscribe, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET asset=EXCLUDED.asset, category=EXCLUDED.category, daily_bps=EXCLUDED.daily_bps,
 lock_days=EXCLUDED.lock_days, min_subscribe=EXCLUDED.min_subscribe, max_position=EXCLUDED.max_position,
 early_bps=EXCLUDED.early_bps, active=EXCLUDED.active, auto_subscribe=EXCLUDED.auto_subscribe, updated_at=EXCLUDED.updated_at,
 deleted_at=NULL`
	   }
	  _,
		   err := r.db.Conn(ctx).ExecContext(ctx, stmt,
		  row.ID, strings.ToUpper(strings.TrimSpace(row.Asset)), strings.ToLower(strings.TrimSpace(row.Category)),
		   row.DailyBPS, row.LockDays, row.MinSubscribe, row.MaxPosition, row.EarlyBPS, active, auto, row.UpdatedAt)
	   return err
  }

// ListProducts 全量产品目录（Hydrate / 运营面）。
  func (r *EarnStore) ListProducts(ctx context.Context) ([]EarnProduct, error) {
	  if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	   }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx,
		   `SELECT id, asset, category, daily_bps, lock_days, min_subscribe, max_position, early_bps, active, auto_subscribe, updated_at
 FROM aa_earn_products WHERE deleted_at IS NULL ORDER BY id`)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	  out := make([]EarnProduct, 0)
	   for rows.Next() {
		  var row EarnProduct
		   var active, auto int
		  if err := rows.Scan(&row.ID, &row.Asset, &row.Category, &row.DailyBPS, &row.LockDays, &row.MinSubscribe, &row.MaxPosition,
			   &row.EarlyBPS, &active, &auto, &row.UpdatedAt); err != nil {
			   return nil, err
		   }
		  row.Active = active != 0
		   row.AutoSubscribe = auto != 0
		  out = append(out, row)
	  }
	  return out, rows.Err()
   }

// GetProduct 按产品 id 读取。
  func (r *EarnStore) GetProduct(ctx context.Context, id string) (EarnProduct, error) {
	   if !sqlOn(r.db) {
		   return EarnProduct{}, ErrUnavailable
	  }
	  var row EarnProduct
	  var active, auto int
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT id, asset, category, daily_bps, lock_days, min_subscribe, max_position, early_bps, active, auto_subscribe, updated_at
 FROM aa_earn_products WHERE id=? AND deleted_at IS NULL`, strings.TrimSpace(id),
	   ).Scan(&row.ID, &row.Asset, &row.Category, &row.DailyBPS, &row.LockDays, &row.MinSubscribe, &row.MaxPosition,
		   &row.EarlyBPS, &active, &auto, &row.UpdatedAt)
	  if errors.Is(err, sql.ErrNoRows) {
		   return EarnProduct{}, ErrNotFound
	  }
	  if err != nil {
		   return EarnProduct{}, err
	  }
	   row.Active = active != 0
	  row.AutoSubscribe = auto != 0
	  return row, nil
   }

// CountPositions 仓位行数（健康面）。
  func (r *EarnStore) CountPositions(ctx context.Context) (int, error) {
	  if !sqlOn(r.db) {
		   return 0, ErrUnavailable
	  }
	  var n int
	  err := r.db.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM aa_earn_positions`).Scan(&n)
	  return n, err
   }

// CountProducts 产品行数（健康面）。

  func (r *EarnStore) CountProducts(ctx context.Context) (int, error) {
	   if !sqlOn(r.db) {
		   return 0, ErrUnavailable
	   }
	   var n int
	   err := r.db.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM aa_earn_products WHERE deleted_at IS NULL`).Scan(&n)
	   return n, err
  }

// PositionListFilter 仓位 SQL 分页/status 过滤（运营与用户面）。

type PositionListFilter struct {
	UserID string
	Status   string
	Page     int
	PageSize int
}

// PositionPage 分页结果。

type PositionPage struct {
	Items []EarnPosition
	Total int
	Page int
	PageSize int
	TotalPage int
}

// ListPositionsPaged 按用户/status SQL 分页列仓位（超越全量 Hydrate 读面）。
// page/pageSize 默认 1/50，硬顶 200；status 空不过滤。
// Hydrate 仍走全量 ListPositions；本面仅查询，不改内存表。
   func (r *EarnStore) ListPositionsPaged(ctx context.Context, f PositionListFilter) (PositionPage, error) {
	  out := PositionPage{Page: 1,
		  PageSize: 50}
	  if !sqlOn(r.db) {
		  return out, ErrUnavailable
	  }
	  if f.Page <= 0 {
		   f.Page = 1
	  }
	   if f.PageSize <= 0 {
		   f.PageSize = 50
	   }
	   if f.PageSize > 200 {
		   f.PageSize = 200
	  }
	  out.Page, out.PageSize = f.Page, f.PageSize
	  userID := strings.TrimSpace(f.UserID)
	  status := strings.TrimSpace(strings.ToLower(f.Status))
	  where := []string{"1=1"}
	   args := make([]any, 0, 4)
	   if userID != "" {
		  where = append(where, "user_id=?")
		   args = append(args, userID)
	   }
	  if status != "" {
		  where = append(where, "status=?")
		  args = append(args, status)
	  }
	   w := strings.Join(where, " AND ")
	   var total int
	   if err := r.db.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM aa_earn_positions WHERE `+w, args...).Scan(&total); err != nil {
		   return out, err
	   }
	   out.Total = total
	   out.TotalPage = (total + f.PageSize - 1) / f.PageSize
	   if out.TotalPage == 0 {
		   out.TotalPage = 1
	   }
	  offset := (f.Page - 1) * f.PageSize
	   q := `SELECT id, user_id, product_id, asset, principal, accrued, claimed, penalty, opened_at, unlock_at, last_accrue,
 status, transfer_id, idempotency_key, auto_subscribe, updated_at FROM aa_earn_positions WHERE ` + w +
		   ` ORDER BY opened_at DESC LIMIT ? OFFSET ?`
	  args2 := append(append([]any{}, args...), f.PageSize, offset)
	  rows, err := r.db.Conn(ctx).QueryContext(ctx, q, args2...)
	   if err != nil {
		   return out, err
	  }
	   defer rows.Close()
	   items := make([]EarnPosition, 0, f.PageSize)
	  for rows.Next() {
		   row, err := r.scanPosition(rows)
		   if err != nil {
			  return out, err
		  }
		   items = append(items, row)
	  }
	   out.Items = items
	   return out, rows.Err()
  }

// SoftDeleteProductsExcept replace 时软删不在 keepIDs 内的产品行（deleted_at）。
// 保留历史行供审计；Hydrate 跳过已删。
// keep 空则拒，避免误清空目录。

   func (r *EarnStore) SoftDeleteProductsExcept(ctx context.Context, keepIDs []string) (int64, error) {
	   if !sqlOn(r.db) {
		  return 0, ErrUnavailable
	  }
	   keep := make([]string, 0, len(keepIDs))
	   seen := map[string]struct{}{}
	   for _, id := range keepIDs {
		  id = strings.TrimSpace(id)
		   if id == "" {
			  continue
		   }
		  if _, ok := seen[id]; ok {
			   continue
		   }
		  seen[id] = struct{}{}
		   keep = append(keep, id)
	   }
	   if len(keep) == 0 {
		  return 0, fmt.Errorf("keep ids required")
	  }
	  ph := make([]string, len(keep))
	  args := []any{time.Now().UTC()}
	   for i, id := range keep {
		  ph[i] = "?"
		  args = append(args, id)
	  }
	  q := `UPDATE aa_earn_products SET deleted_at=?, active=0 WHERE deleted_at IS NULL AND id NOT IN (` + strings.Join(ph, ",") + `)`
	  res, err := r.db.Conn(ctx).ExecContext(ctx, q, args...)
	  if err != nil {
		  return 0, err
	   }
	   n, _ := res.RowsAffected()
	   return n, nil
  }

// UndeleteProduct 恢复软删产品（运营纠错）。
// 清 deleted_at 并 active=1；调用方须随后 Hydrate/Put 热表。
// id 不存在仍返回 nil（RowsAffected=0）；须 GetProductAny 校验。
   func (r *EarnStore) UndeleteProduct(ctx context.Context, id string) error {
	   if !sqlOn(r.db) {
		  return ErrUnavailable
	   }
	  id = strings.TrimSpace(id)
	   if id == "" {
		   return ErrNotFound
	   }
	  res, err := r.db.Conn(ctx).ExecContext(ctx,
		   `UPDATE aa_earn_products SET deleted_at=NULL, active=1, updated_at=? WHERE id=?`,
		  time.Now().UTC(), id)
	  if err != nil {
		   return err
	  }
	   n, _ := res.RowsAffected()
	   if n == 0 {
		   return ErrNotFound
	   }
	   return nil
   }

// GetProductAny 按 id 读产品（含软删行），供 undelete 装回。

   func (r *EarnStore) GetProductAny(ctx context.Context, id string) (EarnProduct, error) {
	  if !sqlOn(r.db) {
		  return EarnProduct{}, ErrUnavailable
	  }
	   var row EarnProduct
	  var active, auto int
	   err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT id, asset, category, daily_bps, lock_days, min_subscribe, max_position, early_bps, active, auto_subscribe, updated_at
 FROM aa_earn_products WHERE id=?`, strings.TrimSpace(id),
	  ).Scan(&row.ID, &row.Asset, &row.Category, &row.DailyBPS, &row.LockDays, &row.MinSubscribe, &row.MaxPosition,
		   &row.EarlyBPS, &active, &auto, &row.UpdatedAt)
	  if errors.Is(err, sql.ErrNoRows) {
		  return EarnProduct{}, ErrNotFound
	  }
	  if err != nil {
		   return EarnProduct{}, err
	  }
	   row.Active = active != 0
	  row.AutoSubscribe = auto != 0
	   return row, nil
  }

// ListDeletedProducts 列出软删产品（运营恢复面）。

   func (r *EarnStore) ListDeletedProducts(ctx context.Context) ([]EarnProduct, error) {
	  if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	  }
	  rows,
		   err := r.db.Conn(ctx).QueryContext(ctx,
		   `SELECT id, asset, category, daily_bps, lock_days, min_subscribe, max_position, early_bps, active, auto_subscribe, updated_at
 FROM aa_earn_products WHERE deleted_at IS NOT NULL ORDER BY id`)
	  if err != nil {
		   return nil, err
	   }
	   defer rows.Close()
	   out := make([]EarnProduct, 0)
	  for rows.Next() {
		   var row EarnProduct
		  var active, auto int
		   if err := rows.Scan(&row.ID,
			  &row.Asset, &row.Category, &row.DailyBPS, &row.LockDays, &row.MinSubscribe, &row.MaxPosition,
			  &row.EarlyBPS, &active, &auto, &row.UpdatedAt); err != nil {
			   return nil, err
		  }
		   row.Active = active != 0
		  row.AutoSubscribe = auto != 0
		   out = append(out, row)
	  }
	  return out, rows.Err()
   }

// EarnProductAudit 理财目录软删/恢复审计行。

type EarnProductAudit struct {
	ID int64
	ProductID string
	Action    string // soft_delete | undelete
	Actor     string
	Note string
	At time.Time
}

// PutProductAudit 写入软删/恢复审计。

   func (r *EarnStore) PutProductAudit(ctx context.Context, a EarnProductAudit) error {
	  if !sqlOn(r.db) {
		  return ErrUnavailable
	   }
	  a.ProductID = strings.TrimSpace(a.ProductID)
	   a.Action = strings.TrimSpace(a.Action)
	  a.Actor = strings.TrimSpace(a.Actor)
	  if a.ProductID == "" || a.Action == "" {
		   return fmt.Errorf("product_id and action required")
	  }
	   if a.At.IsZero() {
		   a.At = time.Now().UTC()
	  }
	  _, err := r.db.Conn(ctx).ExecContext(ctx,
		  `INSERT INTO aa_earn_product_audit (product_id, action, actor, note, at) VALUES (?,?,?,?,?)`,
		   a.ProductID, a.Action, a.Actor, a.Note, a.At)
	   return err
   }

// ListProductAudit 按产品（可空=全部）列出软删审计，新在前。

  func (r *EarnStore) ListProductAudit(ctx context.Context, productID string, limit int) ([]EarnProductAudit, error) {
	   if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	   }
	   if limit <= 0 || limit > 500 {
		  limit = 100
	   }
	  productID = strings.TrimSpace(productID)
	   var (
		  rows *sql.Rows
		   err error
	  )
	  if productID == "" {
		   rows, err = r.db.Conn(ctx).QueryContext(ctx,
			   `SELECT id, product_id, action, actor, note, at FROM aa_earn_product_audit ORDER BY at DESC, id DESC LIMIT ?`, limit)
	  } else {
		   rows, err = r.db.Conn(ctx).QueryContext(ctx,
			  `SELECT id, product_id, action, actor, note, at FROM aa_earn_product_audit WHERE product_id=? ORDER BY at DESC, id DESC LIMIT ?`,
			   productID, limit)
	  }
	   if err != nil {
		  return nil, err
	   }
	   defer rows.Close()
	   out := make([]EarnProductAudit, 0)
	   for rows.Next() {
		   var a EarnProductAudit
		   if err := rows.Scan(&a.ID, &a.ProductID, &a.Action, &a.Actor, &a.Note, &a.At); err != nil {
			   return nil, err
		  }
		   out = append(out, a)
	   }
	  return out, rows.Err()
   }

// SoftDeleteProductsExceptWithActor 软删 keep 外产品并写审计（谁删）。
// 先选出待删 id，再 UPDATE，再逐条 PutProductAudit。
// keep 空拒；审计失败仍保留已软删（调用方看返回错误补写）。
  func (r *EarnStore) SoftDeleteProductsExceptWithActor(ctx context.Context, keepIDs []string, actor, note string) (int64, error) {
	  if !sqlOn(r.db) {
		   return 0, ErrUnavailable
	  }
	   keep := make([]string, 0, len(keepIDs))
	  seen := map[string]struct{}{}
	   for _, id := range keepIDs {
		  id = strings.TrimSpace(id)
		  if id == "" {
			  continue
		  }
		  if _, ok := seen[id]; ok {
			  continue
		  }
		  seen[id] = struct{}{}
		   keep = append(keep, id)
	   }
	   if len(keep) == 0 {
		  return 0, fmt.Errorf("keep ids required")
	  }
	   ph := make([]string, len(keep))
	   args := make([]any, 0, len(keep))
	  for i, id := range keep {
		   ph[i] = "?"
		  args = append(args, id)
	   }
	  qSel := `SELECT id FROM aa_earn_products WHERE deleted_at IS NULL AND id NOT IN (` + strings.Join(ph, ",") + `)`
	   rows, err := r.db.Conn(ctx).QueryContext(ctx, qSel, args...)
	  if err != nil {
		   return 0, err
	   }
	   toDel := make([]string, 0)
	  for rows.Next() {
		  var id string
		  if err := rows.Scan(&id); err != nil {
			  rows.Close()
			  return 0, err
		  }
		   toDel = append(toDel, id)
	   }
	   rows.Close()
	  if err := rows.Err(); err != nil {
		   return 0, err
	  }
	   if len(toDel) == 0 {
		   return 0, nil
	  }
	  now := time.Now().UTC()
	  ph2 := make([]string, len(toDel))
	  args2 := []any{now}
	   for i, id := range toDel {
		   ph2[i] = "?"
		   args2 = append(args2, id)
	  }
	   res, err := r.db.Conn(ctx).ExecContext(ctx,
		  `UPDATE aa_earn_products SET deleted_at=?, active=0 WHERE deleted_at IS NULL AND id IN (`+strings.Join(ph2, ",")+`)`,
		   args2...)
	   if err != nil {
		   return 0, err
	  }
	  n, _ := res.RowsAffected()
	  actor = strings.TrimSpace(actor)
	   note = strings.TrimSpace(note)
	   for _, id := range toDel {
		   if err := r.PutProductAudit(ctx, EarnProductAudit{ProductID: id, Action: "soft_delete", Actor: actor, Note: note, At: now}); err != nil {
			   return n, err
		   }
	   }
	   return n, nil
  }

// UndeleteProductWithActor 恢复软删并写 undelete 审计。
   func (r *EarnStore) UndeleteProductWithActor(ctx context.Context, id, actor, note string) error {
	  if err := r.UndeleteProduct(ctx, id); err != nil {
		   return err
	  }
	  return r.PutProductAudit(ctx, EarnProductAudit{
		   ProductID: strings.TrimSpace(id),
		   Action:    "undelete",
		   Actor: strings.TrimSpace(actor),
		  Note:  strings.TrimSpace(note),
		  At:    time.Now().UTC(),
	   })
   }

// ListProductAuditWindow 按时间窗列出审计，新在前。
// at 闭开区间 [start, end]；limit 硬顶 5000（CSV 流用）。

   func (r *EarnStore) ListProductAuditWindow(ctx context.Context, productID string, start, end time.Time, limit int) ([]EarnProductAudit, error) {
	   if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	  }
	   if limit <= 0 || limit > 5000 {
		  limit = 1000
	   }
	   productID = strings.TrimSpace(productID)
	  var (
		   rows *sql.Rows
		   err  error
	  )
	  if productID == "" {
		   rows, err = r.db.Conn(ctx).QueryContext(ctx,
			   `SELECT id, product_id, action, actor, note, at FROM aa_earn_product_audit
 WHERE at>=? AND at<=? ORDER BY at DESC, id DESC LIMIT ?`, start, end, limit)
	  } else {
		  rows, err = r.db.Conn(ctx).QueryContext(ctx,
			   `SELECT id, product_id, action, actor, note, at FROM aa_earn_product_audit
 WHERE product_id=? AND at>=? AND at<=? ORDER BY at DESC, id DESC LIMIT ?`,
			   productID, start, end, limit)
	  }
	  if err != nil {
		  return nil, err
	   }
	  defer rows.Close()
	   out := make([]EarnProductAudit, 0)
	  for rows.Next() {
		   var a EarnProductAudit
		   if err := rows.Scan(&a.ID, &a.ProductID, &a.Action, &a.Actor, &a.Note, &a.At); err != nil {
			  return nil, err
		  }
		  out = append(out, a)
	   }
	   return out, rows.Err()
   }

// PurgeProductAuditBefore 删除 at < cutoff 的审计行。

  func (r *EarnStore) PurgeProductAuditBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	  if !sqlOn(r.db) {
		  return 0, ErrUnavailable
	  }
	   res, err := r.db.Conn(ctx).ExecContext(ctx,
		  `DELETE FROM aa_earn_product_audit WHERE at<?`, cutoff)
	  if err != nil {
		   return 0, err
	  }
	   return res.RowsAffected()
   }

// CountProductAudit 审计总行数（ops 健康）。
   func (r *EarnStore) CountProductAudit(ctx context.Context) (int, error) {
	  if !sqlOn(r.db) {
		  return 0, ErrUnavailable
	   }
	  var n int
	  err := r.db.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM aa_earn_product_audit`).Scan(&n)
	  return n, err
  }

// OldestProductAuditAt 最旧审计时间（保留窗年龄指标）。
   func (r *EarnStore) OldestProductAuditAt(ctx context.Context) (time.Time, error) {
	  if !sqlOn(r.db) {
		   return time.Time{}, ErrUnavailable
	  }
	  var t sql.NullTime
	   err := r.db.Conn(ctx).QueryRowContext(ctx, `SELECT MIN(at) FROM aa_earn_product_audit`).Scan(&t)
	  if err != nil {
		  return time.Time{}, err
	   }
	  if !t.Valid {
		   return time.Time{}, nil
	   }
	   return t.Time.UTC(), nil
  }

// EarnProductAuditCold 理财审计冷存清单行。
type EarnProductAuditCold struct {
	ID        int64  `json:"id"`
	ArchiveID string `json:"archive_id"`
	ProductID string `json:"product_id"`
	Path      string `json:"path"`
	RowCount  int    `json:"row_count"`
	StartAt time.Time `json:"start_at"`
	EndAt   time.Time `json:"end_at"`
	SHA256 string `json:"sha256"`
	Actor string    `json:"actor"`
	At    time.Time `json:"at"`
}

// PutProductAuditCold 写入冷存清单。
  func (r *EarnStore) PutProductAuditCold(ctx context.Context, row EarnProductAuditCold) error {
	   if !sqlOn(r.db) {
		   return ErrUnavailable
	   }
	  if row.At.IsZero() {
		  row.At = time.Now().UTC()
	  }
	  _, err := r.db.Conn(ctx).ExecContext(ctx,
		  `INSERT INTO aa_earn_product_audit_cold
 (archive_id, product_id, path, row_count, start_at, end_at, sha256, actor, at)
 VALUES (?,?,?,?,?,?,?,?,?)`,
		  strings.TrimSpace(row.ArchiveID), strings.TrimSpace(row.ProductID), strings.TrimSpace(row.Path),
		  row.RowCount, row.StartAt.UTC(), row.EndAt.UTC(), strings.TrimSpace(row.SHA256),
		  strings.TrimSpace(row.Actor), row.At.UTC())
	  return err
  }

// ListProductAuditCold 冷存清单，新在前。
   func (r *EarnStore) ListProductAuditCold(ctx context.Context, productID string, limit int) ([]EarnProductAuditCold, error) {
	   if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	  }
	  if limit <= 0 || limit > 500 {
		   limit = 100
	   }
	   productID = strings.TrimSpace(productID)
	  var (
		  rows *sql.Rows
		   err error
	   )
	   if productID == "" {
		   rows,
			  err = r.db.Conn(ctx).QueryContext(ctx,
			  `SELECT id, archive_id, product_id, path, row_count, start_at, end_at, sha256, actor, at 
 FROM aa_earn_product_audit_cold ORDER BY at DESC, id DESC LIMIT ?`, limit)
	  } else {
		  rows, err = r.db.Conn(ctx).QueryContext(ctx,
			   `SELECT id, archive_id, product_id, path, row_count, start_at, end_at, sha256, actor, at
 FROM aa_earn_product_audit_cold WHERE product_id=? ORDER BY at DESC, id DESC LIMIT ?`,
			  productID, limit)
	  }
	  if err != nil {
		   return nil, err
	  }
	  defer rows.Close()
	  out := make([]EarnProductAuditCold,
		   0)
	   for rows.Next() {
		   var a EarnProductAuditCold
		   if err := rows.Scan(&a.ID, &a.ArchiveID, &a.ProductID, &a.Path, &a.RowCount,
			   &a.StartAt, &a.EndAt, &a.SHA256, &a.Actor, &a.At); err != nil {
			  return nil, err
		  }
		   out = append(out, a)
	   }
	   return out, rows.Err()
  }

// CountProductAuditCold 冷存清单行数。
   func (r *EarnStore) CountProductAuditCold(ctx context.Context) (int, error) {
	  if !sqlOn(r.db) {
		   return 0, ErrUnavailable
	   }
	   var n int
	   err := r.db.Conn(ctx).QueryRowContext(ctx, `SELECT COUNT(*) FROM aa_earn_product_audit_cold`).Scan(&n)
	   return n, err
  }

// DistinctProductIDsInAuditWindow 窗内有审计的产品分片键。
   func (r *EarnStore) DistinctProductIDsInAuditWindow(ctx context.Context, start, end time.Time) ([]string, error) {
	   if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	   }
	  rows, err := r.db.Conn(ctx).QueryContext(ctx,
		  `SELECT DISTINCT product_id FROM aa_earn_product_audit WHERE at>=? AND at<=? ORDER BY product_id`,
		   start, end)
	  if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	  out := make([]string, 0)
	   for rows.Next() {
		  var id string
		  if err := rows.Scan(&id); err != nil {
			  return nil, err
		   }
		   id = strings.TrimSpace(id)
		  if id != "" {
			  out = append(out, id)
		  }
	  }
	   return out, rows.Err()
  }
