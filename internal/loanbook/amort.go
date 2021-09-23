package loanbook

import (
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/pkg/money"
)

// Schedule 等额本息。各期本金之和必须等于原额；末期吃余数。
   func Schedule(principal string, annualBPS, months int, start time.Time) (payment string, rows []Installment, err error) {
	  prin, err := money.Parse(principal)
	  if err != nil {
		   return "", nil, ErrAmount
	   }
	  if months < 1 || months > 360 {
		  return "", nil, ErrTerm
	   }
	  if annualBPS < 0 || annualBPS > 10000 {
		  return "", nil, ErrRate
	   }
	   if start.IsZero() {
		   start = time.Now().UTC()
	  }
	   start = start.UTC()
	   pay,
		  err := monthlyPayment(prin, annualBPS, months)
	  if err != nil {
		   return "", nil, err
	  }
	   remain := prin
	  out := make([]Installment,
		   0, months)
	   paidPrin := money.Zero()
	  monthlyRate := decimal.NewFromInt(int64(annualBPS)).Div(decimal.NewFromInt(120000))
	   for i := 1; i <= months; i++ {
		  interest := clipAmt(money.FromDecimal(remain.Dec().Mul(monthlyRate)))
		  if interest.IsNegative() {
			   return "", nil, ErrSchedule
		  }
		   var part money.Amount
		  if i == months {
			  part = remain
		  } else {
			  part, err = pay.Sub(interest)
			   if err != nil || !part.IsPositive() {
				   part = remain
			   }
			  if part.Cmp(remain) > 0 {
				   part = remain
			  }
		   }
		  next, err := remain.Sub(part)
		  if err != nil {
			   return "", nil, err
		   }
		   instPay, err := part.Add(interest)
		   if err != nil {
			   return "", nil, err
		  }
		   out = append(out, Installment{
			   Seq: i, DueAt: start.AddDate(0, i, 0),
			   Principal: part.String(), Interest: interest.String(),
			   Payment: instPay.String(), Remaining: next.String(), Status: InstDue,
		   })
		  paidPrin, err = paidPrin.Add(part)
		   if err != nil {
			  return "", nil, err
		   }
		  remain = next
	  }
	  if err := money.Conserve(prin, paidPrin); err != nil {
		   return "", nil, ErrSchedule
	   }
	   if !remain.IsZero() {
		  return "", nil, ErrSchedule
	   }
	  return pay.String(), out, nil
   }

   func monthlyPayment(prin money.Amount, annualBPS, months int) (money.Amount, error) {
	  if annualBPS == 0 {
		   parts, err := prin.Split(months)
		   if err != nil {
			   return money.Zero(), err
		  }
		  return parts[0], nil
	   }
	   r := decimal.NewFromInt(int64(annualBPS)).Div(decimal.NewFromInt(120000))
	  one := decimal.NewFromInt(1)
	  pow := one.Add(r).Pow(decimal.NewFromInt(int64(months)))
	  den := pow.Sub(one)
	  if den.IsZero() {
		   return money.Zero(), ErrRate
	  }
	   pmt := prin.Dec().Mul(r).Mul(pow).Div(den)
	   if pmt.IsNegative() || pmt.IsZero() {
		  return money.Zero(), ErrSchedule
	   }
	   return clipAmt(money.FromDecimal(pmt)), nil
  }

   func clipAmt(a money.Amount) money.Amount {
	   return money.FromDecimal(a.Dec().Round(18))
   }
