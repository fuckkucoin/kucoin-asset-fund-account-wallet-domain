// 启动顺序：
// 1. New(store, policy, pool, inv, shared, approval)
// 2. 可选 BindSQL → 切换 SQL 存储
// 3. StartWorkers(ctx) 启动后台 goroutine
// 线程安全：各子组件自身保证线程安全，GuardService 不额外加锁。

// Author: Likeo
// Date: 2021.04.18

package dustguard

import (
	"context"
	"time"
	"account-assets.kucoin.net/funding/internal/approvestore"
	"account-assets.kucoin.net/funding/internal/persist"
)

// GuardService 粉尘防护服务总门面。
type GuardService struct {
	store     Store
	policyMgr *PolicyManager
	detector  *Detector
	fsm       *QuarantineFSM
	rotator   *Rotator
	ticketMgr *TicketReleaser
	allocHook   *AllocateHook
	observeHook *ObserveHook
}

// GuardOptions 构造选项。

type GuardOptions struct {
	DB        *persist.SQLDB
	PolicyTTL time.Duration
	Pool PoolForceReleaser
	Inv    InventoryReserver
	Shared SharedMemoReg
	SharedChk SharedChecker
	Approvals *approvestore.Store
}

// New 构造完整的防护服务。
// 若 opts.DB 为 nil，使用内存存储（仅限开发）。

   func New(opts GuardOptions) *GuardService {
	  store := NewStore(opts.DB)
	  policy := NewPolicyManager(store, opts.PolicyTTL)
	  var rotation RotateApproval
	  if opts.Approvals != nil {
		  rotation = NewRotateApprovalStore(opts.Approvals)
	   }
	   detector := NewDetector(store, policy, opts.SharedChk)
	   fsm := NewQuarantineFSM(store, policy)
	  rotator := NewRotator(store, fsm, policy, opts.Pool, opts.Inv, opts.Shared, rotation)
	   ticketMgr := NewTicketReleaser(store)
	  allocHook := NewAllocateHook(fsm, detector, policy, store)
	  observeHook := NewObserveHook(fsm, detector, policy, store)
	   return &GuardService{
		  store:     store,
		  policyMgr: policy,
		   detector:  detector,
		   fsm:       fsm,
		  rotator:   rotator,
		  ticketMgr: ticketMgr,
		  allocHook:   allocHook,
		   observeHook: observeHook,
	  }
  }

// AllocateHook 返回分配前防护钩子（供 addrpool.Pool 注入）。
  func (s *GuardService) AllocateHook() *AllocateHook {
	   if s == nil {
		  return nil
	  }
	   return s.allocHook
  }

// ObserveHook 返回观察路径防护钩子（供 deposit.Service 注入）。
  func (s *GuardService) ObserveHook() *ObserveHook {
	   if s == nil {
		  return nil
	   }
	   return s.observeHook
  }

// Detector 返回检测器（供 worker 调用 FlushScores）。
   func (s *GuardService) Detector() *Detector {
	   if s == nil {
		  return nil
	   }
	   return s.detector
  }

// FSM 返回隔离状态机。

   func (s *GuardService) FSM() *QuarantineFSM {
	   if s == nil {
		  return nil
	   }
	  return s.fsm
   }

// Rotator 返回轮换编排器。
  func (s *GuardService) Rotator() *Rotator {
	   if s == nil {
		  return nil
	  }
	   return s.rotator
  }

// PolicyManager 返回策略管理器。
  func (s *GuardService) PolicyManager() *PolicyManager {
	   if s == nil {
		   return nil
	   }
	   return s.policyMgr
   }

// Store 返回底层存储（HTTP 审计查询用）。
   func (s *GuardService) Store() Store {
	  if s == nil {
		  return nil
	   }
	  return s.store
   }

// RecordObservation 把一笔链观察信号写入检测器（由入金工人调用，不阻断主路径）。
// chain/address/fromAddr 为必填；amount/txHash 辅助。

  func (s *GuardService) RecordObservation(ctx context.Context, chain, address, fromAddr, amount, txHash, asset string) (AttackScore, error) {
	  if s == nil {
		   return AttackScore{}, nil
	   }
	   sig := DustSignal{
		  Chain:   chain,
		  Address: address,
		  FromAddr: fromAddr,
		   Amount:   amount,
		  TxHash: txHash,
		   Asset:      asset,
		  SignalKind: SignalFlood,
	  }
	  return s.detector.Record(ctx, sig)
  }

// StartWorkers 启动三个后台 worker goroutine（非阻塞）。
// detectorInterval: 推荐 30s；quarantineInterval: 推荐 60s；waterInterval: 推荐 120s。
   func (s *GuardService) StartWorkers(ctx context.Context, wlr WaterLevelReader,
	  detectorInterval, quarantineInterval, waterInterval time.Duration) {
	   if s == nil {
		   return
	  }
	  go RunDetector(ctx, s.detector, s.fsm, s.policyMgr, detectorInterval)
	   go RunQuarantineTimer(ctx, s.fsm, quarantineInterval)
	   go RunWaterLevel(ctx, s.rotator, s.policyMgr, wlr, waterInterval)
   }
