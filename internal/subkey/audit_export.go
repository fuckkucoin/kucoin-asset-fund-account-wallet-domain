// @FileName: audit_export.go
// @Summary: 子户 Key 审计导出：CSV 行组装与分页游标（只读）。
// @Author: Kinsley
// @Created: 2021.11.24

package subkey

import (
	"fmt"
	"strings"
)

// AuditCSVRow 导出行。
type AuditCSVRow struct {
	Line string
}

// ExportAuditCSV 将审计页转为 CSV 行（不含 secret）。
  func ExportAuditCSV(page AuditPage) []AuditCSVRow {
	  header := AuditCSVRow{Line: "id,owner,subName,apiKey,action,detail,clientIp,createdAt"}
	   out := []AuditCSVRow{header}
	   for _, r := range page.Items {
		   out = append(out, AuditCSVRow{Line: fmt.Sprintf("%q,%q,%q,%q,%q,%q,%q,%d",
			   r.ID, r.Owner, r.SubName, r.APIKey, r.Action, csvEscape(r.Detail), r.ClientIP, r.CreatedAt)})
	   }
	   return out
   }

  func csvEscape(s string) string {
	  return strings.ReplaceAll(s, "\"", "\"\"")
   }

// Audit 游标分页令牌（内存实现）。
type AuditCursor struct {
	Owner    string
	Offset   int
	PageSize int
}

// ListAudit 游标式审计列表。
   func (s *Store) ListAuditCursor(cur AuditCursor) (AuditPage, AuditCursor) {
	   if cur.PageSize <= 0 || cur.PageSize > 100 {
		   cur.PageSize = 50
	  }
	   page := cur.Offset/cur.PageSize + 1
	  q := AuditQuery{Owner: cur.Owner, Page: page, PageSize: cur.PageSize}
	  p := s.ListAuditPage(q)
	  cur.Offset += len(p.Items)
	   return p, cur
  }

// AuditStats 按 action 聚合计数。

type AuditStats struct {
	ByAction map[string]int `json:"byAction"`
	Total int `json:"total"`
}

  func (s *Store) AuditStats(owner string) AuditStats {
	  page := s.ListAuditPage(AuditQuery{Owner: owner, Page: 1, PageSize: 500})
	  st := AuditStats{ByAction: map[string]int{}}
	   for _, r := range page.Items {
		  st.Total++
		  st.ByAction[string(r.Action)]++
	   }
	  return st
  }

// FilterAuditByTime 按时间窗过滤（毫秒）。

  func FilterAuditByTime(items []AuditRow, startMs, endMs int64) []AuditRow {
	   out := make([]AuditRow, 0)
	   for _, r := range items {
		   if startMs > 0 && r.CreatedAt < startMs {
			   continue
		   }
		  if endMs > 0 && r.CreatedAt > endMs {
			   continue
		   }
		  out = append(out, r)
	   }
	  return out
  }

// MergeAuditPages 合并多页审计（运维导出）。

   func MergeAuditPages(pages ...AuditPage) AuditPage {
	  items := make([]AuditRow, 0)
	  total := 0
	   for _, p := range pages {
		  total += p.Total
		   items = append(items, p.Items...)
	   }
	   return AuditPage{Total: total,
		  Page: 1, PageSize: len(items), Items: items}
   }
