package fundgraph

// 边类型在本包冻结。
const (
	EdgeObserveCredit EdgeType = "observe_to_credit"
	EdgeCreditTransfer EdgeType = "credit_to_transfer"
	EdgeTransferHop      EdgeType = "transfer_hop"
	EdgeTransferFreeze   EdgeType = "transfer_to_freeze"
	EdgeFreezeBroadcast  EdgeType = "freeze_to_broadcast"
	EdgeBroadcastConfirm EdgeType = "broadcast_to_confirm"
	EdgeObserveRecon     EdgeType = "observe_to_recon"
	EdgeCreditRecon EdgeType = "credit_to_recon"
	EdgeFreezeRecon    EdgeType = "freeze_to_recon"
	EdgeBroadcastRecon EdgeType = "broadcast_to_recon"
	EdgeFreezeApproval   EdgeType = "freeze_to_approval"
	EdgeApprovalRelease  EdgeType = "approval_to_release"
	EdgeSweepPlanProof   EdgeType = "sweep_plan_to_proof"
	EdgeSweepProofCredit EdgeType = "sweep_proof_to_credit"
	EdgeAddrpoolProtect  EdgeType = "addrpool_protect"
	EdgeHotReserveHold   EdgeType = "hot_reserve_hold"
	EdgeReserveBroadcast EdgeType = "reserve_to_broadcast"
	EdgeDepositRollback EdgeType = "deposit_rollback"
	EdgeTransferCompensate EdgeType = "transfer_compensate"
	EdgeEnvelopeRef        EdgeType = "envelope_ref"
)

// 节点种类。只有 spineStages 里的种类参与主链断链判断。

const (
	NodeDepositObserve NodeKind = "deposit_observe"
	NodeDepositCredit  NodeKind = "deposit_credit"
	NodeDepositRollback NodeKind = "deposit_rollback"
	NodeTransferHop    NodeKind = "transfer_hop"
	NodeWithdrawFreeze NodeKind = "withdraw_freeze"
	NodeWithdrawBroadcast NodeKind = "withdraw_broadcast"
	NodeWithdrawConfirm NodeKind = "withdraw_confirm"
	NodeSweepPlan NodeKind = "sweep_plan"
	NodeSweepProof  NodeKind = "sweep_proof"
	NodeReconTicket NodeKind = "recon_ticket"
	NodeAddrpoolProtect NodeKind = "addrpool_protect"
	NodeWithdrawApproval NodeKind = "withdraw_approval"
	NodeWithdrawRelease  NodeKind = "withdraw_release"
	NodeHotReserve NodeKind = "hot_reserve"
	NodeEnvelopeRef NodeKind = "envelope_ref"
)

// EdgeTypeCount 是冻结边类型的个数。用它挡住静默增删。
const EdgeTypeCount = 20

// NodeKindCount 是冻结节点种类的个数。
const NodeKindCount = 15

// EdgeSpec 描述一条冻结边：两端节点种类、是否主链、子节点是否只允许一个父边。
type EdgeSpec struct {
	Type  EdgeType `json:"type"`
	From  NodeKind `json:"from"`
	To    NodeKind `json:"to"`
	Spine bool     `json:"spine"`
	SingleParent bool `json:"single_parent"`
	Title string `json:"title"`
}

// NodeSpec 描述一种节点。

type NodeSpec struct {
	Kind  NodeKind `json:"kind"`
	Spine bool     `json:"spine"`
	Title string   `json:"title"`
}

// spineStages 是主链阶段，下标越小越靠近入金观测。

var spineStages = []NodeKind{
	NodeDepositObserve,
	NodeDepositCredit,
	NodeTransferHop,
	NodeWithdrawFreeze,
	NodeWithdrawBroadcast,
	NodeWithdrawConfirm,
}

