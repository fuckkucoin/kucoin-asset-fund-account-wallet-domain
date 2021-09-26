package loanwire

  func ExtraHourCases() []Case {
	   const hour = int64(1_700_086_400_000)
	   return []Case{
		   {ID: "X-H01", User: "u-xh01", Currency: "ETH", Mode: "CROSS", Principal: "10", RateHourly: "0.00008", RateDaily: "0.00192", PeriodKind: "HOUR", AnchorMs: hour, WantInt: "0.0008"},
		  {ID: "X-H02", User: "u-xh02", Currency: "USDT", Mode: "ISOLATED", Symbol: "ETH-USDT", Principal: "2500", RateHourly: "0.00015", RateDaily: "0.0036", PeriodKind: "HOUR", AnchorMs: hour, WantInt: "0.375"},
		   {ID: "X-H03", User: "u-xh03", Currency: "USDC", Mode: "CROSS", Principal: "80", RateHourly: "0.00025", RateDaily: "0.006", FreeLimit: "30", PeriodKind: "HOUR", AnchorMs: hour, WantInt: "0.0125"},
		  {ID: "X-H04", User: "u-xh04", Currency: "BTC", Mode: "CROSS", Principal: "0.5", RateHourly: "0.00004", RateDaily: "0.00096", PeriodKind: "HOUR", AnchorMs: hour, WantInt: "0.00002"},
		  {ID: "X-H05", User: "u-xh05", Currency: "USDT", Mode: "CROSS", Principal: "9999", RateHourly: "0.00001", RateDaily: "0.00024", PeriodKind: "HOUR", AnchorMs: hour, WantInt: "0.09999"},
	   }
  }

  func ExtraDayCases() []Case {
	  const day = int64(1_700_172_800_000)
	   return []Case{
		   {ID: "X-D01", User: "u-xd01", Currency: "USDT", Mode: "CROSS", Principal: "400", RateHourly: "0.0001", RateDaily: "0.0024", PeriodKind: "DAY", AnchorMs: day, WantInt: "0.96"},
		  {ID: "X-D02", User: "u-xd02", Currency: "USDC", Mode: "ISOLATED", Symbol: "BTC-USDC", Principal: "800", RateHourly: "0.00005", RateDaily: "0.0012", PeriodKind: "DAY", AnchorMs: day, WantInt: "0.96"},
	   }
   }

  func ExtraAll() []Case {
	  out := append(AllCases(), ExtraHourCases()...)
	   return append(out,
		  ExtraDayCases()...)
  }
