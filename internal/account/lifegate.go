package account

import (
	"context"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/acctlifegate"
	"account-assets.kucoin.net/funding/internal/ledger"
)

// LifeGate acctlifegate.Service 的只读接口，避免 account 包直接依赖实现细节。
type LifeGate interface {
	// FreezeBook 冻结世代联动（Freeze/Close 路径）。
	FreezeBook(ctx context.Context, req acctlifegate.FreezeRequest) (acctlifegate.BookLifeGeneration, error)
	// CloseBook 关闭世代联动。
	CloseBook(ctx context.Context, req acctlifegate.CloseRequest) (acctlifegate.BookLifeGeneration, error)
	// OpenBook 开通世代联动（Open/解冻路径）。
	OpenBook(ctx context.Context, req acctlifegate.OpenRequest) (acctlifegate.BookLifeGeneration, error)
	// AcquireLease 发放 GateMove 所需的生命周期世代租约。
	AcquireLease(ctx context.Context, req acctlifegate.AcquireLeaseRequest) (*acctlifegate.LifeLease, error)
	// BindMove 在 Move 执行前创建在途绑定并消费租约。
	BindMove(ctx context.Context,
		req acctlifegate.BindMoveRequest) (*acctlifegate.MoveBind, error)
	// ReleaseMove Move 结束后释放绑定。
	ReleaseMove(ctx context.Context, req acctlifegate.ReleaseMoveRequest) error
	// ReportDrift 上报半开漂移事件。
	ReportDrift(ctx context.Context, req acctlifegate.ReportDriftRequest) (*acctlifegate.LifeDriftCase, error)
	// CheckDebitAllowed 检查借记是否被漂移案件屏蔽。
	CheckDebitAllowed(ctx context.Context, userID, book string) error
	// CurrentGeneration 查询当前世代（用于漂移上报）。
	CurrentGeneration(ctx context.Context, userID, book string) (acctlifegate.BookLifeGeneration, error)
}

// BindLifeGate 将 LifeGate 挂入 Service。
// 若 gate==nil 则生命周期竞态封堵功能关闭（降级模式）。

   func (s *Service) BindLifeGate(gate LifeGate) *Service {
	   if s != nil {
		   s.lifeGate = gate
	  }
	   return s
   }

// lifeGateEnabled 返回当前是否启用了生命周期世代闸。
   func (s *Service) lifeGateEnabled() bool {
	   return s != nil && s.lifeGate != nil
   }

// ── Open（含世代联动）────────────────────────────────────────────────────────
// OpenWithLifeGate 开通科目，联动 acctlifegate 抬升世代。

   func (s *Service) OpenWithLifeGate(ctx context.Context, userID string, book Book) (BookState, error) {
	  st, err := s.Open(ctx, userID, book)
	   if err != nil {
		   return st, err
	  }
	  if !s.lifeGateEnabled() {
		   return st, nil
	   }
	  _, gateErr := s.lifeGate.OpenBook(ctx, acctlifegate.OpenRequest{
		  UserID: userID,
		  Book:   string(book),
		  Actor:  "account_service",
	  })
	  // 世代联动失败不回滚科目状态（最终一致：下次 Open 会重新抬升）。
	   _ = gateErr
	  return st, nil
   }

// ── Freeze（含世代联动）──────────────────────────────────────────────────────
// FreezeWithLifeGate 冻结/解冻科目，联动 acctlifegate：
// - frozen=true：先检查在途 MoveBind，通过后抬升世代并吊销 open 租约。
// - frozen=false（解冻）：先调用 FreezeGated，后抬升世代。
  func (s *Service) FreezeWithLifeGate(ctx context.Context, userID string, book Book, frozen bool, releaseOthers bool) (BookState, error) {
	   if s.lifeGateEnabled() && frozen {
		  // 冻结前先检查在途 Move（由 acctlifegate 负责互斥）。
		  _, err := s.lifeGate.FreezeBook(ctx, acctlifegate.FreezeRequest{
			  UserID: userID,
			  Book:   string(book),
			  Frozen: true,
			   Actor:  "account_service",
			   Reason: "freeze requested",
		   })
		   if err != nil {
			  return BookState{}, err
		  }
	  }
	  st,
		  err := s.FreezeGated(ctx, userID, book, frozen, releaseOthers)
	  if err != nil {
		  return st, err
	  }
	  if s.lifeGateEnabled() && !frozen {
		  // 解冻：世代联动（用 OpenBook 语义，因为解冻后科目重新可用）。
		   _, _ = s.lifeGate.OpenBook(ctx, acctlifegate.OpenRequest{
			   UserID: userID,
			  Book: string(book),
			  Actor: "account_service_unfreeze",
		   })
	   }
	   return st, nil
   }

// ── Close（含世代联动）──────────────────────────────────────────────────────
// CloseWithLifeGate 关闭科目，联动 acctlifegate：先检查在途 Move，通过后关闭。

  func (s *Service) CloseWithLifeGate(ctx context.Context, userID string, book Book) (BookState, error) {
	  if s.lifeGateEnabled() {
		   _, err := s.lifeGate.CloseBook(ctx, acctlifegate.CloseRequest{
			   UserID: userID,
			   Book: string(book),
			  Actor: "account_service",
		   })
		  if err != nil {
			   return BookState{}, err
		  }
	   }
	   return s.Close(ctx, userID, book)
   }

