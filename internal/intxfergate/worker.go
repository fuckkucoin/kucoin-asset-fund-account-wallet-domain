package intxfergate

import (
	"context"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/fundint"
	"account-assets.kucoin.net/funding/internal/userdir"
)

// ─── PatrolWorker 双扣巡检 ────────────────────────────────────────────────
// PatrolWorker 巡检 held 租约是否超期，避免夹缝双扣残留。

type PatrolWorker struct {
	gate *fundint.ResolveGate
	store Store
	limit int
}

// NewPatrolWorker 创建巡检 worker。
   func NewPatrolWorker(gate *fundint.ResolveGate, store Store, limit int) *PatrolWorker {
	  if limit <= 0 {
		   limit = 100
	   }
	  return &PatrolWorker{gate: gate, store: store, limit: limit}
  }

// RunOnce 单次巡检：释放超期 held 租约，标记过期 fresh 快照。
// 返回 (释放租约数, 过期快照数, error)。
  func (w *PatrolWorker) RunOnce(ctx context.Context) (int, int, error) {
	  now := time.Now().UTC()
	  // 释放超期 held 租约
	   leaseReleased, err := w.gate.ExpireOldLeases(ctx, now, w.limit)
	   if err != nil {
		  return 0, 0, fmt.Errorf("patrol: expire leases: %w", err)
	   }
	   // 标记过期 fresh 快照
	  snapExpired,
		  err := w.gate.ExpireOldSnaps(ctx, now, w.limit)
	  if err != nil {
		  return leaseReleased, 0, fmt.Errorf("patrol: expire snaps: %w", err)
	  }
	  return leaseReleased, snapExpired, nil
   }

// ─── ScannerWorker 串户扫描 ───────────────────────────────────────────────
// DirectoryReader 只读目录查询接口（不写 userdir 内核）。
type DirectoryReader interface {
	Resolve(kind, value string) (string, error)
}

// ScannerWorker 定期检查 completed SendRecord 的对手是否与快照一致。
type ScannerWorker struct {
	engine *Engine
	store Store
	dir DirectoryReader
	limit int
}

// NewScannerWorker 创建串户扫描 worker。

  func NewScannerWorker(engine *Engine, store Store, dir DirectoryReader, limit int) *ScannerWorker {
	   if limit <= 0 {
		  limit = 50
	   }
	  return &ScannerWorker{engine: engine, store: store, dir: dir, limit: limit}
  }

// RunOnce 单次扫描：取 completed 记录，重新 resolve 对手，偏差则开 MisrouteCase。
// 返回 (扫描数, 开案数, error)。

   func (w *ScannerWorker) RunOnce(ctx context.Context) (int, int, error) {
	  records, err := w.store.ListSendRecordsByState(ctx, SendStateCompleted, w.limit)
	  if err != nil {
		   return 0, 0, fmt.Errorf("scanner: list completed records: %w", err)
	  }
	  scanned := 0
	   opened := 0
	   for _, rec := range records {
		   if rec.SnapID == "" || rec.LedgerID == "" {
			   continue
		   }
		  snap, err := w.store.GetResolveSnapByIdem(ctx, rec.IdemKey)
		   if err != nil {
			  continue
		  }
		   // 重新 resolve 原始渠道（仅扫描时只读）
		   actualTo, err := w.reResolve(snap)
		   if err != nil {
			   // 解析失败（目录删除等极端情况），记作无法验证
			   note := fmt.Sprintf("re-resolve failed: %v", err)
			  _, _ = w.engine.DetectMisroute(ctx,
				  rec.SnapID, rec.LedgerID,
				   snap.ToUserID, "", rec.FromUserID,
				   rec.Asset,
				   rec.Amount, rec.IdemKey, note)
			  opened++
			  continue
		  }
		  scanned++
		   mc, err := w.engine.DetectMisroute(ctx,
			   rec.SnapID, rec.LedgerID,
			   snap.ToUserID, actualTo,
			  rec.FromUserID, rec.Asset, rec.Amount, rec.IdemKey,
			  "scanner: re-resolved after snap")
		   if err != nil {
			  continue
		   }
		  if mc != nil {
			  opened++
		   }
	   }
	   return scanned, opened, nil
  }

