package riskgate

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

// ScreenStrict 比 Screen 更严：缺包、缺目标、坏金额、halt、空引擎一律拒。
   func (s *Service) ScreenStrict(req ScreenReq) (Verdict, error) {
	  if s == nil {
		   return Verdict{}, ErrUnavailable
	  }
	   if s.Halted() {
		  return Verdict{Reason: "halted"}, ErrHalt
	   }
	   if s.eng == nil {
		  return Verdict{Reason: "engine missing"}, ErrEval
	  }
	   if strings.TrimSpace(req.Pack) == "" {
		  return Verdict{Reason: "pack required"}, ErrPack
	   }
	  if _, err := validate.UserID(req.UserID); err != nil {
		   return Verdict{}, ErrUser
	  }
	   if _, err := validate.Asset(req.Asset); err != nil {
		   return Verdict{}, ErrAsset
	  }
	   if _, err := money.Parse(req.Amount); err != nil {
		   return Verdict{}, ErrAmount
	  }
	   kind := strings.ToLower(strings.TrimSpace(req.Kind))
	   if kind == "" || kind == "withdraw" {
		  if strings.TrimSpace(req.Dest) == "" && !req.Allowlisted {
			   return Verdict{Pack: req.Pack, Reason: "dest required"}, ErrDeny
		   }
	   }
	   return s.Screen(req)
  }

// DefaultsClosed 缺引擎或 halt 时不得放行。
   func (s *Service) DefaultsClosed() bool {
	   if s == nil || s.eng == nil {
		   return true
	  }
	   return s.Halted()
   }

type PackSnap struct {
	Name string `json:"name"`
	Mode string `json:"mode"`
}

   func (s *Service) Packs() []PackSnap {
	   if s == nil || s.eng == nil {
		   return nil
	   }
	   names := s.eng.Catalog().Names()
	   out := make([]PackSnap, 0, len(names))
	  mode := string(s.eng.Mode())
	  for _, n := range names {
		  out = append(out, PackSnap{Name: n, Mode: mode})
	  }
	  return out
  }
