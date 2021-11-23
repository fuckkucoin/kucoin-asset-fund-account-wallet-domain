package recon

import (
	"strings"
	"time"
)

// TicketFilter 工单列表过滤；空字段表示不过滤。

type TicketFilter struct {
	UserID string
	State    State
	Assignee string
	Breached *bool
	Limit    int
	Offset   int
}

// PageViews 分页读面结果。
type PageViews struct {
	Items []TicketView `json:"items"`
	Total int `json:"total"`
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

// StatusAggregate 工单状态聚合。

type StatusAggregate struct {
	Open  int `json:"open"`
	Acked int `json:"acked"`
	Resolved int `json:"resolved"`
	Rejected int `json:"rejected"`
	Breached int `json:"breached"`
	Total    int `json:"total"`
}

// ListViewsFiltered 按状态/受理人/是否违约过滤并分页。
// 运营台列表筛选。先全量视图再过滤，保证 SLA 排序稳定；offset 超出返回空页。内存簿很大时全量扫描成本升高，须靠 SQL 侧索引承接。

  func (w *Workflow) ListViewsFiltered(f TicketFilter, now time.Time) PageViews {
	  if now.IsZero() {
		  now = time.Now().UTC()
	   }
	  limit := f.Limit
	  if limit <= 0 {
		   limit = 50
	   }
	   if limit > 200 {
		   limit = 200
	  }
	  offset := f.Offset
	   if offset < 0 {
		  offset = 0
	   }
	   views := w.ListViews(f.UserID, 500, now)
	   filtered := make([]TicketView,
		  0, len(views))
	  for _, v := range views {
		  if f.State != "" && v.Ticket.State != f.State {
			  continue
		   }
		  if f.Assignee != "" && !strings.EqualFold(v.Ticket.Assignee, f.Assignee) {
			   continue
		   }
		  if f.Breached != nil && v.Breached != *f.Breached {
			  continue
		   }
		   filtered = append(filtered, v)
	   }
	   total := len(filtered)
	  if offset >= total {
		   return PageViews{Items: nil, Total: total, Offset: offset, Limit: limit}
	  }
	  end := offset + limit
	   if end > total {
		   end = total
	  }
	   return PageViews{Items: filtered[offset:end],
		   Total: total, Offset: offset, Limit: limit}
   }

// Aggregate 工单状态聚合读面。

   func (w *Workflow) Aggregate(now time.Time) StatusAggregate {
	   if now.IsZero() {
		   now = time.Now().UTC()
	  }
	  views := w.ListViews("", 500, now)
	  var a StatusAggregate
	  for _, v := range views {
		   a.Total++
		   switch v.Ticket.State {
		  case StateOpen:
			   a.Open++
		  case StateAcked:
			   a.Acked++
		  case StateResolved:
			   a.Resolved++
		   case StateRejected:
			   a.Rejected++
		   }
		  if v.Breached {
			   a.Breached++
		  }
	  }
	   return a
   }
