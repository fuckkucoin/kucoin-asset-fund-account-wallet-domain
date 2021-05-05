package earn

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
)

// ProductAuditRetention 理财目录软删审计保留窗（约一年，一致充提历史）。

const ProductAuditRetention = 365 * 24 * time.Hour

// ProductAuditExportWindow 审计 CSV 单次导出窗（一致充提 100 天）。
const ProductAuditExportWindow = 100 * 24 * time.Hour

// ErrProductAuditWindow 超导出/保留窗。

var ErrProductAuditWindow = fmt.Errorf("earn product audit window exceeds limit")

// ListProductAuditWindow 按时间窗列出审计；超保留窗拒。
// 缺省近 100 天导出窗；保留硬顶 365 天。
// 超窗不得绕过；无 SQL 拒。

   func (c *Catalog) ListProductAuditWindow(ctx context.Context, productID string, start, end time.Time, limit int) ([]sqlrepo.EarnProductAudit, error) {
	  if c == nil || !c.sqlOn() {
		  return nil, ErrUnavailable
	   }
	   if ctx == nil {
		   ctx = context.Background()
	  }
	   now := time.Now().UTC()
	  if end.IsZero() {
		  end = now
	  }
	  if start.IsZero() {
		   start = end.Add(-ProductAuditExportWindow)
	   }
	   if end.Before(start) {
		  return nil, fmt.Errorf("%w: end before start", ErrProductAuditWindow)
	  }
	   if end.Sub(start) > ProductAuditExportWindow {
		   return nil, fmt.Errorf("%w: export max %s", ErrProductAuditWindow, ProductAuditExportWindow)
	  }
	  retStart := now.Add(-ProductAuditRetention)
	  if start.Before(retStart) {
		   return nil, fmt.Errorf("%w: retention %s", ErrProductAuditWindow, ProductAuditRetention)
	  }
	  return sqlrepo.NewEarn(c.sql).ListProductAuditWindow(ctx, productID, start, end, limit)
   }

// PurgeProductAudit 清扫超保留窗审计行；运营对账后调用。
// 只删 at < now-retention；返回删除行数。
// 不可恢复；须先 CSV 导出。
   func (c *Catalog) PurgeProductAudit(ctx context.Context, olderThan time.Duration) (int64, error) {
	   if c == nil || !c.sqlOn() {
		   return 0, ErrUnavailable
	   }
	   if ctx == nil {
		  ctx = context.Background()
	   }
	   if olderThan <= 0 {
		  olderThan = ProductAuditRetention
	  }
	   if olderThan < ProductAuditRetention {
		   // 不得短于法定保留窗误删。
		   olderThan = ProductAuditRetention
	  }
	  cutoff := time.Now().UTC().Add(-olderThan)
	   n, err := sqlrepo.NewEarn(c.sql).PurgeProductAuditBefore(ctx, cutoff)
	   if err != nil {
		  return 0, err
	   }
	  metrics.EarnProductAudit.WithLabelValues("purge").Add(float64(n))
	   return n, nil
  }

// WriteProductAuditCSV 审计流水 CSV 写出。

   func WriteProductAuditCSV(w io.Writer, rows []sqlrepo.EarnProductAudit) error {
	   cw := csv.NewWriter(w)
	  if err := cw.Write([]string{"id", "product_id", "action", "actor", "note", "at"}); err != nil {
		   return err
	   }
	  for _, a := range rows {
		  if err := cw.Write([]string{
			  fmt.Sprintf("%d", a.ID),
			  a.ProductID,
			  a.Action,
			   a.Actor,
			   a.Note,
			   a.At.UTC().Format(time.RFC3339Nano),
		   }); err != nil {
			   return err
		  }
	   }
	  cw.Flush()
	   return cw.Error()
  }

// EmitProductAuditMetrics 推送审计计数/年龄/写失败到 Prometheus。
  func EmitProductAuditMetrics(ctx context.Context, c *Catalog) {
	   if c == nil || !c.sqlOn() {
		  return
	  }
	   if ctx == nil {
		   ctx = context.Background()
	  }
	  store := sqlrepo.NewEarn(c.sql)
	  n, err := store.CountProductAudit(ctx)
	  if err != nil {
		   metrics.EarnProductAuditWriteErr.Set(1)
		  return
	   }
	   metrics.EarnProductAuditWriteErr.Set(0)
	   metrics.EarnProductAuditRows.Set(float64(n))
	  oldest, err := store.OldestProductAuditAt(ctx)
	  if err != nil || oldest.IsZero() {
		   metrics.EarnProductAuditMaxAge.Set(0)
		   return
	   }
	   age := time.Now().UTC().Sub(oldest).Seconds()
	  if age < 0 {
		  age = 0
	   }
	  metrics.EarnProductAuditMaxAge.Set(age)
	  if nCold, err := store.CountProductAuditCold(ctx); err == nil {
		  metrics.EarnProductAuditColdRows.Set(float64(nCold))
	  }
   }

// RecordProductAuditAction 审计动作计数（soft_delete/undelete/export）。

   func RecordProductAuditAction(action string) {
	  action = strings.TrimSpace(action)
	  if action == "" {
		  action = "unknown"
	   }
	   metrics.EarnProductAudit.WithLabelValues(action).Inc()
   }
