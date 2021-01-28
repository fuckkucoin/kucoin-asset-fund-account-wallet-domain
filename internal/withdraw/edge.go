package withdraw

import (
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/persist"
)

var (
	ErrTicketReopen = fmt.Errorf("withdraw ticket reopen requires an open mismatch")
	ErrTicketRole   = fmt.Errorf("withdraw ticket role refused")
	ErrTicketSLA    = fmt.Errorf("withdraw ticket past SLA remains open")
)

// TicketRole 结案/重开只给运维或出金复核。普通用户失败即关。
  func TicketRole(role perm.Role) error {
	   if perm.Allowed(role, perm.ActOpsAdmin) || perm.Allowed(role, perm.ActWithdrawDecide) {
		  return nil
	   }
	   return ErrTicketRole
  }

// AutoClose 禁止。超 SLA 仍须第三人结案。
  func (d *Desk) AutoClose(id string, now time.Time, sla time.Duration) error {
	  if d == nil {
		  return ErrTicketMiss
	  }
	  d.mu.Lock()
	   defer d.mu.Unlock()
	   row, ok := d.rows[id]
	  if !ok {
		  return ErrTicketMiss
	   }
	  if row.State == TicketClosed {
		  return ErrTicketClosed
	  }
	   if sla <= 0 {
		   sla = 4 * time.Hour
	  }
	   if now.Sub(row.CreatedAt) >= sla {
		   return ErrTicketSLA
	  }
	   return ErrTicketSLA
  }

// Reopen 已结案且观察差异仍在才允许。开单人与结案人不得自行重开。

  func (d *Desk) Reopen(id, actor string, epoch int64, token string, stillMismatch bool) (Exception, error) {
	   if d == nil || d.clock == nil {
		   return Exception{}, ErrTicketMiss
	  }
	   if !stillMismatch {
		   return Exception{}, ErrTicketReopen
	  }
	  if err := ticketActor(actor); err != nil {
		  return Exception{}, err
	   }
	  if err := d.fence.Check(id, epoch, token); err != nil {
		  return Exception{}, err
	  }
	  d.mu.Lock()
	  defer d.mu.Unlock()
	   row, ok := d.rows[id]
	   if !ok {
		  return Exception{}, ErrTicketMiss
	   }
	  if row.State != TicketClosed {
		  return Exception{}, ErrTicketReopen
	   }
	   if strings.EqualFold(row.ClosedBy, strings.TrimSpace(actor)) || strings.EqualFold(row.OpenedBy, strings.TrimSpace(actor)) {
		  return Exception{}, ErrTicketAssign
	   }
	   f, err := d.fence.Bump(id, persist.NewToken())
	  if err != nil {
		   return Exception{}, err
	  }
	  from := string(row.State)
	  row.State = TicketOpen
	  row.Assignee = ""
	   row.ClosedBy = ""
	   row.Fence = f
	   row.UpdatedAt = d.clock()
	   if err := d.persistRow(row, actor, "reopen", from, string(TicketOpen)); err != nil {
		   return Exception{}, err
	  }
	   return *row, nil
   }
