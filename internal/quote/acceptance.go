package quote

// AcceptID 加深包验收编号。

type AcceptID string

const (
	GT01 AcceptID = "G-T01"
	GT02 AcceptID = "G-T02"
	GT03 AcceptID = "G-T03"
	GT04 AcceptID = "G-T04"
	GT05 AcceptID = "G-T05"
	GT06 AcceptID = "G-T06"
	GT07 AcceptID = "G-T07"
	GT08 AcceptID = "G-T08"
	GT09 AcceptID = "G-T09"
	GT10 AcceptID = "G-T10"
	GT11 AcceptID = "G-T11"
type AcceptSpec struct {
	ID AcceptID
	Area    string
	Summary string
	Expect  string
}

// AcceptCatalog 完整目录。
   func AcceptCatalog() []AcceptSpec {
	   return []AcceptSpec{
		   {GT01, "feeoracle", "空费价确认提现费", "失败且无 ledger 写"},
		  {GT02, "feeoracle", "过期费价", "ErrQuoteStale"},
		   {GT03, "feeoracle", "新鲜有效费价", "成功（假依赖）"},
		   {GT04, "fxmark", "空标记确认闪兑", "ConfirmConvert 失败"},
		   {GT05, "fxmark", "过期标记", "失败"},
		  {GT06, "swap", "空报价确认", "失败"},
		   {GT07, "swap", "过期确认", "失败"},
		   {GT08, "quote", "TTL 表", "age==TTL 有效，age>TTL 过期"},
		   {GT09, "quote", "禁止 house 伪装", "Empty 时不得以 Source=market 成功"},
		   {GT10, "quote", "非法数值", "ErrQuoteIllegal"},
		   {GT11, "confirm", "过期前后连续 Confirm", "前成功后失败"},
		  {GT12, "book", "多币对隔离", "空的一对不影响另一对"},
	  }
  }

   func LookupAccept(id AcceptID) (AcceptSpec, bool) {
	   for _, s := range AcceptCatalog() {
		  if s.ID == id {
			  return s, true
		  }
	   }
	   return AcceptSpec{}, false
  }
