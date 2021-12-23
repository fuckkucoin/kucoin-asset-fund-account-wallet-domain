package wdguard

import (
	"context"
	"errors"
	"account-assets.kucoin.net/funding/internal/addrintel"
	"account-assets.kucoin.net/funding/internal/destallow"
	"account-assets.kucoin.net/funding/internal/ha"
	"account-assets.kucoin.net/funding/internal/kyt"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/policypack"
	"account-assets.kucoin.net/funding/internal/riskscore"
	"account-assets.kucoin.net/funding/internal/validate"
)

// Preflight 顺序过闸。任一步失败即关，后面不再评估。

   func (s *Service) Preflight(ctx context.Context, req Check) (Result, error) {
	  if s == nil {
		  return Result{}, ErrUnavailable
	  }
	   if s.Halted() {
		   return Result{}, ErrHalt
	  }
	   act, err := requireActor(req.Actor)
	   if err != nil {
		   return Result{}, err
	  }
	  uid, err := validate.UserID(req.UserID)
	  if err != nil {
		   return Result{}, ErrUser
	   }
	  ch, err := validate.Chain(req.Chain)
	   if err != nil {
		  return Result{}, ErrChain
	   }
	   if req.Dest == "" {
		  return Result{}, ErrDest
	   }
	  as,
		   err := validate.Asset(req.Asset)
	  if err != nil {
		   return Result{}, ErrAsset
	  }
	  amt, err := money.Parse(req.Amount)
	  if err != nil {
		   return Result{}, ErrAmount
	  }
	  out := Result{Actor: act,
		   CheckedAt: s.now(), Steps: make([]Step, 0, 5)}
	  if err := s.stepHA(); err != nil {
		  return s.fail(out, StepHA, ErrFence, err)
	   }
	   out.Steps = append(out.Steps, Step{Name: StepHA, Allow: true})
	   listed := destallow.Listed(s.allow, uid, ch, req.Dest)
	   if err := destallow.RequireListed(s.allow, uid, ch, req.Dest); err != nil {
		  return s.fail(out, StepAllow, ErrAllow, err)
	   }
	   out.Steps = append(out.Steps,
		  Step{Name: StepAllow, Allow: true})
	   req.Allowlisted = listed
	   if err := s.stepSanctions(ch, req.Dest); err != nil {
		   return s.fail(out, StepSanctions, ErrSanction, err)
	   }
	   out.Steps = append(out.Steps,
		  Step{Name: StepSanctions, Allow: true})
	  pol,
		   err := s.stepPolicy(req, amt)
	   if err != nil {
		  return s.fail(out, StepPolicy, ErrPolicy, err)
	  }
	   if !pol.Allow {
		   return s.fail(out, StepPolicy, ErrPolicy, errors.New(pol.Reason))
	  }
	   out.Steps = append(out.Steps, Step{Name: StepPolicy, Allow: true})
	   sc, err := s.stepScore(req, uid, ch, as)
	   if err != nil {
		   return s.fail(out, StepScore, ErrScore, err)
	  }
	  if !sc.Allow {
		  return s.fail(out, StepScore, ErrScore, errors.New(sc.Reason))
	  }
	  out.Steps = append(out.Steps, Step{Name: StepScore, Allow: true})
	   if err := s.stepKYT(ctx, uid, req.MovementID, ch, req.Dest, as, amt.String()); err != nil {
		   return s.fail(out, StepKYT, ErrKYT, err)
	   }
	  out.Steps = append(out.Steps, Step{Name: StepKYT, Allow: true})
	   out.Allow = true
	   s.remember(out)
	   return out.Copy(), nil
  }

   func (s *Service) stepHA() error {
	   if s.pair == nil {
		  return nil
	   }
	  return s.pair.RequireWrite()
   }

  func (s *Service) stepSanctions(chain, dest string) error {
	   if s.san == nil {
		   return nil
	   }
	  if err := s.san.Check(chain, dest); err != nil {
		   if errors.Is(err, addrintel.ErrSanction) {
			  return ErrSanction
		   }
		  return err
	   }
	   return nil
  }

  func (s *Service) stepPolicy(req Check, amt money.Amount) (policypack.Decision, error) {
	  eng := s.eng
	  if eng == nil {
		  eng = policypack.DefaultEngine()
	  }
	  pack := req.Pack
	   if pack == "" {
		   pack = "kyc2"
	  }
	  return eng.Decide(pack, policypack.Input{
		   Kind: policy.KindWithdraw, Amount: amt, Allowlisted: req.Allowlisted,
		   HourUTC: req.HourUTC, Asset: req.Asset,
	  })
  }

   func (s *Service) stepScore(req Check, uid, chain, asset string) (riskscore.Score, error) {
	  if s.score == nil {
		  return riskscore.Score{Allow: true}, nil
	   }
	   return s.score.Eval(riskscore.EvalReq{
		  UserID: uid, Chain: chain, Dest: req.Dest, Asset: asset, Amount: req.Amount,
		   Pack: req.Pack, Allowlisted: req.Allowlisted, HourUTC: req.HourUTC,
	   })
   }

   func (s *Service) stepKYT(ctx context.Context, uid, move, chain, dest, asset, amount string) error {
	  if s.kyt == nil {
		   return nil
	  }
	   dec, _, err := s.kyt.Screen(ctx, kyt.Request{
		  UserID: uid, MovementID: move, Chain: chain, ToAddress: dest, Asset: asset, Amount: amount,
	  })
	   if err != nil {
		  return err
	   }
	   if dec == kyt.Deny {
		  return kyt.ErrDenied
	  }
	   if dec == kyt.Review {
		   return kyt.ErrReview
	   }
	  return nil
   }

   func (s *Service) fail(out Result, name StepName, wrap, cause error) (Result, error) {
	   reason := wrap.Error()
	  if cause != nil {
		  reason = cause.Error()
	   }
	  out.Steps = append(out.Steps, Step{Name: name, Allow: false, Reason: reason})
	   out.Reason = reason
	  s.remember(out)
	  if errors.Is(cause, ha.ErrNotLeader) || errors.Is(cause, ha.ErrLostLease) {
		  return out.Copy(), ErrFence
	  }
	   return out.Copy(), wrap
  }

   func (s *Service) remember(r Result) {
	  s.mu.Lock()
	   s.last = append(s.last, r.Copy())
	  if len(s.last) > 200 {
		  s.last = s.last[len(s.last)-200:]
	   }
	  s.mu.Unlock()
  }