var edgeCatalog = []EdgeSpec{
	{EdgeObserveCredit, NodeDepositObserve, NodeDepositCredit, true, true, "入金观测指向贷记"},
	{EdgeCreditTransfer, NodeDepositCredit, NodeTransferHop, true, true, "贷记指向划转 hop"},
	{EdgeTransferHop, NodeTransferHop, NodeTransferHop, true, true, "划转 hop 指向下一跳"},
	{EdgeTransferFreeze, NodeTransferHop, NodeWithdrawFreeze, true, true, "划转指向出金冻结"},
	{EdgeFreezeBroadcast, NodeWithdrawFreeze, NodeWithdrawBroadcast, true, true, "冻结指向广播"},
	{EdgeBroadcastConfirm, NodeWithdrawBroadcast, NodeWithdrawConfirm, true, true, "广播指向确认"},
	{EdgeObserveRecon, NodeDepositObserve, NodeReconTicket, false, false, "观测挂对账单"},
	{EdgeCreditRecon, NodeDepositCredit, NodeReconTicket, false, false, "贷记挂对账单"},
	{EdgeFreezeRecon, NodeWithdrawFreeze, NodeReconTicket, false, false, "冻结挂对账单"},
	{EdgeBroadcastRecon, NodeWithdrawBroadcast, NodeReconTicket, false, false, "广播挂对账单"},
	{EdgeFreezeApproval, NodeWithdrawFreeze, NodeWithdrawApproval, false, false, "冻结挂出金审批票"},
	{EdgeApprovalRelease, NodeWithdrawApproval, NodeWithdrawRelease, false, true, "审批票指向放行引用"},
	{EdgeSweepPlanProof, NodeSweepPlan, NodeSweepProof, false, true, "归集计划指向证明号"},
	{EdgeSweepProofCredit, NodeSweepProof, NodeDepositCredit, false, false, "归集证明指向贷记引用"},
	{EdgeAddrpoolProtect, NodeDepositObserve, NodeAddrpoolProtect, false, false, "观测指向地址防护单"},
	{EdgeHotReserveHold, NodeHotReserve, NodeWithdrawFreeze, false, true, "热预留指向冻结"},
	{EdgeReserveBroadcast, NodeHotReserve, NodeWithdrawBroadcast, false, false, "热预留指向广播"},
	{EdgeDepositRollback, NodeDepositCredit, NodeDepositRollback, false, true, "贷记指向回退事实"},
	{EdgeTransferCompensate, NodeTransferHop, NodeTransferHop, false, false, "划转补偿跳"},
	{EdgeEnvelopeRef, NodeDepositObserve, NodeEnvelopeRef, false, false, "事实指向信封号引用"},
}

var nodeCatalog = []NodeSpec{
	{NodeDepositObserve, true, "入金观测"},
	{NodeDepositCredit, true, "入金贷记"},
	{NodeDepositRollback, false, "入金回退"},
	{NodeTransferHop, true, "划转跳"},
	{NodeWithdrawFreeze, true, "出金冻结"},
	{NodeWithdrawBroadcast, true, "出金广播"},
	{NodeWithdrawConfirm, true, "出金确认"},
	{NodeSweepPlan, false, "归集计划"},
	{NodeSweepProof, false, "归集证明"},
	{NodeReconTicket, false, "对账单"},
	{NodeAddrpoolProtect, false, "地址防护单"},
	{NodeWithdrawApproval, false, "出金审批票"},
	{NodeWithdrawRelease, false, "放行引用"},
	{NodeHotReserve, false, "热预留单"},
	{NodeEnvelopeRef, false, "信封引用"},
}

var (
	edgeByType = indexEdges(edgeCatalog)
	nodeByKind = indexNodes(nodeCatalog)
)

  func indexEdges(list []EdgeSpec) map[EdgeType]EdgeSpec {
	  m := make(map[EdgeType]EdgeSpec, len(list))
	   for _, s := range list {
		  m[s.Type] = s
	  }
	  return m
   }

   func indexNodes(list []NodeSpec) map[NodeKind]NodeSpec {
	   m := make(map[NodeKind]NodeSpec, len(list))
	   for _, s := range list {
		   m[s.Kind] = s
	   }
	  return m
   }

// EdgeCatalog 返回冻结边类型的副本。

  func EdgeCatalog() []EdgeSpec {
	  out := make([]EdgeSpec,
		  len(edgeCatalog))
	   copy(out, edgeCatalog)
	  return out
  }

