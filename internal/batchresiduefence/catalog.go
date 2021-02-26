package batchresiduefence

type ReasonCatalogRow struct {
	Code string `json:"code"`
	FailClosed  bool   `json:"fail_closed"`
	AllowsRetry bool   `json:"allows_retry"`
	Note        string `json:"note"`
}

   func ReasonCatalog() []ReasonCatalogRow {
	   return []ReasonCatalogRow{
		  {Code: ReasonDupFreeze, FailClosed: true, AllowsRetry: false, Note: "同一 item_idem 第二冻账腿"},
		  {Code: ReasonNoFence, FailClosed: true, AllowsRetry: false, Note: "无围栏不得重开"},
		  {Code: ReasonFenceExpired, FailClosed: true, AllowsRetry: false, Note: "围栏过期需重新签发"},
		   {Code: ReasonFenceReleased, FailClosed: true, AllowsRetry: false, Note: "围栏已释放"},
		   {Code: ReasonItemSubmitted, FailClosed: true, AllowsRetry: false, Note: "已提交项禁止 Retry"},
		  {Code: ReasonItemSkipped, FailClosed: true, AllowsRetry: false, Note: "已跳过项禁止 Retry"},
		   {Code: ReasonItemNotFailed, FailClosed: true, AllowsRetry: false, Note: "非失败项禁止 Retry"},
		  {Code: ReasonSubmitRetryMutex, FailClosed: true, AllowsRetry: false, Note: "Submit 半切互斥"},
		  {Code: ReasonHalfCutNeedsResume, FailClosed: true, AllowsRetry: true, Note: "半切必须围栏续冻"},
		   {Code: ReasonSecondFullFreeze, FailClosed: true, AllowsRetry: false, Note: "禁止二次全额冻结"},
		  {Code: ReasonGenMismatch, FailClosed: true, AllowsRetry: false, Note: "代数不匹配"},
		   {Code: ReasonFingerprintDrift, FailClosed: true, AllowsRetry: false, Note: "冻账指纹漂移"},
		  {Code: ReasonBatchMismatch, FailClosed: true, AllowsRetry: false, Note: "围栏批次不匹配"},
		  {Code: ReasonFailClosed, FailClosed: true, AllowsRetry: false, Note: "缺证闭锁"},
	   }
  }

   func AuditCatalog() []string {
	  return []string{
		   AuditIssueFence, AuditFreezeFence, AuditReleaseFence, AuditExpireFence,
		   AuditDupProof, AuditHalfOpen, AuditHalfResume, AuditHalfAbandon,
		   AuditRetryReject,
		  AuditSubmitAssert, AuditMutexAcquire, AuditMutexRelease,
		   AuditPatrolExpire, AuditExportProof,
	   }
  }

  func TableCatalog() []string {
	  return []string{
		  TableResidueFences, TableDupFreezeProofs, TableHalfSubmitCases,
		   TableFreezeSlots, TableAuditEvents, TableMutexHolds,
	  }
  }

   func FindReason(code string) (ReasonCatalogRow, bool) {
	   for _, r := range ReasonCatalog() {
		   if r.Code == code {
			   return r, true
		  }
	   }
	   return ReasonCatalogRow{}, false
  }
