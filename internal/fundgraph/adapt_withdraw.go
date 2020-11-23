package fundgraph

import (
	"strings"
	"time"
)

// WithdrawFreezeFact 是出金冻结事实。默认父节点是同一 ref 的划转跳。

type WithdrawFreezeFact struct {
	Ref    string `json:"ref"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount     string   `json:"amount"`
	FreezeID   string   `json:"freeze_id"`
	ParentKind NodeKind `json:"parent_kind"`
	ParentRef  string    `json:"parent_ref"`
	EnvelopeID string    `json:"envelope_id"`
	At         time.Time `json:"at"`
}

// WithdrawBroadcastFact 是广播事实。只记录哈希，不提交链上交易。
type WithdrawBroadcastFact struct {
	Ref string `json:"ref"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Chain       string `json:"chain"`
	TxHash      string `json:"tx_hash"`
	BroadcastID string `json:"broadcast_id"`
	EnvelopeID string `json:"envelope_id"`
	At time.Time `json:"at"`
}

// WithdrawConfirmFact 是广播之后的确认引用。
type WithdrawConfirmFact struct {
	Ref    string `json:"ref"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Chain  string `json:"chain"`
	TxHash string `json:"tx_hash"`
	ConfirmID string `json:"confirm_id"`
	EnvelopeID string    `json:"envelope_id"`
	At         time.Time `json:"at"`
}

// AdaptWithdrawFreeze 生成冻结节点和划转到冻结的边。

  func AdaptWithdrawFreeze(f WithdrawFreezeFact) (IngestBatch, error) {
	   ref, err := cleanRef(f.Ref)
	   if err != nil {
		   return IngestBatch{}, err
	   }
	  amt, err := positiveAmount(f.Amount)
	   if err != nil {
		   return IngestBatch{}, err
	  }
	   if strings.TrimSpace(f.UserID) == "" || strings.TrimSpace(f.Asset) == "" {
		  return IngestBatch{}, invalidf("freeze user and asset required")
	  }
	   parentKind := f.ParentKind
	   if parentKind == "" {
		  parentKind = NodeTransferHop
	   }
	   if parentKind != NodeTransferHop {
		   return IngestBatch{}, invalidf("freeze parent kind")
	   }
	  parentRef := strings.TrimSpace(f.ParentRef)
	   if parentRef == "" {
		  parentRef = ref
	  }
	  edgeType := EdgeTransferFreeze
	  at := atOr(f.At, time.Now())
	  idRef := ref
	   n := baseNode(NodeWithdrawFreeze, idRef, f.UserID, f.Asset, amt, "withdraw", at)
	  n.Attrs = putAttr(nil, "freeze_id", f.FreezeID)
	  n.Attrs = putAttr(n.Attrs, "corr", ref)
	  n.Attrs = putAttr(n.Attrs, "envelope_id", f.EnvelopeID)
	   from := NodeID(parentKind, parentRef)
	  e := baseEdge(from, n.ID, edgeType, ref, f.UserID, f.Asset, amt, "withdraw", f.EnvelopeID, at)
	  return IngestBatch{
		   Source:  "withdraw",
		   Stream:  "freeze",
		   EventAt: at,
		  Nodes:   []FundNode{n},
		   Edges: []FundEdge{e},
	  }, nil
   }

// AdaptWithdrawBroadcast 生成广播节点和冻结到广播的边。

  func AdaptWithdrawBroadcast(f WithdrawBroadcastFact) (IngestBatch, error) {
	  ref, err := cleanRef(f.Ref)
	  if err != nil {
		   return IngestBatch{}, err
	  }
	  amt, err := positiveAmount(f.Amount)
	  if err != nil {
		  return IngestBatch{}, err
	  }
	   if strings.TrimSpace(f.TxHash) == "" {
		  return IngestBatch{}, invalidf("broadcast tx required")
	  }
	   at := atOr(f.At, time.Now())
	   n := baseNode(NodeWithdrawBroadcast, ref, f.UserID, f.Asset, amt, "withdraw", at)
	  n.Chain = strings.ToUpper(strings.TrimSpace(f.Chain))
	  n.TxHash = strings.TrimSpace(f.TxHash)
	  n.Attrs = putAttr(nil, "broadcast_id", f.BroadcastID)
	   n.Attrs = putAttr(n.Attrs, "corr", ref)
	  n.Attrs = putAttr(n.Attrs, "envelope_id", f.EnvelopeID)
	   from := NodeID(NodeWithdrawFreeze, ref)
	   e := baseEdge(from, n.ID, EdgeFreezeBroadcast, ref, f.UserID, f.Asset, amt, "withdraw", f.EnvelopeID, at)
	   return IngestBatch{
		   Source:  "withdraw",
		  Stream:  "broadcast",
		  EventAt: at,
		   Nodes: []FundNode{n},
		  Edges: []FundEdge{e},
	  }, nil
   }

// AdaptWithdrawConfirm 生成确认节点和广播到确认的边。
  func AdaptWithdrawConfirm(f WithdrawConfirmFact) (IngestBatch, error) {
	  ref, err := cleanRef(f.Ref)
	   if err != nil {
		  return IngestBatch{}, err
	   }
	   amt, err := positiveAmount(f.Amount)
	   if err != nil {
		  return IngestBatch{}, err
	  }
	  at := atOr(f.At, time.Now())
	   idRef := strings.TrimSpace(f.ConfirmID)
	  if idRef == "" {
		   idRef = ref
	   }
	  n := baseNode(NodeWithdrawConfirm, idRef, f.UserID, f.Asset, amt, "withdraw", at)
	   n.Chain = strings.ToUpper(strings.TrimSpace(f.Chain))
	  n.TxHash = strings.TrimSpace(f.TxHash)
	   n.Attrs = putAttr(nil, "corr", ref)
	   n.Attrs = putAttr(n.Attrs, "envelope_id", f.EnvelopeID)
	  from := NodeID(NodeWithdrawBroadcast, ref)
	  e := baseEdge(from, n.ID, EdgeBroadcastConfirm, ref, f.UserID, f.Asset, amt, "withdraw", f.EnvelopeID, at)
	  return IngestBatch{
		  Source: "withdraw",
		   Stream: "confirm",
		   EventAt: at,
		   Nodes: []FundNode{n},
		  Edges: []FundEdge{e},
	   }, nil
   }
