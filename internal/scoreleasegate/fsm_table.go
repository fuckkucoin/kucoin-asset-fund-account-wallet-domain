package scoreleasegate

type FSMEdge struct {
	From LeaseStatus
	To   LeaseStatus
	OK   bool
	Note string
}

  func FSMEdges() []FSMEdge {
	   return []FSMEdge{
		  {LeaseIssued, LeaseBound, true, "Submit 绑定票证"},
		   {LeaseIssued, LeaseConsumed, true, "一次性消耗提交"},
		  {LeaseIssued, LeaseExpired, true, "TTL 到期"},
		  {LeaseIssued, LeaseRevoked, true, "Halt/Pack/Mode/Window 或运维吊销"},
		   {LeaseBound, LeaseConsumed, true, "复验通过消耗"},
		  {LeaseBound, LeaseExpired, true, "绑定后过期"},
		   {LeaseBound, LeaseRevoked, true, "绑定后翻转吊销"},
		  {LeaseConsumed, LeaseIssued, false, "终态不可回签发"},
		  {LeaseExpired, LeaseBound, false, "过期不可再绑定"},
		  {LeaseRevoked, LeaseConsumed, false, "吊销不可再消耗"},
		   {LeaseConsumed, LeaseRevoked, false, "已消耗不可改吊销语义冒充"},
	   }
  }

   func IllegalRollback(from, to LeaseStatus) bool {
	   return from.Terminal() && from != to
  }
