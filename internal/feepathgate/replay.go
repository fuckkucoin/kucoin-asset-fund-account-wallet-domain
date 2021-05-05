package feepathgate

import (
	"context"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

// idemRecord 记录某个 idem_key 首次见到的三元组。
type idemRecord struct {
	kind  Kind
	asset string
	gross string
}

// ReplayFence 维护 idem_key → (Kind, Asset, Gross) 的索引；
// 后续调用若三元组不全等则视为冲突。
type ReplayFence struct {
	mu sync.Mutex
	known map[string]idemRecord // idem_key → 首次注册记录
	blocks map[string]*ReplayBlock
	store  Store
	clock func() time.Time
}

  func newReplayFence(store Store, clock func() time.Time) *ReplayFence {
	   return &ReplayFence{
		   known: make(map[string]idemRecord),
		   blocks: make(map[string]*ReplayBlock),
		   store:  store,
		   clock: clock,
	   }
   }

   // CheckAndRegister 对 idem_key 进行幂等冲突检测并注册。
  // 逻辑：
  // 1. 若 idem_key 从未见过：注册 (kind, asset, gross) 并返回 (false, nil)。
   // 2. 若 idem_key 已有记录且三元组全等：返回 (true, nil)（幂等重放，允许）。
   // 3. 若三元组不全等（任一字段不同）：写入 ReplayBlock，返回 (false, ErrReplayConflict)。
  func (f *ReplayFence) CheckAndRegister(
	   ctx context.Context,
	   idemKey string,
	  bizKey string,
	  kind Kind,
	  asset string,
	   gross string,
   ) (duplicate bool, err error) {
	   idemKey = strings.TrimSpace(idemKey)
	  if idemKey == "" {
		  // 无幂等键：跳过检查，由上层负责。
		  return false, nil
	   }
	   f.mu.Lock()
	   defer f.mu.Unlock()
	  rec, exists := f.known[idemKey]
	   if !exists {
		   // 首次注册。
		  f.known[idemKey] = idemRecord{kind: kind, asset: asset, gross: gross}
		   return false, nil
	  }
	   // 全等检查。
	  if rec.kind == kind && strings.EqualFold(rec.asset, asset) && rec.gross == gross {
		   return true, nil
	  }
	  // 三元组不等：封堵。
	  block := &ReplayBlock{
		   ID: uuid.NewString(),
		  IdemKey: idemKey,
		   BusinessKey: bizKey,
		  OrigKind:  rec.kind,
		  OrigAsset: rec.asset,
		   OrigGross: rec.gross,
		  ReqKind:  kind,
		   ReqAsset: asset,
		  ReqGross:  gross,
		   BlockedAt: f.clock(),
	  }
	  f.blocks[idemKey] = block
	  if f.store != nil {
		   _ = f.store.InsertReplayBlock(ctx, block) // 审计写入失败不阻断错误返回
	   }
	  return false, ErrReplayConflict
   }

// GetBlock 查询指定 idem_key 的封堵记录（用于审计 API）。
   func (f *ReplayFence) GetBlock(ctx context.Context, idemKey string) (*ReplayBlock, error) {
	   f.mu.Lock()
	  defer f.mu.Unlock()
	   if b := f.blocks[idemKey]; b != nil {
		   cp := *b
		  return &cp, nil
	  }
	   if f.store == nil {
		  return nil, ErrLeaseNotFound
	  }
	  return f.store.GetReplayBlock(ctx, idemKey)
  }

// Stats 返回当前内存中的计数器（供指标导出）。

   func (f *ReplayFence) Stats() (knownCount, blockCount int) {
	  f.mu.Lock()
	  defer f.mu.Unlock()
	   return len(f.known), len(f.blocks)
  }

// IdemRegistration 记录 idem_key 的已注册信息，供外部查询。
type IdemRegistration struct {
	IdemKey string `json:"idem_key"`
	Kind Kind `json:"kind"`
	Asset string `json:"asset"`
	Gross string `json:"gross"`
	RegisteredAt time.Time `json:"registered_at"`
}

// GetRegistration 返回指定 idem_key 的注册信息（内存视图）。
   func (f *ReplayFence) GetRegistration(idemKey string) (IdemRegistration, bool) {
	  f.mu.Lock()
	  defer f.mu.Unlock()
	   rec, ok := f.known[idemKey]
	   if !ok {
		   return IdemRegistration{}, false
	   }
	  return IdemRegistration{
		   IdemKey: idemKey,
		   Kind:  rec.kind,
		  Asset: rec.asset,
		   Gross: rec.gross,
	   }, true
   }
