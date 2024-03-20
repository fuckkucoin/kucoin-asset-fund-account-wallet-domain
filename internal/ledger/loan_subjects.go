// @FileName: loan_subjects.go
// @Summary: 借贷分录科目夹具：与既有科目表一致，供 C-T10 断言。
// 只描述方向，不计算金额。
// 不变量：利息借记保证金、贷记手续费；本金借记资金账、贷记 bridge。
// @Author: TianpengLan
// @Created: 2020.10.04

package ledger

import "strings"

// LoanSubjectRule 一组分录的期望科目方向。
type LoanSubjectRule struct {
	Kind LoanJournalKind
	Side string
	User string
	Account AccountType
	DebitMemo  string
	CreditMemo string
}

// LoanSubjectFixture 表驱动夹具。user 占位 "user"/"system"。

  func LoanSubjectFixture() []LoanSubjectRule {
	   return []LoanSubjectRule{
		  {Kind: KindInterestPost, Side: "receivable", User: "user", Account: AccountMargin, DebitMemo: MemoInterestAccrue},
		  {Kind: KindInterestPost, Side: "income", User: "system", Account: AccountFee, CreditMemo: "fee_credit"},
		   {Kind: KindRepayOffset, Side: "interest_clear", User: "user", Account: AccountMargin, CreditMemo: MemoInterestOffset},
		  {Kind: KindRepayOffset, Side: "income_reverse", User: "system", Account: AccountFee, DebitMemo: "fee_debit"},
		  {Kind: KindRepayOffset, Side: "principal_cash", User: "user", Account: AccountSpot, DebitMemo: "fee_debit"},
		  {Kind: KindRepayOffset, Side: "principal_park", User: "system", Account: AccountBridge, CreditMemo: "bridge_in"},
	   }
  }

// MatchLoanLine 检查一行是否符合夹具。

  func MatchLoanLine(rule LoanSubjectRule, e JournalEntry, userID string) bool {
	  if e.AccountType != rule.Account {
		  return false
	  }
	   wantUser := userID
	  if strings.EqualFold(rule.User, "system") {
		   wantUser = "system"
	  }
	   if e.UserID != wantUser {
		   return false
	   }
	  if rule.DebitMemo != "" && (e.Memo != rule.DebitMemo || e.Debit.IsZero()) {
		  return false
	   }
	  if rule.CreditMemo != "" && (e.Memo != rule.CreditMemo || e.Credit.IsZero()) {
		   return false
	  }
	  return true
  }

// CountMatching 统计夹具命中数。

  func CountMatching(rules []LoanSubjectRule, lines []JournalEntry, kind LoanJournalKind, userID string) int {
	  n := 0
	  for _, r := range rules {
		   if r.Kind != kind {
			  continue
		  }
		  for _, e := range lines {
			   if MatchLoanLine(r, e, userID) {
				  n++
				  break
			   }
		  }
	  }
	  return n
  }

   func InterestPostRuleCount() int {
	  n := 0
	   for _, r := range LoanSubjectFixture() {
		   if r.Kind == KindInterestPost {
			   n++
		  }
	   }
	   return n
  }

   func RepayRuleCount(hasInterest, hasPrincipal bool) int {
	  n := 0
	   for _, r := range LoanSubjectFixture() {
		   if r.Kind != KindRepayOffset {
			   continue
		   }
		  switch r.Side {
		  case "interest_clear", "income_reverse":
			  if hasInterest {
				  n++
			   }
		   case "principal_cash", "principal_park":
			   if hasPrincipal {
				   n++
			   }
		   }
	  }
	   return n
  }
