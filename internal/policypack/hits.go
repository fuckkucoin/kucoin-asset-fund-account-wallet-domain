package policypack

import "sync"

type Hit struct {
	Pack string
	Version int
	Rule  string
	Mode  string
	Allow bool
}

type HitBook struct {
	mu   sync.Mutex
	rows []Hit
}

  func NewHitBook() *HitBook { return &HitBook{} }

  func (b *HitBook) Add(h Hit) {
	   if b == nil {
		  return
	  }
	   b.mu.Lock()
	   b.rows = append(b.rows, h)
	   b.mu.Unlock()
   }

   func (b *HitBook) List() []Hit {
	  if b == nil {
		  return nil
	  }
	  b.mu.Lock()
	   defer b.mu.Unlock()
	  out := make([]Hit, len(b.rows))
	   copy(out, b.rows)
	  return out
  }

type Rate struct {
	Pack string
	Rule string
	Total int
	Denied int
}

   func (b *HitBook) Rates() []Rate {
	  if b == nil {
		  return nil
	   }
	   b.mu.Lock()
	  defer b.mu.Unlock()
	   idx := map[string]int{}
	   out := make([]Rate, 0)
	  for _, h := range b.rows {
		   k := h.Pack + "|" + h.Rule
		   i, ok := idx[k]
		   if !ok {
			   i = len(out)
			  idx[k] = i
			  out = append(out, Rate{Pack: h.Pack, Rule: h.Rule})
		  }
		   out[i].Total++
		   if !h.Allow {
			  out[i].Denied++
		  }
	   }
	   return out
  }
