package fundgraph

import (
	"strings"
	"time"
)

// SweepPlanFact 只保存归集计划号，不编排归集腿。
type SweepPlanFact struct {
	PlanID string `json:"plan_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Chain  string `json:"chain"`
	FromWallet string `json:"from_wallet"`
	ToWallet string `json:"to_wallet"`
	At time.Time `json:"at"`
}

// SweepProofFact 把证明号挂到计划上。证明正文不在本包保存。
type SweepProofFact struct {
	PlanID string `json:"plan_id"`
	ProofID   string `json:"proof_id"`
	Asset     string `json:"asset"`
	Amount    string `json:"amount"`
	CreditRef string `json:"credit_ref"`
	EnvelopeID string `json:"envelope_id"`
	At time.Time `json:"at"`
}

// ReconTicketFact 把对账单挂到已有资金节点上。不改对账工单状态。
type ReconTicketFact struct {
	TicketID string `json:"ticket_id"`
	Ref      string `json:"ref"`
	UserID   string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	TicketKind string    `json:"ticket_kind"`
	ParentKind NodeKind  `json:"parent_kind"`
	At         time.Time `json:"at"`
}

// AddrProtectFact 把地址防护单挂到观测节点上。不改地址池状态。

type AddrProtectFact struct {
	TicketID string `json:"ticket_id"`
	Ref      string `json:"ref"`
	UserID string `json:"user_id"`
	Chain  string `json:"chain"`
	Address string `json:"address"`
	Reason  string `json:"reason"`
	At time.Time `json:"at"`
}

// ApprovalTicketFact 把出金审批票挂到冻结节点上。不跑审批链。
type ApprovalTicketFact struct {
	TicketID string `json:"ticket_id"`
	Ref      string `json:"ref"`
	UserID   string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Level string `json:"level"`
	At time.Time `json:"at"`
}

// ReleaseRefFact 只记录审批票到放行引用的边，不记放行会计。

type ReleaseRefFact struct {
	ReleaseID string `json:"release_id"`
	TicketID  string `json:"ticket_id"`
	Ref       string `json:"ref"`
	Amount string `json:"amount"`
	Asset  string `json:"asset"`
	At time.Time `json:"at"`
}

// HotReserveFact 把热预留单指向冻结或广播。不记预留科目分录。

type HotReserveFact struct {
	TicketID string `json:"ticket_id"`
	Ref      string `json:"ref"`
	Asset    string `json:"asset"`
	Amount string `json:"amount"`
	WalletID string    `json:"wallet_id"`
	Target   NodeKind  `json:"target"`
	At       time.Time `json:"at"`
}

// EnvelopeRefFact 把不透明信封号挂成引用节点。不写入外部审计仓。

type EnvelopeRefFact struct {
	Ref        string    `json:"ref"`
	EnvelopeID string    `json:"envelope_id"`
	ParentKind NodeKind  `json:"parent_kind"`
	ParentRef  string    `json:"parent_ref"`
	At         time.Time `json:"at"`
}

// AdaptSweepPlan 生成归集计划节点。
   func AdaptSweepPlan(f SweepPlanFact) (IngestBatch, error) {
	   id, err := cleanRef(f.PlanID)
	  if err != nil {
		  return IngestBatch{}, err
	   }
	  amt, err := positiveAmount(f.Amount)
	  if err != nil {
		   return IngestBatch{}, err
	  }
	   at := atOr(f.At, time.Now())
	   n := baseNode(NodeSweepPlan, id, "", f.Asset, amt, "sweep", at)
	   n.Chain = strings.ToUpper(strings.TrimSpace(f.Chain))
	  n.Attrs = putAttr(nil, "from_wallet", f.FromWallet)
	   n.Attrs = putAttr(n.Attrs, "to_wallet", f.ToWallet)
	   return IngestBatch{Source: "sweep", Stream: "plan", EventAt: at, Nodes: []FundNode{n}}, nil
   }

// AdaptSweepProof 生成证明节点。给出 CreditRef 时再加一条指向贷记引用的边。
   func AdaptSweepProof(f SweepProofFact) (IngestBatch, error) {
	   plan, err := cleanRef(f.PlanID)
	   if err != nil {
		  return IngestBatch{}, err
	  }
	  proof, err := cleanRef(f.ProofID)
	   if err != nil {
		   return IngestBatch{}, err
	   }
	  amt, err := positiveAmount(f.Amount)
	  if err != nil {
		   return IngestBatch{}, err
	   }
	  at := atOr(f.At, time.Now())
	  n := baseNode(NodeSweepProof, proof, "", f.Asset, amt, "sweep", at)
	   n.Attrs = putAttr(nil, "plan_id", plan)
	  n.Attrs = putAttr(n.Attrs, "corr", plan)
	   n.Attrs = putAttr(n.Attrs, "envelope_id", f.EnvelopeID)
	  from := NodeID(NodeSweepPlan, plan)
	   e := baseEdge(from, n.ID, EdgeSweepPlanProof, plan, "", f.Asset, amt, "sweep", f.EnvelopeID, at)
	   batch := IngestBatch{Source: "sweep", Stream: "proof", EventAt: at, Nodes: []FundNode{n}, Edges: []FundEdge{e}}
	   if strings.TrimSpace(f.CreditRef) != "" {
		  credit := NodeID(NodeDepositCredit,
			  strings.TrimSpace(f.CreditRef))
		   link := baseEdge(n.ID, credit, EdgeSweepProofCredit, plan, "", f.Asset, amt, "sweep", f.EnvelopeID, at)
		  batch.Edges = append(batch.Edges, link)
	   }
	  return batch, nil
  }

// AdaptReconTicket 按父节点种类选择对账边。
  func AdaptReconTicket(f ReconTicketFact) (IngestBatch, error) {
	  ticket, err := cleanRef(f.TicketID)
	  if err != nil {
		  return IngestBatch{}, err
	   }
	  ref, err := cleanRef(f.Ref)
	  if err != nil {
		  return IngestBatch{}, err
	   }
	   parent := f.ParentKind
	  if parent == "" {
		   parent = NodeDepositObserve
	  }
	   edgeType, ok := reconEdge(parent)
	   if !ok {
		  return IngestBatch{}, invalidf("recon parent kind")
	   }
	  at := atOr(f.At, time.Now())
	  amt := strings.TrimSpace(f.Amount)
	   if amt != "" {
		   amt, err = positiveAmount(amt)
		   if err != nil {
			  return IngestBatch{}, err
		  }
	   }
	  n := baseNode(NodeReconTicket, ticket, f.UserID, f.Asset, amt, "recon", at)
	  n.Attrs = putAttr(nil, "corr", ref)
	  n.Attrs = putAttr(n.Attrs, "ticket_kind", f.TicketKind)
	  from := NodeID(parent, ref)
	  e := baseEdge(from, n.ID, edgeType, ref, f.UserID, f.Asset, amt, "recon", "", at)
	  return IngestBatch{Source: "recon", Stream: "ticket", EventAt: at, Nodes: []FundNode{n}, Edges: []FundEdge{e}}, nil
   }

  func reconEdge(parent NodeKind) (EdgeType, bool) {
	  switch parent {
	  case NodeDepositObserve:
		   return EdgeObserveRecon, true
	   case NodeDepositCredit:
		  return EdgeCreditRecon, true
	  case NodeWithdrawFreeze:
		   return EdgeFreezeRecon, true
	   case NodeWithdrawBroadcast:
		   return EdgeBroadcastRecon, true
	  default:
		   return "", false
	   }
  }

// AdaptAddrProtect 生成地址防护单，并把它挂在观测之后。
  func AdaptAddrProtect(f AddrProtectFact) (IngestBatch, error) {
	   ticket, err := cleanRef(f.TicketID)
	  if err != nil {
		  return IngestBatch{}, err
	  }
	   ref, err := cleanRef(f.Ref)
	   if err != nil {
		   return IngestBatch{}, err
	  }
	  if strings.TrimSpace(f.Address) == "" {
		  return IngestBatch{}, invalidf("protect address required")
	  }
	   at := atOr(f.At, time.Now())
	   n := baseNode(NodeAddrpoolProtect, ticket, f.UserID, "", "", "addrpool", at)
	   n.Chain = strings.ToUpper(strings.TrimSpace(f.Chain))
	   n.Attrs = putAttr(nil, "corr", ref)
	   n.Attrs = putAttr(n.Attrs, "address", f.Address)
	  n.Attrs = putAttr(n.Attrs, "reason", f.Reason)
	   from := NodeID(NodeDepositObserve, ref)
	   e := baseEdge(from, n.ID, EdgeAddrpoolProtect, ref, f.UserID, "", "", "addrpool", "", at)
	  return IngestBatch{Source: "addrpool", Stream: "protect", EventAt: at, Nodes: []FundNode{n}, Edges: []FundEdge{e}}, nil
   }

// AdaptApprovalTicket 生成审批票节点。不计算审批层级。
   func AdaptApprovalTicket(f ApprovalTicketFact) (IngestBatch, error) {
	  ticket, err := cleanRef(f.TicketID)
	  if err != nil {
		  return IngestBatch{}, err
	  }
	  ref, err := cleanRef(f.Ref)
	   if err != nil {
		  return IngestBatch{}, err
	  }
	   amt, err := positiveAmount(f.Amount)
	  if err != nil {
		   return IngestBatch{}, err
	   }
	   at := atOr(f.At, time.Now())
	   n := baseNode(NodeWithdrawApproval, ticket, f.UserID, f.Asset, amt, "approval", at)
	  n.Attrs = putAttr(nil, "corr", ref)
	   n.Attrs = putAttr(n.Attrs, "level", f.Level)
	   from := NodeID(NodeWithdrawFreeze, ref)
	   e := baseEdge(from, n.ID, EdgeFreezeApproval, ref, f.UserID, f.Asset, amt, "approval", "", at)
	  return IngestBatch{Source: "approval", Stream: "ticket", EventAt: at, Nodes: []FundNode{n}, Edges: []FundEdge{e}}, nil
   }

// AdaptReleaseRef 生成放行引用节点。
   func AdaptReleaseRef(f ReleaseRefFact) (IngestBatch, error) {
	   rel, err := cleanRef(f.ReleaseID)
	  if err != nil {
		   return IngestBatch{}, err
	   }
	   ticket,
		  err := cleanRef(f.TicketID)
	   if err != nil {
		   return IngestBatch{}, err
	   }
	   ref, err := cleanRef(f.Ref)
	   if err != nil {
		   return IngestBatch{}, err
	   }
	   amt, err := positiveAmount(f.Amount)
	  if err != nil {
		   return IngestBatch{}, err
	  }
	   at := atOr(f.At, time.Now())
	  n := baseNode(NodeWithdrawRelease, rel, "", f.Asset, amt, "approval", at)
	  n.Attrs = putAttr(nil, "corr", ref)
	  n.Attrs = putAttr(n.Attrs, "ticket_id", ticket)
	   from := NodeID(NodeWithdrawApproval, ticket)
	   e := baseEdge(from, n.ID, EdgeApprovalRelease, ref, "", f.Asset, amt, "approval", "", at)
	  return IngestBatch{Source: "approval", Stream: "release", EventAt: at, Nodes: []FundNode{n}, Edges: []FundEdge{e}}, nil
  }

// AdaptHotReserve 生成热预留单。Target 只能是冻结或广播。

   func AdaptHotReserve(f HotReserveFact) (IngestBatch, error) {
	  ticket, err := cleanRef(f.TicketID)
	  if err != nil {
		  return IngestBatch{}, err
	  }
	   ref, err := cleanRef(f.Ref)
	  if err != nil {
		   return IngestBatch{}, err
	   }
	   amt, err := positiveAmount(f.Amount)
	  if err != nil {
		   return IngestBatch{}, err
	   }
	  target := f.Target
	  if target == "" {
		   target = NodeWithdrawFreeze
	   }
	   var edgeType EdgeType
	   switch target {
	   case NodeWithdrawFreeze:
		  edgeType = EdgeHotReserveHold
	  case NodeWithdrawBroadcast:
		  edgeType = EdgeReserveBroadcast
	   default:
		  return IngestBatch{}, invalidf("reserve target")
	  }
	   at := atOr(f.At, time.Now())
	  n := baseNode(NodeHotReserve, ticket, "", f.Asset, amt, "hotreserve", at)
	   n.Attrs = putAttr(nil, "corr", ref)
	  n.Attrs = putAttr(n.Attrs, "wallet_id", f.WalletID)
	   to := NodeID(target, ref)
	   e := baseEdge(n.ID, to, edgeType, ref, "", f.Asset, amt, "hotreserve", "", at)
	   return IngestBatch{Source: "hotreserve", Stream: "ticket", EventAt: at, Nodes: []FundNode{n}, Edges: []FundEdge{e}}, nil
   }

// AdaptEnvelopeRef 保存信封号引用。父节点种类必须在冻结枚举内。
  func AdaptEnvelopeRef(f EnvelopeRefFact) (IngestBatch, error) {
	  ref, err := cleanRef(f.Ref)
	  if err != nil {
		   return IngestBatch{}, err
	   }
	   env := strings.TrimSpace(f.EnvelopeID)
	   if env == "" {
		  return IngestBatch{}, invalidf("envelope id required")
	   }
	  parent := f.ParentKind
	   if parent == "" {
		  parent = NodeDepositObserve
	   }
	  if !KnownNodeKind(parent) {
		  return IngestBatch{}, invalidf("envelope parent kind")
	  }
	  parentRef := strings.TrimSpace(f.ParentRef)
	  if parentRef == "" {
		   parentRef = ref
	  }
	   at := atOr(f.At, time.Now())
	  n := baseNode(NodeEnvelopeRef, env, "", "", "", "fundgraph", at)
	   n.Attrs = putAttr(nil, "corr", ref)
	  n.Attrs = putAttr(n.Attrs, "envelope_id", env)
	   from := NodeID(parent, parentRef)
	   e := baseEdge(from, n.ID, EdgeEnvelopeRef, ref, "", "", "", "fundgraph", env, at)
	   return IngestBatch{Source: "fundgraph", Stream: "envelope", EventAt: at, Nodes: []FundNode{n}, Edges: []FundEdge{e}}, nil
   }
