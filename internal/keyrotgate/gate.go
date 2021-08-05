// @FileName: gate.go
// @Summary: keyrotgate 主入口 Gate——聚合 Engine / Binder / Detector，提供业务层 API。
// - Gate 是唯一对外暴露的业务接口；HTTP handler、Worker、cmd 均通过 Gate 调用。
// - Gate 不直接依赖 keycer.Service；通过 SlotReader 只读接口获取槽状态。
// - rotate_gen 计数器在 Gate 内维护（原子递增）；各 Engine 共享同一 Store。
// 不变量：
// - ExecuteRotate 必须先 BeginExecute（互斥保护）再执行，后 FinishExecute 落证明。
// - HasOpenDrift 为 true 时 ExecuteRotate 必须失败。
// - ProofRequired(slotID) 为 true 时 Unlock/Rotate 必须先提供有效证明。
// @Author: Likeo
// @Created: 2021.08.04

package keyrotgate

import (
	"context"
	"sync/atomic"
)

// SlotReader 调用方注入的只读槽状态接口（不依赖 keycer 包类型）。

type SlotReader interface {
	// ReadSlot 读取指定槽状态；slotRetired 表示槽是否已退役。
	ReadSlot(ctx context.Context, slotID string) (fingerprint string, slotRetired bool, err error)
}

// Gate keyrotgate 主业务门面。
type Gate struct {
	store  Store
	engine *Engine
	binder *Binder
	detector *Detector
	reader   SlotReader
	// rotate_gen 全局单调递增计数器（进程内原子；持久化由 SaveLease 携带）
	rotateGen atomic.Int64
}

// NewGate 构造 Gate。reader 可为 nil（环境）。
   func NewGate(store Store, reader SlotReader) *Gate {
	  if store == nil {
		   store = NewMemStore()
	  }
	   return &Gate{
		  store: store,
		   engine: NewEngine(store),
		   binder:   NewBinder(store),
		   detector: NewDetector(store),
		   reader:   reader,
	   }
   }

// nextRotateGen 原子获取下一个 rotate_gen（单调递增）。
  func (g *Gate) nextRotateGen() int64 {
	  return g.rotateGen.Add(1)
  }

// ── RotateLease API ──────────────────────────────────────────────────────────
// OpenLease OpenRotate 成功后签发轮换世代租约。
// 调用方应在 keycer.OpenRotate 成功后立即调用。

   func (g *Gate) OpenLease(ctx context.Context, req OpenLeaseReq) (*RotateLease, error) {
	   if g == nil {
		  return nil, ErrUnavailable
	  }
	   gen := g.nextRotateGen()
	  return g.engine.OpenLease(ctx, req, gen)
   }

// ApproveLease 第二人批准轮换租约。

  func (g *Gate) ApproveLease(ctx context.Context, req ApproveLeaseReq) (*RotateLease, error) {
	  if g == nil {
		   return nil, ErrUnavailable
	   }
	   return g.engine.ApproveLease(ctx, req)
  }

// ExecuteRotate 执行轮换：互斥检查 + 指纹校验 + 进入 Executing + 执行槽操作 + 落证明。
// slotOpFn：调用方注入的实际槽操作函数（执行 keycer.ExecuteRotate 等）；
// 传入源槽当前指纹；返回目标槽新指纹与可能的错误。
// 若存在未结 SignBind，租约进入 Blocked 并返回 ErrInflightSignExists。
// 若存在未关闭漂移案件，返回 ErrOpenDriftExists。

   func (g *Gate) ExecuteRotate(ctx context.Context, req ExecuteLeaseReq, slotOpFn func(srcFP string) (afterFP string, err error)) (*FingerprintProof, error) {
	  if g == nil {
		  return nil, ErrUnavailable
	  }
	   // 获取租约以得到源槽 ID
	  lease, err := g.store.GetLease(ctx, req.RotateLeaseID)
	  if err != nil {
		  return nil, err
	  }
	  // 检查源槽是否有未关闭的漂移案件
	   hasOpenDrift, err := g.detector.HasOpenDrift(ctx, lease.SrcSlotID)
	  if err != nil {
		   return nil, err
	  }
	   if hasOpenDrift {
		   return nil, ErrOpenDriftExists
	   }
	  // 读取源槽当前指纹
	  var currentFP string
	  if g.reader != nil {
		   fp, _, rdErr := g.reader.ReadSlot(ctx, lease.SrcSlotID)
		   if rdErr != nil {
			  return nil, rdErr
		  }
		   currentFP = fp
	   } else {
		  currentFP = lease.SrcFingerprint
	   }
	   // 查询在途签名绑定
	   activeBinds, err := g.store.ListActiveSignBinds(ctx, lease.SrcSlotID)
	   if err != nil {
		  return nil, err
	  }
	  // BeginExecute：校验指纹一致性、在途互斥、四眼
	  if _, err := g.engine.BeginExecute(ctx, req, currentFP, activeBinds); err != nil {
		  if err == ErrInflightSignExists {
			  // 进入 Blocked 队列
			  _, _ = g.engine.BlockLease(ctx, req.RotateLeaseID, "inflight sign binds exist")
			  MetricExecuteBlockedInflight.Inc()
		  }
		   return nil, err
	   }
	  // 执行实际槽操作（由调用方注入）
	  afterFP, opErr := slotOpFn(currentFP)
	  if opErr != nil {
		  // 执行失败：阻塞租约
		   _, _ = g.engine.BlockLease(ctx, req.RotateLeaseID, "slot op failed: "+opErr.Error())
		   MetricExecuteBlockedSlotOp.Inc()
		  return nil, opErr
	   }
	   // 落指纹证明
	   proof, err := g.engine.FinishExecute(ctx, req.RotateLeaseID, afterFP, req.ExecutedBy)
	  if err != nil {
		  return nil, err
	  }
	  return proof, nil
  }

  // GetLease 查询轮换租约。

   func (g *Gate) GetLease(ctx context.Context, id string) (*RotateLease, error) {
	  if g == nil {
		  return nil, ErrUnavailable
	  }
	  return g.store.GetLease(ctx,
		   id)
  }

