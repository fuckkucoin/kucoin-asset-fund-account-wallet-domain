package money

import "fmt"

// TransferPair 同一资产 A→B：借出、贷入。调用方先 Balanced 再落 journal。
   func TransferPair(asset string, amt Amount, fromMemo, toMemo string) []Line {
	  return []Line{
		   {Asset: asset, Debit: amt, Memo: fromMemo},
		   {Asset: asset, Credit: amt, Memo: toMemo},
	  }
   }

// CreditPair 入金：桥出、用户进。金额必须来自观察，不得用请求猜测。

  func CreditPair(asset string, amt Amount) []Line {
	   return TransferPair(asset, amt, "bridge_debit", "user_credit")
  }

// DebitPair 出金入账：用户冻账转出、桥或外发。
   func DebitPair(asset string, amt Amount) []Line {
	  return TransferPair(asset, amt, "user_debit", "bridge_credit")
   }

// FeeCut 从本金切费后的三行：用户实发、手续费科目、桥对端。
// fee+net 必须等于 gross；任一边空则退回 TransferPair，避免半边费。

   func FeeCut(asset string, gross Amount, bps int) ([]Line, error) {
	   fee, net, err := gross.FeeBPS(bps)
	  if err != nil {
		   return nil, err
	   }
	  if fee.IsZero() {
		   return DebitPair(asset, gross), nil
	  }
	   lines := []Line{
		  {Asset: asset, Debit: gross, Memo: "user_debit_gross"},
		  {Asset: asset, Credit: net, Memo: "bridge_net"},
		  {Asset: asset, Credit: fee, Memo: "system_fee"},
	  }
	  if err := Balanced(lines); err != nil {
		   return nil, fmt.Errorf("fee cut: %w", err)
	  }
	   return lines, nil
  }

// InterestPair 利息：桥出、理财进。金额须能过 ParseBook（允许极小正数）。
  func InterestPair(asset string, amt Amount) []Line {
	  return TransferPair(asset, amt, "bridge_interest", "earn_credit")
   }

// SweepPair 热→冷。金额须已过库存闸，这里只组凭证。
   func SweepPair(asset string, amt Amount) []Line {
	  return TransferPair(asset, amt, "hot_debit", "cold_credit")
   }

// MustBalanced 组凭证后立刻轧平，不平直接 panic——只给和静态表用。

   func MustBalanced(lines []Line) []Line {
	   if err := Balanced(lines); err != nil {
		  panic(err)
	   }
	  return lines
   }