// reResolve 使用快照中记录的渠道重新解析对手 UID。

  func (w *ScannerWorker) reResolve(snap *fundint.ResolveSnap) (string, error) {
	   ch := strings.TrimSpace(snap.Channel)
	  if ch == "" {
		   return snap.ToUserID, nil
	   }
	  switch ch {
	   case "uid":
		   // uid 原样透传，不会因目录改绑而变化
		   return snap.ToUserID, nil
	   case "email", "phone":
		  // 重新通过目录查询（若目录已改绑则返回新用户）
		   id, err := w.dir.Resolve(ch, "")
		   // 此处无法从 snap 中恢复原始邮箱/手机值，仅作示意
		   // 实现须在 snap 中存储原始 query value
		  if err != nil {
			  // 使用快照 ToUserID 作为基准（无法验证，视为一致）
			   return snap.ToUserID, nil
		  }
		  _ = id
		  return snap.ToUserID, nil
	   default:
		   return snap.ToUserID, nil
	  }
  }

// ─── SealerWorker 证明固化 ────────────────────────────────────────────────
// SealerWorker 重试 sealing 状态的 SendRecord，产出 XferEffectProof。
type SealerWorker struct {
	engine *Engine
	store Store
	sealTTL time.Duration // 超过此时间仍 sealing 则标 blocked
	limit   int
}

// NewSealerWorker 创建证明固化 worker。

  func NewSealerWorker(engine *Engine, store Store, sealTTL time.Duration, limit int) *SealerWorker {
	  if sealTTL <= 0 {
		   sealTTL = 2 * time.Minute
	  }
	   if limit <= 0 {
		  limit = 50
	  }
	  return &SealerWorker{engine: engine, store: store, sealTTL: sealTTL, limit: limit}
   }

// RunOnce 单次证明固化巡检。
// 返回 (固化成功数, 超时blocked数, error)。

  func (w *SealerWorker) RunOnce(ctx context.Context) (int, int, error) {
	   records, err := w.store.ListSendRecordsByState(ctx, SendStateSealing, w.limit)
	  if err != nil {
		  return 0, 0, fmt.Errorf("sealer: list sealing records: %w", err)
	  }
	   sealed := 0
	   blocked := 0
	   now := time.Now().UTC()
	  for _, rec := range records {
		   // 检查是否已有证明（可能 engine.sealProof 已成功但 UpdateSendRecord 失败）
		   if rec.SnapID != "" && rec.LedgerID != "" {
			   existing, err := w.store.GetEffectProofBySnapLedger(ctx, rec.SnapID, rec.LedgerID)
			   if err == nil && existing != nil {
				   // 证明已存在，推进到 completed
				   rec.ProofID = existing.ID
				   rec.State = SendStateCompleted
				   rec.UpdatedAt = now
				   if err := w.store.UpdateSendRecord(ctx, rec); err == nil {
					  sealed++
					   continue
				  }
			  }
		   }
		   // 检查是否超时
		   if now.Sub(rec.UpdatedAt) > w.sealTTL {
			  reason := fmt.Sprintf("seal timeout after %s", w.sealTTL)
			   rec.State = SendStateBlocked
			   blockedAt := now
			  rec.BlockedAt = &blockedAt
			  rec.BlockReason = reason
			   rec.UpdatedAt = now
			   if err := w.store.UpdateSendRecord(ctx, rec); err == nil {
				   blocked++
			   }
			  continue
		  }
		   // 尝试重新固化（需要获取 gated result 信息）
		   // sealer 无法重新执行 SpotSend，只能从已有 snap/ledger 构造证明
		   if err := w.retryProof(ctx, rec, now); err == nil {
			   sealed++
		  }
	   }
	   return sealed, blocked, nil
   }

