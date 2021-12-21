package warroom

import (
	"context"
	"strings"
	"github.com/google/uuid"
)

// OpenUnfreeze 开解冻单。开单人不能自己批、不能自己关。

  func (s *Service) OpenUnfreeze(req TicketReq) (Ticket, error) {
	  if s == nil {
		   return Ticket{}, ErrUnavailable
	  }
	   act, err := requireActor(req.Actor)
	  if err != nil {
		  return Ticket{}, err
	   }
	   why,
		   err := requireReason(req.Reason)
	   if err != nil {
		  return Ticket{}, err
	   }
	  scope, err := parseScope(req.Scope)
	   if err != nil {
		   return Ticket{}, err
	   }
	  uid := ""
	   if scope == ScopeUser {
		  uid, err = requireUser(req.UserID)
		  if err != nil {
			  return Ticket{}, err
		   }
	  }
	   now := s.now()
	   row := &Ticket{
		   ID: uuid.NewString(), UserID: uid, Scope: scope, Status: TicketOpen,
		   Reason: why, OpenedBy: act, OpenedAt: now, UpdatedAt: now,
	  }
	   s.mu.Lock()
	   s.tickets[row.ID] = row
	   s.noteLocked("unfreeze_open", act, uid, why)
	  s.mu.Unlock()
	  return row.Copy(), nil
   }

   func (s *Service) Approve(id, actor string) (Ticket, error) {
	  return s.close(id, actor, TicketApproved)
   }

  func (s *Service) Deny(id, actor string) (Ticket, error) {
	   return s.close(id, actor, TicketDenied)
  }

   func (s *Service) close(id, actor string, to TicketStatus) (Ticket, error) {
	  if s == nil {
		   return Ticket{}, ErrUnavailable
	  }
	   act, err := requireActor(actor)
	  if err != nil {
		  return Ticket{}, err
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   cur, ok := s.tickets[strings.TrimSpace(id)]
	  if !ok {
		  return Ticket{}, ErrTicket
	  }
	  if cur.Status.Terminal() || cur.Status == TicketApproved && to != TicketApproved {
		  if cur.Status != TicketOpen {
			  return cur.Copy(), ErrState
		  }
	   }
	   if cur.Status != TicketOpen {
		  return cur.Copy(), ErrState
	   }
	  if strings.EqualFold(act, cur.OpenedBy) {
		  return cur.Copy(), ErrFourEye
	   }
	  cur.Status = to
	   cur.ClosedBy = act
	   cur.UpdatedAt = s.now()
	  s.noteLocked("unfreeze_"+string(to), act, cur.UserID, cur.ID)
	  return cur.Copy(), nil
   }

// Apply 已批准的工单才动冻结。范围用户解冻不关 freeze-all；全局工单才关开关。

   func (s *Service) Apply(ctx context.Context, id, actor string) (Ticket, error) {
	  if s == nil {
		   return Ticket{}, ErrUnavailable
	  }
	   act, err := requireActor(actor)
	  if err != nil {
		  return Ticket{}, err
	  }
	   s.mu.Lock()
	   cur, ok := s.tickets[strings.TrimSpace(id)]
	  if !ok {
		   s.mu.Unlock()
		  return Ticket{}, ErrTicket
	  }
	   if cur.Status != TicketApproved {
		  s.mu.Unlock()
		  return cur.Copy(), ErrState
	  }
	   if strings.EqualFold(act, cur.OpenedBy) {
		   s.mu.Unlock()
		   return cur.Copy(), ErrFourEye
	   }
	  uid := cur.UserID
	  scope := cur.Scope
	   cur.Status = TicketApplied
	   cur.ClosedBy = act
	   cur.UpdatedAt = s.now()
	   if scope == ScopeGlobal {
		   s.sw.FrozenAll = false
		  s.sw.Actor = act
		  s.sw.Reason = "unfreeze_all"
		  s.sw.At = s.now()
	   } else {
		   s.frozen[uid] = false
	  }
	  s.noteLocked("unfreeze_apply", act, uid, cur.ID)
	   s.mu.Unlock()
	   if s.led != nil && uid != "" {
		  if err := s.led.SetFrozen(ctx, uid, false); err != nil {
			   return Ticket{}, err
		   }
	   }
	   return cur.Copy(), nil
   }

  func (s *Service) TicketOf(id string) (Ticket, error) {
	   if s == nil {
		  return Ticket{}, ErrUnavailable
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   cur,
		  ok := s.tickets[strings.TrimSpace(id)]
	  if !ok {
		   return Ticket{}, ErrTicket
	   }
	  return cur.Copy(), nil
   }

   func (s *Service) Tickets(status string) []Ticket {
	  if s == nil {
		  return nil
	  }
	  status = strings.TrimSpace(status)
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  out := make([]Ticket, 0, len(s.tickets))
	   for _, t := range s.tickets {
		   if status != "" && string(t.Status) != status {
			  continue
		   }
		  out = append(out, t.Copy())
	   }
	  return out
   }
