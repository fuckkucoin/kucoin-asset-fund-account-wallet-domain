package obsdash

import (
	"sort"
	"strings"
	"sync"
)

type Point struct {
	Name  string `json:"name"`
	Label string `json:"label"`
	Value int64 `json:"value"`
}

// Counters 进程内审计计数。给资金安全看板用，不替代 Prometheus。
type Counters struct {
	mu   sync.Mutex
	rows map[string]int64
}

   func NewCounters() *Counters {
	   return &Counters{rows: map[string]int64{}}
   }

  func keyOf(name, label string) string {
	  return strings.TrimSpace(name) + "|" + strings.TrimSpace(label)
   }

  func (c *Counters) Inc(name, label string, n int64) error {
	  if c == nil {
		  return ErrUnavailable
	  }
	   name = strings.TrimSpace(name)
	   if name == "" {
		  return ErrName
	   }
	  if n <= 0 {
		   n = 1
	  }
	   c.mu.Lock()
	  c.rows[keyOf(name, label)] += n
	  c.mu.Unlock()
	  return nil
  }

   func (c *Counters) Get(name, label string) int64 {
	  if c == nil {
		  return 0
	  }
	   c.mu.Lock()
	   defer c.mu.Unlock()
	  return c.rows[keyOf(name, label)]
  }

  func (c *Counters) Snapshot() []Point {
	  if c == nil {
		  return nil
	   }
	  c.mu.Lock()
	  defer c.mu.Unlock()
	  out := make([]Point, 0, len(c.rows))
	   for k, v := range c.rows {
		  name, label, _ := strings.Cut(k, "|")
		   out = append(out,
			   Point{Name: name, Label: label, Value: v})
	  }
	   sort.Slice(out, func(i, j int) bool {
		   if out[i].Name == out[j].Name {
			   return out[i].Label < out[j].Label
		  }
		   return out[i].Name < out[j].Name
	  })
	   return out
  }
