package webhook

import (
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Endpoint struct {
	ID string `json:"id"`
	URL    string `json:"url"`
	Secret string `json:"secret,omitempty"`
	Kinds []string `json:"kinds"`
}

type Registry struct {
	mu   sync.Mutex
	rows map[string]Endpoint
}

   func NewRegistry() *Registry { return &Registry{rows: map[string]Endpoint{}} }

   func (r *Registry) Put(e Endpoint) error {
	  if r == nil {
		   return ErrUnavailable
	  }
	  e.ID = strings.TrimSpace(e.ID)
	  e.URL = strings.TrimSpace(e.URL)
	   if e.ID == "" || e.URL == "" || e.Secret == "" {
		  return ErrEndpoint
	   }
	  if _, err := validate.Reason(e.ID); err != nil {
		  return ErrEndpoint
	   }
	  for _, k := range e.Kinds {
		   if !Known(k) {
			  return ErrKind
		  }
	  }
	   r.mu.Lock()
	  r.rows[e.ID] = e
	  r.mu.Unlock()
	   return nil
  }

  func (r *Registry) Get(id string) (Endpoint, bool) {
	  if r == nil {
		   return Endpoint{}, false
	   }
	  r.mu.Lock()
	   defer r.mu.Unlock()
	   e,
		   ok := r.rows[strings.TrimSpace(id)]
	  return e, ok
   }

   func (r *Registry) Matching(kind string) []Endpoint {
	  if r == nil {
		   return nil
	   }
	   r.mu.Lock()
	   defer r.mu.Unlock()
	   out := make([]Endpoint, 0)
	   for _, e := range r.rows {
		   if len(e.Kinds) == 0 {
			  out = append(out, e)
			  continue
		  }
		  for _, k := range e.Kinds {
			   if k == kind {
				   out = append(out, e)
				   break
			   }
		  }
	   }
	   return out
  }

  func (r *Registry) List() []Endpoint {
	  r.mu.Lock()
	  defer r.mu.Unlock()
	  out := make([]Endpoint, 0, len(r.rows))
	  for _, e := range r.rows {
		   e.Secret = ""
		   out = append(out, e)
	  }
	   return out
  }

   func (r *Registry) listRaw() []Endpoint {
	   r.mu.Lock()
	  defer r.mu.Unlock()
	   out := make([]Endpoint, 0, len(r.rows))
	  for _, e := range r.rows {
		   out = append(out, e)
	  }
	   return out
  }
