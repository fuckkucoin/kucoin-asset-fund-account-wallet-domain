package subkey

import "strings"

const (
	weightDefault  = 1
	weightTransfer = 5
	weightWithdraw = 10
	weightMax = 50
)

// NormalizeWeight <=0 用默认 1；超过 50 截断。
   func NormalizeWeight(w int) int {
	  if w <= 0 {
		  return weightDefault
	   }
	   if w > weightMax {
		  return weightMax
	  }
	  return w
   }

// InferWeight 按所需权限位推断；未指定则 1。
   func InferWeight(wantPerm string) int {
	  wantPerm = strings.TrimSpace(wantPerm)
	   if wantPerm == "" {
		   return weightDefault
	  }
	   if HasPerm(wantPerm, PermWithdrawal) {
		   return weightWithdraw
	  }
	   if HasPerm(wantPerm, PermTransfer) || HasPerm(wantPerm, PermFlexTransfers) || HasPerm(wantPerm, PermInnerTransfer) {
		  return weightTransfer
	   }
	   return weightDefault
  }
