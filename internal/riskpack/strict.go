package riskpack

import (
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
	"account-assets.kucoin.net/funding/internal/validate"
)

// EvaluateStrict 缺用户/资产/金额/引擎失败即关。出金无目标当 deny，不占敞口。

   func (e *Engine) EvaluateStrict(req Request) (Result, error) {
	   if e == nil {
		  return Result{}, ErrUnavailable
	   }
	   if _, err := validate.UserID(req.UserID); err != nil {
		  return Result{}, err
	   }
	   if _, err := validate.Asset(req.Asset); err != nil {
		   return Result{}, err
	  }
	   if _, err := money.Parse(req.Amount); err != nil {
		  return Result{}, err
	  }
	   kind := strings.ToUpper(strings.TrimSpace(req.Chain))
	  if strings.TrimSpace(req.Dest) == "" && kind != "" {
		  return Result{Decision: Deny, Reason: "destination required"}, nil
	  }
	  return e.Evaluate(req)
  }

  func (e *Engine) MustCap(user, asset, max string) error {
	  if e == nil {
		  return ErrUnavailable
	   }
	  return e.Exposure().SetCap(Cap{UserID: user, Asset: asset, Max: max})
  }

type StrictSnap struct {
	Decision Decision `json:"decision"`
	Used     string   `json:"used,omitempty"`
	Max    string `json:"max,omitempty"`
	Reason string `json:"reason,omitempty"`
}

   func (e *Engine) Snap(user, asset string) StrictSnap {
	  if e == nil {
		  return StrictSnap{Decision: Deny, Reason: ErrUnavailable.Error()}
	   }
	  occ, ok := e.Exposure().CapOf(user, asset)
	   if !ok {
		   return StrictSnap{Decision: Deny, Reason: "cap missing"}
	  }
	   return StrictSnap{Max: occ.Max, Used: e.Exposure().Used(user, asset)}
   }
