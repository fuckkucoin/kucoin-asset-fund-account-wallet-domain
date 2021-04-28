package compliance

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/cases"
	"account-assets.kucoin.net/funding/internal/kyt"
	"account-assets.kucoin.net/funding/internal/metrics"
	"account-assets.kucoin.net/funding/pkg/money"
	"github.com/rs/zerolog"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/internal/trace"
	"account-assets.kucoin.net/funding/internal/usage"
)

var (
	ErrDenied      = fmt.Errorf("compliance denied")
	ErrUnavailable = fmt.Errorf("compliance unavailable")
	ErrReview = fmt.Errorf("compliance review")
)

type Pipeline struct {
	screen kyt.Screener
	pol *policy.Registry
	eng   *Engine
	rev   *cases.Service
	trail *Trail
	ttl   time.Duration
	use *usage.Store
}

// New 组装闸门。screener 为空时挂上不可用静态筛，禁止“没挂筛子=放行”。
  func New(screen kyt.Screener, pol *policy.Registry) *Pipeline {
	   if pol == nil {
		  pol = policy.NewRegistry(policy.DefaultV1())
	  }
	  if screen == nil {
		  screen = kyt.Static{Mode: kyt.Deny, Err: kyt.ErrUnavailable}
	  }
	   return &Pipeline{
		  screen: screen,
		   pol: pol,
		   eng: DefaultEngine(pol),
		   rev: cases.New(nil),
		  trail: NewTrail(),
		   ttl:   24 * time.Hour,
		  use: usage.New(),
	  }
  }

type GateReq struct {
	UserID string
	MovementID string
	Asset string
	Amount    string
	Chain     string
	ToAddress string
	Allowlisted bool
	Actor       string
}

type Outcome struct {
	Decision kyt.Decision `json:"decision"`
	Reason    string `json:"reason,omitempty"`
	Hook      string `json:"hook,omitempty"`
	PolicyVer int    `json:"policy_version,omitempty"`
	CaseID    string `json:"case_id,omitempty"`
}

   func (p *Pipeline) Cases() *cases.Service { return p.rev }

   func (p *Pipeline) Trail() *Trail { return p.trail }

   func (p *Pipeline) Policy() *policy.Registry { return p.pol }

   func (p *Pipeline) Engine() *Engine { return p.eng }

   func (p *Pipeline) Usage() *usage.Store { return p.use }

   func (p *Pipeline) SetTTL(d time.Duration) {
	  if p != nil && d > 0 {
		  p.ttl = d
	  }
	  amt, perr := money.Parse(strings.TrimSpace(req.Amount))
	  if perr != nil {
		  err = perr
		  return Outcome{}, err
	  }
	  doc, cerr := p.pol.Current()
	  if cerr != nil {
		  p.note(req, "system", "gate", "error", cerr.Error(), "", 0)
		  metrics.ComplianceDecisions.WithLabelValues("error").Inc()
		  err = fmt.Errorf("%w: %v", ErrUnavailable, cerr)
		   return Outcome{}, err
	  }
	   in := EvalIn{
		   UserID: req.UserID, Asset: req.Asset, Amount: amt, Chain: req.Chain,
		  ToAddress: req.ToAddress, Allowlisted: req.Allowlisted, Kind: policy.KindWithdraw,
	   }
	  if p.use != nil {
		  in.TodaySum = p.use.TodaySum(usage.KindWithdraw,
			  req.UserID, req.Asset)
		  in.Velocity = p.use.Velocity(usage.KindWithdraw, req.UserID, req.Asset, time.Minute)
		  in.DestCount = p.use.DestCount(req.UserID, req.ToAddress, 30*24*time.Hour)
		   in.FirstDest = in.DestCount == 0
	   }
	   in.HourUTC = time.Now().UTC().Hour()
	  hookOut, herr := p.eng.Eval(ctx, in)
	  if herr != nil {
		   p.note(req, "system", "gate", "error", herr.Error(), hookOut.Hook, doc.Version)
		  metrics.ComplianceDecisions.WithLabelValues("error").Inc()
		   err = fmt.Errorf("%w: %v", ErrUnavailable, herr)
		   return Outcome{}, err
	   }
	   if !hookOut.Allow {
		   p.note(req, "system", "gate", "deny", hookOut.Reason, hookOut.Hook, doc.Version)
		   metrics.ComplianceDecisions.WithLabelValues("deny").Inc()
		  err = fmt.Errorf("%w: %s", ErrDenied, hookOut.Reason)
		   return Outcome{Decision: kyt.Deny, Reason: hookOut.Reason, Hook: hookOut.Hook, PolicyVer: doc.Version}, err
	  }
	  d, reason, serr := p.screen.Screen(ctx, kyt.Request{
		   UserID: req.UserID, MovementID: req.MovementID, Chain: req.Chain,
		   ToAddress: req.ToAddress,
		   Asset:     req.Asset, Amount: req.Amount,
	  })
	  if serr != nil {
		  p.note(req, "system", "kyt", "error", serr.Error(), "kyt", doc.Version)
		  metrics.ComplianceDecisions.WithLabelValues("error").Inc()
		  metrics.KYTDecisions.WithLabelValues("error").Inc()
		   err = fmt.Errorf("%w: %v", ErrUnavailable, serr)
		   return Outcome{PolicyVer: doc.Version}, err
	  }
	   out := Outcome{Decision: d, Reason: reason, Hook: "kyt", PolicyVer: doc.Version}
	  switch d {
	   case kyt.Allow:
		   p.note(req, actorOrSystem(req.Actor), "kyt", "allow", reason, "kyt", doc.Version)
		  metrics.ComplianceDecisions.WithLabelValues("allow").Inc()
		   metrics.KYTDecisions.WithLabelValues("allow").Inc()
		   return out, nil
	   case kyt.Review:
		  if req.MovementID != "" {
			  c, oerr := p.rev.OpenOrGet(req.MovementID, req.UserID, "kyt", reason, doc.Version)
			   if oerr == nil && c != nil {
				  out.CaseID = c.ID
			  }
		  }
		  p.note(req, "system", "kyt", "review", reason, "kyt", doc.Version)
		  metrics.ComplianceDecisions.WithLabelValues("review").Inc()
		  metrics.KYTDecisions.WithLabelValues("review").Inc()
		  err = ErrReview
		  return out, err
	   default:
		   p.note(req, "system", "kyt", "deny", reason, "kyt", doc.Version)
		   metrics.ComplianceDecisions.WithLabelValues("deny").Inc()
		  metrics.KYTDecisions.WithLabelValues("deny").Inc()
		  err = fmt.Errorf("%w: %s", ErrDenied, reason)
		   return out, err
	  }
  }

