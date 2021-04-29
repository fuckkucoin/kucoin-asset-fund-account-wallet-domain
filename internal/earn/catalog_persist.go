package earn

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

// BindSQL 挂 SQL 作为产品目录权威源；OpenMem 不绑。
// Open 后须 Hydrate；运营 PUT 先写库再热更新。
// 未绑定时仅进程内 DefaultCatalog，重启丢失调参。

   func (c *Catalog) BindSQL(db *persist.SQLDB) *Catalog {
	   if c != nil {
		   c.sql = db
	  }
	   return c
  }

  func (c *Catalog) sqlOn() bool {
	   return c != nil && c.sql != nil && c.sql.DB != nil
   }

// Hydrate 从 SQL 装回产品目录（含 EarlyBPS）。
// 有行则覆盖默认目录；无行保留 DefaultCatalog。
// Hydrate 失败不得空目录放行申购。
   func (c *Catalog) Hydrate(ctx context.Context) error {
	  if c == nil || !c.sqlOn() {
		  return nil
	  }
	  if ctx == nil {
		   ctx = context.Background()
	   }
	  list, err := sqlrepo.NewEarn(c.sql).ListProducts(ctx)
	  c.mu.Lock()
	   defer c.mu.Unlock()
	  if err != nil {
		   c.hydrateErr = err.Error()
		  return err
	  }
	   if len(list) == 0 {
		  c.hydratedN = 0
		  c.hydratedAt = time.Now().UTC()
		   c.hydrateErr = ""
		  return nil
	   }
	  next := make(map[string]Product, len(list))
	   for _, row := range list {
		   next[row.ID] = fromSQLProd(row)
	  }
	  c.rows = next
	   c.hydratedN = len(list)
	   c.hydratedAt = time.Now().UTC()
	  c.hydrateErr = ""
	  return nil
   }

// PersistProduct 单产品写 SQL 并热更新内存。
  func (c *Catalog) PersistProduct(ctx context.Context, p Product) error {
	  if c == nil {
		   return ErrUnavailable
	   }
	   if err := c.Put(p); err != nil {
		  return err
	   }
	   if !c.sqlOn() {
		   return nil
	   }
	  if ctx == nil {
		   ctx = context.Background()
	  }
	  got, _ := c.Get(p.ID)
	  return sqlrepo.NewEarn(c.sql).PutProduct(ctx, toSQLProd(got))
   }

// PersistAll 把当前内存目录全量写回 SQL。
  func (c *Catalog) PersistAll(ctx context.Context) error {
	   if c == nil || !c.sqlOn() {
		   return nil
	   }
	  if ctx == nil {
		  ctx = context.Background()
	  }
	  store := sqlrepo.NewEarn(c.sql)
	  for _, p := range c.List(false) {
		  if err := store.PutProduct(ctx, toSQLProd(p)); err != nil {
			  return err
		   }
	   }
	   return nil
  }

// PersistReplace 替换热表后写库，并软删库内多余产品行。
// 先 Put 全量 keep，再 SoftDeleteProductsExcept；最后 Hydrate 确认。
// keep 空拒；软删行 Hydrate 不再装回，不得靠全量 upsert 残留复活。

  func (c *Catalog) PersistReplace(ctx context.Context, products []Product, actor string) (softDeleted int64, err error) {
	   if c == nil {
		   return 0, ErrUnavailable
	  }
	  if len(products) == 0 {
		   return 0, fmt.Errorf("%w: replace empty", ErrProduct)
	  }
	   if ctx == nil {
		  ctx = context.Background()
	   }
	   c.mu.Lock()
	   c.rows = map[string]Product{}
	   c.mu.Unlock()
	   keep := make([]string,
		  0, len(products))
	   for _, p := range products {
		   if err := c.Put(p); err != nil {
			   return 0, err
		   }
		   keep = append(keep,
			  strings.TrimSpace(p.ID))
	   }
	   if !c.sqlOn() {
		  return 0, nil
	  }
	   store := sqlrepo.NewEarn(c.sql)
	   for _, p := range c.List(false) {
		  if err := store.PutProduct(ctx, toSQLProd(p)); err != nil {
			  return 0, err
		  }
	  }
	  n, err := store.SoftDeleteProductsExceptWithActor(ctx, keep, actor, "catalog_replace")
	   if err != nil {
		   // 兼容：无审计表旧库回退仅软删。
		   if n2, err2 := store.SoftDeleteProductsExcept(ctx, keep); err2 == nil {
			   n, err = n2, nil
		  } else {
			   return 0, err
		   }
	   }
	   if err != nil {
		  return 0, err
	  }
	   if n > 0 {
		  RecordProductAuditAction("soft_delete")
	   }
	  if err := c.Hydrate(ctx); err != nil {
		   return n, err
	   }
	  return n, nil
   }

