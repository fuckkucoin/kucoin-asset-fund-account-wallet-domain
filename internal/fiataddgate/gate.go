package fiataddgate

import (
	"context"
	"errors"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/fiatadd"
	"account-assets.kucoin.net/funding/internal/ledger"
	"github.com/shopspring/decimal"
)

const (
	// DefaultLeaseTTL 默认确认租约有效期。
	DefaultLeaseTTL = 30 * time.Second
)

// LedgerCreator ledger.Service.Create 的最小接口，便于注入。
type LedgerCreator interface {
	Create(ctx context.Context, req ledger.CreateReq) (*ledger.Transfer, error)
}

// GateConfig 确认闸配置。
type GateConfig struct {
	// SignKey 签名密钥，同 fiatadd.Book.key。
	SignKey string
	// LeaseTTL 租约有效期，默认 30s。
	LeaseTTL time.Duration
	// Actor 操作者标识，写入租约/案例。
	Actor string
}

  func (c *GateConfig) leaseTTL() time.Duration {
	   if c == nil || c.LeaseTTL <= 0 {
		   return DefaultLeaseTTL
	   }
	   return c.LeaseTTL
   }

// Gate 确认世代闸主引擎。
// 调用顺序（正常路径）：
// 1. Gate.GateConfirm(ctx, req) → (ConfirmResult, error)
// 内部：Begin(CAS pending→confirming) → ledger.Create → CommitCredit(confirming→credited)
// 任何步骤失败自动 RecordFork（若 Create 已成功）
type Gate struct {
	book *fiatadd.Book
	leases LeaseStore
	effects EffectStore
	forks   ForkCaseStore
	led   LedgerCreator
	cfg   GateConfig
	clock func() time.Time
}

// NewGate 构造确认闸。

  func NewGate(
	   book *fiatadd.Book,
	   leases LeaseStore,
	  effects EffectStore,
	  forks ForkCaseStore,
	   led LedgerCreator,
	  cfg GateConfig,
  ) *Gate {
	   return &Gate{
		   book: book,
		  leases: leases,
		  effects: effects,
		  forks:   forks,
		  led:     led,
		   cfg:   cfg,
		   clock: func() time.Time { return time.Now().UTC() },
	  }
   }

