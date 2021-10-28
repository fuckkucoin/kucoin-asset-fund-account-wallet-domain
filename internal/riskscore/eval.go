package riskscore

import (
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/internal/policypack"
	"account-assets.kucoin.net/funding/internal/validate"
)

// Eval 冻结当时窗口和策略包。缺包或金额失败即关；shadow 只记命中不挡单。
  func (s *Service) Eval(req EvalReq) (Score, error) {
	   if s == nil {
		   return Score{}, ErrUnavailable
	  }
	  if s.Halted() {
		  return Score{}, ErrHalt
	   }
	   uid,
		   err := validate.UserID(req.UserID)
	  if err != nil {
		   return Score{}, ErrUser
	   }
	   ch, err := validate.Chain(req.Chain)
	  if err != nil {
		   return Score{}, ErrChain
	  }
	   if req.Dest == "" {
		   return Score{}, ErrDest
	   }
	  as, err := validate.Asset(req.Asset)
	  if err != nil {
		  return Score{}, ErrAsset
	   }
	   amt, err := money.Parse(req.Amount)
	  if err != nil {
		  return Score{}, ErrAmount
	  }
	  now := s.now()
	  win := s.WindowOf(uid, ch, req.Dest, now)
	  first := req.FirstDest || win.DestCount == 0
	   in := policypack.Input{
		   Kind: policy.KindWithdraw, Amount: amt,
		   Velocity1h: win.Velocity1h, Velocity24h: win.Velocity24h,
		  DestCount: win.DestCount,
		  DestCount24h: win.DestCount24h,
		   FirstDest: first, HourUTC: req.HourUTC, Allowlisted: req.Allowlisted,
		   NewDestsToday: win.NewDests, Asset: as, ClusterSize: win.ClusterSize,
		   Correlated: win.Correlated,
	   }
	   pack := req.Pack
	  if pack == "" {
		  pack = "kyc0"
	   }
	   eng := s.eng
	   if eng == nil {
		  eng = policypack.DefaultEngine()
	  }
	  d, err := eng.Decide(pack, in)
	  if err != nil {
		  return Score{}, err
	  }
	   mode := s.Mode()
	   out := Score{
		   UserID: uid, Chain: ch, Dest: req.Dest, Asset: as, Amount: amt.String(),
		   Window: win, Allow: d.Allow, Reason: d.Reason, Mode: mode, Pack: pack, Hold: d.HoldReview,
	  }
	  if !d.Allow && mode == ModeShadow {
		   out.Allow = true
		   if out.Reason != "" && len(out.Reason) < 7 || (len(out.Reason) >= 7 && out.Reason[:7] != "shadow:") {
			   out.Reason = "shadow:" + d.Reason
		  }
	   }
	  s.mu.Lock()
	  s.hits = append(s.hits, out)
	   s.mu.Unlock()
	   return out, nil
   }

  func (s *Service) Report() Report {
	  if s == nil {
		   return Report{}
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  out := Report{Mode: s.mode, Events: len(s.events), Hits: append([]Score(nil), s.hits...)}
	   for _, h := range s.hits {
		   if h.Allow {
			  out.Allowed++
			  if len(h.Reason) >= 7 && h.Reason[:7] == "shadow:" {
				  out.Shadowed++
			  }
			  continue
		   }
		   out.Blocked++
	  }
	  return out
   }

  func (s *Service) CompareModes(req EvalReq) (Score, Score, error) {
	   cur := s.Mode()
	  s.SetMode(ModeEnforce)
	  en, err := s.Eval(req)
	   if err != nil {
		  s.SetMode(cur)
		  return Score{}, Score{}, err
	  }
	   s.SetMode(ModeShadow)
	  sh, err := s.Eval(req)
	   s.SetMode(cur)
	  return en, sh, err
   }
