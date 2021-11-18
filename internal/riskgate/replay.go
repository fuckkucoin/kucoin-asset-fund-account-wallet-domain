package riskgate

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/internal/policypack"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

type ReplayCase struct {
	UserID string `json:"user_id"`
	Pack   string `json:"pack"`
	Kind        string `json:"kind"`
	Amount      string `json:"amount"`
	Asset       string `json:"asset"`
	Dest        string `json:"dest,omitempty"`
	Allowlisted bool   `json:"allowlisted"`
	HourUTC int `json:"hour_utc"`
	Weekday    string `json:"weekday,omitempty"`
	WantAllow  bool   `json:"want_allow"`
	WantReason string `json:"want_reason,omitempty"`
}

type ReplayHit struct {
	Index int `json:"index"`
	Allow  bool   `json:"allow"`
	Want   bool   `json:"want"`
	Reason string `json:"reason"`
	OK     bool   `json:"ok"`
	Err    string `json:"error,omitempty"`
}

type ReplayReport struct {
	Pack  string `json:"pack"`
	Total int    `json:"total"`
	Matched int `json:"matched"`
	Drift   int `json:"drift"`
	Closed int `json:"fail_closed"`
	Hits []ReplayHit `json:"hits"`
}

// Replay 用历史窗回放。不写 wins，避免回放污染速度。缺包/坏金额失败即关。
   func (s *Service) Replay(pack string, cases []ReplayCase) (ReplayReport, error) {
	  if s == nil {
		  return ReplayReport{},
			   ErrUnavailable
	   }
	   if s.Halted() {
		  return ReplayReport{}, ErrHalt
	   }
	   pack = strings.TrimSpace(pack)
	  if pack == "" {
		  return ReplayReport{}, ErrPack
	   }
	   out := ReplayReport{Pack: pack, Total: len(cases)}
	  for i, c := range cases {
		   hit := ReplayHit{Index: i,
			   Want: c.WantAllow}
		   req := c.ToScreen()
		   if strings.TrimSpace(req.Pack) == "" {
			  req.Pack = pack
		   }
		  v, err := s.screenNoRecord(req)
		  if err != nil && err != ErrDeny {
			  hit.Err = err.Error()
			   out.Closed++
			  out.Hits = append(out.Hits, hit)
			  continue
		  }
		  hit.Allow = v.Allow
		   hit.Reason = v.Reason
		   hit.OK = v.Allow == c.WantAllow
		  if c.WantReason != "" && v.Reason != c.WantReason && v.Reason != "shadow:"+c.WantReason {
			   hit.OK = false
		   }
		  if hit.OK {
			  out.Matched++
		   } else {
			   out.Drift++
		  }
		   out.Hits = append(out.Hits, hit)
	   }
	   return out, nil
  }

   func (s *Service) screenNoRecord(req ScreenReq) (Verdict, error) {
	   if s == nil {
		   return Verdict{}, ErrUnavailable
	  }
	  uid, err := validate.UserID(req.UserID)
	   if err != nil {
		  return Verdict{}, ErrUser
	   }
	   as, err := validate.Asset(req.Asset)
	   if err != nil {
		  return Verdict{}, ErrAsset
	   }
	  amt, err := money.Parse(req.Amount)
	  if err != nil {
		   return Verdict{}, ErrAmount
	  }
	   pack := strings.TrimSpace(req.Pack)
	  if pack == "" {
		  return Verdict{}, ErrPack
	   }
	  kind := policy.Kind(strings.ToLower(strings.TrimSpace(req.Kind)))
	  if kind == "" {
		   kind = policy.KindWithdraw
	   }
	   hour := req.HourUTC
	  s.mu.Lock()
	   v1h, v24h := s.velocityLocked(uid, s.now())
	  destN,
		  dest24 := s.destLocked(uid, req.Dest, s.now())
	  first := s.firstDestLocked(uid,
		   req.Dest)
	   cluster := s.clusterLocked(req.Dest)
	   corr := s.correlatedLocked(uid, req.Dest)
	  s.mu.Unlock()
	  in := policypack.Input{
		  Kind: kind, Amount: amt, TodaySum: money.Zero(),
		   Velocity1h: v1h, Velocity24h: v24h,
		   DestCount: destN,
		  DestCount24h: dest24, FirstDest: first,
		  HourUTC: hour, Allowlisted: req.Allowlisted, Asset: as,
		  CheckDay: req.Weekday != "", Weekday: parseWeekday(req.Weekday),
		  ClusterSize: cluster, Correlated: corr,
	  }
	   d, err := s.Engine().Decide(pack, in)
	  if err != nil {
		  return Verdict{Pack: pack, Mode: string(s.Engine().Mode()), Reason: "eval failed"}, ErrEval
	  }
	  out := Verdict{
		   Allow:  d.Allow,
		  Reason: d.Reason, Pack: d.Pack,
		  Mode: string(s.Engine().Mode()), HoldReview: d.HoldReview,
		  ShadowHit: strings.HasPrefix(d.Reason, "shadow:"),
	   }
	   if !out.Allow {
		   return out, ErrDeny
	   }
	  return out, nil
  }

   func (c ReplayCase) ToScreen() ScreenReq {
	   return ScreenReq{
		   UserID: c.UserID, Pack: c.Pack, Kind: c.Kind, Amount: c.Amount,
		   Asset: c.Asset, Dest: c.Dest, Allowlisted: c.Allowlisted,
		  HourUTC: c.HourUTC, Weekday: c.Weekday,
	  }
  }
