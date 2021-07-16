package holdopsgate

import (
	"context"
	"fmt"
)

type ReplayStep struct {
	Name string `json:"name"`
	Action string   `json:"action"`
	Issue  IssueReq `json:"issue,omitempty"`
	Exec   ExecReq  `json:"exec,omitempty"`
	Bump   BumpReq  `json:"bump,omitempty"`
	Expect string   `json:"expect,omitempty"`
}

type ReplayResult struct {
	Name string `json:"name"`
	OK bool `json:"ok"`
	Err string `json:"error,omitempty"`
	Proof   string `json:"proof_kind,omitempty"`
	LeaseID string `json:"lease_id,omitempty"`
}

   func Replay(ctx context.Context, svc *Service, steps []ReplayStep) []ReplayResult {
	  out := make([]ReplayResult, 0, len(steps))
	   lastLease := ""
	   for _, st := range steps {
		   res := ReplayResult{Name: st.Name}
		  switch st.Action {
		   case "issue":
			   got, err := svc.Issue(ctx, st.Issue)
			   if err != nil {
				  res.Err = err.Error()
			   } else {
				  res.OK = true
				   res.LeaseID = got.ID
				   lastLease = got.ID
			   }
		   case "exec":
			  if st.Exec.LeaseID == "" {
				   st.Exec.LeaseID = lastLease
			  }
			   got, p, err := svc.Exec(ctx, st.Exec)
			  if got != nil {
				   res.LeaseID = got.ID
			   }
			   if p != nil {
				  res.Proof = string(p.Kind)
			   }
			   if err != nil {
				   res.Err = err.Error()
				  res.OK = st.Expect != "" && err.Error() == st.Expect || st.Expect == "deny"
				  if st.Expect == "deny" {
					   res.OK = true
				  }
			  } else {
				   res.OK = st.Expect == "" || st.Expect == "ok"
			  }
		   case "bump":
			   _, err := svc.Bump(ctx, st.Bump)
			  if err != nil {
				  res.Err = err.Error()
			   } else {
				  res.OK = true
			  }
		   default:
			  res.Err = fmt.Sprintf("unknown action %s", st.Action)
		  }
		   out = append(out, res)
	  }
	   return out
  }
