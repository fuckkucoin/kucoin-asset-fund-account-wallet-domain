// 功能：金额与资产规范化。
// 重点：同值不同写法必须同一指纹。
// 风险点：规范化不一致会把重放打成冲突。

// 作者：Cason
// 日期：2021.06.14

package fundmesh

type AmountCanonRow struct {
	Raw string
	Canon string
	OK    bool
}

   func AmountCanonRows() []AmountCanonRow {
	   return []AmountCanonRow{
		   {Raw: "10", Canon: "10", OK: true},
		  {Raw: "10.0", Canon: "10", OK: true},
		   {Raw: "10.100", Canon: "10.1", OK: true},
		   {Raw: "010", Canon: "10", OK: true},
		  {Raw: "0.50", Canon: "0.5", OK: true},
		   {Raw: "1.2300", Canon: "1.23", OK: true},
		  {Raw: "0", Canon: "0", OK: true},
		   {Raw: "0.0", Canon: "0", OK: true},
		  {Raw: "-1", Canon: "", OK: false},
		   {Raw: "", Canon: "", OK: false},
		   {Raw: "1.2.3", Canon: "", OK: false},
		  {Raw: "1e2", Canon: "", OK: false},
		  {Raw: "abc", Canon: "", OK: false},
		   {Raw: "+3.0", Canon: "3", OK: true},
	   }
  }

type AssetCanonRow struct {
	Raw   string
	Canon string
}

  func AssetCanonRows() []AssetCanonRow {
	  return []AssetCanonRow{
		  {Raw: "usdt", Canon: "USDT"},
		   {Raw: "Usdt", Canon: "USDT"},
		  {Raw: " usdc ", Canon: "USDC"},
		  {Raw: "btc", Canon: "BTC"},
		   {Raw: "ETH", Canon: "ETH"},
	  }
   }

  func SameIntentAfterNorm(a, b OutboundRequest) bool {
	   if ValidateOutboundRequest(&a) != nil || ValidateOutboundRequest(&b) != nil {
		   return false
	  }
	  fa, ea := FingerprintOf(a)
	  fb, eb := FingerprintOf(b)
	  if ea != nil || eb != nil {
		   return false
	  }
	  return SameFingerprint(fa, fb)
   }
