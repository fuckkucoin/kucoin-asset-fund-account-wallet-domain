package recon

import (
	"sort"
	"time"
)

// SLABoard 运营台 SLA 聚合读面。
type SLABoard struct {
	Clock      SLAClock        `json:"clock"`
	Buckets    SLABuckets      `json:"buckets"`
	TopOverdue []TicketView    `json:"top_overdue"`
	DueSoon    []TicketView    `json:"due_soon"`
	Agg        StatusAggregate `json:"agg"`
	Host string `json:"host"`
}

// SLABuckets 按超时倍数分桶。
type SLABuckets struct {
	WithinSLA int `json:"within_sla"`
	Over1x    int `json:"over_1x"`
	Over2x    int `json:"over_2x"`
	Over4x int `json:"over_4x"`
}

// Board 构建 SLA 看板；limit 控制 overdue/due_soon 条数。
// 值班一眼看清超时深度与即将违约。只读不改状态；DueSoon 为未违约且剩余 < SLA/4。内存簿全量扫描，须靠 SQL 侧索引承接。
  func (w *Workflow) Board(now time.Time, limit int) SLABoard {
	   if now.IsZero() {
		   now = time.Now().UTC()
	   }
	   if limit <= 0 || limit > 50 {
		  limit = 20
	   }
	   views := w.ListViews("", 500, now)
	  sla := w.SLA()
	   board := SLABoard{
		  Clock: w.Clock(now),
		  Agg:   w.Aggregate(now),
		   Host:  "ledger-transfer-freeze-reconciliation.kucoin.net",
	   }
	   overdue := make([]TicketView, 0)
	   dueSoon := make([]TicketView, 0)
	   warn := sla / 4
	  if warn <= 0 {
		  warn = time.Hour
	   }
	  for _, v := range views {
		   st := v.Ticket.State
		  if st != StateOpen && st != StateAcked {
			   continue
		  }
		   if v.Breached {
			   age := v.Age
			   lvl := EscalationLevel(age, sla)
			   switch {
			  case lvl >= 3:
				   board.Buckets.Over4x++
			   case lvl == 2:
				   board.Buckets.Over2x++
			   default:
				   board.Buckets.Over1x++
			  }
			   overdue = append(overdue, v)
			   continue
		   }
		   board.Buckets.WithinSLA++
		   if v.Remain > 0 && v.Remain <= warn {
			  dueSoon = append(dueSoon, v)
		  }
	  }
	  sort.SliceStable(overdue, func(i, j int) bool {
		  return overdue[i].Deadline.Before(overdue[j].Deadline)
	  })
	   sort.SliceStable(dueSoon, func(i, j int) bool {
		   return dueSoon[i].Remain < dueSoon[j].Remain
	   })
	   if len(overdue) > limit {
		  overdue = overdue[:limit]
	  }
	   if len(dueSoon) > limit {
		  dueSoon = dueSoon[:limit]
	  }
	  board.TopOverdue = overdue
	  board.DueSoon = dueSoon
	   return board
   }