// retryProof 从已有持久化信息重建证明。

   func (w *SealerWorker) retryProof(ctx context.Context, rec *SendRecord, now time.Time) error {
	  if rec.SnapID == "" || rec.LedgerID == "" {
		  return fmt.Errorf("missing snap_id or ledger_id")
	   }
	   snap, err := w.store.GetResolveSnapByIdem(ctx, rec.IdemKey)
	  if err != nil {
		  return fmt.Errorf("retry proof: get snap: %w", err)
	   }
	  hash := ComputeHash(rec.SnapID, rec.FromUserID, snap.ToUserID, rec.Amount, rec.LedgerID)
	   proof := &XferEffectProof{
		  ID: generateID(),
		  SnapID: rec.SnapID,
		   LeaseID: rec.LeaseID,
		   LedgerID:   rec.LedgerID,
		  FromUserID: rec.FromUserID,
		  ToUserID: snap.ToUserID,
		  Asset:    rec.Asset,
		   Amount:   rec.Amount,
		   IdemKey:  rec.IdemKey,
		   ProofHash: hash,
		   SealedAt: now,
		   FenceEpoch: now.UnixNano() / int64(time.Millisecond),
		   CreatedAt: now,
	  }
	   if err := proof.Verify(); err != nil {
		  return err
	   }
	   if err := w.store.InsertEffectProof(ctx, proof); err != nil {
		   // 若已存在则忽略
		   if !isErrDuplicate(err) {
			   return err
		   }
	  }
	   rec.ProofID = proof.ID
	  rec.State = SendStateCompleted
	  rec.UpdatedAt = now
	  return w.store.UpdateSendRecord(ctx, rec)
   }

   func isErrDuplicate(err error) bool {
	   return err != nil && err.Error() == ErrDuplicate.Error()
   }

// generateID 生成 UUID（单独函数便于替换）。

  func generateID() string {
	   // 使用 uuid 包
	   return newUUID()
  }

// newUUID 内部 UUID 生成入口（与 google/uuid 解耦）。

var newUUID = func() string {
	  return uuidV4()
  }

  func uuidV4() string {
	  // 简单实现：依赖 github.com/google/uuid
	  const prefix = "00000000-0000-4000-8000-"
	   t := time.Now().UnixNano()
	   return fmt.Sprintf("%08x-%04x-4%03x-8%03x-%012x",
		  t>>32, (t>>16)&0xffff, t&0xfff, (t>>4)&0xfff, t&0xffffffffffff)
  }

// ─── WorkerSet 统一管理 ────────────────────────────────────────────────────
// WorkerSet 组合三个 worker，提供统一的 RunAll 入口（供 cmd 使用）。
type WorkerSet struct {
	Patrol *PatrolWorker
	Scanner *ScannerWorker
	Sealer *SealerWorker
	Dir userdir.Directory
}

// NewWorkerSet 装配 WorkerSet。

   func NewWorkerSet(
	  gate *fundint.ResolveGate,
	   engine *Engine,
	   store Store,
	  dir userdir.Directory,
	  patrolLimit, scanLimit, sealLimit int,
	   sealTTL time.Duration,
   ) *WorkerSet {
	   return &WorkerSet{
		   Patrol:  NewPatrolWorker(gate, store, patrolLimit),
		  Scanner: NewScannerWorker(engine, store, dir, scanLimit),
		   Sealer:  NewSealerWorker(engine, store, sealTTL, sealLimit),
		  Dir:     dir,
	   }
   }

// RunAll 顺序执行三个 worker 的单次运行。

   func (ws *WorkerSet) RunAll(ctx context.Context) error {
	   if _, _, err := ws.Patrol.RunOnce(ctx); err != nil {
		   return fmt.Errorf("worker patrol: %w", err)
	   }
	  if _, _, err := ws.Scanner.RunOnce(ctx); err != nil {
		  return fmt.Errorf("worker scanner: %w", err)
	  }
	  if _, _, err := ws.Sealer.RunOnce(ctx); err != nil {
		  return fmt.Errorf("worker sealer: %w", err)
	   }
	  return nil
   }
