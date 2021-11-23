package recon

import (
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/persist"
)

var ErrRole = fmt.Errorf("reconcile actor role refused")

// RefuseRole 结案/重开只给运维或复核。普通用户失败即关。
  func RefuseRole(role perm.Role) error {
	  if perm.Allowed(role, perm.ActOpsAdmin) || perm.Allowed(role, perm.ActAMLClose) {
		  return nil
	   }
	  return ErrRole
   }

// AutoClose 禁止自动结案。超 SLA 仍须人工 CloseSafe。

  func (w *Workflow) AutoClose(id string, now time.Time) error {
	  if w == nil || w.book == nil {
		   return ErrNotFound
	  }
	   t, err := w.book.Get(id)
	   if err != nil {
		  return err
	   }
	   if t.State == StateResolved || t.State == StateRejected {
		  return ErrClosed
	   }
	   if now.Sub(t.CreatedAt) >= w.sla {
		   return ErrSLA
	  }
	  return ErrSLA
   }

// Reopen 已结案且差异仍在才允许。开单人与结案人不得自行重开；围栏不对失败即关。

  func (w *Workflow) Reopen(id, actor string, epoch int64, token string, stillMismatch bool) (*Ticket, error) {
	   if w == nil || w.book == nil {
		   return nil, ErrNotFound
	   }
	  if !stillMismatch {
		   return nil, ErrReopen
	   }
	   if err := actorOK(actor); err != nil {
		   return nil, err
	  }
	  cur, err := w.book.Get(id)
	   if err != nil {
		   return nil, err
	   }
	  if cur.State != StateResolved && cur.State != StateRejected {
		  return nil, ErrReopen
	   }
	  if strings.EqualFold(cur.ClosedBy, strings.TrimSpace(actor)) || strings.EqualFold(cur.OpenedBy, strings.TrimSpace(actor)) {
		   return nil, ErrFourEye
	  }
	  return w.book.reopen(id, actor, epoch, token)
  }

   func (b *Book) reopen(id, actor string, epoch int64, token string) (*Ticket, error) {
	  if b == nil || b.clock == nil {
		   return nil, ErrNotFound
	   }
	   if err := b.fence.Check(id, epoch, token); err != nil {
		  return nil, err
	   }
	  b.mu.Lock()
	  defer b.mu.Unlock()
	   t, ok := b.rows[id]
	   if !ok {
		  return nil, ErrNotFound
	  }
	   if t.State != StateResolved && t.State != StateRejected {
		   cp := *t
		  return &cp, ErrReopen
	   }
	  f, err := b.fence.Bump(id, persist.NewToken())
	  if err != nil {
		  return nil, err
	  }
	  from := string(t.State)
	   t.State = StateOpen
	   t.Assignee = ""
	  t.AckedBy = ""
	   t.ClosedBy = ""
	   t.Fence = f
	  t.UpdatedAt = b.clock()
	   if err := b.persistRow(t, actor, "reopen", from, string(StateOpen)); err != nil {
		   return nil, err
	  }
	   metrics.ReconTickets.WithLabelValues("reopen").Inc()
	   metrics.ReconBacklog.Set(b.backlogLocked())
	  cp := *t
	   return &cp, nil
   }
