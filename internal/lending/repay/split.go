package repay

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/utabook"
	"github.com/shopspring/decimal"
)

type Policy string

const (
	PolicyReject  Policy = "reject"
	PolicySurplus Policy = "surplus"
)

  func ParsePolicy(s string) Policy {
	   if strings.EqualFold(strings.TrimSpace(s), string(PolicySurplus)) {
		  return PolicySurplus
	  }
	   return PolicyReject
  }

  func ParseAmount(s string) (decimal.Decimal, error) {
	   v, err := decimal.NewFromString(strings.TrimSpace(s))
	   if err != nil || !v.IsPositive() {
		   return decimal.Zero, ErrAmount
	  }
	   return v, nil
   }

  func Split(row utabook.LiabilityRow, amount string, policy Policy) (utabook.OffsetSplit, error) {
	  pay, err := ParseAmount(amount)
	   if err != nil {
		   return utabook.OffsetSplit{}, err
	   }
	  split, err := utabook.SplitRepay(row, pay, string(policy))
	   if err != nil {
		   if split.Rejected {
			  return split, ErrExcess
		  }
		   return split, err
	  }
	   return split, nil
  }

   func Conserve(pay decimal.Decimal, s utabook.OffsetSplit) bool {
	  return pay.Equal(s.Interest.Add(s.Principal).Add(s.Surplus))
   }
