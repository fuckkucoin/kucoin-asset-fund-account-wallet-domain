package vipfee

import (
	"fmt"
	"strings"
)

  func ParseTier(s string) (Tier, error) {
	   switch Tier(strings.ToLower(strings.TrimSpace(s))) {
	   case TierKYC0:
		   return TierKYC0, nil
	   case TierStandard:
		  return TierStandard, nil
	  case TierVIP:
		  return TierVIP, nil
	   case "vip+", "vipplus", "vip_plus":
		   return TierVIPPlus, nil
	   default:
		   return "", fmt.Errorf("%w: %s", ErrTier, s)
	  }
  }

  func ParseKind(s string) (Kind, error) {
	  switch Kind(strings.ToLower(strings.TrimSpace(s))) {
	   case KindMaker, KindTaker, KindRebate, KindWithdraw:
		   return Kind(strings.ToLower(strings.TrimSpace(s))), nil
	   default:
		  return "", fmt.Errorf("%w: %s", ErrKind, s)
	  }
   }

  func scheduleKey(tier Tier, kind Kind, asset string) string {
	  asset = strings.ToUpper(strings.TrimSpace(asset))
	   if asset == "" {
		   asset = "*"
	  }
	  return string(tier) + "|" + string(kind) + "|" + asset
   }

   func validBPS(bps int) error {
	   if bps < 0 || bps > 10000 {
		   return ErrBPS
	  }
	   return nil
  }
