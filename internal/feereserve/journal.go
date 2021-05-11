package feereserve

import (
	"fmt"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Memo 凭证说明常量。
const (
	MemoUserFeeHold      = "user_fee_hold"
	MemoSystemFeeReserve = "system_fee_reserve"
	MemoSystemFee     = "system_fee"
	MemoUserFeeRefund = "user_fee_refund"
	MemoUserFeeTopup  = "user_fee_topup"
)

// JournalLine 单条凭证行（借贷方向相对 "本账户" 视角，正负区分）。

type JournalLine struct {
	Account string `json:"account"`
	Asset   string `json:"asset"`
	Debit money.Amount `json:"debit,omitempty"`
	Credit money.Amount `json:"credit,omitempty"`
	Memo   string       `json:"memo"`
}

// JournalSet 一次操作产生的完整凭证集合（借贷双方各一组）。
type JournalSet struct {
	Lines    []JournalLine `json:"lines"`
	Balanced bool          `json:"balanced"`
	// Note 给 ops/审计用的摘要说明。
	Note string `json:"note"`
}

// journalReserve 生成"冻结预估费"凭证集合（两行）。

   func journalReserve(asset string, reserved money.Amount) (JournalSet, error) {
	   if !reserved.IsPositive() {
		   return JournalSet{}, fmt.Errorf("%w: reserved must be positive for journal", ErrAmount)
	  }
	   lines := []JournalLine{
		   {Account: "user", Asset: asset, Debit: reserved, Memo: MemoUserFeeHold},
		  {Account: "system", Asset: asset, Credit: reserved, Memo: MemoSystemFeeReserve},
	  }
	  if err := checkBalanced(lines); err != nil {
		   return JournalSet{}, fmt.Errorf("%w: reserve journal: %v", ErrConserve, err)
	  }
	   return JournalSet{Lines: lines, Balanced: true, Note: fmt.Sprintf("fee reserve: %s %s", reserved, asset)}, nil
  }

// journalSettleZero 生成"结算（实际 == 预估）"凭证（两行）。

  func journalSettleZero(asset string, amount money.Amount) (JournalSet, error) {
	   if !amount.IsPositive() {
		  return JournalSet{}, fmt.Errorf("%w: settle amount must be positive", ErrAmount)
	   }
	   lines := []JournalLine{
		  {Account: "system", Asset: asset, Debit: amount, Memo: MemoSystemFeeReserve},
		   {Account: "system", Asset: asset, Credit: amount, Memo: MemoSystemFee},
	   }
	   if err := checkBalanced(lines); err != nil {
		   return JournalSet{}, fmt.Errorf("%w: settle_zero journal: %v", ErrConserve, err)
	  }
	   return JournalSet{
		  Lines:    lines,
		  Balanced: true,
		  Note:     fmt.Sprintf("fee settle zero: %s %s (actual==reserved)", amount, asset),
	   }, nil
  }

// journalSettleOver 生成"结算（实际 < 预估，退还 delta）"凭证（三行）。
// reserved = actual + delta，三行之和必须平衡（总借 = 总贷 = reserved）。

  func journalSettleOver(asset string, reserved, actual, delta money.Amount) (JournalSet, error) {
	   if !reserved.IsPositive() || !actual.IsPositive() || !delta.IsPositive() {
		  return JournalSet{}, fmt.Errorf("%w: settle_over all amounts must be positive", ErrAmount)
	  }
	   // 校验 actual + delta == reserved。
	   sum, err := actual.Add(delta)
	  if err != nil {
		   return JournalSet{}, err
	  }
	   if err := money.Conserve(sum, reserved); err != nil {
		  return JournalSet{}, fmt.Errorf("%w: settle_over actual+delta != reserved: %v", ErrConserve, err)
	  }
	  lines := []JournalLine{
		  // 借方：从 system:fee_reserve 贷出总预估额。
		  {Account: "system", Asset: asset, Debit: reserved, Memo: MemoSystemFeeReserve},
		   // 贷方一：实际手续费入 system:fee。
		  {Account: "system", Asset: asset, Credit: actual, Memo: MemoSystemFee},
		  // 贷方二：多冻的 delta 退还用户。
		  {Account: "user", Asset: asset, Credit: delta, Memo: MemoUserFeeRefund},
	   }
	  if err := checkBalanced(lines); err != nil {
		  return JournalSet{}, fmt.Errorf("%w: settle_over journal: %v", ErrConserve, err)
	  }
	   return JournalSet{
		   Lines:    lines,
		   Balanced: true,
		   Note:     fmt.Sprintf("fee settle over: reserved=%s actual=%s delta_refund=%s %s", reserved, actual, delta, asset),
	  }, nil
  }

// journalSettleUnder 生成"结算（实际 > 预估，追收 delta）"凭证（三行）。
// actual = reserved + delta，三行之和必须平衡。
   func journalSettleUnder(asset string, reserved, actual, delta money.Amount) (JournalSet, error) {
	   if !reserved.IsPositive() || !actual.IsPositive() || !delta.IsPositive() {
		  return JournalSet{}, fmt.Errorf("%w: settle_under all amounts must be positive", ErrAmount)
	  }
	  // 校验 reserved + delta == actual。
	   sum, err := reserved.Add(delta)
	  if err != nil {
		  return JournalSet{}, err
	  }
	  if err := money.Conserve(sum, actual); err != nil {
		   return JournalSet{}, fmt.Errorf("%w: settle_under reserved+delta != actual: %v", ErrConserve, err)
	   }
	  lines := []JournalLine{
		   // 借方一：从 system:fee_reserve 贷出预估额。
		  {Account: "system", Asset: asset, Debit: reserved, Memo: MemoSystemFeeReserve},
		   // 借方二：追收用户补足差额。
		  {Account: "user", Asset: asset, Debit: delta, Memo: MemoUserFeeTopup},
		  // 贷方：实际手续费（全额）入 system:fee。
		   {Account: "system", Asset: asset, Credit: actual, Memo: MemoSystemFee},
	  }
	   if err := checkBalanced(lines); err != nil {
		  return JournalSet{}, fmt.Errorf("%w: settle_under journal: %v", ErrConserve, err)
	  }
	  return JournalSet{
		  Lines:    lines,
		  Balanced: true,
		  Note:     fmt.Sprintf("fee settle under: reserved=%s actual=%s delta_topup=%s %s", reserved, actual, delta, asset),
	  }, nil
   }

// journalRelease 生成"取消出金，退还冻结"凭证（两行，reverse of journalReserve）。
   func journalRelease(asset string, reserved money.Amount) (JournalSet, error) {
	  if !reserved.IsPositive() {
		  return JournalSet{}, fmt.Errorf("%w: release amount must be positive", ErrAmount)
	   }
	   lines := []JournalLine{
		   {Account: "system", Asset: asset, Debit: reserved, Memo: MemoSystemFeeReserve},
		   {Account: "user", Asset: asset, Credit: reserved, Memo: MemoUserFeeHold},
	  }
	   if err := checkBalanced(lines); err != nil {
		   return JournalSet{}, fmt.Errorf("%w: release journal: %v", ErrConserve, err)
	  }
	  return JournalSet{
		   Lines: lines,
		  Balanced: true,
		   Note:     fmt.Sprintf("fee release: %s %s (withdraw cancelled)", reserved, asset),
	  }, nil
  }

// BuildSettleJournal 根据 delta 方向选择正确的结算凭证生成器。
   func BuildSettleJournal(asset string, reserved, actual money.Amount, dir DeltaDir, delta money.Amount) (JournalSet, error) {
	   switch dir {
	  case DeltaZero:
		   return journalSettleZero(asset, reserved)
	  case DeltaOver:
		   return journalSettleOver(asset, reserved, actual, delta)
	   case DeltaUnder:
		   return journalSettleUnder(asset, reserved, actual, delta)
	  default:
		  return JournalSet{}, fmt.Errorf("%w: unknown delta direction: %q", ErrConserve, dir)
	  }
  }

// checkBalanced 校验凭证集合借贷平衡（总借 == 总贷）。
// 平衡：sum(Debit) == sum(Credit)。
   func checkBalanced(lines []JournalLine) error {
	  var totalDebit,
		   totalCredit money.Amount
	   var err error
	   for _, l := range lines {
		   if l.Debit.IsPositive() {
			  totalDebit, err = totalDebit.Add(l.Debit)
			   if err != nil {
				  return err
			  }
		  }
		  if l.Credit.IsPositive() {
			  totalCredit, err = totalCredit.Add(l.Credit)
			  if err != nil {
				  return err
			   }
		   }
	   }
	   return money.Conserve(totalDebit, totalCredit)
  }

// ToDeltaLines 将 JournalSet 转换为 DeltaLine 列表（用于 HTTP 响应）。
   func (js JournalSet) ToDeltaLines() []DeltaLine {
	  out := make([]DeltaLine, 0, len(js.Lines))
	  for _, l := range js.Lines {
		   if l.Debit.IsPositive() {
			   out = append(out, DeltaLine{
				   Memo: l.Memo,
				  Asset: l.Asset,
				  Amount:    l.Debit.String(),
				  Direction: "debit",
			   })
		   }
		  if l.Credit.IsPositive() {
			   out = append(out, DeltaLine{
				   Memo:      l.Memo,
				  Asset:     l.Asset,
				  Amount:    l.Credit.String(),
				  Direction: "credit",
			  })
		   }
	  }
	  return out
  }
