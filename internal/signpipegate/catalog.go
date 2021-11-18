package signpipegate

// FaultSpec 描述一种可注入故障。

type FaultSpec struct {
	Kind FaultKind `json:"kind"`
	MustFail bool `json:"must_fail"`
	BlocksRetry bool   `json:"blocks_retry"`
	NeedsLease  bool   `json:"needs_lease"`
	Summary     string `json:"summary"`
}

  func FaultCatalog() []FaultSpec {
	   return []FaultSpec{
		  {Kind: FaultSigner, MustFail: true, BlocksRetry: false, NeedsLease: true, Summary: "HSM/signer 返回失败，作业进入 failed，可在新世代重试"},
		  {Kind: FaultExpire, MustFail: true, BlocksRetry: true, NeedsLease: true, Summary: "TTL 越过，租约过期，不得再签"},
		   {Kind: FaultFence, MustFail: true, BlocksRetry: false, NeedsLease: true, Summary: "围栏过期 token，旧 epoch 不得进入成功路径"},
		   {Kind: FaultActor, MustFail: true, BlocksRetry: false, NeedsLease: true, Summary: "开单/复核/执行角色重叠，三分互斥"},
		  {Kind: FaultHSMTimeout, MustFail: true, BlocksRetry: false, NeedsLease: true, Summary: "HSM 超时，管线不可用，不得落签名"},
		   {Kind: FaultCanonicalMismatch, MustFail: true, BlocksRetry: false, NeedsLease: true, Summary: "canonical 与开单不一致，防止批一张单签另一笔"},
		   {Kind: FaultDualBroadcast, MustFail: true, BlocksRetry: true, NeedsLease: true, Summary: "同 movement 已有密封产物，双广播即双花"},
		  {Kind: FaultRetryExhausted, MustFail: true, BlocksRetry: true, NeedsLease: true, Summary: "重试世代耗尽，必须收口"},
		   {Kind: FaultGenerationStale, MustFail: true, BlocksRetry: false, NeedsLease: true, Summary: "CAS 世代落后，并发重试失败者必须回读"},
		   {Kind: FaultInflightRace, MustFail: true, BlocksRetry: false, NeedsLease: true, Summary: "同 movement 在途互斥，后到者不得并行签"},
	   }
  }

  func LookupFaultSpec(kind FaultKind) (FaultSpec, bool) {
	   for _, s := range FaultCatalog() {
		  if s.Kind == kind {
			   return s, true
		  }
	   }
	   return FaultSpec{}, false
   }