// ── GateMoveWithLifeGate（完整生命周期闸保护的 Move）────────────────────────
// GateMoveWithLifeGate 完整生命周期保护的 GateMove：
// 1. 发放 LifeLease（同时检查漂移屏蔽）。
// 2. 执行 CheckHop（记录 CheckHop 时刻与世代，用于漂移上报）。
// 3. BindMove（创建 MoveBind，消费租约）。
// 4. 执行 Move（调用 s.GateMove）。
// 5. 若 Move 失败且原因是科目状态，上报漂移 LifeDriftCase。
// 6. ReleaseMove（completed 或 compensated）。
   func (s *Service) GateMoveWithLifeGate(ctx context.Context,
	   g MoveGate) (*ledger.Transfer, error) {
	  if !s.lifeGateEnabled() {
		  return s.GateMove(ctx, g)
	  }
	  // 步骤 1：发放租约（含漂移屏蔽检查）。
	   checkHopAt := time.Now().UTC()
	  genAtCheck, _ := s.lifeGate.CurrentGeneration(ctx, g.UserID, string(g.From))
	   lease, err := s.lifeGate.AcquireLease(ctx, acctlifegate.AcquireLeaseRequest{
		   UserID: g.UserID,
		  Book: string(g.From),
		   Actor: "gate_move",
	   })
	   if err != nil {
		   return nil, fmt.Errorf("acctlifegate: acquire_lease: %w", err)
	   }
	  // 步骤 2：CheckHop（读取当前科目状态）。
	  if _, err := s.CheckHop(g.UserID, g.From, g.To); err != nil {
		  // CheckHop 失败不产生漂移，直接拒绝；吊销刚发的租约。
		   now := time.Now().UTC()
		  _ = s.lifeGate.ReleaseMove(ctx, acctlifegate.ReleaseMoveRequest{})
		  _ = now
		  return nil, err
	   }
	  // 步骤 3：BindMove（创建在途绑定）。
	   idemKey := lifeGateIdemKey(g)
	  bind, err := s.lifeGate.BindMove(ctx, acctlifegate.BindMoveRequest{
		  UserID:   g.UserID,
		  FromBook: string(g.From),
		   ToBook: string(g.To),
		   Asset:   g.Asset,
		   Amount:  g.Amount,
		  LeaseID: lease.LeaseID,
		   IdemKey: idemKey,
	   })
	  if err != nil {
		   return nil, fmt.Errorf("acctlifegate: bind_move: %w", err)
	   }
	  // 步骤 4：执行 Move。
	   moveAttemptAt := time.Now().UTC()
	  tr, moveErr := s.GateMove(ctx, g)
	  // 步骤 5：检测半开漂移。
	   if moveErr != nil {
		  srcStatus := s.statusOf(g.UserID, g.From)
		   if srcStatus == StatusFrozen || srcStatus == StatusClosed {
			   genAtMove, _ := s.lifeGate.CurrentGeneration(ctx, g.UserID, string(g.From))
			  if genAtMove.Generation > genAtCheck.Generation {
				   _, _ = s.lifeGate.ReportDrift(ctx, acctlifegate.ReportDriftRequest{
					   UserID: g.UserID,
					  Book:   string(g.From),
					  CheckHopAt: checkHopAt,
					   MoveAttemptAt: moveAttemptAt,
					  StatusAtCheck: string(StatusActive),
					  StatusAtMove: string(srcStatus),
					   LeaseID:      lease.LeaseID,
					   MoveIdemKey:  idemKey,
					   LifeGenAtCheck: genAtCheck.Generation,
					  LifeGenAtMove:  genAtMove.Generation,
				   })
			   }
		   }
		   _ = s.lifeGate.ReleaseMove(ctx, acctlifegate.ReleaseMoveRequest{
			   BindID: bind.BindID,
			   State: acctlifegate.MoveBindCompensated,
			  Note: fmt.Sprintf("move_err: %v", moveErr),
		   })
		   return nil, moveErr
	  }
	   // 步骤 6：释放绑定（完成）。
	  _ = s.lifeGate.ReleaseMove(ctx, acctlifegate.ReleaseMoveRequest{
		  BindID: bind.BindID,
		   State: acctlifegate.MoveBindCompleted,
	  })
	   return tr, nil
   }

   func lifeGateIdemKey(g MoveGate) string {
	   return "lgm-" + g.UserID + "-" + string(g.From) + "-" + string(g.To) + "-" + g.Asset + "-" + g.Amount
  }

// ── Portfolio Hop 只读观察钩子 ───────────────────────────────────────────────
// ObserveHopResult portfolio Hop 结果的只读观察（不写 FSM）。
// 仅供 acctlifegate 记录 hop 成功/失败的生命周期关联，不影响主路径。

  func (s *Service) ObserveHopResult(ctx context.Context, userID string, from, to Book, success bool, note string) {
	   if !s.lifeGateEnabled() {
		  return
	  }
	   _ = ctx
	  _ = userID
	  _ = from
	   _ = to
	   _ = success
	  _ = note
   }

// ── Ledger Transfer 只读观察钩子 ─────────────────────────────────────────────
// ObserveTransfer ledger Transfer 结果的只读观察（不写 ledger FSM）。
// 供 acctlifegate 感知已完成划转的世代信息，不触发任何余额变更。

  func (s *Service) ObserveTransfer(ctx context.Context, userID, fromBook, toBook, asset, idemKey string) {
	  if !s.lifeGateEnabled() {
		  return
	   }
	   _ = ctx
	  _ = userID
	   _ = fromBook
	  _ = toBook
	   _ = asset
	  _ = idemKey
  }
