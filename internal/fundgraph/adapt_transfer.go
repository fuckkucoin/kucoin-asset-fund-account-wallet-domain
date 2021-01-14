package fundgraph

import (
	"strconv"
	"strings"
	"time"
)

// TransferHopFact 是一跳划转。ParentKind 为空时默认连到同一 ref 的贷记节点。
type TransferHopFact struct {
	Ref string `json:"ref"`
	HopRef string `json:"hop_ref"`
	Hop int `json:"hop"`
	UserID   string `json:"user_id"`
	FromUser string `json:"from_user"`
	ToUser   string `json:"to_user"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	ParentKind NodeKind `json:"parent_kind"`
	ParentRef  string    `json:"parent_ref"`
	Compensate bool      `json:"compensate"`
	EnvelopeID string    `json:"envelope_id"`
	At         time.Time `json:"at"`
}

// AdaptTransferHop 生成划转节点和指向父事实的边。不调用账本入账。

  func AdaptTransferHop(f TransferHopFact) (IngestBatch, error) {
	  ref, err := cleanRef(f.Ref)
	  if err != nil {
		   return IngestBatch{}, err
	  }
	  amt, err := positiveAmount(f.Amount)
	  if err != nil {
		   return IngestBatch{}, err
	   }
	   if strings.TrimSpace(f.Asset) == "" {
		   return IngestBatch{}, invalidf("transfer asset required")
	   }
	  if f.Hop < 0 {
		   return IngestBatch{}, invalidf("hop index")
	  }
	   hopRef := strings.TrimSpace(f.HopRef)
	  if hopRef == "" {
		   hopRef = ref
		   if f.Hop > 0 {
			   hopRef = ref + "#hop-" + strconv.Itoa(f.Hop)
		   }
	   }
	   parentKind := f.ParentKind
	   if parentKind == "" {
		  if f.Hop > 0 {
			  parentKind = NodeTransferHop
		   } else {
			  parentKind = NodeDepositCredit
		  }
	   }
	   if !KnownNodeKind(parentKind) {
		   return IngestBatch{}, invalidf("transfer parent kind")
	   }
	  parentRef := strings.TrimSpace(f.ParentRef)
	   if parentRef == "" {
		  if f.Hop > 1 {
			   parentRef = ref + "#hop-" + strconv.Itoa(f.Hop-1)
		   } else if f.Hop == 1 {
			   parentRef = ref
		  } else {
			  parentRef = ref
		  }
	   }
	   edgeType := EdgeCreditTransfer
	  switch parentKind {
	   case NodeDepositCredit:
		  edgeType = EdgeCreditTransfer
	   case NodeTransferHop:
		  if f.Compensate {
			   edgeType = EdgeTransferCompensate
		  } else {
			   edgeType = EdgeTransferHop
		   }
	  default:
		  return IngestBatch{}, invalidf("transfer parent kind unsupported")
	  }
	   at := atOr(f.At, time.Now())
	  n := baseNode(NodeTransferHop, hopRef, f.UserID, f.Asset, amt, "transfer", at)
	  n.Attrs = putAttr(nil, "corr", ref)
	   n.Attrs = putAttr(n.Attrs, "hop", strconv.Itoa(f.Hop))
	   n.Attrs = putAttr(n.Attrs, "from_user", f.FromUser)
	  n.Attrs = putAttr(n.Attrs, "to_user", f.ToUser)
	   n.Attrs = putAttr(n.Attrs, "envelope_id", f.EnvelopeID)
	   from := NodeID(parentKind, parentRef)
	   e := baseEdge(from, n.ID, edgeType, ref, f.UserID, f.Asset, amt, "transfer", f.EnvelopeID, at)
	  e.Attrs = putAttr(nil, "hop", strconv.Itoa(f.Hop))
	  return IngestBatch{
		   Source: "transfer",
		  Stream: "hop",
		   EventAt: at,
		   Nodes:   []FundNode{n},
		   Edges:   []FundEdge{e},
	   }, nil
  }
