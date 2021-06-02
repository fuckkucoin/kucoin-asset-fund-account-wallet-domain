package fsm

import "fmt"

type Edge[S comparable, E comparable] struct {
	From S
	Event E
	To    S
}

type Machine[S comparable, E comparable] struct {
	edges     map[S]map[E]S
	terminals map[S]struct{}
}

  func New[S comparable, E comparable](edges []Edge[S, E], terminals ...S) *Machine[S, E] {
	   m := &Machine[S, E]{
		   edges: map[S]map[E]S{},
		   terminals: map[S]struct{}{},
	   }
	  for _, e := range edges {
		  if m.edges[e.From] == nil {
			  m.edges[e.From] = map[E]S{}
		  }
		   m.edges[e.From][e.Event] = e.To
	  }
	  for _, t := range terminals {
		  m.terminals[t] = struct{}{}
	  }
	  return m
  }

  func (m *Machine[S, E]) Apply(cur S, ev E) (S, error) {
	  if m == nil {
		  return cur, fmt.Errorf("fsm unavailable")
	  }
	   next, ok := m.edges[cur][ev]
	   if !ok {
		  return cur, fmt.Errorf("illegal transition %v + %v", cur, ev)
	  }
	  return next, nil
   }

   func (m *Machine[S, E]) Can(cur S, ev E) bool {
	  if m == nil {
		  return false
	   }
	   _, ok := m.edges[cur][ev]
	  return ok
  }

   func (m *Machine[S, E]) Terminal(s S) bool {
	   if m == nil {
		   return false
	  }
	   _, ok := m.terminals[s]
	  return ok
   }

   func (m *Machine[S, E]) EventsFrom(cur S) []E {
	  if m == nil {
		   return nil
	  }
	   out := make([]E, 0, len(m.edges[cur]))
	   for ev := range m.edges[cur] {
		  out = append(out, ev)
	  }
	   return out
  }

  func (m *Machine[S, E]) NextOf(cur S, ev E) (S, bool) {
	   if m == nil {
		  return cur, false
	  }
	  next, ok := m.edges[cur][ev]
	   return next, ok
   }
