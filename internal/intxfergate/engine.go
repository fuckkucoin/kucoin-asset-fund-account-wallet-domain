// 功能：intxfergate.Engine：站内转账闸核心编排。
// 职责：
// - 串联 fundint.ServiceExt（含 ResolveGate）与 intxfergate.Store。
// - 管理 SendRecord FSM（resolving→debiting→sealing→completed/blocked）。
// - 产出 XferEffectProof；缺失时禁止标记 completed。
// - 驱动 MisrouteCase 检测：贷记用户与快照不符时开案。
// 不包含：跨 book 划转、链上出金广播、P2P 通道。
// 与 xfer 半截 run_id 键空间隔离：FenceEpoch 从不交叉使用 r6 D 的 run_id 前缀。

// 作者：Draven
// 日期：2021.07.27

package intxfergate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/fundint"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// Engine 站内转账闸编排核心。
type Engine struct {
	ext *fundint.ServiceExt // 含 ResolveGate 的站内转账服务
	store Store // 闸专属持久化
}

// NewEngine 装配 Engine。

  func NewEngine(ext *fundint.ServiceExt, store Store) *Engine {
	  return &Engine{ext: ext, store: store}
   }

// ─── SendRequest / SendReply ───────────────────────────────────────────────
// SendRequest 站内转账闸请求（含全量标识）。
type SendRequest struct {
	UserID         string
	ToUID          string
	ToEmail        string
	ToPhone        string
	Asset          string
	Amount         decimal.Decimal
	IdempotencyKey string
	Remark string
}

// SendReply 站内转账闸结果。
type SendReply struct {
	RecordID string // SendRecord.ID
	SnapID   string // ResolveSnap.ID
	LeaseID  string // DebitLease.ID
	LedgerID string // ledger.Transfer.ID
	ProofID  string // XferEffectProof.ID（若已产出）
	State    SendState
}

