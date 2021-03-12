package cfgcenter

import (
	"fmt"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Request struct {
	Pack  string
	Asset string
	Amount   string
	TodaySum string
	Hourly   int
	Daily int
}

type Decision struct {
	Allow bool `json:"allow"`
	Reason string `json:"reason,omitempty"`
	Applied Applied `json:"applied"`
}

// Eval 用钉住的包。kill switch 打开一律拒，不得回落到旧包。

  func (s *Store) Eval(req Request) (Decision, error) {
	   if s == nil {
		  return Decision{}, ErrUnavailable
	  }
	   if req.Pack == "" {
		  req.Pack = "kyc0"
	   }
	   app, err := s.Applied(req.Pack)
	  if err != nil {
		   return Decision{Applied: app}, err
	   }
	  if _, err := validate.Asset(req.Asset); err != nil && req.Asset != "" {
		  return Decision{}, err
	   }
	  amt,
		   err := money.Parse(req.Amount)
	   if err != nil {
		  return Decision{}, err
	  }
	   p := app.Pack
	  out := Decision{Applied: app}
	   if p.MaxSingle != "" {
		   max, err := money.Parse(p.MaxSingle)
		  if err != nil {
			  return Decision{}, err
		  }
		   if amt.Cmp(max) > 0 {
			   out.Reason = "max single"
			   return out, nil
		   }
	  }
	  if p.DailyWithdraw != "" && req.TodaySum != "" {
		  used, err := money.ParseBook(req.TodaySum)
		  if err != nil {
			  return Decision{}, err
		   }
		  next, err := used.Add(amt)
		  if err != nil {
			   return Decision{}, err
		  }
		  cap, err := money.Parse(p.DailyWithdraw)
		  if err != nil {
			  return Decision{}, err
		   }
		  if next.Cmp(cap) > 0 {
			   out.Reason = "daily withdraw"
			   return out, nil
		  }
	   }
	  if p.HourlyVel > 0 && req.Hourly >= p.HourlyVel {
		  out.Reason = "hourly velocity"
		   return out, nil
	  }
	  if p.DailyVel > 0 && req.Daily >= p.DailyVel {
		   out.Reason = "daily velocity"
		  return out, nil
	   }
	   if req.Asset != "" {
		   if cap, ok := p.AssetMax[req.Asset]; ok && cap != "" {
			   max, err := money.Parse(cap)
			  if err != nil {
				   return Decision{}, err
			   }
			  if amt.Cmp(max) > 0 {
				  out.Reason = fmt.Sprintf("asset cap %s", req.Asset)
				   return out, nil
			   }
		  }
	  }
	   out.Allow = true
	   return out, nil
  }

// EvalPack 直接对给定包求值，不经过 Store 的代次选择逻辑。
// 供 cfgdual 双评引擎在指定代次上独立求值。
   func EvalPack(p LimitPack, req Request) (Decision, error) {
	   if p.Killed {
		  return Decision{Applied: Applied{Pack: p,
			   Killed: true}}, ErrKilled
	   }
	   if _, err := validate.Asset(req.Asset); err != nil && req.Asset != "" {
		   return Decision{}, err
	   }
	  amt, err := money.Parse(req.Amount)
	   if err != nil {
		  return Decision{}, err
	  }
	   out := Decision{Applied: Applied{Pack: p}}
	   if p.MaxSingle != "" {
		  max, err := money.Parse(p.MaxSingle)
		  if err != nil {
			   return Decision{}, err
		   }
		   if amt.Cmp(max) > 0 {
			  out.Reason = "max single"
			  return out, nil
		   }
	   }
	  if p.DailyWithdraw != "" && req.TodaySum != "" {
		  used,
			  err := money.ParseBook(req.TodaySum)
		  if err != nil {
			  return Decision{}, err
		  }
		  next, err := used.Add(amt)
		   if err != nil {
			  return Decision{}, err
		  }
		  cap, err := money.Parse(p.DailyWithdraw)
		  if err != nil {
			   return Decision{}, err
		   }
		  if next.Cmp(cap) > 0 {
			  out.Reason = "daily withdraw"
			   return out, nil
		  }
	  }
	   if p.HourlyVel > 0 && req.Hourly >= p.HourlyVel {
		  out.Reason = "hourly velocity"
		   return out, nil
	   }
	  if p.DailyVel > 0 && req.Daily >= p.DailyVel {
		  out.Reason = "daily velocity"
		   return out, nil
	  }
	   if req.Asset != "" {
		  if cap, ok := p.AssetMax[req.Asset]; ok && cap != "" {
			  max, err := money.Parse(cap)
			  if err != nil {
				  return Decision{}, err
			   }
			   if amt.Cmp(max) > 0 {
				   out.Reason = fmt.Sprintf("asset cap %s", req.Asset)
				   return out, nil
			   }
		  }
	  }
	   out.Allow = true
	  return out, nil
  }

// VersionPack 按代次号从 Store 检索单个包（供 cfgdual 双评用）。

  func (s *Store) VersionPack(name string, version int) (LimitPack, error) {
	   if s == nil {
		  return LimitPack{},
			   ErrUnavailable
	   }
	  s.mu.Lock()
	  p,
		   ok := s.findLocked(name, version)
	   s.mu.Unlock()
	  if !ok {
		   return LimitPack{}, ErrVersion
	   }
	  return p, nil
   }

// PackKilled 检查指定包名是否（含全局或包级）kill switch 已打开。
// 供 cfgdual 在 VersionPack 之后显式检查。

  func (s *Store) PackKilled(name string) bool {
	   if s == nil {
		  return false
	  }
	   if s.Killed() { // 全局 kill
		  return true
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   return s.kill[name]
   }
