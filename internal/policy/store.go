package policy

import (
	"fmt"
	"sync"
	"time"
)

// Registry 只追加代次。Current 指向最高代次号。

type Registry struct {
	mu   sync.RWMutex
	docs map[int]Doc
	cur  int
}

  func NewRegistry(seed Doc) *Registry {
	  r := &Registry{docs: map[int]Doc{}}
	   if seed.Version > 0 {
		   _ = r.Publish(seed)
	  }
	   return r
   }

   func (r *Registry) Publish(doc Doc) error {
	  if doc.Version <= 0 {
		  return fmt.Errorf("version required")
	   }
	  r.mu.Lock()
	  defer r.mu.Unlock()
	  if _, ok := r.docs[doc.Version]; ok {
		   return fmt.Errorf("policy version %d exists", doc.Version)
	  }
	   if doc.PublishedAt.IsZero() {
		   doc.PublishedAt = time.Now().UTC()
	  }
	   r.docs[doc.Version] = doc
	   if doc.Version > r.cur {
		   r.cur = doc.Version
	  }
	   return nil
   }

  func (r *Registry) Get(version int) (Doc, error) {
	   r.mu.RLock()
	   defer r.mu.RUnlock()
	   d, ok := r.docs[version]
	  if !ok {
		  return Doc{}, fmt.Errorf("policy version %d not found", version)
	   }
	  return d, nil
   }

   func (r *Registry) Current() (Doc, error) {
	   r.mu.RLock()
	   defer r.mu.RUnlock()
	  if r.cur == 0 {
		   return Doc{}, fmt.Errorf("no policy published")
	  }
	  return r.docs[r.cur], nil
  }

  func (r *Registry) Versions() []int {
	   r.mu.RLock()
	  defer r.mu.RUnlock()
	   out := make([]int, 0, len(r.docs))
	  for v := range r.docs {
		   out = append(out, v)
	  }
	   return out
   }
