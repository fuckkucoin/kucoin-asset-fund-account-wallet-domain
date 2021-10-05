package pumpmarkfence

import "fmt"

var genTransitions = map[GenState][]GenState{
	GenIssued:    {GenDelivered, GenExpired},
	GenDelivered: {GenMarked, GenExpired},
	GenMarked:  {},
	GenExpired: {},
}

var halfTransitions = map[HalfState][]HalfState{
	HalfOpen: {HalfResumed, HalfAbandoned},
	HalfResumed: {},
	HalfAbandoned: {},
}

// CanTransitionGen 检查世代跃迁。
  func CanTransitionGen(from, to GenState) bool {
	   targets, ok := genTransitions[from]
	   if !ok {
		   return false
	   }
	  for _, t := range targets {
		   if t == to {
			  return true
		  }
	  }
	  return false
  }

// TransitionGen 流转世代状态。
   func TransitionGen(row *DeliverEffectGen, to GenState) error {
	  if row == nil {
		   return fmt.Errorf("%w: nil deliver_effect_gen", ErrIllegalTransit)
	  }
	  from := row.State
	   if from == "" {
		  from = GenIssued
	  }
	   if !CanTransitionGen(from, to) {
		  return fmt.Errorf("%w: %s -> %s", ErrIllegalTransit, from, to)
	  }
	  row.State = to
	  if to == GenMarked {
		  row.Applied = true
	   }
	  if to == GenExpired {
		  row.Applied = false
	  }
	  return nil
   }

// CanTransitionHalf 检查半截案跃迁。
  func CanTransitionHalf(from, to HalfState) bool {
	   targets, ok := halfTransitions[from]
	   if !ok {
		  return false
	   }
	  for _, t := range targets {
		  if t == to {
			  return true
		  }
	  }
	   return false
   }

// TransitionHalf 流转半截案。
   func TransitionHalf(row *HalfMarkCase, to HalfState) error {
	   if row == nil {
		   return fmt.Errorf("%w: nil half-mark case", ErrIllegalTransit)
	  }
	  from := row.State
	   if from == "" {
		  from = HalfOpen
	   }
	   if !CanTransitionHalf(from, to) {
		   return fmt.Errorf("%w: %s -> %s", ErrIllegalTransit, from, to)
	  }
	  row.State = to
	   return nil
   }

// GenMatrixRows 导出世代 FSM 供夹具。

  func GenMatrixRows() [][2]string {
	   order := []GenState{GenIssued, GenDelivered, GenMarked, GenExpired}
	  var out [][2]string
	  for _, from := range order {
		  for _, to := range genTransitions[from] {
			  out = append(out, [2]string{string(from), string(to)})
		  }
	  }
	  return out
  }

// HalfMatrixRows 导出半截 FSM。

   func HalfMatrixRows() [][2]string {
	  order := []HalfState{HalfOpen, HalfResumed, HalfAbandoned}
	   var out [][2]string
	  for _, from := range order {
		  for _, to := range halfTransitions[from] {
			   out = append(out, [2]string{string(from), string(to)})
		  }
	   }
	   return out
  }

// MustIssued 空状态视为 issued。

   func MustIssued(row *DeliverEffectGen) {
	   if row != nil && row.State == "" {
		   row.State = GenIssued
	   }
  }

// MustHalfOpen 空状态视为 open。

  func MustHalfOpen(row *HalfMarkCase) {
	   if row != nil && row.State == "" {
		  row.State = HalfOpen
	   }
   }

// ForbiddenGenPairs 明确禁止的回退。

  func ForbiddenGenPairs() [][2]string {
	   return [][2]string{
		  {"marked", "issued"},
		  {"marked", "delivered"},
		  {"expired", "issued"},
		  {"expired", "delivered"},
		  {"expired", "marked"},
		   {"delivered", "issued"},
		  {"marked", "expired"},
	   }
  }
