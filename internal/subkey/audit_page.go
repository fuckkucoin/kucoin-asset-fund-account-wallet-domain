package subkey

import (
	"strings"
	"sync"
	"time"
)

// AuditRow 单条审计记录。

type AuditRow struct {
	ID      string `json:"id"`
	Owner   string `json:"owner"`
	SubName string `json:"subName"`
	APIKey    string      `json:"apiKey"`
	Action    AuditAction `json:"action"`
	Detail    string      `json:"detail"`
	ClientIP  string      `json:"clientIp,omitempty"`
	CreatedAt int64       `json:"createdAt"`
}

// AuditQuery 分页查询。空 Owner 不返回任何行。

type AuditQuery struct {
	Owner   string
	SubName string
	APIKey  string
	Action  string
	Page     int
	PageSize int
}

// AuditPage 分页结果。

type AuditPage struct {
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
	Items []AuditRow `json:"items"`
}

type auditBuf struct {
	mu   sync.Mutex
	rows []AuditRow
	max  int
}

  func newAuditBuf() *auditBuf {
	   return &auditBuf{rows: make([]AuditRow,
		  0, 64), max: 5000}
  }

   func (s *Store) appendAuditPage(ev AuditEvent) {
	   if s == nil {
		  return
	   }
	   if s.audit == nil {
		  s.audit = newAuditBuf()
	  }
	   id, err := randHex(8)
	   if err != nil {
		   id = time.Now().UTC().Format("20060102150405")
	   }
	   row := AuditRow{
		  ID: "aud-" + id, Owner: ev.Owner, SubName: ev.SubName, APIKey: ev.APIKey,
		   Action: AuditAction(ev.Action), Detail: ev.Detail, ClientIP: ev.ActorIP, CreatedAt: ev.At,
	  }
	   s.audit.mu.Lock()
	  s.audit.rows = append(s.audit.rows, row)
	   if len(s.audit.rows) > s.audit.max {
		  s.audit.rows = s.audit.rows[len(s.audit.rows)-s.audit.max:]
	  }
	  s.audit.mu.Unlock()
  }

// AppendAuditRow SQL 灌入用。

  func (s *Store) AppendAuditRow(row AuditRow) {
	  if s == nil || row.ID == "" {
		   return
	  }
	  if s.audit == nil {
		  s.audit = newAuditBuf()
	   }
	   s.audit.mu.Lock()
	  s.audit.rows = append(s.audit.rows, row)
	  s.audit.mu.Unlock()
  }

// ListAuditPage 只读分页。Owner 为空时返回空页，不跨户列出。
   func (s *Store) ListAuditPage(q AuditQuery) AuditPage {
	   if q.Page <= 0 {
		  q.Page = 1
	  }
	  if q.PageSize <= 0 || q.PageSize > 100 {
		  q.PageSize = 50
	  }
	   empty := AuditPage{Page: q.Page, PageSize: q.PageSize, Items: []AuditRow{}}
	  if s == nil || s.audit == nil || strings.TrimSpace(q.Owner) == "" {
		  return empty
	  }
	   s.audit.mu.Lock()
	  filtered := make([]AuditRow, 0)
	   for i := len(s.audit.rows) - 1; i >= 0; i-- {
		  r := s.audit.rows[i]
		   if r.Owner != q.Owner {
			  continue
		   }
		   if q.SubName != "" && r.SubName != q.SubName {
			   continue
		   }
		  if q.APIKey != "" && r.APIKey != q.APIKey {
			   continue
		   }
		  if q.Action != "" && string(r.Action) != q.Action {
			  continue
		  }
		  filtered = append(filtered, r)
	   }
	   s.audit.mu.Unlock()
	  total := len(filtered)
	   start := (q.Page - 1) * q.PageSize
	  if start >= total {
		   return AuditPage{Total: total, Page: q.Page, PageSize: q.PageSize, Items: []AuditRow{}}
	  }
	   end := start + q.PageSize
	   if end > total {
		  end = total
	   }
	   return AuditPage{Total: total, Page: q.Page, PageSize: q.PageSize, Items: filtered[start:end]}
  }
