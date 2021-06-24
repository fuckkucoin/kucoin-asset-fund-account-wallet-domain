package fundguard

import (
	"account-assets.kucoin.net/funding/pkg/money"
)

// PartialAmt 升格后的部分批次摘要，包含金额维度。

type PartialAmt struct {
	Submitted int `json:"submitted"`
	Skipped   int `json:"skipped"`
	Failed int `json:"failed"`
	Total        int    `json:"total"`
	SubmittedAmt string `json:"submitted_amt"`
	SkippedAmt string `json:"skipped_amt"`
	FailedAmt     string `json:"failed_amt"`
	ExpectedTotal string `json:"expected_total"`
}

// PartialAmtResult 升格守恒的结果摘要。

type PartialAmtResult struct {
	CountOK bool `json:"count_ok"`
	AmountOK bool `json:"amount_ok"`
	ActualTotal   string `json:"actual_total,omitempty"`
	ExpectedTotal string `json:"expected_total,omitempty"`
	Delta         string `json:"delta,omitempty"`
}

// CheckPartialAmt 升格部分批次守恒：计数 + 金额双重核验。
// 返回结果摘要；任何失衡均返回对应错误。
// 与原 CheckPartial 的区别：
// - 计数不平衡：返回 ErrPartial（语义不变）；
// - 计数平衡但金额失衡：返回 ErrPartialAmt + PartialAmtResult 摘要。

   func (s *Service) CheckPartialAmt(p PartialAmt) (PartialAmtResult, error) {
	   // 计数守恒（与 CheckPartial 一致）
	  if p.Total < 0 || p.Submitted < 0 || p.Skipped < 0 || p.Failed < 0 {
		   return PartialAmtResult{CountOK: false},
			  ErrPartial
	  }
	  if p.Submitted+p.Skipped+p.Failed != p.Total {
		  return PartialAmtResult{CountOK: false}, ErrPartial
	  }
	   // 金额守恒
	  submittedAmt, err := money.ParseBook(orZ(p.SubmittedAmt))
	   if err != nil {
		  return PartialAmtResult{CountOK: true},
			  ErrAmount
	   }
	  skippedAmt, err := money.ParseBook(orZ(p.SkippedAmt))
	   if err != nil {
		  return PartialAmtResult{CountOK: true}, ErrAmount
	  }
	   failedAmt,
		  err := money.ParseBook(orZ(p.FailedAmt))
	   if err != nil {
		  return PartialAmtResult{CountOK: true},
			  ErrAmount
	  }
	   expectedTotal, err := money.ParseBook(orZ(p.ExpectedTotal))
	  if err != nil {
		   return PartialAmtResult{CountOK: true}, ErrAmount
	  }
	   t1, err := submittedAmt.Add(skippedAmt)
	   if err != nil {
		  return PartialAmtResult{CountOK: true}, ErrAmount
	   }
	  actualTotal, err := t1.Add(failedAmt)
	  if err != nil {
		  return PartialAmtResult{CountOK: true}, ErrAmount
	   }
	  if cerr := money.Conserve(expectedTotal,
		  actualTotal); cerr != nil {
		  deltaAmt,
			  derr := actualTotal.Sub(expectedTotal)
		  if derr != nil {
			   deltaAmt, _ = expectedTotal.Sub(actualTotal)
		   }
		   return PartialAmtResult{
			   CountOK:  true,
			   AmountOK: false,
			   ActualTotal: actualTotal.String(),
			   ExpectedTotal: expectedTotal.String(),
			  Delta: deltaAmt.String(),
		   }, ErrPartialAmt
	   }
	   return PartialAmtResult{
		   CountOK:  true,
		  AmountOK: true,
		  ActualTotal: actualTotal.String(),
		  ExpectedTotal: expectedTotal.String(),
	   }, nil
  }
