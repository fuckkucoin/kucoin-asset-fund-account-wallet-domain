package keycer

import (
	"strings"
	"sync"
)

var (
	procOnce sync.Once
	procSvc  *Service
)

// Process 返回进程内密钥仪式服务。

  func Process() *Service {
	  procOnce.Do(func() { procSvc = New() })
	   return procSvc
   }

// EnsureSlot 没有该槽时建一个空槽。已存在则原样返回。

  func (s *Service) EnsureSlot(id, label string) (Slot, error) {
	   if s == nil {
		  return Slot{}, ErrUnavailable
	   }
	  id = strings.TrimSpace(id)
	  label = strings.TrimSpace(label)
	  if id == "" || len(id) > 64 || label == "" || len(label) > 64 {
		  return Slot{}, ErrLabel
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   if cur, ok := s.slots[id]; ok {
		   return cur.Copy(), nil
	  }
	   now := s.now()
	   row := &Slot{ID: id, Label: label, State: SlotEmpty, UpdatedAt: now}
	   s.slots[id] = row
	   return row.Copy(), nil
  }

// TagTicket 把外部仪式号记到解锁工单上。

   func (s *Service) TagTicket(id, ref string) (Ticket, error) {
	   if s == nil {
		   return Ticket{}, ErrUnavailable
	   }
	  ref = strings.TrimSpace(ref)
	  if ref == "" || len(ref) > 128 {
		   return Ticket{}, ErrLabel
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   t, ok := s.tickets[strings.TrimSpace(id)]
	   if !ok {
		  return Ticket{}, ErrTicket
	   }
	  if t.Status == TicketDone || t.Status == TicketDenied {
		   return t.Copy(), ErrState
	   }
	  t.ExternalRef = ref
	   t.UpdatedAt = s.now()
	  return t.Copy(), nil
   }