// WithClock 注入时钟。

  func (g *Gate) WithClock(c func() time.Time) *Gate {
	  if g != nil && c != nil {
		  g.clock = c
	   }
	   return g
  }

  // ConfirmRequest 确认闸输入。

   type ConfirmRequest struct {
	   UserID    string
	  SessionID string
	  // Signature 必须使用 BoundSig(key, sessionID, amount, gen) 计算。
	   // gen 由服务端在 Begin 阶段抬升后通过 challenge 回传客户端，客户端重新签名提交。
	  // 若使用旧版签名（不含 gen），设置 LegacySig=true（仅兼容路径使用）。
	  Signature string
	   LegacySig bool
  }

  // ConfirmResult 确认结果。

   type ConfirmResult struct {
	   Session fiatadd.Session
	   Lease   ConfirmLease
	  Effect CreditEffect
   }

  // GateConfirm 执行带世代保护的 Confirm 完整流程。
   // 并发安全保证：
  // - 两路同时调用同一 SessionID：第一路 CAS pending→confirming 成功并签发租约；
  // 第二路 ConfirmWithGen 返回 ErrConfirming，进入幂等回放路径（轮询等待第一路完成）。
  // - Create 成功 Save 失败：自动产出 ForkCase，阻止后续 Confirm，直到四眼收口。
   // - confirm_gen 抬升后旧签名不得再贷：BoundSig 包含 gen，旧签名校验失败。

   func (g *Gate) GateConfirm(ctx context.Context, req ConfirmRequest) (ConfirmResult, error) {
	   if g == nil {
		   return ConfirmResult{}, fmt.Errorf("fiataddgate: gate nil")
	   }
	  // 1. 检查分叉阻断：同 Session 存在 open ForkCase 则拒绝
	  if _, err := g.forks.GetOpenCase(ctx, req.SessionID); err == nil {
		  return ConfirmResult{}, ErrForkBlocked
	  }
	   // 2. 读取当前 Session 状态
	  sess, err := g.book.Get(ctx, req.UserID, req.SessionID)
	  if err != nil {
		  return ConfirmResult{}, fmt.Errorf("fiataddgate: load session: %w", err)
	   }
	  // 3. 幂等回放：已 credited 且有对应 effect
	   if sess.Status == "credited" {
		  effect, effErr := g.effects.GetBySessionGen(ctx, req.SessionID, sess.ConfirmGen)
		   if effErr == nil {
			  lease, _ := g.leases.GetBySessionGen(ctx, req.SessionID, sess.ConfirmGen)
			  return ConfirmResult{Session: sess, Lease: lease, Effect: effect}, nil
		   }
		  // credited 但 effect 缺失：仍返回成功（Session 已是终态）
		   return ConfirmResult{Session: sess}, nil
	  }
	   // 4. 正在 confirming：等待后幂等回放
	  if sess.Status == "confirming" {
		   return g.waitAndReplay(ctx, req)
	  }
	   // 5. 签发新确认租约
	   leaseID := newLeaseID()
	  now := g.clock()
	   expires := now.Add(g.cfg.leaseTTL())
	  // 6. CAS pending→confirming，confirm_gen 抬升
	  sess, err = g.book.ConfirmWithGen(ctx, req.UserID, req.SessionID, req.Signature, leaseID)
	  if err != nil {
		  if errors.Is(err, fiatadd.ErrConfirming) {
			  return g.waitAndReplay(ctx,
				  req)
		   }
		   return ConfirmResult{}, fmt.Errorf("fiataddgate: CAS confirming: %w", err)
	  }
	   // 7. 校验世代绑定签名
	  sigOK := false
	   if req.LegacySig {
		  sigOK = ValidateLegacySig(g.cfg.SignKey, sess.ID, sess.Amount, req.Signature)
	  } else {
		  sigOK = ValidateBoundSig(g.cfg.SignKey, sess.ID, sess.Amount, req.Signature, sess.ConfirmGen)
	  }
	   if !sigOK {
		  // 签名与世代不匹配：打开 ForkCase 阻断后续操作，由运营确认签名问题后收口
		  _ = g.openForkCaseAndMark(ctx, sess, leaseID, "", ForkLedgerCreditedSessionPending,
			   "signature bound to stale or invalid generation")
		  return ConfirmResult{}, fiatadd.ErrGenBound
	  }
	  sigDigest := SigDigest(req.Signature)
	  // 8. 持久化确认租约
	   lease := ConfirmLease{
		   LeaseID:   leaseID,
		  SessionID: sess.ID,
		   UserID:    sess.UserID,
		   Asset:     sess.Asset,
		   Amount: sess.Amount,
		  ConfirmGen: sess.ConfirmGen,
		   SigDigest: sigDigest,
		   State: LeaseIssued,
		   IssuedAt:  now,
		   ExpiresAt: expires,
		  Actor:     g.cfg.Actor,
	   }
	  if err := g.leases.Insert(ctx, lease); err != nil {
		  if errors.Is(err, ErrLeaseConflict) {
			   return g.waitAndReplay(ctx, req)
		   }
		  return ConfirmResult{}, fmt.Errorf("fiataddgate: insert lease: %w", err)
	   }
	  // 9. 执行 ledger 贷记
	  var creditID string
	  if g.led != nil {
		   idemKey := fmt.Sprintf("fiat-add-gate:%s:%d", sess.ID, sess.ConfirmGen)
		   amt, _ := decimal.NewFromString(sess.Amount)
		  tr, ledErr := g.led.Create(ctx, ledger.CreateReq{
			   UserID:    sess.UserID,
			   Direction: ledger.FiatOnramp,
			   Asset: sess.Asset,
			  Amount: amt,
			   IdempotencyKey: idemKey,
		  })
		   if ledErr != nil {
			   // ledger Create 失败：租约标记 forked
			  now2 := g.clock()
			   _ = g.leases.Transition(ctx, leaseID, LeaseIssued, LeaseForked, ConfirmLease{
				  ForkedAt: &now2,
				  Note:     fmt.Sprintf("ledger create failed: %v", ledErr),
			  })
			  _ = g.openForkCaseAndMark(ctx, sess, leaseID, "", ForkLedgerCreditedSessionPending,
				  fmt.Sprintf("ledger create failed: %v", ledErr))
			   return ConfirmResult{}, fmt.Errorf("fiataddgate: ledger create: %w", ledErr)
		   }
		   if tr != nil {
			   creditID = tr.ID
		   }
	   }
	   // 10. 写入不可变 CreditEffect（double-credit 证明）
	   effect := CreditEffect{
		  EffectID: newEffectID(),
		   SessionID: sess.ID,
		  ConfirmGen: sess.ConfirmGen,
		  LeaseID: leaseID,
		   CreditID:  creditID,
		   SigDigest: sigDigest,
		   Amount: sess.Amount,
		   Asset:  sess.Asset,
		   UserID: sess.UserID,
		   CreatedAt: g.clock(),
	   }
	  if insErr := g.effects.Insert(ctx, effect); insErr != nil {
		  if errors.Is(insErr, ErrEffectDup) {
			   // 幂等键冲突：另一路已成功写入效果，读取并返回
			   existing, _ := g.effects.GetBySessionGen(ctx, sess.ID, sess.ConfirmGen)
			  creditID = existing.CreditID
			  effect = existing
		  } else {
			   // 写入 effect 失败，贷记可能已成功，打开 ForkCase
			   now2 := g.clock()
			   _ = g.leases.Transition(ctx, leaseID, LeaseIssued, LeaseForked, ConfirmLease{
				  CreditID: creditID,
				  ForkedAt: &now2,
				  Note:     fmt.Sprintf("credit effect insert failed: %v", insErr),
			  })
			   _ = g.openForkCaseAndMark(ctx, sess, leaseID, creditID, ForkLedgerCreditedSessionPending,
				  fmt.Sprintf("credit effect insert failed: %v", insErr))
			   return ConfirmResult{}, fmt.Errorf("fiataddgate: effect insert: %w", insErr)
		   }
	  }
	   // 11. CommitCredit：Session confirming→credited（原子写）
	  committed, commitErr := g.book.CommitCredit(ctx, sess.ID, creditID, sess.ConfirmGen)
	  if commitErr != nil {
		  // Session Save 失败 → 分叉！ledger 已贷，Session 仍 confirming
		  now2 := g.clock()
		   _ = g.leases.Transition(ctx, leaseID, LeaseIssued, LeaseForked, ConfirmLease{
			   CreditID: creditID, ForkedAt: &now2,
			   Note: fmt.Sprintf("commit credit failed: %v", commitErr),
		   })
		  _ = g.openForkCaseAndMark(ctx, sess, leaseID, creditID, ForkLedgerCreditedSessionPending,
			  fmt.Sprintf("commit credit failed; ledger already credited: %v", commitErr))
		   return ConfirmResult{}, fmt.Errorf("fiataddgate: commit credit: %w", commitErr)
	  }
	  // 12. 租约推进到 credited，更新进程内 lease 副本
	  now3 := g.clock()
	   _ = g.leases.Transition(ctx, leaseID, LeaseIssued, LeaseCredited, ConfirmLease{
		  CreditID:   creditID,
		  CreditedAt: &now3,
	   })
	   lease.State = LeaseCredited
	  lease.CreditID = creditID
	   lease.CreditedAt = &now3
	   return ConfirmResult{Session: committed,
		   Lease: lease, Effect: effect}, nil
   }

