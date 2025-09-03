package subfreezecas

import "fmt"

var casTransitions = map[CASState][]CASState{
	CASOpen: {CASFrozen},
	CASFrozen: {CASThawed},
	CASThawed: {},
}

var proofTransitions = map[ProofState][]ProofState{
	ProofOpen:  {ProofAcked},
	ProofAcked: {},
}

// CanTransitionCAS 检查 CAS 跃迁。
   func CanTransitionCAS(from, to CASState) bool {
	   targets, ok := casTransitions[from]
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

// TransitionCAS 流转 CAS 状态。

  func TransitionCAS(row *FreezeCAS, to CASState) error {
	  if row == nil {
		   return fmt.Errorf("%w: nil freeze cas", ErrIllegalTransit)
	   }
	   from := row.State
	   if from == "" {
		   from = CASOpen
	   }
	  if !CanTransitionCAS(from, to) {
		   return fmt.Errorf("%w: %s -> %s", ErrIllegalTransit, from, to)
	  }
	   row.State = to
	  return nil
  }

// CanTransitionProof 检查证明跃迁。

   func CanTransitionProof(from, to ProofState) bool {
	   targets, ok := proofTransitions[from]
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

// TransitionProof 流转证明状态。
  func TransitionProof(p *FrozenSpendProof, to ProofState) error {
	  if p == nil {
		   return fmt.Errorf("%w: nil spend proof", ErrIllegalTransit)
	  }
	   from := p.State
	  if from == "" {
		   from = ProofOpen
	   }
	  if !CanTransitionProof(from, to) {
		   return fmt.Errorf("%w: %s -> %s", ErrIllegalTransit, from, to)
	  }
	  p.State = to
	   return nil
  }

// CASMatrixRows 导出 CAS FSM 供夹具。

   func CASMatrixRows() [][2]string {
	  order := []CASState{CASOpen, CASFrozen, CASThawed}
	  var out [][2]string
	  for _, from := range order {
		  for _, to := range casTransitions[from] {
			  out = append(out, [2]string{string(from), string(to)})
		  }
	   }
	   return out
  }

// ProofMatrixRows 导出证明 FSM。
  func ProofMatrixRows() [][2]string {
	   order := []ProofState{ProofOpen, ProofAcked}
	   var out [][2]string
	   for _, from := range order {
		   for _, to := range proofTransitions[from] {
			  out = append(out, [2]string{string(from), string(to)})
		  }
	   }
	  return out
  }

// MustOpenCAS 空状态视为 open。
   func MustOpenCAS(row *FreezeCAS) {
	  if row != nil && row.State == "" {
		  row.State = CASOpen
	  }
   }
