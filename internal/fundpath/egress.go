package fundpath

import (
	"context"
	"strings"
	"account-assets.kucoin.net/funding/internal/kyt"
	"account-assets.kucoin.net/funding/internal/travelvasp"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/internal/wdguard"
	"account-assets.kucoin.net/funding/pkg/money"
)

   func (s *Service) Egress(ctx context.Context,
	  req EgressReq) ([]Step, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	   if s.Halted() {
		  return nil, ErrHalt
	  }
	  uid, err := validate.UserID(req.UserID)
	  if err != nil {
		  return nil, ErrUser
	  }
	   ch, err := validate.Chain(req.Chain)
	   if err != nil {
		   return nil, ErrChain
	  }
	   as, err := validate.Asset(req.Asset)
	  if err != nil {
		   return nil, ErrAsset
	   }
	   if _, err := money.Parse(req.Amount); err != nil {
		  return nil, ErrAmount
	   }
	  dest, err := validate.Address(ch, req.Dest)
	  if err != nil {
		   return nil, ErrAddr
	  }
	  out := make([]Step, 0, 5)
	  if err := s.allow.RequireStrict(uid, ch, dest); err != nil {
		   out = append(out, Step{Name: StepAllow, Allow: false, Reason: err.Error()})
		   return out, ErrAllow
	  }
	   out = append(out,
		   Step{Name: StepAllow, Allow: true})
	   if s.kyt != nil {
		   got, err := s.kyt.Screen(ctx, kyt.Request{
			   UserID: uid, MovementID: req.MovementID, Chain: ch,
			   ToAddress: dest,
			  Asset:     as, Amount: req.Amount,
		   })
		   if err != nil {
			  out = append(out, Step{Name: StepKYT, Allow: false, Reason: err.Error()})
			   return out, ErrKYT
		  }
		  if got.Decision == kyt.Deny || got.Decision == kyt.Review {
			  out = append(out, Step{Name: StepKYT, Allow: false, Reason: string(got.Decision)})
			  return out, ErrKYT
		   }
		  out = append(out, Step{Name: StepKYT, Allow: true, Ref: string(got.Decision)})
	  }
	   actor := strings.TrimSpace(req.Actor)
	   gate, err := s.travel.GateOutbound(ctx, travelvasp.OutboundReq{
		  UserID: uid, MovementID: req.MovementID, Actor: actor,
		   Asset: as, Amount: req.Amount, Chain: ch, Dest: dest, PeerCode: req.VASP,
		   Originator: travelvasp.Party{Name: uid,
			  Account: uid},
		  Beneficiary: travelvasp.Party{Name: dest, Account: dest},
	   }, s.travelThresh)
	   if err != nil {
		   out = append(out, Step{Name: StepTravel, Allow: false, Reason: err.Error()})
		  return out, ErrTravel
	   }
	   if gate.Required && !gate.AllowBroadcast {
		   out = append(out, Step{Name: StepTravel, Allow: false, Reason: gate.Reason, Ref: gate.Case.ID})
		   return out, ErrTravel
	   }
	   ref := gate.Case.ID
	  if ref == "" {
		  ref = "below-threshold"
	   }
	  out = append(out, Step{Name: StepTravel, Allow: true, Ref: ref})
	   if s.wd != nil {
		   pf, err := s.wd.Preflight(ctx, wdguard.Check{
			  UserID: uid, Chain: ch, Dest: dest, Asset: as, Amount: req.Amount,
			   Actor: actor, MovementID: req.MovementID,
			  Pack: "kyc2", HourUTC: 12,
		   })
		  if err != nil {
			  out = append(out, Step{Name: StepWD, Allow: false, Reason: err.Error()})
			  return out, ErrWithdraw
		   }
		  if !pf.Allow {
			  out = append(out, Step{Name: StepWD, Allow: false, Reason: pf.Reason})
			   return out, ErrWithdraw
		   }
		  out = append(out, Step{Name: StepWD, Allow: true})
	  }
	   if s.hot != nil && s.store != nil {
		  if err := s.hot.Check(ctx, s.store, as); err != nil {
			   out = append(out, Step{Name: StepHot, Allow: false, Reason: err.Error()})
			   return out, ErrHot
		   }
		  out = append(out, Step{Name: StepHot, Allow: true})
	  }
	   return out, nil
  }
