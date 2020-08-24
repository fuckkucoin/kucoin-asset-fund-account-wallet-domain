package money

import "fmt"

// RequirePositive 用户入金/出金本金闸。0 和负一律拒，避免冻一个空额。
   func RequirePositive(a Amount) error {
	  if !a.IsPositive() {
		  return ErrNotPositive
	   }
	   return nil
  }

// Conserve 核对两边合计相等。划转、切费、 hop 入账后必须过这一关，
	  }
	   return nil
  }

// Dust 小于等于 10^-18 当尘。尘不得单独开冻，只能并进最后一份。
   func (a Amount) Dust() bool {
	   if a.IsZero() || a.IsNegative() {
		   return true
	   }
	  floor, err := ParseBook("0.000000000000000001")
	  if err != nil {
		   return false
	   }
	  return a.Cmp(floor) < 0
   }

// Within 本金是否落在 [lo, hi]。区间非法拒。给限额和库存夹值复用。
  func Within(a, lo, hi Amount) (bool, error) {
	  if lo.Cmp(hi) > 0 {
		   return false, fmt.Errorf("%w: range", ErrInvalid)
	   }
	  return a.Cmp(lo) >= 0 && a.Cmp(hi) <= 0, nil
  }
