package marginhook

import (
	"fmt"
)

type ScanReport struct {
	Users   int `json:"users"`
	Checked int `json:"checked"`
	Frozen int `json:"frozen"`
	Inside int `json:"inside"`
	Errs   int `json:"errors"`
}

// Scan 扫开仓借款的 LTV。超 cap 才 FreezeForLTV；闸内不动。
// 单笔冻失败记 errors，不中断整轮。
   func (s *Service) Scan() ScanReport {
	   out := ScanReport{}
	   if s == nil {
		   return out
	  }
	  seen := map[string]struct{}{}
	   for _, r := range s.book.OpenBorrows() {
		   if _, ok := seen[r.UserID]; !ok {
			   seen[r.UserID] = struct{}{}
			   out.Users++
		  }
		  out.Checked++
		   h, ok := s.book.Hold(r.Collateral)
		   if !ok {
			   out.Errs++
			  continue
		   }
		  if err := CheckLTV(r.Amount, h.Amount, h.CapBPS); err == nil {
			   out.Inside++
			  continue
		  }
		  if _, err := s.FreezeForLTV(r.UserID, r.ID); err != nil {
			   out.Errs++
			  continue
		   }
		  out.Frozen++
	   }
	   return out
  }

   func (s *Service) JobString() string {
	  if s == nil {
		   return "margin-scan<nil>"
	   }
	   return fmt.Sprintf("margin-scan users=%d open=%d", len(s.book.Users()), len(s.book.OpenBorrows()))
  }
