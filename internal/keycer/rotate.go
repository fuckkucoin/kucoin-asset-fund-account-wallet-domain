package keycer

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
	"github.com/google/uuid"
)

  func (s *Service) OpenRotate(from, to, actor, note string) (Ticket, error) {
	  if s == nil {
		   return Ticket{}, ErrUnavailable
	  }
	  act, err := validate.Actor(actor)
	  if err != nil {
		  return Ticket{}, ErrActor
	  }
	   if strings.TrimSpace(from) == strings.TrimSpace(to) {
		   return Ticket{}, ErrRotate
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   src, ok := s.slots[strings.TrimSpace(from)]
	   if !ok {
		   return Ticket{}, ErrSlot
	   }
	  dst, ok := s.slots[strings.TrimSpace(to)]
	   if !ok {
		   return Ticket{}, ErrSlot
	  }
	   if src.State != SlotLoaded && src.State != SlotLocked {
		  return Ticket{}, ErrState
	   }
	   if dst.State != SlotEmpty && dst.State != SlotLoaded {
		  return Ticket{}, ErrState
	   }
	  now := s.now()
	   t := &Ticket{
		  ID: uuid.NewString(), SlotID: src.ID + ">" + dst.ID, Kind: "rotate",
		   Status: TicketOpen, OpenedBy: act, Note: strings.TrimSpace(note),
		  OpenedAt: now, UpdatedAt: now,
	   }
	   s.tickets[t.ID] = t
	   return t.Copy(), nil
   }

   func (s *Service) ExecuteRotate(id, actor string) (Rotation, error) {
	   if s == nil {
		  return Rotation{}, ErrUnavailable
	   }
	  act, err := validate.Actor(actor)
	  if err != nil {
		   return Rotation{}, ErrActor
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   t, ok := s.tickets[strings.TrimSpace(id)]
	   if !ok || t.Kind != "rotate" {
		   return Rotation{}, ErrTicket
	  }
	  if t.Status != TicketApproved {
		   return Rotation{}, ErrState
	   }
	  if strings.EqualFold(act, t.OpenedBy) {
		  return Rotation{}, ErrFourEye
	   }
	   parts := strings.Split(t.SlotID, ">")
	  if len(parts) != 2 {
		   return Rotation{}, ErrRotate
	  }
	  src, ok := s.slots[parts[0]]
	  if !ok {
		   return Rotation{}, ErrSlot
	   }
	  dst, ok := s.slots[parts[1]]
	  if !ok {
		  return Rotation{}, ErrSlot
	  }
	  if dst.State == SlotEmpty {
		   dst.Fingerprint = src.Fingerprint
		  dst.Label = src.Label + "-rot"
		   dst.State = SlotLoaded
	  }
	  src.State = SlotRetired
	   src.UpdatedAt = s.now()
	   dst.UpdatedAt = s.now()
	  t.Status = TicketDone
	   t.UpdatedAt = s.now()
	   row := Rotation{ID: uuid.NewString(), From: src.ID, To: dst.ID, Actor: act, Note: t.Note, At: s.now()}
	  s.rots = append(s.rots, row)
	  return row, nil
  }

   func (s *Service) Journal() []Rotation {
	  if s == nil {
		   return nil
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  out := make([]Rotation, len(s.rots))
	  copy(out, s.rots)
	   return out
  }
