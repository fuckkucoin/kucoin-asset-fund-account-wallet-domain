package transfers

import (
	"strings"
)

const (
	KindDeposit  = "deposit"
	KindWithdraw = "withdraw"
	KindTransfer = "transfer"
	StatePending = "pending"
	StateProcessing = "processing"
	StateCompleted  = "completed"
	StateFailed     = "failed"
	StateRolledBack = "rolled_back"
	// 半成品相位见 reclaim.go。
)

// Rec 一条可恢复的充提流水快照。
type Rec struct {
	ID string
	Kind string
	State  string
	UserID string
	Asset string
}

// Incomplete 仅 pending/processing（及等价未终态）入恢复集。
  func Incomplete(rows []Rec) []Rec {
	  out := make([]Rec, 0, len(rows))
	  for _, r := range rows {
		  if IsTerminal(r.State) {
			  continue
		  }
		  if strings.TrimSpace(r.ID) == "" {
			   continue
		   }
		  out = append(out, r)
	  }
	  return out
   }

// Store 持久化流水读面。

type Store interface {
	List(limit int) ([]Rec, error)
}

// 恢复游标：按 ID 续扫，避免整表重复入队。
type Cursor struct {
	After string
}

   func (c *Cursor) Filter(rows []Rec) []Rec {
	  if c == nil || c.After == "" {
		  return rows
	   }
	  out := make([]Rec, 0, len(rows))
	  seen := false
	  for _, r := range rows {
		  if !seen {
			   if r.ID == c.After {
				   seen = true
			   }
			  continue
		  }
		   out = append(out, r)
	   }
	   return out
   }

   func (c *Cursor) Advance(id string) {
	  if c == nil {
		  return
	  }
	   id = strings.TrimSpace(id)
	   if id != "" {
		  c.After = id
	   }
   }

// LoadIncomplete 扫持久化未终态流水。

  func LoadIncomplete(st Store, cur *Cursor, limit int) ([]Rec, error) {
	  if st == nil {
		   return nil, ErrStore
	   }
	   if limit <= 0 {
		   limit = 200
	  }
	   rows, err := st.List(limit)
	   if err != nil {
		  return nil, err
	  }
	   rows = Incomplete(rows)
	  if cur != nil {
		  rows = cur.Filter(rows)
	   }
	   return rows, nil
   }
