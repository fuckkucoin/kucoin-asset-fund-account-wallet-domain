package keycer

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

   func (s *Service) OpenUnlock(slotID, actor, note string) (Ticket, error) {
	   if s == nil {
		   return Ticket{}, ErrUnavailable
	   }
	  act, err := validate.Actor(actor)
	  if err != nil {
		  return Ticket{}, ErrActor
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   cur, ok := s.slots[strings.TrimSpace(slotID)]
	   if !ok {
		  return Ticket{}, ErrSlot
	   }
	  if cur.State != SlotLocked {
		  return Ticket{}, ErrState
	   }
	   now := s.now()
	   t := &Ticket{
		   ID: uuid.NewString(), SlotID: cur.ID, Kind: "unlock", Status: TicketOpen,
		   OpenedBy: act, Note: strings.TrimSpace(note), OpenedAt: now, UpdatedAt: now,
	   }
	  s.tickets[t.ID] = t
	   return t.Copy(), nil
   }

// Approve 第二人批准。开单人自己批失败即关。

  func (s *Service) Approve(id, actor string) (Ticket, error) {
	  if s == nil {
		  return Ticket{},
			   ErrUnavailable
	  }
	   act, err := validate.Actor(actor)
	  if err != nil {
		   return Ticket{}, ErrActor
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   t, ok := s.tickets[strings.TrimSpace(id)]
	   if !ok {
		   return Ticket{}, ErrTicket
	  }
	   if t.Status != TicketOpen {
		  return t.Copy(), ErrState
	  }
	  if strings.EqualFold(act, t.OpenedBy) {
		   return t.Copy(), ErrFourEye
	   }
	   t.Status = TicketApproved
	   t.ClosedBy = act
	   t.UpdatedAt = s.now()
	   return t.Copy(), nil
  }

  func (s *Service) Unlock(id, actor string) (Slot, error) {
	  if s == nil {
		   return Slot{}, ErrUnavailable
	  }
	   act, err := validate.Actor(actor)
	  if err != nil {
		  return Slot{}, ErrActor
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  t, ok := s.tickets[strings.TrimSpace(id)]
	   if !ok {
		   return Slot{}, ErrTicket
	  }
	  if t.Status != TicketApproved {
		   return Slot{}, ErrState
	   }
	   if strings.EqualFold(act, t.OpenedBy) {
		  return Slot{}, ErrFourEye
	  }
	  cur, ok := s.slots[t.SlotID]
	  if !ok {
		  return Slot{}, ErrSlot
	   }
	   if cur.State != SlotLocked {
		   return cur.Copy(), ErrState
	   }
	  cur.State = SlotLoaded
	   cur.UpdatedAt = s.now()
	   t.Status = TicketDone
	  t.UpdatedAt = s.now()
	  return cur.Copy(), nil
  }

  func (s *Service) Deny(id, actor string) (Ticket, error) {
	   if s == nil {
		  return Ticket{}, ErrUnavailable
	  }
	  if _, err := validate.Actor(actor); err != nil {
		   return Ticket{}, ErrActor
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   t, ok := s.tickets[strings.TrimSpace(id)]
	  if !ok {
		  return Ticket{}, ErrTicket
	   }
	   if t.Status != TicketOpen {
		   return t.Copy(), ErrState
	  }
	  t.Status = TicketDenied
	  t.ClosedBy = actor
	  t.UpdatedAt = s.now()
	  return t.Copy(), nil
   }
