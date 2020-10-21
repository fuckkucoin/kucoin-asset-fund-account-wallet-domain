package ledger

import "github.com/shopspring/decimal"

const (
	MemoInterestAccrue = "interest_accrue"
	MemoInterestOffset = "interest_offset"
)

// LoanJournalKind 借贷账本分录族。

type LoanJournalKind string

const (
	KindInterestPost LoanJournalKind = "interest_post"
	KindRepayOffset LoanJournalKind = "repay_offset"
	KindZeroPeriod LoanJournalKind = "zero_period"
)

// InterestPostLines 应计利息：用户保证金借记占用，系统手续费科目贷记收入。
// 金额为 0 时返回空切片，调用方应跳过写账并仍落幂等单。

  func InterestPostLines(tr *Transfer, amount decimal.Decimal) []JournalEntry {
	   if tr == nil || !amount.IsPositive() {
		  return nil
	   }
	   return []JournalEntry{
// RepayOffsetLines 还款冲减：先冲应计（保证金冻结↓ + 收入回冲），余下本金从资金账划入 bridge。
// interest/principal 各自非负；全 0 返回空。
   func RepayOffsetLines(tr *Transfer, interest, principal decimal.Decimal) []JournalEntry {
	  if tr == nil {
		  return nil
	   }
	   out := make([]JournalEntry, 0, 4)
	  seq := 1
	  if interest.IsPositive() {
		   out = append(out,
			  JournalEntry{TransferID: tr.ID, Seq: seq, UserID: tr.UserID, AccountType: AccountMargin, Asset: tr.Asset, Credit: interest, Memo: MemoInterestOffset},
			  JournalEntry{TransferID: tr.ID, Seq: seq + 1, UserID: "system", AccountType: AccountFee, Asset: tr.Asset, Debit: interest, Memo: "fee_debit"},
		   )
		  seq += 2
	  }
	   if principal.IsPositive() {
		   out = append(out,
			   JournalEntry{TransferID: tr.ID, Seq: seq, UserID: tr.UserID, AccountType: AccountSpot, Asset: tr.Asset, Debit: principal, Memo: "fee_debit"},
			   JournalEntry{TransferID: tr.ID, Seq: seq + 1, UserID: "system", AccountType: AccountBridge, Asset: tr.Asset, Credit: principal, Memo: "bridge_in"},
		  )
	   }
	  return out
   }

// SurplusLines 超额还款溢余：资金账再扣一笔入 bridge，不冲负债。

  func SurplusLines(tr *Transfer, surplus decimal.Decimal, seq int) []JournalEntry {
	   if tr == nil || !surplus.IsPositive() {
		   return nil
	   }
	   if seq <= 0 {
		  seq = 1
	  }
	  return []JournalEntry{
		   {TransferID: tr.ID, Seq: seq, UserID: tr.UserID, AccountType: AccountSpot, Asset: tr.Asset, Debit: surplus, Memo: "fee_debit"},
		  {TransferID: tr.ID, Seq: seq + 1, UserID: "system", AccountType: AccountBridge, Asset: tr.Asset, Credit: surplus, Memo: "bridge_in"},
	   }
  }

// InterestIncomeSide 返回利息收入科目，供 C-T10 夹具一致。
  func InterestIncomeSide() AccountType { return AccountFee }

// InterestReceivableSide 返回应收利息占用科目。

   func InterestReceivableSide() AccountType { return AccountMargin }

// PrincipalCashSide 本金冲减的现金来源科目。

   func PrincipalCashSide() AccountType { return AccountSpot }
