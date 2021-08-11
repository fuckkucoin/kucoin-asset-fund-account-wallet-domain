package isodebt

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/isobook"
)

// Service isodebt 统一服务入口。

type Service struct {
	store Store
	genMgr *GenerationManager
	leaseMgr *LeaseManager
	adjud    *GateAdjudicator
	detector *DriftDetector
	proofMgr *DriftProofManager
	mutex    *DebitTransferMutex
	cfg      Config
	clock    func() time.Time
}

// NewService 构造 Service（使用指定 Store 与 isobook）。
  func NewService(store Store, book *isobook.Book, cfg Config) *Service {
	  if store == nil {
		   store = NewMemStore()
	   }
	   if book == nil {
		   book = isobook.New()
	   }
	   mutex := NewDebitTransferMutex()
	  genMgr := NewGenerationManager(store,
		  book)
	  leaseMgr := NewLeaseManager(store, genMgr, cfg)
	   detector := NewDriftDetector(store, genMgr, book, cfg)
	  proofMgr := NewDriftProofManager(store)
	   adjud := NewGateAdjudicator(store, leaseMgr, mutex)
	   return &Service{
		   store:  store,
		   genMgr: genMgr,
		   leaseMgr: leaseMgr,
		   adjud: adjud,
		   detector: detector,
		  proofMgr: proofMgr,
		  mutex:    mutex,
		  cfg: cfg,
		   clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 注入统一时钟（传播至所有子管理器）。
  func (s *Service) WithClock(c func() time.Time) *Service {
	  if s == nil || c == nil {
		  return s
	  }
	  s.clock = c
	  s.genMgr.WithClock(c)
	  s.leaseMgr.WithClock(c)
	   s.adjud.WithClock(c)
	   s.detector.WithClock(c)
	  s.proofMgr.WithClock(c)
	  return s
   }

   // ─────────────────────────────────────────────
   // 世代 API
  // ─────────────────────────────────────────────
   // CreditAndBump Credit 操作后调用，递增世代并重算 DebtRatio；同时废止旧租约。

   func (s *Service) CreditAndBump(ctx context.Context, userID, symbol, sourceTxnID string) (PairGeneration, error) {
	   gen, err := s.genMgr.BumpOnCredit(ctx, userID, symbol, sourceTxnID)
	  if err != nil {
		   return PairGeneration{}, err
	   }
	   _, _ = s.store.ExpireStaleLeases(ctx, userID, symbol, gen.Generation, s.clock())
	  return gen, nil
   }

// ListGenerations 查询世代历史。
   func (s *Service) ListGenerations(ctx context.Context, userID, symbol string, limit int) ([]PairGeneration, error) {
	  return s.store.ListGenerations(ctx, userID, symbol, limit)
  }

// ─────────────────────────────────────────────
// 租约 API
// ─────────────────────────────────────────────
// IssueRatioLease 签发比率租约（供 TransferIn 发起方获取）。
  func (s *Service) IssueRatioLease(ctx context.Context, userID, symbol, issuedBy string) (RatioLease, error) {
	   return s.leaseMgr.IssueRatioLease(ctx, userID, symbol, issuedBy)
  }

// ValidateLease 校验租约（TransferIn 执行前调用）。
   func (s *Service) ValidateLease(ctx context.Context, leaseID string) (RatioLease, error) {
	   return s.leaseMgr.ValidateLease(ctx, leaseID)
  }

// GetLease 查询租约详情。
  func (s *Service) GetLease(ctx context.Context, leaseID string) (RatioLease, error) {
	   return s.store.GetLease(ctx, leaseID)
   }

// ─────────────────────────────────────────────
// 转入闸 API
// ─────────────────────────────────────────────
// AdjudicateTransferIn 裁决 TransferIn 请求。
   func (s *Service) AdjudicateTransferIn(ctx context.Context, req AdjudicateRequest) (TransferGate, error) {
	  return s.adjud.AdjudicateTransferIn(ctx, req)
  }

// GetGate 查询转入门详情。

  func (s *Service) GetGate(ctx context.Context, gateID string) (TransferGate, error) {
	   return s.store.GetGate(ctx, gateID)
   }

// ─────────────────────────────────────────────
// 借记互斥 API
// ─────────────────────────────────────────────
// LockDebit 锁定 Pair 的 Debit 互斥（调用方 defer unlock）。

   func (s *Service) LockDebit(userID, symbol, debitTxnID string) func() {
	  return s.mutex.LockDebit(userID, symbol, debitTxnID)
  }

// ─────────────────────────────────────────────
// 漂移 API
// ─────────────────────────────────────────────
// DetectDrift 对指定 Pair 执行漂移检测。
   func (s *Service) DetectDrift(ctx context.Context, userID, symbol string) (DetectResult, error) {
	   return s.detector.Detect(ctx, userID, symbol)
  }

// SubmitDriftProof 提交漂移证明。
  func (s *Service) SubmitDriftProof(ctx context.Context, driftCaseID, submittedBy, summary, evidence string) (RatioDriftProof, error) {
	  return s.proofMgr.SubmitProof(ctx, driftCaseID, submittedBy, summary, evidence)
   }

// ConfirmDriftClose 四眼确认关闭漂移案例。

  func (s *Service) ConfirmDriftClose(ctx context.Context, driftCaseID, confirmedBy string) (RatioDriftCase, error) {
	   return s.proofMgr.ConfirmClose(ctx, driftCaseID, confirmedBy)
  }

// GetDriftCase 查询漂移案例。
  func (s *Service) GetDriftCase(ctx context.Context, caseID string) (RatioDriftCase, error) {
	   return s.store.GetDriftCase(ctx,
		   caseID)
  }

// ExportDriftProof 导出证明（只读）。
   func (s *Service) ExportDriftProof(ctx context.Context, proofID string) (RatioDriftProof, error) {
	   return s.proofMgr.ExportProof(ctx, proofID)
   }

// ListOpenDriftCases 列出 open 漂移案例。
  func (s *Service) ListOpenDriftCases(ctx context.Context, userID, symbol string) ([]RatioDriftCase, error) {
	   return s.store.ListOpenDriftCases(ctx, userID, symbol)
  }

// ─────────────────────────────────────────────
// 监控 API
// ─────────────────────────────────────────────
// ActiveDebitLocks 返回当前活跃 Debit 锁列表（用于监控/ops）。
   func (s *Service) ActiveDebitLocks() []string {
	  return s.mutex.ActiveDebits()
   }

// ─────────────────────────────────────────────
// 内部辅助
// ─────────────────────────────────────────────
// debitAndBumpWithLock 带互斥锁执行 Debit 世代递增（内部使用）。
  func (s *Service) debitAndBumpWithLock(ctx context.Context, userID, symbol, txnID string) (PairGeneration, error) {
	  userID = strings.TrimSpace(userID)
	  symbol = strings.ToUpper(strings.TrimSpace(symbol))
	  if userID == "" || symbol == "" || txnID == "" {
		  return PairGeneration{}, fmt.Errorf("%w: userID, symbol, txnID required", ErrInvalidInput)
	   }
	   unlock := s.mutex.LockDebit(userID, symbol, txnID)
	   defer unlock()
	  return s.DebitAndBump(ctx, userID, symbol, txnID)
   }

// DebitWithMutex 封装带互斥的 Debit 世代递增（公开给需要自动加锁的调用方）。

   func (s *Service) DebitWithMutex(ctx context.Context, userID, symbol, txnID string) (PairGeneration, error) {
	   return s.debitAndBumpWithLock(ctx, userID, symbol, txnID)
  }

// RecomputeRatio 主动重算（不递增世代）。
   func (s *Service) RecomputeRatio(ctx context.Context, userID, symbol string) (PairGeneration, error) {
	  return s.genMgr.Recompute(ctx, userID, symbol)
  }

// IsTransferInBlocked 检查 Pair 当前是否因漂移或比率超限而阻断转入。
// 返回 (blocked bool, reason string, err error)。

   func (s *Service) IsTransferInBlocked(ctx context.Context, userID, symbol string) (bool, string, error) {
	   // 漂移检查
	   drifted, caseID, err := IsDriftBlocking(ctx, s.store, userID, symbol)
	  if err != nil {
		   return false, "", err
	  }
	  if drifted {
		  return true, fmt.Sprintf("drift_case=%s", caseID), nil
	  }
	   // 比率检查
	  gen, err := s.store.GetGeneration(ctx, userID, symbol)
	   if err != nil {
		  return false, "", err
	   }
	  if gen.DebtRatio.GreaterThanOrEqual(s.cfg.MaxDebtRatio) {
		   return true, fmt.Sprintf("debt_ratio=%s>=max=%s",
			   gen.DebtRatio.StringFixed(6), s.cfg.MaxDebtRatio.StringFixed(6)), nil
	   }
	   return false, "", nil
   }

// RatioSnapshot 返回指定 Pair 的 DebtRatio 快照（当前世代）。
   func (s *Service) RatioSnapshot(ctx context.Context, userID, symbol string) (decimal.Decimal, int64, error) {
	   gen, err := s.store.GetGeneration(ctx, userID, symbol)
	   if err != nil {
		   return decimal.Zero, 0, err
	  }
	   return gen.DebtRatio, gen.Generation, nil
  }
