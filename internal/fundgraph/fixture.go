package fundgraph

import (
	"strconv"
	"time"
)

// FixtureClock 是夹具使用的固定时钟，避免和巡检滞后阈值搅在一起。
var FixtureClock = time.Date(2026, 3, 21, 8, 0, 0, 0, time.UTC)

// SpineParams 描述一条观测到广播的主链。

type SpineParams struct {
	Ref    string
	UserID string
	Asset  string
	Amount string
	Chain string
	TxHash string
	EnvelopeID string
	At time.Time
}

  func (p SpineParams) fill() SpineParams {
	   if p.Ref == "" {
		   p.Ref = "ref-1"
	  }
	  if p.UserID == "" {
		   p.UserID = "u1"
	   }
	  if p.Asset == "" {
		   p.Asset = "USDT"
	   }
	   if p.Amount == "" {
		  p.Amount = "1"
	   }
	  if p.Chain == "" {
		   p.Chain = "ETH"
	   }
	   if p.TxHash == "" {
		  p.TxHash = "0xspine"
	  }
	   if p.At.IsZero() {
		   p.At = FixtureClock
	  }
	  return p
   }

// FixtureSpine 用各域适配器拼出观测、贷记、划转、冻结、广播。

   func FixtureSpine(p SpineParams) (IngestBatch, error) {
	  p = p.fill()
	   obs, err := AdaptDepositObserve(DepositObserveFact{
		  Ref:    p.Ref,
		   UserID: p.UserID, Asset: p.Asset, Amount: p.Amount,
		   Chain: p.Chain, TxHash: p.TxHash, EnvelopeID: p.EnvelopeID, ObservedAt: p.At,
	   })
	  if err != nil {
		  return IngestBatch{}, err
	   }
	  credit, err := AdaptDepositCredit(DepositCreditFact{
		  Ref: p.Ref, UserID: p.UserID, Asset: p.Asset, Amount: p.Amount,
		  CreditRef: "led-" + p.Ref, EnvelopeID: p.EnvelopeID, CreditedAt: p.At,
	   })
	  if err != nil {
		   return IngestBatch{}, err
	   }
	  hop, err := AdaptTransferHop(TransferHopFact{
		  Ref: p.Ref, UserID: p.UserID, FromUser: p.UserID, ToUser: p.UserID,
		  Asset: p.Asset, Amount: p.Amount, EnvelopeID: p.EnvelopeID, At: p.At,
	  })
	  if err != nil {
		   return IngestBatch{}, err
	   }
	   freeze, err := AdaptWithdrawFreeze(WithdrawFreezeFact{
		   Ref:    p.Ref,
		  UserID: p.UserID, Asset: p.Asset, Amount: p.Amount,
		   FreezeID: "fr-" + p.Ref, EnvelopeID: p.EnvelopeID, At: p.At,
	  })
	  if err != nil {
		  return IngestBatch{}, err
	   }
	  br, err := AdaptWithdrawBroadcast(WithdrawBroadcastFact{
		  Ref: p.Ref, UserID: p.UserID, Asset: p.Asset, Amount: p.Amount,
		  Chain: p.Chain, TxHash: p.TxHash, BroadcastID: "br-" + p.Ref,
		   EnvelopeID: p.EnvelopeID, At: p.At,
	   })
	   if err != nil {
		  return IngestBatch{}, err
	   }
	   return MergeBatches(obs, credit, hop, freeze, br), nil
   }

// WithoutMiddle 去掉划转节点以及连到它的边，用来制造主链断口。
   func WithoutMiddle(b IngestBatch) IngestBatch {
	   drop := map[string]bool{}
	  nodes := make([]FundNode, 0, len(b.Nodes))
	  for _, n := range b.Nodes {
		   if n.Kind == NodeTransferHop {
			  drop[n.ID] = true
			   continue
		   }
		   nodes = append(nodes, n)
	   }
	  edges := make([]FundEdge,
		   0, len(b.Edges))
	   for _, e := range b.Edges {
		   if drop[e.FromID] || drop[e.ToID] {
			  continue
		  }
		   switch e.EdgeType {
		   case EdgeCreditTransfer,
			   EdgeTransferFreeze, EdgeTransferHop, EdgeTransferCompensate:
			   continue
		  }
		   edges = append(edges, e)
	   }
	  b.Nodes = nodes
	  b.Edges = edges
	  return b
   }

// FixtureCycle 注入一个三节点有向环。
  func FixtureCycle(ref string) IngestBatch {
	  if ref == "" {
		   ref = "cycle-1"
	  }
	   ids := []string{
		  NodeID(NodeTransferHop, ref+"#1"),
		  NodeID(NodeTransferHop, ref+"#2"),
		  NodeID(NodeTransferHop, ref+"#3"),
	   }
	  nodes := make([]FundNode, 0, 3)
	  for i, id := range ids {
		  nodes = append(nodes, FundNode{
			  ID: id,
			   Kind: NodeTransferHop,
			   Ref:    ref + "#" + strconv.Itoa(i+1),
			  UserID: "u-cycle",
			   Asset:  "USDT",
			   Amount: "1",
			  Source: "fixture",
			  OccurredAt: FixtureClock,
			  Attrs:      map[string]string{"corr": ref, "hop": strconv.Itoa(i + 1)},
		   })
	  }
	  edges := make([]FundEdge, 0, 3)
	  for i := 0; i < 3; i++ {
		   j := (i + 1) % 3
		  edges = append(edges, FundEdge{
			   FromID: ids[i],
			   ToID:   ids[j],
			   EdgeType: EdgeTransferHop,
			  Ref:      ref,
			   UserID: "u-cycle",
			   Asset:  "USDT",
			   Amount: "1",
			   Source:     "fixture",
			   OccurredAt: FixtureClock,
		   })
	   }
	  return IngestBatch{Source: "fixture", Stream: "cycle", EventAt: FixtureClock, Nodes: nodes, Edges: edges}
   }