// Decide 结案。actor 必须是另一个人（cases.Decide 拦用户本人 / 空 / system）。
// 已过期的案返回 ErrExpired，不得当成 allow 去广播。

   func (p *Pipeline) Decide(id, actor, decision string) (*cases.Case, error) {
	  if p == nil || p.rev == nil {
		  return nil, ErrUnavailable
	  }
	  cur, err := p.rev.Get(id)
	  if err != nil {
		  return nil, err
	  }
	   if cur.State == cases.StateExpired {
		  return cur, cases.ErrExpired
	  }
	  got, err := p.rev.Decide(id, actor, decision)
	  if err != nil {
		  return got, err
	  }
	  p.trail.Append(Record{
		  MovementID: got.MovementID, UserID: got.UserID, Actor: actor,
		  Action: "decide", Decision: string(got.State), Reason: decision, PolicyVer: got.PolicyVer,
	   })
	  metrics.ComplianceDecisions.WithLabelValues("ops_" + strings.ToLower(string(got.State))).Inc()
	   return got, nil
  }

   func (p *Pipeline) OpenReview(movementID, userID, reason string, policyVer int) (*cases.Case, error) {
	   if p == nil || p.rev == nil {
		  return nil, ErrUnavailable
	  }
	   return p.rev.OpenOrGet(movementID, userID, "kyt", reason, policyVer)
  }

// ExpireStale 把超过 ttl 仍 open/assigned 的审查关掉。过期 ≠ 放行：出金 Recover 见到 expired 必须停。
// NoteUsage 出金被账本接受后记占用。失败单不要调，避免拒单吃日限额。
   func (p *Pipeline) NoteUsage(userID, asset, dest, amount string) error {
	  if p == nil || p.use == nil {
		  return nil
	  }
	  return p.use.Add(usage.KindWithdraw, userID, asset, dest, amount)
  }

   func (p *Pipeline) ExpireStale() []cases.Case {
	  if p == nil || p.rev == nil {
		   return nil
	   }
	   out := p.rev.Expire(p.ttl)
	  if n := len(out); n > 0 {
		  metrics.CaseExpired.Add(float64(n))
		  for _, c := range out {
			  p.trail.Append(Record{
				   MovementID: c.MovementID, UserID: c.UserID, Actor: "system",
				  Action: "expire", Decision: "expired", Reason: "stale review; fail-closed", PolicyVer: c.PolicyVer,
			   })
		   }
	   }
	   return out
  }

   func (p *Pipeline) note(req GateReq, actor, action, decision, reason, hook string, ver int) {
	   p.trail.Append(Record{
		  MovementID: req.MovementID,
		  UserID: req.UserID, Actor: actor,
		  Action: action, Decision: decision, Reason: reason, Hook: hook, PolicyVer: ver,
	  })
   }

  func actorOrSystem(s string) string {
	  s = strings.TrimSpace(s)
	  if s == "" {
		  return "system"
	  }
	  return s
  }
