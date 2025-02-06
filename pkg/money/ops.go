package money

import (
	"fmt"
	"github.com/shopspring/decimal"
)

// FeeBPS 从本金切手续费。
// 入参：本金 a（必须已过 Parse / ParseBook）、bps 万分比，区间 [0, 10000]。
// 出参：fee 与 net，fee+net 必须等于 a。bps 非法返回 ErrInvalid，调用方不得另造一个数去冻。
// 出金仍按全额冻，fee 在入账时再切到 system.fee。切出来的 fee 非正（本金太小、bps 太低）
// 时返回 0 费和原额，避免冻一个四舍五入出来的幽灵数。10000 bps 会把本金全切走，
// net 非正同样退回 0 费——全额当费必须走独立科目，不走这条切分。
  func (a Amount) FeeBPS(bps int) (fee Amount, net Amount, err error) {
	   if bps < 0 || bps > 10000 {
		   return Zero(), a, fmt.Errorf("%w: bps", ErrInvalid)
	   }
	  if bps == 0 || a.IsZero() {
		   return Zero(), a, nil
	  }
	   feeDec := a.d.Mul(decimal.NewFromInt(int64(bps))).Div(decimal.NewFromInt(10000))
	  if feeDec.IsNegative() {
		  return Zero(), Zero(), ErrNegative
	   }
	   if !feeDec.IsPositive() {
		   return Zero(), a, nil
	  }
	   fee = Amount{d: feeDec}
	  net, err = a.Sub(fee)
	  if err != nil {
		   return Zero(), Zero(), err
	   }
	   if !net.IsPositive() {
		  return Zero(), a, nil
	  }
	  return fee, net, nil
  }

// MulBPS 本金 * bps / 10000。用于利息计提。
// 入参：本金、非负 bps。出参必须能过 ParseBook（允许 0）。负 bps 直接拒，
// 避免把“减免”写成乘法翻成负余额。溢出走 ErrOverflow，和 Add 同一上限。
   func (a Amount) MulBPS(bps int) (Amount, error) {
	   if bps < 0 {
		   return Zero(), ErrNegative
	  }
	  out := a.d.Mul(decimal.NewFromInt(int64(bps))).Div(decimal.NewFromInt(10000))
	  if out.IsNegative() {
		   return Zero(), ErrNegative
	  }
	  if out.GreaterThan(maxInt) {
		  return Zero(), ErrOverflow
	  }
	  return Amount{d: out}, nil
   }

// CmpBPS 本金按万分比折算后与阈值比。给限额闸用，不改本金。
  func (a Amount) CmpBPS(bps int, thresh Amount) (int, error) {
	  got, err := a.MulBPS(bps)
	   if err != nil {
		  return 0, err
	   }
	  return got.Cmp(thresh), nil
  }
