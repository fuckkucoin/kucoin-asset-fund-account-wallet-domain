// 功能：回收效果世代闸门 — 对外统一入口。
// - Gate 封装 Engine + Detector + MutexStore，提供幂等回收路径。
// - GuardedRecover 是 incdef.RecoverStuck 的替代包装；完成互斥检查→租约获取→动作→证明固化。
// - 不改写 withdraw.RecoverStuck 主路径；通过 RecoverFn 回调调用。
// 不变量：
// - GuardedRecover 返回 ErrDupBlocked 时外层绝对不得发起第二次资金动作。
// - EffectProof 写入失败时 StuckCase 不标记 Recovered。

// 作者：Bronson
// 日期：2021.11.22

package stucksettle

import (
	"fmt"
	"strings"
	"time"
)

// RecoverFn 实际回收执行回调（由 withdraw.RecoverStuck 适配器注入）。
// 返回 (action, ledgerRef, error)。
type RecoverFn func(transferID string, now time.Time) (action string, ledgerRef string, err error)

// Gate 世代闸门；统一对外入口。
type Gate struct {
	engine *Engine
	detector *Detector
	mutex *MutexStore
	recover RecoverFn
	nodeID  string // 当前节点标识（用于 AdvanceLock holderNode）
}

// GateConfig 闸门配置。
type GateConfig struct {
	Store  Store
	NodeID string
	Recover RecoverFn
}

// NewGate 构造闸门。
  func NewGate(cfg GateConfig) *Gate {
	  mutex := NewMutexStore()
	  store := cfg.Store
	   if store == nil {
		   store = NewMemStore()
	   }
	  nodeID := strings.TrimSpace(cfg.NodeID)
	   if nodeID == "" {
		  nodeID = "node-default"
	   }
	  return &Gate{
		   engine: NewEngine(store,
			  mutex),
		  detector: NewDetector(store),
		   mutex:   mutex,
		   recover: cfg.Recover,
		  nodeID:  nodeID,
	  }
   }

// WithClock 注入时钟（用）。
  func (g *Gate) WithClock(fn func() time.Time) *Gate {
	  if fn != nil {
		  g.engine.WithClock(fn)
		   g.detector.WithClock(fn)
	  }
	   return g
  }

  // GuardedRecover 带世代闸保护的回收执行。
   // 返回：
  // - (*EffectProof, nil)：回收成功，证明已固化。
  // - (nil, ErrDupBlocked)：重复请求被封堵，资金动作未执行。
   // - (nil, ErrAdvanceLocked)：AdvanceStuck 持锁中，请重试。
  // - (nil, ErrHoldBypass)：Hold 态禁止旁路 Recover。
   // - (nil, ErrProofMissing/ErrProofInvalid)：证明固化失败，状态未转 Recovered。
   func (g *Gate) GuardedRecover(transferID, holderKey string, isHoldState bool) (*EffectProof, error) {
	  if g == nil || g.engine == nil {
		   return nil, ErrUnavailable
	  }
	  trID := strings.TrimSpace(transferID)
	  hk := strings.TrimSpace(holderKey)
	  if trID == "" || hk == "" {
		   return nil, ErrInvalidInput
	  }
	   // 申请租约（含互斥检查）
	  req := AcquireLeaseReq{
		   TransferID: trID,
		  HolderKey:  hk,
	   }
	   lease, err := g.engine.AcquireLease(req, isHoldState)
	  if err != nil {
		  return nil, err
	  }
	   if lease.State == LeaseDupBlocked {
		  return nil, ErrDupBlocked
	  }
	  // 执行实际回收动作
	   if g.recover == nil {
		   // 无回调时仅完成租约（模式）
		  proof, cErr := g.engine.CommitRecover(CommitRecoverReq{
			  LeaseID: lease.LeaseID,
			   Action:  "noop",
			  LedgerRef: "",
			  IssuedBy:  g.nodeID,
		   })
		  return proof, cErr
	  }
	  now := g.engine.now()
	   action,
		  ledgerRef, recErr := g.recover(trID, now)
	  if recErr != nil {
		  // 回收失败，不固化证明；租约超时后自动 Expired
		   return nil, fmt.Errorf("stucksettle: recover action failed: %w", recErr)
	  }
	   // 固化证明
	  proof, err := g.engine.CommitRecover(CommitRecoverReq{
		  LeaseID:   lease.LeaseID,
		   Action:    action,
		  LedgerRef: ledgerRef,
		  IssuedBy: g.nodeID,
	   })
	   return proof, err
   }

// HoldAdvance AdvanceStuck 开始时持锁，防止旁路 Recover。
  func (g *Gate) HoldAdvance(transferID string) error {
	  if g == nil || g.mutex == nil {
		   return ErrUnavailable
	   }
	  now := g.engine.now()
	   _,
		   err := g.mutex.TryHold(transferID, g.nodeID, now)
	  return err
   }

// ReleaseAdvance AdvanceStuck 完成后释放锁。

  func (g *Gate) ReleaseAdvance(transferID string) error {
	  if g == nil || g.mutex == nil {
		  return ErrUnavailable
	   }
	   now := g.engine.now()
	  return g.mutex.Release(transferID,
		   g.nodeID, now)
  }

// ScanDups 对指定 transfer 执行双结扫描。
  func (g *Gate) ScanDups(transferID string) (DetectorResult, error) {
	   if g == nil || g.detector == nil {
		   return DetectorResult{}, ErrUnavailable
	   }
	   return g.detector.ScanTransfer(transferID)
   }

// ExportProof 导出效果证明（审计）。
   func (g *Gate) ExportProof(req ProofExportReq) (*ProofExport, error) {
	   if g == nil || g.engine == nil {
		   return nil, ErrUnavailable
	  }
	  return g.engine.ExportProof(req)
  }

// ConfirmDupCase 双人确认关闭双结案件。
   func (g *Gate) ConfirmDupCase(req ConfirmDupCaseReq) (*DupRecoverCase, error) {
	   if g == nil || g.engine == nil {
		  return nil, ErrUnavailable
	  }
	   return g.engine.ConfirmDupCase(req)
  }

// DupCasesByTransfer 查询 transfer 的全部双结案件。

   func (g *Gate) DupCasesByTransfer(transferID string) ([]DupRecoverCase, error) {
	  if g == nil || g.engine == nil {
		   return nil, ErrUnavailable
	   }
	   return g.engine.DupCasesByTransfer(transferID)
   }

// GenStatus 查询世代状态。
  func (g *Gate) GenStatus(transferID string) (*StuckGeneration, error) {
	   if g == nil || g.engine == nil {
		   return nil, ErrUnavailable
	  }
	  return g.engine.GetOrCreateGen(transferID)
  }

// MutexSnapshot 返回互斥锁快照（运维）。

  func (g *Gate) MutexSnapshot() []AdvanceLock {
	  if g == nil || g.mutex == nil {
		   return nil
	   }
	   return g.mutex.Snapshot()
   }

// GetEffectProof 代理 Engine.GetEffectProof（供 incdef 层在标记 Recovered 前校验证明）。
  func (g *Gate) GetEffectProof(transferID string) (*EffectProof, error) {
	  if g == nil || g.engine == nil {
		  return nil, ErrUnavailable
	  }
	   return g.engine.GetEffectProof(transferID)
   }

// Engine 返回内部引擎（供和 incdef 层使用）。
  func (g *Gate) Engine() *Engine {
	  if g == nil {
		   return nil
	  }
	  return g.engine
   }
