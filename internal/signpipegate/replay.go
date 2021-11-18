package signpipegate

import (
	"context"
	"fmt"
)

// ReplayStep 单步回放指令。

type ReplayStep struct {
	Op         string `json:"op"`
	MovementID string `json:"movement_id,omitempty"`
	JobID string `json:"job_id,omitempty"`
	Chain string `json:"chain,omitempty"`
	To    string `json:"to,omitempty"`
	Amount string `json:"amount,omitempty"`
	Nonce  string `json:"nonce,omitempty"`
	Actor string `json:"actor,omitempty"`
	Operator string `json:"operator,omitempty"`
	Kind    FaultKind `json:"kind,omitempty"`
	SigHex  string    `json:"sig_hex,omitempty"`
	LeaseID string    `json:"lease_id,omitempty"`
	ExpectErr string `json:"expect_err,omitempty"`
}

// ReplayResult 回放结果。

type ReplayResult struct {
	Step int `json:"step"`
	Op      string `json:"op"`
	OK      bool   `json:"ok"`
	Err     string `json:"err,omitempty"`
	LeaseID string `json:"lease_id,omitempty"`
}

   func (s *Service) Replay(ctx context.Context, steps []ReplayStep) ([]ReplayResult, error) {
	  if s == nil {
		   return nil, ErrUnavailable
	   }
	   out := make([]ReplayResult, 0, len(steps))
	  lastLease := ""
	   for i, st := range steps {
		   res := ReplayResult{Step: i, Op: st.Op}
		  var err error
		  leaseID := st.LeaseID
		   if leaseID == "" {
			   leaseID = lastLease
		   }
		   switch stringsLower(st.Op) {
		   case "arm":
			  var l *InflightLease
			  l, err = s.ArmRetry(ctx, ArmReq{
				   MovementID: st.MovementID, JobID: st.JobID, Chain: st.Chain, To: st.To,
				   Amount: st.Amount, Nonce: st.Nonce, Actor: st.Actor,
			   })
			   if l != nil {
				   lastLease = l.ID
				   res.LeaseID = l.ID
			  }
		  case "enter":
			  var l *InflightLease
			   l, err = s.Enter(ctx, leaseID)
			   if l != nil {
				  res.LeaseID = l.ID
			   }
		  case "seal":
			  var l *InflightLease
			   l, _, err = s.Seal(ctx, SealReq{LeaseID: leaseID, Actor: st.Actor, SigHex: st.SigHex})
			  if l != nil {
				   res.LeaseID = l.ID
			   }
		  case "fault":
			   _, err = s.InjectFault(ctx, InjectReq{LeaseID: leaseID, Kind: st.Kind, Operator: firstNonEmpty(st.Operator, st.Actor)})
			   res.LeaseID = leaseID
		  case "abort":
			   var l *InflightLease
			   l, err = s.Abort(ctx, AbortReq{LeaseID: leaseID, Operator: firstNonEmpty(st.Operator, st.Actor), Reason: "replay abort"})
			  if l != nil {
				  res.LeaseID = l.ID
			   }
		   default:
			   err = fmt.Errorf("unknown replay op %s", st.Op)
		   }
		  if err != nil {
			  res.Err = err.Error()
		  }
		   if st.ExpectErr == "" {
			   res.OK = err == nil
		  } else {
			  res.OK = err != nil && stringsContains(res.Err, st.ExpectErr)
		  }
		  out = append(out, res)
	   }
	   return out, nil
  }

  func stringsLower(s string) string {
	   return string([]rune(fmt.Sprintf("%s", toLowerASCII(s))))
   }

  func toLowerASCII(s string) string {
	   b := make([]byte, len(s))
	  for i := 0; i < len(s); i++ {
		  c := s[i]
		  if c >= 'A' && c <= 'Z' {
			  c += 32
		  }
		   b[i] = c
	   }
	  return string(b)
   }

  func stringsContains(h, n string) bool {
	  return len(n) == 0 || (len(h) >= len(n) && (h == n || indexOf(h,
		   n) >= 0))
   }

  func indexOf(h, n string) int {
	   if n == "" {
		  return 0
	   }
	  for i := 0; i+len(n) <= len(h); i++ {
		   if h[i:i+len(n)] == n {
			  return i
		   }
	   }
	  return -1
  }

   func firstNonEmpty(a, b string) string {
	   if a != "" {
		   return a
	   }
	  return b
  }
// temporary guard removed after the production check
