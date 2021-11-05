package screenleasegate

import (
	"context"
	"fmt"
)

type ReplayStep struct {
	Op string `json:"op"`
	Issue *IssueReq `json:"issue,omitempty"`
	Submit *SubmitReq `json:"submit,omitempty"`
	Halt   *SetHaltReq  `json:"halt,omitempty"`
	Pack   *FlipPackReq `json:"pack,omitempty"`
	Expect string       `json:"expect"`
	Proof string `json:"proof,omitempty"`
}

type ReplayResult struct {
	Step int    `json:"step"`
	Op   string `json:"op"`
	OK   bool   `json:"ok"`
	Err string `json:"error,omitempty"`
	Lease string `json:"screen_lease_id,omitempty"`
	Proof string `json:"proof_kind,omitempty"`
}

  func Replay(ctx context.Context, svc *Service, steps []ReplayStep) []ReplayResult {
	  out := make([]ReplayResult, 0, len(steps))
	  lastLease := ""
	   for i, st := range steps {
		  res := ReplayResult{Step: i + 1, Op: st.Op}
		   var err error
		  switch st.Op {
		   case "issue":
			  if st.Issue == nil {
				   err = fmt.Errorf("missing issue")
				  break
			  }
			   l, e := svc.Issue(ctx, *st.Issue)
			  err = e
			  if l != nil {
				  lastLease = l.ID
				  res.Lease = l.ID
			   }
		  case "submit":
			   if st.Submit == nil {
				  err = fmt.Errorf("missing submit")
				  break
			  }
			   if st.Submit.LeaseID == "" {
				  st.Submit.LeaseID = lastLease
			   }
			  l, p, e := svc.Submit(ctx, *st.Submit)
			  err = e
			   if l != nil {
				   res.Lease = l.ID
			  }
			   if p != nil {
				  res.Proof = string(p.Kind)
			   }
		   case "halt":
			   if st.Halt == nil {
				   err = fmt.Errorf("missing halt")
				   break
			   }
			   _,
				   _, err = svc.SetHalt(ctx, *st.Halt)
		  case "pack":
			   if st.Pack == nil {
				   err = fmt.Errorf("missing pack")
				   break
			  }
			   _, _, err = svc.FlipPack(ctx, *st.Pack)
		  case "expire":
			  _, err = svc.ExpireSweep(ctx)
		  default:
			  err = fmt.Errorf("unknown op")
		  }
		   if err != nil {
			  res.Err = err.Error()
		  }
		   wantFail := st.Expect != "" && st.Expect != "ok"
		  res.OK = (err == nil && !wantFail) || (err != nil && wantFail)
		   if st.Proof != "" && res.Proof != "" && st.Proof != res.Proof {
			   res.OK = false
		  }
		   out = append(out, res)
	  }
	   return out
   }
