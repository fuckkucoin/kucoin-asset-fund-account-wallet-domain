package brokerfence

import (
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// BookReader 提供主/子账本余额读取接口，由 brokerbook.Service 适配。
type BookReader interface {
	// TotalOf 读取 masterID 下所有子账+主账的可用合计（字符串十进制）。
	TotalOf(masterID, asset string) (string, error)
}

// ConserveStore 守恒证明持久化接口，由 persist.go 实现。

type ConserveStore interface {
	SaveProof(p *ConserveProof) error
	GetProof(id string) (*ConserveProof, error)
	LatestProof(leaseID string) (*ConserveProof, error)
	ListProofs(leaseID string) ([]*ConserveProof, error)
}

// ConserveEngine 主子守恒证明引擎。
type ConserveEngine struct {
	mu     sync.Mutex
	store  ConserveStore
	reader BookReader
	clock func() time.Time
	// driftedLeases 记录当前漂移中的 leaseID，挡写期间写路径快速查询。
	driftedLeases map[string]bool
}

// NewConserveEngine 构造守恒证明引擎。
   func NewConserveEngine(store ConserveStore, reader BookReader) *ConserveEngine {
	   return &ConserveEngine{
		  store: store,
		   reader:        reader,
		   clock:         func() time.Time { return time.Now().UTC() },
		   driftedLeases: make(map[string]bool),
	  }
  }

   func (ce *ConserveEngine) now() time.Time {
	   if ce.clock != nil {
		   return ce.clock()
	   }
	  return time.Now().UTC()
  }

// SampleBefore 围栏激活时采样主子合计（BeforeTotal）。
// 返回证明快照（IsDrifted=false，AfterTotal=""）；调用方须将 ID 存入 FenceLease.ProofID。

   func (ce *ConserveEngine) SampleBefore(leaseID, subID, masterID, asset string) (*ConserveProof, error) {
	   leaseID = strings.TrimSpace(leaseID)
	   subID = strings.TrimSpace(subID)
	   masterID = strings.TrimSpace(masterID)
	  asset = strings.TrimSpace(asset)
	  if leaseID == "" || masterID == "" || asset == "" {
		  return nil, ErrInput
	  }
	   total, err := ce.reader.TotalOf(masterID, asset)
	  if err != nil {
		  return nil, err
	  }
	  now := ce.now()
	   p := &ConserveProof{
		   ID:      uuid.NewString(),
		  LeaseID: leaseID,
		  SubID:   subID,
		   MasterID: masterID,
		  Asset: asset,
		   BeforeTotal: total,
		   AfterTotal:  "",
		   Delta:       "0",
		   IsDrifted:   false,
		  SampledAt: now,
		   CreatedAt: now,
	  }
	  ce.mu.Lock()
	  err = ce.store.SaveProof(p)
	  ce.mu.Unlock()
	   return p, err
  }

// SampleAfter 解围前采样主子合计（AfterTotal），计算 Delta，标记漂移。
// 若漂移，将 leaseID 加入挡写集合，写路径调用 CheckDrift 时失败。
   func (ce *ConserveEngine) SampleAfter(leaseID string) (*ConserveProof, error) {
	   leaseID = strings.TrimSpace(leaseID)
	   if leaseID == "" {
		  return nil, ErrInput
	   }
	   ce.mu.Lock()
	  defer ce.mu.Unlock()
	   p,
		   err := ce.store.LatestProof(leaseID)
	   if err != nil {
		   return nil, err
	   }
	  total, err := ce.reader.TotalOf(p.MasterID, p.Asset)
	   if err != nil {
		   return nil, err
	  }
	   before, err := decimal.NewFromString(p.BeforeTotal)
	   if err != nil {
		   return nil, ErrInput
	  }
	   after, err := decimal.NewFromString(total)
	  if err != nil {
		  return nil, ErrInput
	  }
	   delta := after.Sub(before)
	  p.AfterTotal = total
	   p.Delta = delta.String()
	   p.IsDrifted = !delta.IsZero()
	  p.SampledAt = ce.now()
	  if err := ce.store.SaveProof(p); err != nil {
		  return nil, err
	   }
	   if p.IsDrifted {
		   ce.driftedLeases[leaseID] = true
	   }
	  return p, nil
  }

// CheckDrift 写路径调用：若 leaseID 当前处于漂移挡写状态，返回 ErrConserveDrift。

   func (ce *ConserveEngine) CheckDrift(leaseID string) error {
	   ce.mu.Lock()
	  drifted := ce.driftedLeases[leaseID]
	   ce.mu.Unlock()
	  if drifted {
		   return ErrConserveDrift
	   }
	  return nil
  }

// ClearDrift 收口后清除漂移标记（BypassCase 全部关闭后调用）。

   func (ce *ConserveEngine) ClearDrift(leaseID string) {
	  ce.mu.Lock()
	   delete(ce.driftedLeases, strings.TrimSpace(leaseID))
	  ce.mu.Unlock()
  }

// GetProof 按 ID 查询守恒证明（供 API 导出）。
   func (ce *ConserveEngine) GetProof(id string) (*ConserveProof,
	  error) {
	   if strings.TrimSpace(id) == "" {
		  return nil, ErrInput
	   }
	   return ce.store.GetProof(id)
   }

// LatestProof 查询指定 lease 的最新守恒证明。
   func (ce *ConserveEngine) LatestProof(leaseID string) (*ConserveProof, error) {
	  if strings.TrimSpace(leaseID) == "" {
		  return nil, ErrInput
	  }
	   return ce.store.LatestProof(strings.TrimSpace(leaseID))
  }

// IsDriftedLease 判断指定 leaseID 是否处于漂移挡写状态（供 worker 查询）。

  func (ce *ConserveEngine) IsDriftedLease(leaseID string) bool {
	  ce.mu.Lock()
	   d := ce.driftedLeases[strings.TrimSpace(leaseID)]
	  ce.mu.Unlock()
	  return d
  }
