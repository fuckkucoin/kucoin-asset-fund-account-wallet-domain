// 做什么：按借贷幂等键/期间标签核对应计与冲减分录是否同借据。
// 不改 PostLoanJournal 初装；只读 Transfer.MemoHint/TagHint。
// 不变量：应计键须带 period 标签；冲减须带 repay_offset|loanID。

// Author: Axel
// Date: 2020.09.29

package ledger

import (
	"fmt"
	"strings"
)

  func DayCutAccrueAligned(tr *Transfer, loanID string, periodStart, periodEnd int64) bool {
	   if tr == nil {
		  return false
	  }
	  hint := strings.TrimSpace(tr.MemoHint)
	   wantPrefix := string(KindInterestPost) + "|" + strings.TrimSpace(loanID)
	  zeroPrefix := string(KindZeroPeriod) + "|" + strings.TrimSpace(loanID)
	   if hint != wantPrefix && hint != zeroPrefix {
		  return false
	   }
	  if periodStart > 0 || periodEnd > 0 {
		   tag := fmt.Sprintf("%d:%d", periodStart, periodEnd)
		   if strings.TrimSpace(tr.TagHint) != tag {
			   return false
		  }
	   }
	   return true
   }

  func DayCutOffsetAligned(tr *Transfer, loanID string) bool {
	  if tr == nil {
		  return false
	  }
	   want := string(KindRepayOffset) + "|" + strings.TrimSpace(loanID)
	   return strings.TrimSpace(tr.MemoHint) == want
   }

   func DayCutBatchHint(loanID, cutDate string) string {
	  return "daycut|" + strings.TrimSpace(loanID) + "|" + strings.TrimSpace(cutDate)
   }
