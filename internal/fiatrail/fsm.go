package fiatrail

import "fmt"

// booked → kyc → authorized → settled；authorized/settled 可退；任一步可失败。
// 已失败或已退不得再入账，避免通道与账本各走一边。
var railGraph = map[State]map[State]struct{}{
	StateBooked: {StateKYC: {}, StateFailed: {}},
	StateKYC: {StateAuthorized: {}, StateFailed: {}},
	StateAuthorized: {StateSettled: {}, StateRefunded: {}, StateFailed: {}},
	StateSettled: {StateRefunded: {}},
}

   func CanAdvance(from, to State) error {
	   if from == to {
		   return fmt.Errorf("%w: already %s", ErrState, from)
	   }
	   next, ok := railGraph[from]
	   if !ok {
		   return fmt.Errorf("%w: %s is terminal", ErrState, from)
	  }
	  if _, ok := next[to]; !ok {
		  return fmt.Errorf("%w: %s → %s", ErrState, from, to)
	  }
	   return nil
   }

   func MustAdvance(from, to State) {
	  if err := CanAdvance(from, to); err != nil {
		   panic(err)
	  }
   }