// FixtureHopChain 生成一条只有划转跳的长链，用来验证跳数和分页。

  func FixtureHopChain(n int, ref string) IngestBatch {
	   if n < 1 {
		  n = 1
	  }
	  if ref == "" {
		   ref = "hops"
	   }
	  nodes := make([]FundNode, 0, n)
	  var edges []FundEdge
	   var prev string
	  for i := 0; i < n; i++ {
		  id := NodeID(NodeTransferHop, ref+"#"+strconv.Itoa(i))
		  nodes = append(nodes, FundNode{
			  ID:   id,
			   Kind: NodeTransferHop,
			  Ref:    ref + "#" + strconv.Itoa(i),
			  UserID: "u-hop",
			  Asset:  "USDT",
			   Amount: "1",
			  Source: "fixture",
			   OccurredAt: FixtureClock,
			  Attrs: map[string]string{"corr": ref, "hop": strconv.Itoa(i)},
		   })
		   if prev != "" {
			   edges = append(edges, FundEdge{
				   FromID:   prev,
				  ToID:     id,
				  EdgeType: EdgeTransferHop,
				   Ref: ref,
				  UserID: "u-hop",
				  Asset:  "USDT",
				  Amount: "1",
				  Source: "fixture",
				   OccurredAt: FixtureClock,
			  })
		   }
		   prev = id
	   }
	   return IngestBatch{Source: "fixture", Stream: "hops", EventAt: FixtureClock, Nodes: nodes, Edges: edges}
  }

// FixtureMultiParent 让同一贷记节点挂上两个观测父边。

  func FixtureMultiParent(ref string) IngestBatch {
	  if ref == "" {
		  ref = "mp-1"
	  }
	  credit := FundNode{
		   ID: NodeID(NodeDepositCredit, ref), Kind: NodeDepositCredit, Ref: ref,
		   UserID: "u-mp", Asset: "USDT", Amount: "2", Source: "fixture", OccurredAt: FixtureClock,
	   }
	   o1 := FundNode{
		   ID: NodeID(NodeDepositObserve, ref+"#a"), Kind: NodeDepositObserve, Ref: ref + "#a",
		  UserID: "u-mp", Asset: "USDT", Amount: "2", Source: "fixture", OccurredAt: FixtureClock,
		  Attrs: map[string]string{"corr": ref}, Chain: "ETH", TxHash: "0xa",
	  }
	  o2 := FundNode{
		  ID: NodeID(NodeDepositObserve, ref+"#b"), Kind: NodeDepositObserve, Ref: ref + "#b",
		   UserID: "u-mp", Asset: "USDT", Amount: "2", Source: "fixture", OccurredAt: FixtureClock,
		   Attrs: map[string]string{"corr": ref}, Chain: "ETH", TxHash: "0xb",
	   }
	  e1 := FundEdge{FromID: o1.ID, ToID: credit.ID, EdgeType: EdgeObserveCredit, Ref: ref, UserID: "u-mp", Asset: "USDT", Amount: "2", Source: "fixture", OccurredAt: FixtureClock}
	   e2 := FundEdge{FromID: o2.ID, ToID: credit.ID, EdgeType: EdgeObserveCredit, Ref: ref, UserID: "u-mp", Asset: "USDT", Amount: "2", Source: "fixture", OccurredAt: FixtureClock}
	  return IngestBatch{Source: "fixture", Stream: "multi", EventAt: FixtureClock, Nodes: []FundNode{o1, o2, credit}, Edges: []FundEdge{e1, e2}}
   }

// FixtureDangling 有一条指向不存在节点的边。
  func FixtureDangling(ref string) IngestBatch {
	   if ref == "" {
		  ref = "dang-1"
	  }
	   n := FundNode{
		  ID: NodeID(NodeDepositObserve, ref), Kind: NodeDepositObserve, Ref: ref,
		   UserID: "u-d", Asset: "USDT", Amount: "1", Chain: "ETH", TxHash: "0xd",
		   Source: "fixture", OccurredAt: FixtureClock,
	  }
	  e := FundEdge{
		  FromID: n.ID, ToID: "missing-node", EdgeType: EdgeObserveCredit, Ref: ref,
		  UserID: "u-d", Asset: "USDT", Amount: "1", Source: "fixture", OccurredAt: FixtureClock,
	   }
	  return IngestBatch{Source: "fixture", Stream: "dangling", EventAt: FixtureClock, Nodes: []FundNode{n}, Edges: []FundEdge{e}}
   }
