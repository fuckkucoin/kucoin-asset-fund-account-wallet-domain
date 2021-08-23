package loanwire

import "account-assets.kucoin.net/funding/internal/utabook"

type Case struct {
	ID string
	User string
	Currency string
	Mode   string
	Symbol string
	Principal string
	RateHourly string
	RateDaily string
	FreeLimit string
	PeriodKind string
	AnchorMs   int64
	Factor     string
	WantInt string
	Repay    string
	Policy   string
	WantAccr string
	WantPrin string
	ExcessOK bool
	Zero     bool
}

   func DefaultTier(c Case) utabook.RateTier {
	  return utabook.RateTier{
		  Currency: c.Currency, Tier: 0,
		   RateHourly: c.RateHourly, RateDaily: c.RateDaily,
		   BorrowLimit: "1000000", InterestFreeLimit: c.FreeLimit, HoldLimit: "0",
	   }
   }

   func DefaultRow(c Case) utabook.LiabilityRow {
	   return utabook.LiabilityRow{
		  Currency: c.Currency,
		   MarginMode: c.Mode, Symbol: c.Symbol,
		   Principal: c.Principal, Interest: "0", AccruedInterest: "0",
	  }
  }

   func HourCases() []Case {
	  const hour = int64(1_700_000_000_000) // 固定锚，避免 now 漂移
	   return []Case{
		  {ID: "C-T01", User: "u-c01", Currency: "USDT", Mode: "CROSS", Principal: "1000", RateHourly: "0.0001", RateDaily: "0.0024", PeriodKind: "HOUR", AnchorMs: hour, WantInt: "0.1"},
		   {ID: "C-T01b", User: "u-c01b", Currency: "USDC", Mode: "CROSS", Principal: "2000", RateHourly: "0.0002", RateDaily: "0.0048", PeriodKind: "HOUR", AnchorMs: hour, WantInt: "0.4"},
		   {ID: "C-T01c", User: "u-c01c", Currency: "BTC", Mode: "ISOLATED", Symbol: "BTC-USDT", Principal: "2", RateHourly: "0.00005", RateDaily: "0.0012", PeriodKind: "HOUR", AnchorMs: hour, WantInt: "0.0001"},
		  {ID: "C-T04", User: "u-c04", Currency: "USDT", Mode: "CROSS", Principal: "100", RateHourly: "0", RateDaily: "0", PeriodKind: "HOUR", AnchorMs: hour, WantInt: "0", Zero: true},
		  {ID: "C-T04b", User: "u-c04b", Currency: "USDT", Mode: "CROSS", Principal: "50", RateHourly: "0.0001", RateDaily: "0.0024", FreeLimit: "50", PeriodKind: "HOUR", AnchorMs: hour, WantInt: "0", Zero: true},
		   {ID: "C-T05", User: "u-c05", Currency: "USDT", Mode: "CROSS", Principal: "1000", RateHourly: "0.0001", RateDaily: "0.0024", PeriodKind: "HOUR", AnchorMs: hour, WantInt: "0.1", Repay: "0.1", WantAccr: "0", WantPrin: "1000"},
		   {ID: "C-T06", User: "u-c06", Currency: "USDT", Mode: "CROSS", Principal: "1000", RateHourly: "0.0001", RateDaily: "0.0024", PeriodKind: "HOUR", AnchorMs: hour, WantInt: "0.1", Repay: "100.1", WantAccr: "0", WantPrin: "900"},
		  {ID: "C-T07", User: "u-c07", Currency: "USDT", Mode: "CROSS", Principal: "100", RateHourly: "0.0001", RateDaily: "0.0024", PeriodKind: "HOUR", AnchorMs: hour, WantInt: "0.01", Repay: "200", Policy: "reject"},
		  {ID: "C-T07s", User: "u-c07s", Currency: "USDT", Mode: "CROSS", Principal: "100", RateHourly: "0.0001", RateDaily: "0.0024", PeriodKind: "HOUR", AnchorMs: hour, WantInt: "0.01", Repay: "200", Policy: "surplus", WantAccr: "0", WantPrin: "0", ExcessOK: true},
		   {ID: "C-T08", User: "u-c08", Currency: "USDT", Mode: "CROSS", Principal: "1000", RateHourly: "0.0001", RateDaily: "0.0024", PeriodKind: "HOUR", AnchorMs: hour, WantInt: "0.1", Repay: "200.1", WantAccr: "0", WantPrin: "800"},
	   }
  }

   func DayCases() []Case {
	  const day = int64(1_700_006_400_000)
	  return []Case{
		  {ID: "C-T01d", User: "u-d01", Currency: "USDT", Mode: "CROSS", Principal: "1000", RateHourly: "0.0001", RateDaily: "0.0024", PeriodKind: "DAY", AnchorMs: day, WantInt: "2.4"},
		   {ID: "C-T03a", User: "u-d03", Currency: "USDC", Mode: "CROSS", Principal: "500", RateHourly: "0.0002", RateDaily: "0.0048", PeriodKind: "HOUR", AnchorMs: day, WantInt: "0.1"},
	   }
  }

   func AllCases() []Case {
	   return append(HourCases(),
		   DayCases()...)
   }