// waitAndReplay 等待同 Session 的活跃 Confirm 完成后幂等回放。
// 最多轮询 6 次（总等待约 630ms）；超时则返回 ErrConfirming。

  func (g *Gate) waitAndReplay(ctx context.Context, req ConfirmRequest) (ConfirmResult, error) {
	   const maxAttempts = 6
	  backoff := 80 * time.Millisecond
	   for i := 0; i < maxAttempts; i++ {
		  select {
		   case <-ctx.Done():
			   return ConfirmResult{}, ctx.Err()
		  case <-time.After(backoff):
		   }
		  backoff *= 2
		  // 分叉检查
		  if _, err := g.forks.GetOpenCase(ctx, req.SessionID); err == nil {
			   return ConfirmResult{},
				   ErrForkBlocked
		   }
		  sess, err := g.book.Get(ctx, req.UserID, req.SessionID)
		   if err != nil {
			   return ConfirmResult{}, err
		   }
		   switch sess.Status {
		  case "credited":
			  effect, _ := g.effects.GetBySessionGen(ctx, req.SessionID, sess.ConfirmGen)
			  lease, _ := g.leases.GetBySessionGen(ctx, req.SessionID, sess.ConfirmGen)
			   return ConfirmResult{Session: sess, Lease: lease, Effect: effect}, nil
		   case "forked":
			   return ConfirmResult{}, ErrForkBlocked
		  }
	   }
	  return ConfirmResult{},
		  fiatadd.ErrConfirming
   }

// openForkCaseAndMark 打开分叉案例并将 Session 标记为 forked。
   func (g *Gate) openForkCaseAndMark(ctx context.Context, sess fiatadd.Session, leaseID, creditID string, kind ForkKind, note string) FiatAddForkCase {
	  fc := FiatAddForkCase{
		  CaseID: newCaseID(),
		   SessionID: sess.ID,
		  ConfirmGen:     sess.ConfirmGen,
		  LeaseID:        leaseID,
		   LedgerCreditID: creditID,
		   SessionStatus:  sess.Status,
		   Kind: kind,
		   State: ForkCaseOpen,
		  OpenedAt: g.clock(),
		   Note: note,
	  }
	  if sess.Status == "confirming" {
		  _, _ = g.book.MarkForked(ctx, sess.ID, sess.ConfirmGen, creditID)
	  }
	  opened, _ := g.forks.Open(ctx, fc)
	  return opened
   }
