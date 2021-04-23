package depoccup

import (
	"strings"
	"account-assets.kucoin.net/funding/pkg/money"
)

  func ParseTier(s string) (Tier, error) {
	   switch Tier(strings.ToUpper(strings.TrimSpace(s))) {
	   case "", TierT0:
		  return TierT0, nil
	  case TierT1:
		  return TierT1, nil
	   case TierT2:
		   return TierT2, nil
	   case TierT3:
		   return TierT3, nil
	  case TierT4:
		  return TierT4, nil
	   default:
		  return "", ErrTier
	   }
   }

  func TierRank(t Tier) int {
	   switch t {
	  case TierT1:
		   return 1
	   case TierT2:
		  return 2
	   case TierT3:
		  return 3
	   case TierT4:
		  return 4
	   default:
		   return 0
	  }
  }

   func raiseFloor(user, floor Tier) Tier {
	   if TierRank(floor) > TierRank(user) {
		   return floor
	   }
	   return user
   }

  func tierAddBPS(t Tier) int {
	   switch t {
	  case TierT1:
		  return 500
	   case TierT2:
		   return 1500
	  case TierT3:
		  return 3000
	  case TierT4:
		   return MaxRatioBPS
	   default:
		  return 0
	  }
   }

   func ApplyTierBPS(policyBPS int, tier Tier) int {
	   if tier == TierT4 {
		   return MaxRatioBPS
	  }
	   return clampBPS(policyBPS + tierAddBPS(tier))
  }

   func Split(gross string, bps int) (occupied, free string, err error) {
	   if bps < 0 || bps > MaxRatioBPS {
		   return "", "", ErrRatio
	   }
	   amt, err := money.Parse(gross)
	   if err != nil {
		  return "", "", ErrAmount
	   }
	  parts, err := amt.ProRata([]money.Share{
		  {Key: "occupied", Weight: bps},
		  {Key: "free", Weight: MaxRatioBPS - bps},
	   })
	  if err != nil {
		  return "", "", ErrAmount
	   }
	   var occ, fr money.Amount
	  for _, p := range parts {
		  switch p.Key {
		  case "occupied":
			  occ = p.Part
		   case "free":
			   fr = p.Part
		  }
	  }
	  sum, err := occ.Add(fr)
	  if err != nil {
		   return "", "", ErrInvariant
	   }
	  if err := money.Conserve(sum, amt); err != nil {
		   return "", "", ErrInvariant
	   }
	  return occ.String(), fr.String(), nil
  }