// ── SignBind API ──────────────────────────────────────────────────────────────
// BindSign 发起签名/解锁时绑定源槽指纹。

  func (g *Gate) BindSign(ctx context.Context, req BindSignReq) (*SignBind, error) {
	   if g == nil {
		   return nil, ErrUnavailable
	  }
	   var slotRetired bool
	   if g.reader != nil {
		   _,
			   retired, err := g.reader.ReadSlot(ctx, req.SrcSlotID)
		   if err != nil {
			  return nil, err
		  }
		   slotRetired = retired
	  }
	   return g.binder.BindSign(ctx,
		  req, slotRetired)
  }

// ReleaseSign 签名/解锁完成后主动释放绑定。
   func (g *Gate) ReleaseSign(ctx context.Context, req ReleaseSignReq) (*SignBind, error) {
	   if g == nil {
		   return nil, ErrUnavailable
	   }
	   return g.binder.ReleaseSign(ctx, req)
  }

// GetSignBind 查询签名绑定。
   func (g *Gate) GetSignBind(ctx context.Context, id string) (*SignBind, error) {
	  if g == nil {
		   return nil, ErrUnavailable
	   }
	   return g.store.GetSignBind(ctx, id)
   }

// ── Drift Case API ────────────────────────────────────────────────────────────
// ProveCase 提交漂移案件证明（双人确认）。

  func (g *Gate) ProveCase(ctx context.Context, req ProveRotateReq) (*KeyRotDriftCase, error) {
	   if g == nil {
		   return nil, ErrUnavailable
	  }
	  return g.detector.ProveCase(ctx, req)
   }

// CloseCase 关闭已证明的漂移案件。

   func (g *Gate) CloseCase(ctx context.Context, req CloseDriftCaseReq) (*KeyRotDriftCase, error) {
	   if g == nil {
		   return nil, ErrUnavailable
	   }
	  return g.detector.CloseCase(ctx, req)
   }

// GetDriftCase 查询漂移案件。

   func (g *Gate) GetDriftCase(ctx context.Context, id string) (*KeyRotDriftCase, error) {
	   if g == nil {
		  return nil, ErrUnavailable
	  }
	  return g.store.GetDriftCase(ctx, id)
   }

// ListOpenDriftCases 查询指定源槽的未关闭漂移案件。
  func (g *Gate) ListOpenDriftCases(ctx context.Context, srcSlotID string) ([]*KeyRotDriftCase, error) {
	  if g == nil {
		   return nil, ErrUnavailable
	   }
	   return g.store.ListOpenDriftCases(ctx, srcSlotID)
   }

// ── FingerprintProof API ──────────────────────────────────────────────────────
// GetProof 查询指纹证明。
  func (g *Gate) GetProof(ctx context.Context, id string) (*FingerprintProof, error) {
	  if g == nil {
		   return nil, ErrUnavailable
	   }
	   return g.store.GetProof(ctx, id)
   }

// GetProofByLease 按租约 ID 查询指纹证明。

  func (g *Gate) GetProofByLease(ctx context.Context, leaseID string) (*FingerprintProof, error) {
	   if g == nil {
		  return nil, ErrUnavailable
	  }
	  return g.store.GetProofByLease(ctx,
		  leaseID)
  }

// RequireProof 校验指定源槽是否已有有效指纹证明；证明缺失时挡后续操作。
// 由调用方（keycer Unlock/Rotate 路径）调用。
   func (g *Gate) RequireProof(ctx context.Context, srcSlotID string) error {
	  if g == nil {
		   return ErrUnavailable
	  }
	   // 检查是否有未关闭漂移案件
	  hasOpenDrift,
		   err := g.detector.HasOpenDrift(ctx, srcSlotID)
	  if err != nil {
		   return err
	  }
	   if hasOpenDrift {
		  return ErrOpenDriftExists
	  }
	   return nil
  }

// DetectDrift 外部触发漂移检测（如槽状态发生变化时调用）。

  func (g *Gate) DetectDrift(ctx context.Context, srcSlot SlotInfo, newFP string, bind *SignBind, leaseID string) (*KeyRotDriftCase, error) {
	  if g == nil {
		   return nil, ErrUnavailable
	   }
	  return g.detector.DetectDrift(ctx, srcSlot, newFP, bind, leaseID)
   }

// ── 内部访问器（供 Worker 使用）──────────────────────────────────────────────

   func (g *Gate) store_() Store    { return g.store }
   func (g *Gate) binder_() *Binder { return g.binder }

  func (g *Gate) detector_() *Detector { return g.detector }
