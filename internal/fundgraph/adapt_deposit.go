package fundgraph

import (
	"strconv"
	"strings"
	"time"
)

// DepositObserveFact 是只读入金观测事实。本包不推进充值状态。
type DepositObserveFact struct {
	Ref    string `json:"ref"`
	UserID string `json:"user_id"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Chain  string `json:"chain"`
	TxHash string `json:"tx_hash"`
	OutputIndex int    `json:"output_index"`
	Address     string `json:"address"`
	EnvelopeID  string `json:"envelope_id"`
	ObservedAt time.Time `json:"observed_at"`
}

// DepositCreditFact 是只读贷记事实，用同一 ref 连到观测节点。

type DepositCreditFact struct {
	Ref string `json:"ref"`
	UserID     string `json:"user_id"`
	Asset      string `json:"asset"`
	Amount     string `json:"amount"`
	CreditRef  string `json:"credit_ref"`
	EnvelopeID string `json:"envelope_id"`
	CreditedAt time.Time `json:"credited_at"`
}

// DepositRollbackFact 记录贷记之后的回退引用，不执行回退。
type DepositRollbackFact struct {
	Ref    string `json:"ref"`
	UserID string `json:"user_id"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	RollbackID string `json:"rollback_id"`
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
}

// AdaptDepositObserve 生成观测节点。

   func AdaptDepositObserve(f DepositObserveFact) (IngestBatch, error) {
	   ref, err := cleanRef(f.Ref)
	   if err != nil {
		  return IngestBatch{}, err
	  }
	  amt, err := positiveAmount(f.Amount)
	   if err != nil {
		  return IngestBatch{}, err
	   }
	   if strings.TrimSpace(f.UserID) == "" || strings.TrimSpace(f.Asset) == "" {
		  return IngestBatch{}, invalidf("observe user and asset required")
	  }
	  if strings.TrimSpace(f.Chain) == "" || strings.TrimSpace(f.TxHash) == "" {
		  return IngestBatch{}, invalidf("observe chain and tx required")
	  }
	  at := atOr(f.ObservedAt, time.Now())
	   n := baseNode(NodeDepositObserve, ref, f.UserID, f.Asset, amt, "deposit", at)
	   n.Chain = strings.ToUpper(strings.TrimSpace(f.Chain))
	  n.TxHash = strings.TrimSpace(f.TxHash)
	   n.Attrs = putAttr(nil, "output_index", strconv.Itoa(f.OutputIndex))
	  n.Attrs = putAttr(n.Attrs, "address", f.Address)
	  n.Attrs = putAttr(n.Attrs, "envelope_id", f.EnvelopeID)
	  return IngestBatch{
		   Source: "deposit",
		   Stream:  "observe",
		   EventAt: at,
		   Nodes:   []FundNode{n},
	  }, nil
   }

// AdaptDepositCredit 生成贷记节点，以及观测到贷记的边。观测节点可以稍后才到。

   func AdaptDepositCredit(f DepositCreditFact) (IngestBatch, error) {
	   ref, err := cleanRef(f.Ref)
	  if err != nil {
		   return IngestBatch{}, err
	   }
	   amt, err := positiveAmount(f.Amount)
	   if err != nil {
		   return IngestBatch{}, err
	  }
	   if strings.TrimSpace(f.UserID) == "" || strings.TrimSpace(f.Asset) == "" {
		   return IngestBatch{}, invalidf("credit user and asset required")
	   }
	  at := atOr(f.CreditedAt, time.Now())
	  n := baseNode(NodeDepositCredit, ref, f.UserID, f.Asset, amt, "deposit", at)
	   n.Attrs = putAttr(nil, "credit_ref", f.CreditRef)
	   n.Attrs = putAttr(n.Attrs, "envelope_id", f.EnvelopeID)
	  from := NodeID(NodeDepositObserve, ref)
	  e := baseEdge(from, n.ID, EdgeObserveCredit, ref, f.UserID, f.Asset, amt, "deposit", f.EnvelopeID, at)
	  return IngestBatch{
		   Source: "deposit",
		   Stream:  "credit",
		  EventAt: at,
		   Nodes: []FundNode{n},
		  Edges: []FundEdge{e},
	   }, nil
   }

// AdaptDepositRollback 把回退事实挂到贷记节点之后。
  func AdaptDepositRollback(f DepositRollbackFact) (IngestBatch, error) {
	   ref, err := cleanRef(f.Ref)
	  if err != nil {
		  return IngestBatch{}, err
	  }
	   amt, err := positiveAmount(f.Amount)
	   if err != nil {
		   return IngestBatch{}, err
	  }
	   rb := strings.TrimSpace(f.RollbackID)
	   if rb == "" {
		  return IngestBatch{}, invalidf("rollback id required")
	  }
	  at := atOr(f.At, time.Now())
	  n := baseNode(NodeDepositRollback, rb, f.UserID, f.Asset, amt, "deposit", at)
	   n.Attrs = putAttr(nil, "corr", ref)
	   n.Attrs = putAttr(n.Attrs, "reason", f.Reason)
	  from := NodeID(NodeDepositCredit, ref)
	   e := baseEdge(from, n.ID, EdgeDepositRollback, ref, f.UserID, f.Asset, amt, "deposit", "", at)
	  return IngestBatch{
		   Source:  "deposit",
		   Stream:  "rollback",
		  EventAt: at,
		   Nodes:   []FundNode{n},
		  Edges: []FundEdge{e},
	  }, nil
  }
