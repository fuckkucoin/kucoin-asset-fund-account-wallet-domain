package wdrelease

import (
	"sort"
	"time"
)

// QueueRow 值班台一页里的一行。

type QueueRow struct {
	ID string `json:"id"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	State string `json:"state"`
	PolicyVer string `json:"policy_version"`
	Code      string `json:"code,omitempty"`
	Depth     int       `json:"depth"`
	Approvals int       `json:"approvals"`
	AgeSec    int64     `json:"age_sec"`
	Residual  string    `json:"residual"`
	Frozen    string    `json:"frozen"`
	Deadline  time.Time `json:"deadline,omitempty"`
	Drift     bool      `json:"drift"`
	CreatedAt time.Time `json:"created_at"`
}

// Queue 列出未结票据。state 空则含 draft、pending、partially_approved。

  func (d *Desk) Queue(state string, limit int) []QueueRow {
	  if d == nil {
		   return nil
	   }
	   if limit <= 0 || limit > 200 {
		  limit = 50
	   }
	  d.mu.Lock()
	   defer d.mu.Unlock()
	  d.scanDrift()
	  now := d.now()
	  var rows []QueueRow
	   for _, t := range d.tickets {
		   if state == "" {
			   if !openStates(t.State) {
				   continue
			   }
		  } else if t.State != state {
			   continue
		   }
		  age := int64(now.Sub(t.CreatedAt).Seconds())
		  if age < 0 {
			  age = 0
		  }
		  rows = append(rows, QueueRow{
			  ID: t.ID, UserID: t.UserID, Asset: t.Asset, Amount: t.Amount, State: t.State,
			   PolicyVer: t.PolicyVer,
			   Code: t.Code, Depth: t.Depth, Approvals: t.Approvals,
			  AgeSec: age, Residual: t.Residual, Frozen: t.Frozen, Deadline: t.Deadline,
			   Drift: t.PolicyVer != d.activeID, CreatedAt: t.CreatedAt,
		   })
	   }
	  sort.Slice(rows, func(i, j int) bool {
		  di, dj := rows[i].Deadline, rows[j].Deadline
		  switch {
		  case di.IsZero() && !dj.IsZero():
			  return false
		  case !di.IsZero() && dj.IsZero():
			  return true
		  case !di.Equal(dj):
			  return di.Before(dj)
		   default:
			   return rows[i].CreatedAt.Before(rows[j].CreatedAt)
		   }
	   })
	  if len(rows) > limit {
		  rows = rows[:limit]
	   }
	   d.emitLocked()
	   return rows
   }

// Snap 台面指标快照。年龄取各状态最大值，剩余额度按资产相加。
type Snap struct {
	ActiveVersion string           `json:"active_version"`
	AgeSec        map[string]int64 `json:"age_sec"`
	Residual  map[string]string `json:"residual"`
	Privilege map[string]int64  `json:"privilege"`
	Drift     int               `json:"drift"`
	Open      int               `json:"open"`
}

   func (d *Desk) Snapshot() Snap {
	  if d == nil {
		  return Snap{}
	   }
	  d.mu.Lock()
	   defer d.mu.Unlock()
	   d.scanDrift()
	   d.emitLocked()
	   return d.snapLocked()
   }

   func (d *Desk) snapLocked() Snap {
	   now := d.now()
	   age := map[string]int64{}
	   var residual decimalString
	   open := 0
	  for _, t := range d.tickets {
		  sec := int64(now.Sub(t.CreatedAt).Seconds())
		  if sec < 0 {
			  sec = 0
		  }
		  if sec > age[t.State] {
			  age[t.State] = sec
		  }
		   if openStates(t.State) {
			  open++
		   }
		   residual.add(t.Asset, t.Residual)
	   }
	   return Snap{
		  ActiveVersion: d.activeID,
		   AgeSec: age,
		  Residual: residual.m,
		  Privilege: d.privilegeSnap(),
		  Drift: len(d.driftSet),
		   Open:  open,
	   }
  }

type decimalString struct {
	m map[string]string
}

  func (a *decimalString) add(asset, amount string) {
	   if a.m == nil {
		  a.m = map[string]string{}
	   }
	   cur := a.m[asset]
	   if cur == "" {
		  cur = "0"
	   }
	  sum, err := addDec(cur, amount)
	   if err != nil {
		  return
	   }
	   a.m[asset] = sum
  }
