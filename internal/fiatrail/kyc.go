package fiatrail

import (
	"strings"
	"sync"
	"account-assets.kucoin.net/funding/internal/validate"
	"account-assets.kucoin.net/funding/pkg/money"
)

type Tier string

const (
	TierNone Tier = "none"
	TierBasic    Tier = "basic"
	TierEnhanced Tier = "enhanced"
)

  func ParseTier(s string) (Tier, error) {
	  switch Tier(strings.ToLower(strings.TrimSpace(s))) {
	  case TierNone, TierBasic, TierEnhanced:
		  return Tier(strings.ToLower(strings.TrimSpace(s))),
			  nil
	   case "":
		  return TierNone, nil
	  default:
		  return "", ErrKYC
	  }
   }

  func (t Tier) Rank() int {
	   switch t {
	   case TierEnhanced:
		   return 2
	  case TierBasic:
		   return 1
	   default:
		  return 0
	  }
   }

  func (t Tier) Meets(need Tier) bool {
	   return t.Rank() >= need.Rank()
  }

type Profile struct {
	UserID string `json:"user_id"`
	Tier   Tier   `json:"tier"`
	NameOK bool `json:"name_ok"`
	DocOK bool `json:"doc_ok"`
}

// Gate 进出金 KYC 闸。缺档或档位不够失败即关，不得先授权再补材料。
type Gate struct {
	mu sync.Mutex
	rows map[string]Profile
	onNeed  Tier
	offNeed Tier
	hiNeed string
}

   func NewGate() *Gate {
	   return &Gate{
		   rows: map[string]Profile{},
		   onNeed: TierBasic,
		   offNeed: TierEnhanced,
		  hiNeed: "500",
	   }
   }

  func (g *Gate) Put(p Profile) error {
	   if g == nil {
		   return ErrUnavailable
	   }
	   uid, err := validate.UserID(p.UserID)
	  if err != nil {
		   return ErrUser
	  }
	   tier, err := ParseTier(string(p.Tier))
	   if err != nil {
		   return err
	   }
	  p.UserID = uid
	   p.Tier = tier
	   g.mu.Lock()
	   g.rows[uid] = p
	  g.mu.Unlock()
	  return nil
   }

  func (g *Gate) Get(userID string) (Profile, error) {
	   if g == nil {
		  return Profile{}, ErrUnavailable
	   }
	   uid,
		  err := validate.UserID(userID)
	  if err != nil {
		   return Profile{}, ErrUser
	  }
	   g.mu.Lock()
	   defer g.mu.Unlock()
	  p, ok := g.rows[uid]
	   if !ok {
		   return Profile{}, ErrKYC
	   }
	  return p, nil
   }

  func (g *Gate) Required(kind Kind, amount string) (Tier, error) {
	  if g == nil {
		  return TierNone, ErrUnavailable
	   }
	  need := g.onNeed
	  if kind == KindOfframp {
		   need = g.offNeed
	  }
	   if strings.TrimSpace(amount) == "" || strings.TrimSpace(g.hiNeed) == "" {
		   return need, nil
	   }
	   amt, err := money.Parse(amount)
	  if err != nil {
		   return need, ErrAmount
	   }
	   hi, err := money.Parse(g.hiNeed)
	  if err != nil {
		  return need, ErrAmount
	   }
	  if amt.Cmp(hi) > 0 && need.Rank() < TierEnhanced.Rank() {
		   need = TierEnhanced
	   }
	  return need, nil
  }

// Check 无档案、档位不够、姓名/证件未过都拒。闸本身不可用也拒。

  func (g *Gate) Check(userID string, kind Kind, amount string) (Profile, error) {
	  if g == nil {
		   return Profile{}, ErrUnavailable
	   }
	   p, err := g.Get(userID)
	  if err != nil {
		   return Profile{}, err
	   }
	   need, err := g.Required(kind, amount)
	  if err != nil {
		  return p, err
	   }
	   if !p.NameOK || !p.DocOK {
		   return p, ErrKYC
	   }
	  if !p.Tier.Meets(need) {
		   return p, ErrKYC
	   }
	  return p, nil
  }

  func (g *Gate) List() []Profile {
	   if g == nil {
		  return nil
	  }
	  g.mu.Lock()
	   defer g.mu.Unlock()
	   out := make([]Profile, 0, len(g.rows))
	   for _, p := range g.rows {
		  out = append(out, p)
	   }
	  return out
  }