// Send 站内转账主入口（解析快照 → 借记租约 → SpotSend → 效果证明）。
//
// 幂等（严谨）：
//   - 同 idem_key 且 SendRecord 已为 completed：返回原结果；
//   - 同 idem_key 但仍在 resolving/debiting/sealing/blocked：返回 ErrIdemConflict，禁止当成功；
//   - 证明未固化前不得对外宣称 completed（seal 失败进入 blocked，由 patrol 重试）。
  func (e *Engine) Send(ctx context.Context, req SendRequest) (*SendReply, error) {
	   idem := strings.TrimSpace(req.IdempotencyKey)
	   if idem == "" {
		  return nil, fundint.ErrIdem
	   }
	   fromUID := strings.TrimSpace(req.UserID)
	   if fromUID == "" {
		  return nil, fmt.Errorf("user_id required")
	   }
	   if !req.Amount.IsPositive() {
		  return nil, ledger.ErrInvalidAmount
	   }
	  // 幂等查重
	   existing,
		  err := e.store.GetSendRecordByIdem(ctx, idem)
	  if err != nil && !errors.Is(err, ErrNotFound) {
		   return nil, fmt.Errorf("engine: get send record: %w", err)
	   }
	  if existing != nil {
		   // 与 Insert 竞态分支一致：仅 completed 可幂等返回；进行中/阻塞须报冲突，禁止误报成功。
		   if existing.State == SendStateCompleted {
			   return e.replyFromRecord(existing), nil
		   }
		   return nil, fmt.Errorf("%w: record_id=%s state=%s",
			   fundint.ErrIdemConflict, existing.ID, existing.State)
	   }
	  // 新建 SendRecord（resolving 状态）
	  now := time.Now().UTC()
	   rec := &SendRecord{
		  ID: uuid.NewString(),
		  IdemKey:    idem,
		  FromUserID: fromUID,
		   Asset:  req.Asset,
		  Amount: req.Amount.String(),
		  State:  SendStateResolving,
		  CreatedAt: now,
		  UpdatedAt: now,
	   }
	  if err := e.store.InsertSendRecord(ctx, rec); err != nil {
		   if errors.Is(err, ErrDuplicate) {
			   // 并发竞态：另一个 goroutine 已抢先插入
			   existing2, getErr := e.store.GetSendRecordByIdem(ctx, idem)
			  if getErr == nil && existing2 != nil {
				   // 只有 completed 状态可以幂等返回；其他状态返回冲突错误
				   if existing2.State == SendStateCompleted {
					  return e.replyFromRecord(existing2), nil
				  }
				  return nil, fmt.Errorf("%w: record_id=%s state=%s",
					   fundint.ErrIdemConflict, existing2.ID, existing2.State)
			  }
			  return nil, fundint.ErrIdemConflict
		  }
		  return nil, fmt.Errorf("engine: insert send record: %w", err)
	   }
	   // 调用 ServiceExt.SendGatedFull（含 resolve + snap + lease + SpotSend + commit）
	   gr, err := e.ext.SendGatedFull(ctx, fundint.Req{
		   UserID: req.UserID,
		  ToUID:   req.ToUID,
		  ToEmail: req.ToEmail,
		   ToPhone: req.ToPhone,
		   Asset:          req.Asset,
		  Amount:         req.Amount,
		  IdempotencyKey: idem,
		  Remark: req.Remark,
	  })
	   if err != nil {
		  // 转入 blocked 状态
		  blocked := time.Now().UTC()
		  rec.State = SendStateBlocked
		   rec.BlockedAt = &blocked
		  rec.BlockReason = err.Error()
		   rec.UpdatedAt = blocked
		  _ = e.store.UpdateSendRecord(ctx, rec)
		  return nil, err
	   }
	   // SpotSend 成功：推进到 sealing
	   rec.SnapID = gr.SnapID
	   rec.LeaseID = gr.LeaseID
	   rec.LedgerID = gr.Transfer.ID
	  rec.State = SendStateSealing
	   rec.UpdatedAt = time.Now().UTC()
	  if err := e.store.UpdateSendRecord(ctx, rec); err != nil {
		   return nil, fmt.Errorf("engine: update to sealing: %w", err)
	  }
	  // 产出 EffectProof
	   proof, err := e.sealProof(ctx, rec, gr)
	  if err != nil {
		   // 证明固化失败：blocking；patrol worker 会重试
		   blocked := time.Now().UTC()
		   rec.State = SendStateBlocked
		   rec.BlockedAt = &blocked
		   rec.BlockReason = fmt.Sprintf("proof seal: %v", err)
		   rec.UpdatedAt = blocked
		   _ = e.store.UpdateSendRecord(ctx,
			  rec)
		   return nil, fmt.Errorf("engine: seal proof: %w", err)
	   }
	   // 证明产出后：completed
	  rec.ProofID = proof.ID
	   rec.State = SendStateCompleted
	   rec.UpdatedAt = time.Now().UTC()
	   if err := e.store.UpdateSendRecord(ctx, rec); err != nil {
		   return nil, fmt.Errorf("engine: update to completed: %w", err)
	  }
	   return &SendReply{
		  RecordID: rec.ID,
		   SnapID:   rec.SnapID,
		   LeaseID:  rec.LeaseID,
		   LedgerID: rec.LedgerID,
		  ProofID:  proof.ID,
		   State:    SendStateCompleted,
	   }, nil
   }

// sealProof 产出并持久化效果证明。
   func (e *Engine) sealProof(ctx context.Context, rec *SendRecord, gr *fundint.GatedResult) (*XferEffectProof, error) {
	   hash := ComputeHash(rec.SnapID, rec.FromUserID, gr.ToUserID, rec.Amount, rec.LedgerID)
	   now := time.Now().UTC()
	   proof := &XferEffectProof{
		  ID:       uuid.NewString(),
		  SnapID:   rec.SnapID,
		   LeaseID:  rec.LeaseID,
		  LedgerID: rec.LedgerID,
		   FromUserID: rec.FromUserID,
		  ToUserID:   gr.ToUserID,
		  Asset:      rec.Asset,
		  Amount:     rec.Amount,
		  IdemKey:    rec.IdemKey,
		  ProofHash:  hash,
		  SealedAt: now,
		   FenceEpoch: now.UnixNano() / int64(time.Millisecond), // 毫秒纪元，与 r6 D run_id 键空间隔离
		  CreatedAt:  now,
	  }
	   if err := proof.Verify(); err != nil {
		  return nil, err
	  }
	  if err := e.store.InsertEffectProof(ctx, proof); err != nil {
		   return nil, err
	   }
	   return proof, nil
   }

// replyFromRecord 从已有 SendRecord 构造回复（幂等路径）。

   func (e *Engine) replyFromRecord(r *SendRecord) *SendReply {
	   return &SendReply{
		   RecordID: r.ID,
		   SnapID: r.SnapID,
		   LeaseID:  r.LeaseID,
		  LedgerID: r.LedgerID,
		  ProofID: r.ProofID,
		   State: r.State,
	  }
   }

