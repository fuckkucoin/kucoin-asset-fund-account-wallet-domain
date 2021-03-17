package cfgcenter

import "time"

type EventKind string

const (
	EventPut EventKind = "put"
	EventPin EventKind = "pin"
	EventUnpin EventKind = "unpin"
	EventKill  EventKind = "kill"
	EventResume EventKind = "resume"
)

type Event struct {
	Kind EventKind `json:"kind"`
	Name string    `json:"name"`
	Version int       `json:"version,omitempty"`
	At      time.Time `json:"at"`
}

   func (s *Store) On(fn func(Event)) {
	   if s == nil || fn == nil {
		  return
	   }
	  s.mu.Lock()
	   s.watch = append(s.watch, fn)
	   s.mu.Unlock()
   }

   func (s *Store) notify(ev Event) {
	  s.mu.Lock()
	  fns := append([]func(Event){},
		  s.watch...)
	  clock := s.clock
	  s.mu.Unlock()
	  if ev.At.IsZero() && clock != nil {
		  ev.At = clock()
	   }
	   for _, fn := range fns {
		  fn(ev)
	   }
  }
