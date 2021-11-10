package reclaim

import (
	"sync"
	"time"
)

// LegKind Sweep leg 类型。
type LegKind string

const (
	// LegKindOccupancyRelease 占用释放证明——记录 ReclaimOccupancy.Release 调用。
	LegKindOccupancyRelease LegKind = "occupancy_release"
	// LegKindRefundBroadcast 退款交易已广播——链上 tx_hash 已知。
	LegKindRefundBroadcast LegKind = "refund_broadcast"
	// LegKindRefundConfirmed 退款交易已确认——链上确认数达到阈值。
	LegKindRefundConfirmed LegKind = "refund_confirmed"
	// LegKindSweepToHot 归集至热钱包（平台回收场景）。
	LegKindSweepToHot LegKind = "sweep_to_hot"
	// LegKindProofAnchor 证明链锚定——SHA-256 哈希链最终节点。
	LegKindProofAnchor LegKind = "proof_anchor"
)

// SweepLeg 单条 Sweep leg 记录。

type SweepLeg struct {
	ID string `json:"id"`
	CaseID string `json:"case_id"`
	Seq    int    `json:"seq"`
	Kind   LegKind `json:"kind"`
	TxHash string  `json:"tx_hash,omitempty"` // 链上操作 tx_hash（可选）
	ProofHash string `json:"proof_hash,omitempty"` // SHA-256 证明哈希（可选）
	Note      string `json:"note,omitempty"`
	Actor string `json:"actor"`
	At time.Time `json:"at"`
}

// SweepLegGroup 一个案件的完整 leg 目录。

type SweepLegGroup struct {
	CaseID    string `json:"case_id"`
	OutputKey string `json:"output_key"` // TxHash:Vout
	Chain string `json:"chain"`
	Asset string `json:"asset"`
	Amount string `json:"amount"`
	Legs      []SweepLeg `json:"legs"`
	CreatedAt time.Time  `json:"created_at"`
}

// CreateLegsReq 创建 Sweep leg 目录请求。

type CreateLegsReq struct {
	CaseID    string `json:"case_id"`
	OutputKey string `json:"output_key"`
	Chain string `json:"chain"`
	Asset  string `json:"asset"`
	Amount string `json:"amount"`
	Actor string `json:"actor"`
	// Kinds 指定要创建的 leg 类型列表（顺序即 Seq 顺序）。
	// 通常包含 occupancy_release + refund_broadcast。
	Kinds []LegKind `json:"kinds"`
	// TxHash 退款/归集 tx_hash（如已知）。
	TxHash string `json:"tx_hash,omitempty"`
}

// SweepCatalog 管理所有 SweepLegGroup 的内存目录。

type SweepCatalog struct {
	mu     sync.Mutex
	groups map[string]*SweepLegGroup // key = caseID
}

// NewSweepCatalog 构造新的 Sweep leg 目录管理器。

   func NewSweepCatalog() *SweepCatalog {
	   return &SweepCatalog{
		   groups: make(map[string]*SweepLegGroup),
	   }
   }

// CreateLegs 为指定案件创建 Sweep leg 目录。
// 若案件已有 leg 目录，返回 ErrSweepExists（幂等保障）。
// c 必须为已终态的 ReclaimCase；否则返回 ErrSweepNotTerminal。

   func (cat *SweepCatalog) CreateLegs(c ReclaimCase, req CreateLegsReq) (*SweepLegGroup, error) {
	   if cat == nil {
		   return nil, ErrUnavailable
	   }
	  if !c.State.Terminal() {
		   return nil, ErrSweepNotTerminal
	   }
	   // 仅 StateReclaimed 生成 legs；其他终态不应调用此方法，但防御性检查
	  if c.State != StateReclaimed {
		  return nil, ErrSweepNotTerminal
	   }
	   cat.mu.Lock()
	   defer cat.mu.Unlock()
	   if _, exists := cat.groups[req.CaseID]; exists {
		   return nil, ErrSweepExists
	  }
	  now := time.Now().UTC()
	  group := &SweepLegGroup{
		   CaseID:    req.CaseID,
		  OutputKey: req.OutputKey,
		  Chain:     req.Chain,
		  Asset: req.Asset,
		  Amount:    req.Amount,
		  CreatedAt: now,
	  }
	   for i, kind := range req.Kinds {
		   leg := SweepLeg{
			   ID:     newID(),
			  CaseID: req.CaseID,
			   Seq:   i + 1,
			   Kind:  kind,
			  Actor: req.Actor,
			   At: now,
		   }
		  if kind == LegKindRefundBroadcast || kind == LegKindSweepToHot {
			   leg.TxHash = req.TxHash
		  }
		   if kind == LegKindOccupancyRelease {
			   leg.Note = "occupancy released for output " + req.OutputKey
		  }
		   group.Legs = append(group.Legs, leg)
	   }
	   cat.groups[req.CaseID] = group
	   return group, nil
   }

// Get 返回案件的 Sweep leg 目录；不存在时返回 nil。

  func (cat *SweepCatalog) Get(caseID string) *SweepLegGroup {
	  if cat == nil {
		   return nil
	  }
	  cat.mu.Lock()
	  defer cat.mu.Unlock()
	  g := cat.groups[caseID]
	   if g == nil {
		  return nil
	   }
	  cp := *g
	  legs := make([]SweepLeg, len(g.Legs))
	  copy(legs, g.Legs)
	  cp.Legs = legs
	   return &cp
  }

// Len 返回已创建的 leg 目录数量。

   func (cat *SweepCatalog) Len() int {
	   if cat == nil {
		   return 0
	  }
	  cat.mu.Lock()
	  defer cat.mu.Unlock()
	   return len(cat.groups)
  }
