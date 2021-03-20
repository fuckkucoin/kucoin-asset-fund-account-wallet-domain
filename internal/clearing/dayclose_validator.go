package clearing

import "fmt"

// ValidationResult 关账前置校验结果。
type ValidationResult struct {
	OK bool `json:"ok"`
	Errors []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	OpenSuspense int `json:"open_suspense"`
	PendingItems int `json:"pending_items"`
	OpenExcepts  int `json:"open_excepts"`
}

// DayCloseValidator 关账前置校验器。
type DayCloseValidator struct {
	suspBook *SuspenseBook
	book            *Book
	warnExceptLimit int // 开放异常数告警阈值
}

// NewDayCloseValidator 初始化关账校验器。
   func NewDayCloseValidator(suspBook *SuspenseBook, book *Book) *DayCloseValidator {
	   return &DayCloseValidator{
		  suspBook:        suspBook,
		  book:            book,
		   warnExceptLimit: 5,
	  }
  }

// WithWarnExceptLimit 设置异常数告警阈值。
   func (v *DayCloseValidator) WithWarnExceptLimit(n int) *DayCloseValidator {
	  if v != nil && n >= 0 {
		  v.warnExceptLimit = n
	   }
	  return v
  }

// ValidateForClose 执行关账前置校验。
// batchID 为目标清算批次 ID；dcID 为目标 DayClose ID（当前未使用，预留扩展）。

  func (v *DayCloseValidator) ValidateForClose(batchID, _ string) ValidationResult {
	   if v == nil {
		  return ValidationResult{OK: true}
	  }
	   result := ValidationResult{OK: true}
	   // 检查 open 挂账
	   if v.suspBook != nil {
		   openSuspense := v.suspBook.ListByBatch(batchID)
		  openCount := 0
		  for _, e := range openSuspense {
			   if e.Status == SuspenseOpen {
				   openCount++
			  }
		  }
		  result.OpenSuspense = openCount
		  if openCount > 0 {
			  result.OK = false
			  result.Errors = append(result.Errors,
				  fmt.Sprintf("batch %s has %d open suspense entries, resolve before closing", batchID, openCount))
		  }
	   }
	  // 检查 pending 清算 Item
	   if v.book != nil {
		   items := v.book.Items(batchID)
		  pendingCount := 0
		   for _, it := range items {
			  if it.Status == StatusPending || it.Status == StatusReady {
				   pendingCount++
			  }
		  }
		   result.PendingItems = pendingCount
		  if pendingCount > 0 {
			  result.OK = false
			  result.Errors = append(result.Errors,
				  fmt.Sprintf("batch %s has %d pending items, all must be released or excepted before closing", batchID, pendingCount))
		  }
		   // 检查 open 异常（告警级别）
		  excepts := v.book.Exceptions(batchID)
		   openExcepts := 0
		   for _, ex := range excepts {
			  if ex.Status == ExOpen {
				   openExcepts++
			   }
		   }
		   result.OpenExcepts = openExcepts
		  if openExcepts > v.warnExceptLimit {
			  result.Warnings = append(result.Warnings,
				   fmt.Sprintf("batch %s has %d open exceptions (threshold: %d)", batchID, openExcepts, v.warnExceptLimit))
		  }
	  }
	   return result
  }

// ValidateAccrualBalance 校验应计批次平衡性。
// 接受 totalDebit 和 totalCredit 字符串，返回是否平衡及差异描述。

   func ValidateAccrualBalance(totalDebit, totalCredit string) (bool, string) {
	  if totalDebit == totalCredit {
		  return true, ""
	   }
	   return false, fmt.Sprintf("accrual batch imbalanced: debit=%s credit=%s", totalDebit, totalCredit)
   }