// NodeCatalog 返回冻结节点种类的副本。

   func NodeCatalog() []NodeSpec {
	  out := make([]NodeSpec, len(nodeCatalog))
	  copy(out, nodeCatalog)
	   return out
   }

// KnownEdgeType 判断边类型是否在冻结枚举内。

  func KnownEdgeType(t EdgeType) bool {
	   _, ok := edgeByType[t]
	   return ok
  }

// KnownNodeKind 判断节点种类是否在冻结枚举内。
  func KnownNodeKind(k NodeKind) bool {
	  _, ok := nodeByKind[k]
	  return ok
   }

// EdgeSpecOf 返回边规格。未知类型 ok 为 false。

   func EdgeSpecOf(t EdgeType) (EdgeSpec, bool) {
	   s, ok := edgeByType[t]
	  return s, ok
   }

// SingleParent 表示该边类型下，同一个子节点只应有一个父节点。

  func SingleParent(t EdgeType) bool {
	   s, ok := edgeByType[t]
	  return ok && s.SingleParent
   }

// SpineEdge 表示边属于主链连通判断。

   func SpineEdge(t EdgeType) bool {
	   s, ok := edgeByType[t]
	   return ok && s.Spine
   }

// spineIndex 返回主链阶段下标。非主链种类返回 false。
   func spineIndex(k NodeKind) (int, bool) {
	   for i, s := range spineStages {
		  if s == k {
			   return i, true
		   }
	   }
	  return 0, false
  }

// SpineLink 是进入该主链阶段所要求的边类型。观测阶段没有入边要求。
  func SpineLink(k NodeKind) (EdgeType, bool) {
	   switch k {
	   case NodeDepositCredit:
		   return EdgeObserveCredit, true
	  case NodeTransferHop:
		   return EdgeCreditTransfer, true
	  case NodeWithdrawFreeze:
		  return EdgeTransferFreeze, true
	   case NodeWithdrawBroadcast:
		   return EdgeFreezeBroadcast, true
	   case NodeWithdrawConfirm:
		   return EdgeBroadcastConfirm, true
	   default:
		  return "", false
	   }
  }

// AcceptedIncoming 是反向行走时，该节点视为「有上游」的边类型集合。
// 划转跳同时接受贷记入边和上一跳，避免多跳被误判为断链。
  func AcceptedIncoming(k NodeKind) []EdgeType {
	   switch k {
	  case NodeDepositCredit:
		   return []EdgeType{EdgeObserveCredit}
	  case NodeTransferHop:
		   return []EdgeType{EdgeCreditTransfer,
			   EdgeTransferHop, EdgeTransferCompensate}
	  case NodeWithdrawFreeze:
		  return []EdgeType{EdgeTransferFreeze}
	   case NodeWithdrawBroadcast:
		  return []EdgeType{EdgeFreezeBroadcast}
	  case NodeWithdrawConfirm:
		  return []EdgeType{EdgeBroadcastConfirm}
	   default:
		   return nil
	   }
  }

// PreferredIncoming 决定同一节点多条入边时的遍历先级。

   func PreferredIncoming(k NodeKind) EdgeType {
	   if t, ok := SpineLink(k); ok {
		  return t
	   }
	   switch k {
	  case NodeSweepProof:
		   return EdgeSweepPlanProof
	  case NodeWithdrawRelease:
		  return EdgeApprovalRelease
	   case NodeDepositRollback:
		   return EdgeDepositRollback
	  default:
		   return ""
	  }
  }

// PreferredOutgoing 决定正向影响面的出边先级。

  func PreferredOutgoing(k NodeKind) EdgeType {
	  switch k {
	  case NodeDepositObserve:
		  return EdgeObserveCredit
	  case NodeDepositCredit:
		   return EdgeCreditTransfer
	  case NodeTransferHop:
		   return EdgeTransferHop
	   case NodeWithdrawFreeze:
		   return EdgeFreezeBroadcast
	  case NodeWithdrawBroadcast:
		   return EdgeBroadcastConfirm
	  case NodeSweepPlan:
		   return EdgeSweepPlanProof
	   case NodeWithdrawApproval:
		   return EdgeApprovalRelease
	  default:
		  return ""
	   }
  }
