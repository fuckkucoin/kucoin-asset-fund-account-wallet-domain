package kms

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// Rotator 密钥轮换操作面。计划必须双人；未 Unlock 不得切激活槽。
type Rotator interface {
	Plan(kid, actor string) (RotatePlan, error)
	Approve(id, actor string) (RotatePlan, error)
	Commit(id, actor string) (RotatePlan, error)
}

type RotateState string

const (
	RotateOpen RotateState = "open"
	RotateReady     RotateState = "ready"
	RotateCommitted RotateState = "committed"
	RotateUnlocked RotateState = "unlocked"
	RotateActive   RotateState = "active"
)

type RotatePlan struct {
	ID       string `json:"id"`
	KID      string `json:"kid"`
	OpenedBy string `json:"opened_by"`
	ApprovedBy  string `json:"approved_by,omitempty"`
	CommittedBy string `json:"committed_by,omitempty"`
	UnlockedBy string `json:"unlocked_by,omitempty"`
	State     RotateState `json:"state"`
	Active    bool        `json:"active,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
}

type MemoryRotator struct {
	mu  sync.Mutex
	seq int
	rows  map[string]*RotatePlan
	clock func() time.Time
}

   func NewMemoryRotator() *MemoryRotator {
	   return &MemoryRotator{rows: map[string]*RotatePlan{}, clock: func() time.Time { return time.Now().UTC() }}
  }

   func rotateActor(actor string) error {
	   if strings.TrimSpace(actor) == "" || strings.EqualFold(actor, "system") {
		   return fmt.Errorf("key rotate actor invalid")
	   }
	   return nil
  }

  func (m *MemoryRotator) Plan(kid, actor string) (RotatePlan, error) {
	  if m == nil {
		  return RotatePlan{}, fmt.Errorf("key rotate unavailable")
	   }
	  if err := rotateActor(actor); err != nil {
		   return RotatePlan{}, err
	  }
	   kid = strings.TrimSpace(kid)
	   if kid == "" {
		   return RotatePlan{}, fmt.Errorf("key rotate kid required")
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   m.seq++
	  p := &RotatePlan{ID: fmt.Sprintf("rot-%d", m.seq), KID: kid, OpenedBy: strings.TrimSpace(actor), State: RotateOpen, CreatedAt: m.clock()}
	   m.rows[p.ID] = p
	  return *p, nil
  }

   func (m *MemoryRotator) Approve(id, actor string) (RotatePlan, error) {
	  if err := rotateActor(actor); err != nil {
		   return RotatePlan{}, err
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	   p, ok := m.rows[strings.TrimSpace(id)]
	   if !ok {
		  return RotatePlan{}, fmt.Errorf("key rotate plan not found")
	   }
	  if p.State != RotateOpen {
		  return *p, fmt.Errorf("key rotate plan not open")
	   }
	   if strings.EqualFold(p.OpenedBy, strings.TrimSpace(actor)) {
		  return RotatePlan{}, fmt.Errorf("key rotate requires a distinct approver")
	   }
	  p.ApprovedBy = strings.TrimSpace(actor)
	   p.State = RotateReady
	   return *p, nil
   }

   func (m *MemoryRotator) Commit(id, actor string) (RotatePlan, error) {
	  if err := rotateActor(actor); err != nil {
		   return RotatePlan{}, err
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  p,
		   ok := m.rows[strings.TrimSpace(id)]
	  if !ok {
		   return RotatePlan{}, fmt.Errorf("key rotate plan not found")
	  }
	   if p.State != RotateReady {
		  return *p, fmt.Errorf("key rotate not approved")
	   }
	  if strings.EqualFold(p.ApprovedBy, strings.TrimSpace(actor)) || strings.EqualFold(p.OpenedBy, strings.TrimSpace(actor)) {
		   return RotatePlan{}, fmt.Errorf("key rotate commit requires a third actor")
	  }
	  p.CommittedBy = strings.TrimSpace(actor)
	  p.State = RotateCommitted
	   return *p, nil
  }

// Unlock 提交后才能开槽。执行人不得是计划/复核/提交三人之一。

  func (m *MemoryRotator) Unlock(id, actor string) (RotatePlan, error) {
	  if err := rotateActor(actor); err != nil {
		  return RotatePlan{}, err
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  p, ok := m.rows[strings.TrimSpace(id)]
	  if !ok {
		   return RotatePlan{}, fmt.Errorf("key rotate plan not found")
	   }
	  if p.State != RotateCommitted {
		  return *p, fmt.Errorf("key rotate not committed")
	   }
	  actor = strings.TrimSpace(actor)
	  if strings.EqualFold(p.OpenedBy, actor) || strings.EqualFold(p.ApprovedBy, actor) || strings.EqualFold(p.CommittedBy, actor) {
		  return RotatePlan{}, fmt.Errorf("key rotate unlock requires a distinct actor")
	   }
	   p.UnlockedBy = actor
	   p.State = RotateUnlocked
	  return *p, nil
  }

// Activate 未 Unlock 不得切激活槽。
   func (m *MemoryRotator) Activate(id, actor string) (RotatePlan, error) {
	   if err := rotateActor(actor); err != nil {
		  return RotatePlan{}, err
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  p, ok := m.rows[strings.TrimSpace(id)]
	   if !ok {
		   return RotatePlan{}, fmt.Errorf("key rotate plan not found")
	  }
	  if p.State != RotateUnlocked {
		   return *p, fmt.Errorf("key rotate not unlocked")
	  }
	   if strings.EqualFold(p.UnlockedBy, strings.TrimSpace(actor)) {
		   return RotatePlan{}, fmt.Errorf("key rotate activate requires a distinct actor")
	   }
	   p.State = RotateActive
	   p.Active = true
	   return *p, nil
  }

var _ Rotator = (*MemoryRotator)(nil)
