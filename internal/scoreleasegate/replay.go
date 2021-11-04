package scoreleasegate

import (
	"context"
	"fmt"
	"time"
)

type ReplayCase struct {
	Name string `json:"name"`
	Steps  []ReplayStep `json:"steps"`
	Expect string       `json:"expect"`
}

type ReplayResult struct {
	Step int    `json:"step"`
	Op   string `json:"op"`
	OK bool `json:"ok"`
	Err   string `json:"error,omitempty"`
	Lease string `json:"score_lease_id,omitempty"`
	Proof string `json:"proof_kind,omitempty"`
}

   func Replay(ctx context.Context, svc *Service, steps []ReplayStep) []ReplayResult {
	  out := make([]ReplayResult, 0, len(steps))
	  lastLease := ""
	  for i, st := range steps {
		  res := ReplayResult{Step: i + 1, Op: st.Op}
		  var err error
		   switch st.Op {
		   case "issue", "eval":
			   req := issueFromMap(st.Req)
			   l, e := svc.Issue(ctx, req)
			  err = e
			   if l != nil {
				   lastLease = l.ID
				  res.Lease = l.ID
			  }
		   case "submit":
			  req := submitFromMap(st.Req,
				  lastLease)
			  l, p, e := svc.Submit(ctx, req)
			  err = e
			   if l != nil {
				   res.Lease = l.ID
			   }
			  if p != nil {
				  res.Proof = string(p.Kind)
			  }
		   case "halt":
			   _, _, err = svc.SetHalt(ctx, SetHaltReq{On: st.Req["on"] == "true", Actor: firstNonEmpty(st.Req["actor"], "ops-halt"), Reason: "halt"})
		  case "pack":
			  _, _, err = svc.SetPack(ctx, SetPackReq{Pack: firstNonEmpty(st.Req["pack"], "kyc1"), Actor: firstNonEmpty(st.Req["actor"], "ops-pack"), Reason: "pack"})
		   case "mode":
			  _, _, err = svc.SetMode(ctx, SetModeReq{Mode: firstNonEmpty(st.Req["mode"], "shadow"), Actor: firstNonEmpty(st.Req["actor"], "ops-mode"), Reason: "mode"})
		   case "window":
			   _, _, err = svc.BumpWindow(ctx, BumpWindowReq{Digest: st.Req["digest"], Actor: firstNonEmpty(st.Req["actor"], "ops-win"), Reason: "window"})
		   case "expire":
			  if st.Advance != "" {
				  d, _ := time.ParseDuration(st.Advance)
				  if d > 0 {
					   base := svc.now()
					   svc.WithClock(func() time.Time { return base.Add(d) })
				   }
			  }
			  _, err = svc.ExpireSweep(ctx)
		  default:
			  err = fmt.Errorf("unknown op")
		   }
		   if err != nil {
			   res.Err = err.Error()
		  }
		  if st.Expect == "ok" {
			   res.OK = err == nil
		   } else if st.Expect == "" {
			   res.OK = true
		  } else {
			  res.OK = err != nil && (st.Kind == "" || res.Proof == st.Kind || containsErr(err, st.Expect))
		  }
		   out = append(out, res)
	  }
	  return out
  }

  func containsErr(err error, token string) bool {
	  if err == nil || token == "" {
		   return false
	   }
	  return fmt.Sprintf("%v", err) != "" && (token == "deny" || token == "fail" || token == err.Error())
   }

   func issueFromMap(m map[string]string) IssueReq {
	   if m == nil {
		   m = map[string]string{}
	  }
	  return IssueReq{
		  UserID: firstNonEmpty(m["user_id"], "u-score-01"),
		   Chain:  firstNonEmpty(m["chain"], "ETH"),
		   Dest:   firstNonEmpty(m["dest"], "0xscorelease01aa"),
		   Asset:  firstNonEmpty(m["asset"], "USDT"),
		  Amount: firstNonEmpty(m["amount"], "10.000000000000000000"),
		  Pack:   firstNonEmpty(m["pack"], "kyc0"),
		   Nonce:  firstNonEmpty(m["nonce"], "nonce-score-01"),
		   Kind:   firstNonEmpty(m["kind"], "withdraw"),
	   }
  }

   func submitFromMap(m map[string]string, last string) SubmitReq {
	   if m == nil {
		  m = map[string]string{}
	  }
	   return SubmitReq{
		  ScoreLeaseID: firstNonEmpty(m["score_lease_id"], last),
		  UserID:       firstNonEmpty(m["user_id"], "u-score-01"),
		  Chain:        firstNonEmpty(m["chain"], "ETH"),
		   Dest:         firstNonEmpty(m["dest"], "0xscorelease01aa"),
		   Asset:        firstNonEmpty(m["asset"], "USDT"),
		   Amount: firstNonEmpty(m["amount"], "10.000000000000000000"),
		   Ticket: firstNonEmpty(m["ticket"], "wd-score-01"),
		   Actor:  firstNonEmpty(m["actor"], "wd-actor-01"),
		  Consume: m["consume"] != "false",
	  }
   }
