package wdrelease

// CodeInfo 一条稳定拒绝码。
type CodeInfo struct {
	Code string `json:"code"`
	Meaning string `json:"meaning"`
}

   func CodeCatalog() []CodeInfo {
	  return []CodeInfo{
		   {Code: CodeNoTicket, Meaning: "没有放行票据，冻账前失败关闭"},
		  {Code: CodeQuota, Meaning: "已授权额度不足以覆盖本次冻结"},
		  {Code: CodeApplicant, Meaning: "申请人不能担任任一复核人"},
		  {Code: CodeSame, Meaning: "同一人不能通过两级复核"},
		   {Code: CodeRole, Meaning: "复核角色与当前级数不符"},
		  {Code: CodeClosed, Meaning: "当前状态不允许该跃迁"},
		  {Code: CodeRisk, Meaning: "风险分达到上限，不开可冻结票据"},
		   {Code: CodeDest, Meaning: "目的地类别拒绝"},
		   {Code: CodeTimeout, Meaning: "超过时限，未用段取消"},
		  {Code: CodeSegmentCap, Meaning: "单段金额超过该档上限"},
		   {Code: CodePolicy, Meaning: "策略表无法覆盖该笔"},
		  {Code: CodeEscalateCeiling, Meaning: "已到最大级数，超时改为关闭"},
		   {Code: CodeFingerprint, Meaning: "票据与出金要素不一致"},
		  {Code: CodeDailyCap, Meaning: "用户当日额度不足"},
		  {Code: CodeDestCap, Meaning: "目的地当日额度不足"},
		   {Code: CodePrivilege, Meaning: "越权尝试，不改变授权段"},
		   {Code: CodeVersion, Meaning: "指定的策略版本不存在"},
		   {Code: CodeLive, Meaning: "放行域不提供解冻，链上 hash 仍有效时同样拒绝"},
		  {Code: CodeRejected, Meaning: "复核拒绝，未冻结段取消"},
		  {Code: CodePending, Meaning: "票据尚未提交复核"},
		   {Code: CodeConflict, Meaning: "幂等键已绑定不同要素"},
		  {Code: CodeAmount, Meaning: "金额无效"},
		  {Code: CodeActor, Meaning: "操作人无效"},
		   {Code: CodeOpenHold, Meaning: "未结票据过多"},
		   {Code: CodeApplicantCancel, Meaning: "申请人撤回草稿"},
	   }
   }

  func knownCode(code string) bool {
	   if code == "" {
		   return false
	   }
	  for _, c := range CodeCatalog() {
		   if c.Code == code {
			  return true
		   }
	  }
	  return false
  }
