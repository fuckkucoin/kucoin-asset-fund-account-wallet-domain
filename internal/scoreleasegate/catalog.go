package scoreleasegate

type ProofCatalogEntry struct {
	Kind ProofKind
	Blocks    bool
	NeedsEyes bool
	Revokes   bool
	Desc      string
}

  func ProofCatalog() []ProofCatalogEntry {
	  return []ProofCatalogEntry{
		   {ProofHaltFlip, true, true, true, "Eval Allow 后 SetHalt(true) 抬世代，旧租约立即失效"},
		  {ProofPackFlip, true, true, true, "策略包更换后 PackGen/PackFP 漂移"},
		   {ProofModeFlip, true, true, true, "Enforce/Shadow 切换后 ModeGen 漂移"},
		  {ProofWindowDrift, true, true, true, "速度窗或目的地窗摘要跃迁"},
		  {ProofMissingLease, true, true, false, "Submit 未携带 score_lease_id"},
		  {ProofExpired, true, false, false, "TTL 到期后提交"},
		   {ProofRevoked, true, true, false, "运维或翻转吊销后提交"},
		  {ProofStaleGen, true, true, true, "世代 CAS 并发失败"},
		  {ProofNoAllow, true, false, false, "Eval 未 Allow 仍尝试签发"},
		  {ProofShadowMode, true, true, false, "Shadow 命中冒充 Enforce 租约"},
		  {ProofAmountDrift, true, true, false, "提交金额与租约冻结金额不一致"},
		   {ProofDestDrift, true, true, false, "提交目的地与租约不一致"},
		  {ProofChainDrift, true, true, false, "提交链与租约不一致"},
		  {ProofAssetDrift, true, true, false, "提交资产与租约不一致"},
		   {ProofConcurrent, true, false, false, "同一租约并行 Bind"},
		   {ProofFingerprint, true, true, false, "资产/链/目的地指纹漂移"},
		  {ProofFourEye, false, false, false, "四眼清障关闭证明"},
	  }
  }

  func CatalogByKind(k ProofKind) (ProofCatalogEntry, bool) {
	   for _, e := range ProofCatalog() {
		  if e.Kind == k {
			  return e, true
		  }
	   }
	   return ProofCatalogEntry{}, false
  }
