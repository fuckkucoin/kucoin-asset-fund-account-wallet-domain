package wdguard

import (
	"fmt"
	"account-assets.kucoin.net/funding/internal/destallow"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/policypack"
	"account-assets.kucoin.net/funding/internal/validate"
)

type Link struct {
	List *destallow.Versioned
	Pins *policy.PinBook
	Eng  *policypack.Engine
}

  func NewLink() *Link {
	  return &Link{List: destallow.NewVersioned(), Pins: policy.NewPinBook(), Eng: policypack.DefaultEngine()}
   }

type LinkResult struct {
	Allow bool `json:"allow"`
	Reason  string `json:"reason,omitempty"`
	ListGen int64  `json:"list_generation"`
	PolicyVer int  `json:"policy_version"`
	ShadowHit bool `json:"shadow_hit,omitempty"`
	MovementID string `json:"movement_id,omitempty"`
}

// CheckLinked 出金同时钉白名单世代和策略代次。shadow 只记命中，enforce 挡单。

  func (l *Link) CheckLinked(req Check, pinGen int64) (LinkResult, error) {
	   out := LinkResult{MovementID: req.MovementID}
	   if l == nil || l.List == nil || l.Pins == nil {
		   return out, ErrUnavailable
	  }
	  uid,
		  err := validate.UserID(req.UserID)
	   if err != nil {
		  return out, ErrUser
	  }
	   ch,
		  err := validate.Chain(req.Chain)
	   if err != nil {
		   return out, ErrChain
	  }
	  as, err := validate.Asset(req.Asset)
	  if err != nil {
		   return out, ErrAsset
	  }
	  amt, err := money.Parse(req.Amount)
	   if err != nil {
		  return out, ErrAmount
	  }
	   if err := l.List.RequirePinned(uid, ch, req.Dest, pinGen); err != nil {
		   out.Reason = ErrAllow.Error()
		   return out, ErrAllow
	  }
	   out.ListGen = l.List.Generation()
	   eng := l.Eng
	   if eng == nil {
		   eng = policypack.DefaultEngine()
	   }
	  pack := req.Pack
	  if pack == "" {
		  pack = "kyc2"
	  }
	  dec, err := eng.Decide(pack, policypack.Input{
		  Kind: policy.KindWithdraw, Amount: amt, Allowlisted: true, Asset: as, HourUTC: req.HourUTC,
	   })
	   if err != nil {
		   out.Reason = err.Error()
		  return out, ErrPolicy
	  }
	   out.PolicyVer = dec.Version
	   if req.MovementID != "" {
		   if err := l.Pins.Bind(req.MovementID, dec.Version); err != nil {
			   out.Reason = err.Error()
			   return out, ErrPolicy
		  }
	  }
	   if !dec.Allow {
		  if eng.Mode() == policypack.ModeShadow {
			   out.Allow = true
			   out.ShadowHit = true
			  out.Reason = dec.Reason
			  return out, nil
		   }
		   out.Reason = dec.Reason
		   return out, ErrPolicy
	  }
	  out.Allow = true
	   return out, nil
   }

   func (l *Link) MustPin(movementID string, live int) error {
	   if l == nil || l.Pins == nil {
		   return ErrUnavailable
	  }
	   if err := l.Pins.Must(movementID, live); err != nil {
		  return fmt.Errorf("%w: %s", ErrPolicy, err.Error())
	   }
	   return nil
   }