// SetEarlyBPS 分产品热更新提前赎回 BPS；先写 SQL 再热表。
// 运营调参即时作用于 RedeemEarly；校验 EarlyBPS 范围。
// 产品不存在拒；损坏 BPS 不得绕过罚金闸。
  func (c *Catalog) SetEarlyBPS(ctx context.Context, productID string, earlyBPS int) (Product, error) {
	  if c == nil {
		  return Product{}, ErrUnavailable
	  }
	  productID = strings.TrimSpace(productID)
	   if productID == "" {
		   return Product{}, fmt.Errorf("%w: id", ErrProduct)
	   }
	  if earlyBPS < 0 || earlyBPS > 5000 {
		   return Product{}, fmt.Errorf("%w: early_bps", ErrProduct)
	  }
	   p, err := c.Get(productID)
	   if err != nil {
		  return Product{}, err
	  }
	  p.EarlyBPS = earlyBPS
	  if err := c.PersistProduct(ctx, p); err != nil {
		   return Product{}, err
	  }
	   return p, nil
  }

// Health 目录 Hydrate 健康面。

   func (c *Catalog) Health() map[string]any {
	   if c == nil {
		   return map[string]any{"sql": false}
	  }
	   c.mu.Lock()
	  defer c.mu.Unlock()
	   out := map[string]any{
		  "sql": c.sqlOn(),
		   "hydrated_n":  c.hydratedN,
		  "hydrate_err": c.hydrateErr,
		  "mem_n":       len(c.rows),
	  }
	   if !c.hydratedAt.IsZero() {
		   out["hydrated_at"] = c.hydratedAt.UTC().Format(time.RFC3339Nano)
	  }
	  return out
   }

  func toSQLProd(p Product) sqlrepo.EarnProduct {
	   return sqlrepo.EarnProduct{
		   ID: p.ID, Asset: p.Asset, Category: p.Category, DailyBPS: p.DailyBPS, LockDays: p.LockDays,
		   MinSubscribe: p.MinSubscribe, MaxPosition: p.MaxPosition, EarlyBPS: p.EarlyBPS,
		  Active: p.Active, AutoSubscribe: p.AutoSubscribe, UpdatedAt: time.Now().UTC(),
	   }
   }

   func fromSQLProd(row sqlrepo.EarnProduct) Product {
	   return Product{
		  ID: row.ID, Asset: row.Asset, Category: row.Category, DailyBPS: row.DailyBPS, LockDays: row.LockDays,
		   MinSubscribe: row.MinSubscribe, MaxPosition: row.MaxPosition, EarlyBPS: row.EarlyBPS,
		   Active: row.Active, AutoSubscribe: row.AutoSubscribe,
	  }
   }

// UndeleteProduct 运营恢复软删产品；清 deleted_at 后装回热表。
// 须 SQL；先 GetProductAny 确认存在，再 Undelete，再 Put。
// 无 SQL 拒；恢复后 Active=true 可立即申购。

  func (c *Catalog) UndeleteProduct(ctx context.Context, id, actor string) (Product, error) {
	   if c == nil {
		   return Product{}, ErrUnavailable
	  }
	   if !c.sqlOn() {
		  return Product{}, ErrUnavailable
	  }
	   if ctx == nil {
		   ctx = context.Background()
	   }
	  id = strings.TrimSpace(id)
	   if id == "" {
		  return Product{}, fmt.Errorf("%w: id", ErrProduct)
	   }
	  store := sqlrepo.NewEarn(c.sql)
	   row, err := store.GetProductAny(ctx, id)
	   if err != nil {
		  return Product{}, err
	   }
	   if err := store.UndeleteProductWithActor(ctx, id, actor, "ops_undelete"); err != nil {
		   if err2 := store.UndeleteProduct(ctx, id); err2 != nil {
			  return Product{}, err
		  }
	   }
	  p := fromSQLProd(row)
	   p.Active = true
	  if err := c.Put(p); err != nil {
		   return Product{}, err
	   }
	  RecordProductAuditAction("undelete")
	  return p, nil
   }

// ListDeletedProducts 运营面列出软删产品。

  func (c *Catalog) ListDeletedProducts(ctx context.Context) ([]Product, error) {
	   if c == nil || !c.sqlOn() {
		  return nil, ErrUnavailable
	   }
	  if ctx == nil {
		  ctx = context.Background()
	   }
	  rows, err := sqlrepo.NewEarn(c.sql).ListDeletedProducts(ctx)
	   if err != nil {
		   return nil, err
	   }
	   out := make([]Product, 0, len(rows))
	   for _, row := range rows {
		   out = append(out, fromSQLProd(row))
	  }
	  return out, nil
  }

// ListProductAudit 运营查询软删/恢复审计流水。

  func (c *Catalog) ListProductAudit(ctx context.Context, productID string, limit int) ([]sqlrepo.EarnProductAudit, error) {
	  if c == nil || !c.sqlOn() {
		   return nil, ErrUnavailable
	   }
	   if ctx == nil {
		  ctx = context.Background()
	  }
	  return sqlrepo.NewEarn(c.sql).ListProductAudit(ctx,
		  productID, limit)
  }
