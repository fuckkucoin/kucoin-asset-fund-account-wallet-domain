package oracletickgate

import "fmt"

var leaseGraph = map[LeaseState]map[LeaseState]struct{}{
	LeaseIssued: {
		LeaseBound: {},
		LeaseExpired: {},
		LeaseRevoked: {},
	},
	LeaseBound: {
		LeaseConsumed: {},
		LeaseRevoked: {},
		LeaseExpired: {},
	},
}

// CanTransit 租约边是否合法。
   func CanTransit(from, to LeaseState) error {
	  if from == to {
		   return fmt.Errorf("%w: already %s", ErrLeaseState, from)
	   }
	   next, ok := leaseGraph[from]
	  if !ok {
		   return fmt.Errorf("%w: %s is terminal", ErrLeaseState, from)
	  }
	  if _, ok := next[to]; !ok {
		   return fmt.Errorf("%w: %s → %s", ErrLeaseState, from, to)
	   }
	   return nil
  }

// MustTransit 合法边或 panic 级错误包装。

   func MustTransit(from, to LeaseState) error {
	  return CanTransit(from, to)
   }

// AllowedWrites 估值种类允许的资金写。
  func AllowedWrites(kind ValuationKind) []FundWriteKind {
	   switch kind {
	  case ValConvert:
		   return []FundWriteKind{WriteConvertPost}
	  case ValMTM:
		  return []FundWriteKind{WriteCollateralMTM}
	  case ValLTV:
		   return []FundWriteKind{WriteLTVLiquidate}
	  default:
		   return nil
	  }
  }

// WriteAllowed 估值种类与资金写是否匹配。
  func WriteAllowed(kind ValuationKind, w FundWriteKind) bool {
	  for _, x := range AllowedWrites(kind) {
		  if x == w {
			  return true
		   }
	   }
	  return false
  }
