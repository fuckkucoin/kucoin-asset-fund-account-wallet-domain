package earn

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"os"
	"path/filepath"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/sqlrepo"
	"github.com/google/uuid"
)

// DefaultColdArchiveDir 理财审计冷存根目录（相对工作目录）。

const DefaultColdArchiveDir = "data/earn-audit-cold"

// ColdArchiveResult 单产品分片冷存回执。
type ColdArchiveResult struct {
	Manifest sqlrepo.EarnProductAuditCold `json:"manifest"`
	Rows     int                          `json:"rows"`
}

// ResolveColdArchiveDir 解析冷存目录；空则默认；环境 EARN_AUDIT_COLD_DIR 先。

   func ResolveColdArchiveDir(dir string) string {
	   if v := strings.TrimSpace(os.Getenv("EARN_AUDIT_COLD_DIR")); v != "" {
		  return v
	   }
	  dir = strings.TrimSpace(dir)
	   if dir == "" {
		   return DefaultColdArchiveDir
	  }
	  return dir
   }

// ColdArchiveProductAudit 按产品分片把审计窗写冷存 CSV，并记 SQL 清单。
// 超越 SQL 保留窗+purge——先落盘再可选 purge；分片键 product_id。
// 落盘失败不得 purge；SHA256 后写清单，清单写失败须人工对账磁盘。
   func (c *Catalog) ColdArchiveProductAudit(ctx context.Context, dir, productID, actor string, start, end time.Time, purgeAfter bool) ([]ColdArchiveResult, error) {
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
		   // 冷存默认抓保留窗外沿到现在前 365d 全窗，但单次仍受导出窗限制时按分片翻页。
		   start = end.Add(-ProductAuditRetention)
	  }
	   if end.Before(start) {
		  return nil, fmt.Errorf("%w: end before start", ErrProductAuditWindow)
	   }
	  dir = ResolveColdArchiveDir(dir)
	   if err := os.MkdirAll(dir, 0o750); err != nil {
		  return nil, err
	  }
	  store := sqlrepo.NewEarn(c.sql)
	   productID = strings.TrimSpace(productID)
	   var products []string
	   var err error
	  if productID != "" {
		  products = []string{productID}
	  } else {
		  products, err = store.DistinctProductIDsInAuditWindow(ctx, start, end)
		   if err != nil {
			  return nil, err
		   }
	   }
	  out := make([]ColdArchiveResult, 0, len(products))
	   actor = strings.TrimSpace(actor)
	   for _, pid := range products {
		   res, err := c.coldArchiveOne(ctx, store, dir, pid, actor, start, end)
		  if err != nil {
			  return out, err
		   }
		  if res.Rows == 0 {
			  continue
		   }
		   out = append(out, res)
		  RecordProductAuditAction("cold_archive")
	  }
	   if purgeAfter && len(out) > 0 {
		   // 仅清扫超保留窗；冷存已覆盖更旧分片。
		   if _, err := c.PurgeProductAudit(ctx, ProductAuditRetention); err != nil {
			   return out, fmt.Errorf("cold archive ok but purge: %w", err)
		   }
		   RecordProductAuditAction("purge_after_archive")
	   }
	   EmitProductAuditMetrics(ctx, c)
	   metrics.EarnProductAuditColdArchives.Set(float64(len(out)))
	  return out, nil
  }

   func (c *Catalog) coldArchiveOne(ctx context.Context, store *sqlrepo.EarnStore, dir, productID, actor string, start, end time.Time) (ColdArchiveResult, error) {
	   var empty ColdArchiveResult
	  // 分片内按 100d 导出窗翻页拼全保留窗。
	  all := make([]sqlrepo.EarnProductAudit,
		   0)
	  cursorEnd := end
	  for cursorEnd.After(start) {
		  winStart := cursorEnd.Add(-ProductAuditExportWindow)
		  if winStart.Before(start) {
			   winStart = start
		  }
		   chunk, err := store.ListProductAuditWindow(ctx, productID, winStart, cursorEnd, 5000)
		  if err != nil {
			  return empty, err
		   }
		  all = append(all, chunk...)
		  if !winStart.After(start) {
			  break
		   }
		  cursorEnd = winStart.Add(-time.Nanosecond)
	  }
	  if len(all) == 0 {
		  return empty, nil
	  }
	  archiveID := uuid.NewString()
	  safePID := sanitizeProductID(productID)
	   fname := fmt.Sprintf("%s_%s_%s.csv", safePID, start.UTC().Format("20060102"), end.UTC().Format("20060102T150405"))
	   path := filepath.Join(dir, fname)
	   f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o640)
	  if err != nil {
		  return empty, err
	  }
	  h := sha256.New()
	  w := &hashWriter{f: f, h: h}
	   if err := WriteProductAuditCSV(w, all); err != nil {
		   _ = f.Close()
		   _ = os.Remove(path)
		   return empty, err
	  }
	  if err := f.Close(); err != nil {
		  _ = os.Remove(path)
		  return empty, err
	  }
	  sum := hex.EncodeToString(h.Sum(nil))
	  man := sqlrepo.EarnProductAuditCold{
		   ArchiveID: archiveID,
		   ProductID: productID,
		   Path:      path,
		   RowCount:  len(all),
		   StartAt:   start.UTC(),
		  EndAt: end.UTC(),
		   SHA256: sum,
		   Actor:  actor,
		   At:     time.Now().UTC(),
	  }
	  if err := store.PutProductAuditCold(ctx, man); err != nil {
		   return ColdArchiveResult{Manifest: man, Rows: len(all)}, fmt.Errorf("disk ok manifest: %w", err)
	  }
	   return ColdArchiveResult{Manifest: man, Rows: len(all)}, nil
   }

type hashWriter struct {
	f *os.File
	h hash.Hash
}

  func (w *hashWriter) Write(p []byte) (int, error) {
	   n, err := w.f.Write(p)
	   if n > 0 {
		   _, _ = w.h.Write(p[:n])
	   }
	  return n, err
   }

   func sanitizeProductID(id string) string {
	  id = strings.TrimSpace(id)
	  if id == "" {
		   return "_all"
	   }
	  var b strings.Builder
	   for _, r := range id {
		   switch {
		  case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			   b.WriteRune(r)
		  default:
			  b.WriteByte('_')
		  }
	   }
	   out := b.String()
	   if out == "" {
		  return "_x"
	   }
	   if len(out) > 64 {
		  out = out[:64]
	  }
	  return out
  }

// ListColdArchives 列出冷存清单。
  func (c *Catalog) ListColdArchives(ctx context.Context, productID string, limit int) ([]sqlrepo.EarnProductAuditCold, error) {
	   if c == nil || !c.sqlOn() {
		   return nil, ErrUnavailable
	   }
	  if ctx == nil {
		   ctx = context.Background()
	  }
	  return sqlrepo.NewEarn(c.sql).ListProductAuditCold(ctx, productID, limit)
   }
