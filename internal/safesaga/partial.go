package safesaga

import (
	"fmt"
	"strings"
	"sync"
)

// Partial 记录已成功步骤。失败后必须按相反顺序补偿，补偿失败不得当完成。

type Partial struct {
	ID string `json:"id"`
	Done []Step `json:"done"`
	Undone []Step `json:"undone,omitempty"`
	Cause  string `json:"cause,omitempty"`
	Closed bool   `json:"closed,omitempty"`
}

type Compensator struct {
	mu   sync.Mutex
	rows map[string]*Partial
}

  func NewCompensator() *Compensator {
	  return &Compensator{rows: map[string]*Partial{}}
  }

  func (c *Compensator) Record(id string, step Step) error {
	   if c == nil {
		   return ErrUnavailable
	   }
	   id = strings.TrimSpace(id)
	  if id == "" {
		  return ErrIdem
	  }
	   if step == "" || step == StepCompensate || step == StepAborted || step == StepDone {
		  return ErrStep
	  }
	  c.mu.Lock()
	   defer c.mu.Unlock()
	  p := c.rows[id]
	  if p == nil {
		   p = &Partial{ID: id}
		  c.rows[id] = p
	  }
	   if p.Closed {
		   return ErrStep
	   }
	   p.Done = append(p.Done, step)
	  return nil
  }

   func (c *Compensator) Fail(id, cause string) error {
	  if c == nil {
		  return ErrUnavailable
	  }
	  c.mu.Lock()
	   defer c.mu.Unlock()
	   p := c.rows[strings.TrimSpace(id)]
	   if p == nil {
		   return ErrUnavailable
	   }
	  if p.Closed {
		  return ErrStep
	  }
	  p.Cause = strings.TrimSpace(cause)
	  if p.Cause == "" {
		   p.Cause = "partial fail"
	   }
	   return nil
  }

// Compensate 逆序撤销。undo 失败则停在该步，不得把单标成已结。
  func (c *Compensator) Compensate(id string, undo func(Step) error) error {
	   if c == nil || undo == nil {
		  return ErrUnavailable
	   }
	   c.mu.Lock()
	  p := c.rows[strings.TrimSpace(id)]
	   if p == nil {
		   c.mu.Unlock()
		   return ErrUnavailable
	   }
	   if p.Closed {
		  c.mu.Unlock()
		  return ErrStep
	   }
	  if p.Cause == "" {
		   c.mu.Unlock()
		   return fmt.Errorf("saga compensate refused without fail mark")
	   }
	  todo := append([]Step(nil), p.Done...)
	   c.mu.Unlock()
	   for i := len(todo) - 1; i >= 0; i-- {
		  if err := undo(todo[i]); err != nil {
			   return ErrCompensate
		  }
		  c.mu.Lock()
		   p.Undone = append(p.Undone, todo[i])
		   if len(p.Done) > 0 {
			  p.Done = p.Done[:len(p.Done)-1]
		  }
		   c.mu.Unlock()
	  }
	  c.mu.Lock()
	   p.Closed = true
	   c.mu.Unlock()
	   return nil
   }

   func (c *Compensator) Get(id string) (Partial, error) {
	   if c == nil {
		   return Partial{}, ErrUnavailable
	  }
	   c.mu.Lock()
	   defer c.mu.Unlock()
	  p := c.rows[strings.TrimSpace(id)]
	  if p == nil {
		   return Partial{}, ErrUnavailable
	  }
	  cp := *p
	   cp.Done = append([]Step(nil), p.Done...)
	   cp.Undone = append([]Step(nil), p.Undone...)
	   return cp, nil
  }
