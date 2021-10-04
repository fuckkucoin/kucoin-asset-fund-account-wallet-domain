package prefroute

import (
	"context"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/fundpref"
)

// Service prefroute 统一服务入口。
type Service struct {
	store  Store
	genMgr *fundpref.GenerationManager
	switchMgr *fundpref.SwitchManager
	leaseMgr *LeaseManager
	detector *DriftDetector
	proofMgr *DriftProofManager
	cfg   Config
	clock func() time.Time
}

// NewService 构造 Service。
   func NewService(store Store, cfg Config) *Service {
	  if store == nil {
		  store = NewMemStore()
	   }
	   inflight := NewStoreInflightChecker(store)
	  genMgr := fundpref.NewGenerationManager(store)
	  switchMgr := fundpref.NewSwitchManager(store, genMgr, inflight)
	   leaseMgr := NewLeaseManager(store, cfg)
	   detector := NewDriftDetector(store, leaseMgr)
	  proofMgr := NewDriftProofManager(store)
	   return &Service{
		  store: store,
		  genMgr: genMgr,
		  switchMgr: switchMgr,
		   leaseMgr:  leaseMgr,
		  detector:  detector,
		   proofMgr: proofMgr,
		   cfg:   cfg,
		  clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 注入统一时钟。
   func (s *Service) WithClock(c func() time.Time) *Service {
	   if s == nil || c == nil {
		   return s
	  }
	  s.clock = c
	   s.genMgr.WithClock(c)
	   s.switchMgr.WithClock(c)
	   s.leaseMgr.WithClock(c)
	   s.detector.WithClock(c)
	   s.proofMgr.WithClock(c)
	   return s
   }

  // ─────────────────────────────────────────────
  // 偏好世代 API
   // ─────────────────────────────────────────────
   // CurrentGeneration 查询用户当前偏好世代。

  func (s *Service) CurrentGeneration(ctx context.Context, userID string) (fundpref.PrefGeneration, error) {
	  return s.genMgr.Current(ctx, userID)
   }

// ListGenerations 查询偏好世代历史。

  func (s *Service) ListGenerations(ctx context.Context, userID string, limit int) ([]fundpref.PrefGeneration, error) {
	   return s.genMgr.List(ctx, userID, limit)
  }

// ─────────────────────────────────────────────
// DefaultAccount 切换 API
// ─────────────────────────────────────────────
// PutDefaultAccount 切换 DefaultAccount（含在途贷记互斥）。
// enqueueOnInflight=true 时在途中自动进入 pending 队列；
// false 时在途中直接返回 ErrInflightCredit。
   func (s *Service) PutDefaultAccount(ctx context.Context, req fundpref.PutDefaultAccountReq, enqueueOnInflight bool) (fundpref.SwitchResult, error) {
	  result, err := s.switchMgr.PutDefaultAccount(ctx, req, enqueueOnInflight)
	  if err != nil {
		   return fundpref.SwitchResult{}, err
	  }
	  // 切换立即生效时，废止旧租约
	  if result.Applied {
		  now := s.clock()
		   _, _ = s.store.ExpireStaleLeases(ctx, req.UserID, result.Generation.Generation, now)
	  }
	   return result, nil
  }

// GetDefaultPref 查询用户当前 DefaultAccountPref。

  func (s *Service) GetDefaultPref(ctx context.Context, userID string) (fundpref.DefaultAccountPref, error) {
	   p, err := s.store.GetDefaultPref(ctx, userID)
	   if err != nil {
		  if err == fundpref.ErrNotFound {
			  // 缺省 MAIN
			  return fundpref.DefaultAccountPref{
				  UserID: userID,
				  Book:   fundpref.BookMain,
			  }, nil
		   }
		   return fundpref.DefaultAccountPref{}, err
	  }
	  return p, nil
   }

// AdvancePendingSwitches 推进待切换队列（worker 调用）。
  func (s *Service) AdvancePendingSwitches(ctx context.Context, userID string) ([]fundpref.PendingSwitch, error) {
	  applied, err := s.switchMgr.AdvancePendingSwitch(ctx, userID)
	   if err != nil {
		  return nil, err
	  }
	   // 废止旧租约
	   now := s.clock()
	  for _, ps := range applied {
		   gen, err := s.store.GetGeneration(ctx, userID)
		   if err == nil {
			   _, _ = s.store.ExpireStaleLeases(ctx, userID, gen.Generation, now)
		   }
		   _ = ps
	  }
	   return applied, nil
   }

// ─────────────────────────────────────────────
// 租约 API
// ─────────────────────────────────────────────
// IssuePrefLease 签发偏好租约（贷记发起前调用）。
   func (s *Service) IssuePrefLease(ctx context.Context, req IssuePrefLeaseReq) (PrefLease, error) {
	  return s.leaseMgr.IssueLease(ctx, req)
   }

// BindRouteSnap 绑定路由快照到租约（贷记执行前调用）。

   func (s *Service) BindRouteSnap(ctx context.Context, req BindRouteSnapReq) (PrefLease, RouteSnap, error) {
	  return s.leaseMgr.BindRouteSnap(ctx,
		  req)
  }

// ValidateLease 校验租约是否仍有效。
   func (s *Service) ValidateLease(ctx context.Context, leaseID string) (PrefLease, error) {
	  return s.leaseMgr.ValidateLease(ctx, leaseID)
  }

// GetLease 查询租约详情。
  func (s *Service) GetLease(ctx context.Context, leaseID string) (PrefLease, error) {
	   return s.store.GetPrefLease(ctx,
		   leaseID)
   }

// ─────────────────────────────────────────────
// 路由快照 API
// ─────────────────────────────────────────────
// GetRouteSnap 查询路由快照。
  func (s *Service) GetRouteSnap(ctx context.Context, snapID string) (RouteSnap, error) {
	  return s.store.GetRouteSnap(ctx, snapID)
   }

// ─────────────────────────────────────────────
// 在途贷记 API
// ─────────────────────────────────────────────
// RegisterInflightCredit 注册在途贷记（贷记发起时调用）。

   func (s *Service) RegisterInflightCredit(ctx context.Context, ic InflightCredit) error {
	  return s.store.RegisterInflightCredit(ctx, ic)
   }

// CompleteInflightCredit 标记贷记完成，并触发漂移检测。

   func (s *Service) CompleteInflightCredit(ctx context.Context, txnID string, creditedBook fundpref.AccountBook) (*PrefDriftCase, error) {
	   ic, err := s.store.GetInflightCredit(ctx, txnID)
	   if err != nil {
		  return nil, fmt.Errorf("get inflight credit: %w", err)
	   }
	  now := s.clock()
	  ic.State = InflightStateCompleted
	  ic.UpdatedAt = now
	   if err := s.store.UpdateInflightCredit(ctx, ic); err != nil {
		  return nil, fmt.Errorf("update inflight credit: %w", err)
	  }
	  // 若有绑定 RouteSnap，触发漂移检测
	   if ic.RouteSnapID != "" {
		   req := ConfirmCreditReq{
			  RouteSnapID:  ic.RouteSnapID,
			   CreditedBook: creditedBook,
			  CreditTxnID: txnID,
		  }
		  _, dc, err := s.detector.ConfirmCredit(ctx, req)
		  if err != nil && err != ErrSnapAlreadyCredited {
			   return nil, fmt.Errorf("confirm credit: %w", err)
		  }
		   return dc, nil
	  }
	   return nil, nil
   }

// ─────────────────────────────────────────────
// 漂移 API
// ─────────────────────────────────────────────
// ConfirmCredit 贷记完成后检测漂移（直接调用，不经在途贷记流程）。

   func (s *Service) ConfirmCredit(ctx context.Context, req ConfirmCreditReq) (RouteSnap, *PrefDriftCase, error) {
	  return s.detector.ConfirmCredit(ctx, req)
  }

// SubmitDriftProof 提交漂移证明。

   func (s *Service) SubmitDriftProof(ctx context.Context, req SubmitDriftProofReq) (DriftProof, error) {
	   return s.proofMgr.SubmitProof(ctx, req)
   }

// ConfirmDriftClose 四眼确认关闭漂移案例。
   func (s *Service) ConfirmDriftClose(ctx context.Context, driftCaseID, confirmedBy string) (PrefDriftCase, error) {
	  return s.proofMgr.ConfirmClose(ctx,
		  driftCaseID, confirmedBy)
  }

// GetDriftCase 查询漂移案例。
   func (s *Service) GetDriftCase(ctx context.Context, caseID string) (PrefDriftCase, error) {
	  return s.store.GetDriftCase(ctx, caseID)
   }

// ListOpenDriftCases 列出 open 漂移案例。
  func (s *Service) ListOpenDriftCases(ctx context.Context, userID string) ([]PrefDriftCase, error) {
	  return s.store.ListOpenDriftCases(ctx, userID)
  }

// ExportDriftProof 导出漂移证明（只读）。
  func (s *Service) ExportDriftProof(ctx context.Context, proofID string) (DriftProof, error) {
	  return s.proofMgr.ExportProof(ctx,
		  proofID)
  }

// IsPrefWriteBlocked 检查用户是否因漂移而被阻断偏好写。
  func (s *Service) IsPrefWriteBlocked(ctx context.Context, userID string) (bool, string, error) {
	   return IsDriftBlocking(ctx, s.store, userID)
  }

// ─────────────────────────────────────────────
// 监控 API
// ─────────────────────────────────────────────
// ActiveInflightCredits 列出用户活跃在途贷记（ops 监控）。
  func (s *Service) ActiveInflightCredits(ctx context.Context, userID string) ([]InflightCredit, error) {
	  return s.store.ListInflightCredits(ctx, userID, InflightStatePending)
   }

// ActiveLeases 列出用户活跃租约（ops 监控）。
  func (s *Service) ActiveLeases(ctx context.Context, userID string) ([]PrefLease, error) {
	  return s.store.ListActiveLeases(ctx, userID)
   }

// ─────────────────────────────────────────────
// 内部辅助
// ─────────────────────────────────────────────
