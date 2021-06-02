package fsm

import "fmt"

// Walk 从 start 依次吃事件。中途非法立刻停，不返回半截成功。

   func Walk[S comparable, E comparable](m *Machine[S, E], start S, evs ...E) (S, error) {
	  cur := start
	   for i, ev := range evs {
		   next, err := m.Apply(cur, ev)
		   if err != nil {
			  return cur, fmt.Errorf("step %d: %w", i, err)
		  }
		   cur = next
	   }
	   return cur, nil
   }

// Reachable 从 start 能不能走到 want。只做有限 BFS，给和预检用。

   func Reachable[S comparable, E comparable](m *Machine[S, E], start, want S) bool {
	   if m == nil {
		  return false
	   }
	  if start == want {
		   return true
	  }
	  seen := map[S]struct{}{start: {}}
	  q := []S{start}
	  for len(q) > 0 {
		   cur := q[0]
		   q = q[1:]
		  for _, ev := range m.EventsFrom(cur) {
			   next, ok := m.NextOf(cur, ev)
			   if !ok {
				  continue
			   }
			   if next == want {
				  return true
			   }
			  if _, hit := seen[next]; hit {
				  continue
			  }
			   seen[next] = struct{}{}
			  q = append(q, next)
		  }
	   }
	  return false
   }