// ─── MisrouteCase 检测 ─────────────────────────────────────────────────────
// DetectMisroute 检测目录改绑后的贷记偏差。
// 调用场景：scanner worker 定期查询 completed 的 SendRecord，重新 resolve 对手，
// 若结果与快照 ToUserID 不一致则开 MisrouteCase。

  func (e *Engine) DetectMisroute(ctx context.Context, snapID, ledgerID, snapToUID, actualToUID, fromUID, asset, amount, idemKey, note string) (*MisrouteCase, error) {
	  if strings.EqualFold(snapToUID, actualToUID) {
		  return nil, nil // 无偏差，正常
	  }
	   // 检查是否已有 case
	   existing,
		   err := e.store.GetMisrouteCaseBySnapLedger(ctx, snapID, ledgerID)
	   if err != nil && !errors.Is(err, ErrNotFound) {
		  return nil, fmt.Errorf("detect misroute: get existing: %w", err)
	   }
	   if existing != nil {
		   return existing, nil // 幂等
	  }
	   now := time.Now().UTC()
	  mc := &MisrouteCase{
		   ID:         uuid.NewString(),
		   SnapID:     snapID,
		   LedgerID:   ledgerID,
		  FromUserID: fromUID,
		  SnapToUserID: snapToUID,
		   ActualToUID: actualToUID,
		  Asset:       asset,
		   Amount:      amount,
		  IdemKey: idemKey,
		  State:   MisrouteCaseOpen,
		   ScanNote:  note,
		  CreatedAt: now,
		   UpdatedAt: now,
	  }
	   if err := e.store.InsertMisrouteCase(ctx, mc); err != nil {
		  return nil, fmt.Errorf("detect misroute: insert: %w", err)
	  }
	   return mc, nil
   }

// ConfirmMisroute 管理员确认误路由（open/proven → closed）。
  func (e *Engine) ConfirmMisroute(ctx context.Context, caseID, confirmedBy string) error {
	   mc, err := e.store.GetMisrouteCaseByID(ctx, caseID)
	  if err != nil {
		   return fmt.Errorf("confirm misroute: get case: %w", err)
	  }
	  if err := mc.Transition(MisrouteCaseClosed); err != nil {
		  return err
	   }
	   now := time.Now().UTC()
	  mc.State = MisrouteCaseClosed
	   mc.ConfirmedBy = confirmedBy
	   mc.ConfirmedAt = &now
	  mc.UpdatedAt = now
	   return e.store.UpdateMisrouteCase(ctx, mc)
   }

// ProvenMisroute 将 open case 推进到 proven（proof seal worker 调用）。

   func (e *Engine) ProvenMisroute(ctx context.Context, caseID, proofID string) error {
	  mc, err := e.store.GetMisrouteCaseByID(ctx, caseID)
	   if err != nil {
		   return fmt.Errorf("prove misroute: get case: %w", err)
	  }
	   if err := mc.Transition(MisrouteCaseProven); err != nil {
		  return err
	   }
	   mc.State = MisrouteCaseProven
	   mc.ProofID = proofID
	  mc.UpdatedAt = time.Now().UTC()
	  return e.store.UpdateMisrouteCase(ctx, mc)
  }

// GetSnap 查询解析快照（API 层调用）。
  func (e *Engine) GetSnap(ctx context.Context, idemKey string) (*fundint.ResolveSnap, error) {
	   snap, err := e.store.GetResolveSnapByIdem(ctx, idemKey)
	   if err != nil {
		   return nil, fmt.Errorf("get snap: %w", err)
	  }
	  return snap, nil
   }

// ExportProof 按 snap+ledger 导出效果证明（API 层调用）。

   func (e *Engine) ExportProof(ctx context.Context, snapID, ledgerID string) (*XferEffectProof, error) {
	   proof, err := e.store.GetEffectProofBySnapLedger(ctx, snapID, ledgerID)
	  if err != nil {
		  return nil, fmt.Errorf("export proof: %w", err)
	  }
	   if err := proof.Verify(); err != nil {
		  return nil, fmt.Errorf("export proof: verification failed: %w", err)
	   }
	  return proof, nil
  }

// ListOpenMisroutes 列出待处理的误路由案例（ops API）。
  func (e *Engine) ListOpenMisroutes(ctx context.Context, limit int) ([]*MisrouteCase, error) {
	  return e.store.ListMisrouteCases(ctx, MisrouteCaseOpen, limit)
   }
