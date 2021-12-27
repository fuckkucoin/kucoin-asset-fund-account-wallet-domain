// 功能：交易密码世代引擎 Service 层——组合 GenerationManager、InflightMutex、DriftDetector。
// 重点：
// BumpAndRevoke Change/Reset 核心写路径：抬升世代、原子吊销旧令牌、记录冷却绑定，三步同事务。
// IssueVerifyLease 出金核验通过后颁发令牌；前置检查漂移案件与世代存在性。
// ConsumeVerifyLease 出金提交时消费令牌；gen 漂移时升案件并返回 ErrLeaseGenMismatch。
// MarkCoolWithGen markCool 挂钩：携带 pw_gen 调用 seclock.Mark，成功后确认 CoolBound。
// 风险点：
// BumpAndRevoke 须在数据库事务内（InTx）完成世代抬升与令牌吊销；外部 InTx 由调用方负责。
// IssueVerifyLease 与 BumpAndRevoke 存在 Race 窗口（颁发后立即 Change）：
// RevokeInflight 在 BumpGeneration 成功后原子清扫该窗口内颁发的旧 gen 令牌。

// 作者：Cason
// 日期：2021.12.08

package tradepwgen

import (
	"fmt"
	"time"
)

// Service 交易密码世代引擎服务。
type Service struct {
	gen *GenerationManager
	mutex    *InflightMutex
	detector *DriftDetector
	clock    func() time.Time
}

// NewService 构造服务（内存持久化，适用于与单机模式）。
// leaseTTL 为 0 时 VerifyLease 不过期。
  func NewService(leaseTTL time.Duration) *Service {
	   genStore := NewMemGenerationStore()
	   leaseStore := NewMemLeaseStore()
	   driftStore := NewMemDriftStore()
	   return &Service{
		   gen: NewGenerationManager(genStore, driftStore),
		   mutex: NewInflightMutex(genStore,
			  leaseStore, driftStore, leaseTTL),
		  detector: NewDriftDetector(driftStore),
		   clock: func() time.Time { return time.Now().UTC() },
	  }
   }

// NewServiceWith 用外部组件构造服务（模式，注入 SQL 实现）。

   func NewServiceWith(
	   genStore GenerationStore,
	   leaseStore LeaseStore,
	  driftStore DriftStore,
	  leaseTTL time.Duration,
  ) *Service {
	  return &Service{
		   gen: NewGenerationManager(genStore,
			  driftStore),
		   mutex: NewInflightMutex(genStore, leaseStore, driftStore, leaseTTL),
		   detector: NewDriftDetector(driftStore),
		   clock: func() time.Time { return time.Now().UTC() },
	   }
  }

  func (svc *Service) now() time.Time {
	   if svc.clock != nil {
		   return svc.clock()
	   }
	  return time.Now().UTC()
  }

// ──────────────────────────────────────────────
// 世代写路径（Change / Reset 调用）
// ──────────────────────────────────────────────
// BumpAndRevoke 抬升世代并吊销旧令牌（Change/Reset 核心路径）。
// 流程：
// 1. 读取当前世代（expectedGen）。
// 2. CAS 抬升世代（BumpGeneration）。
// 3. 原子吊销该用户全部 open 旧令牌（RevokeInflight）。
// 4. 返回新世代记录。
// 注意：调用方须在事务内调用本函数（persist.KV.InTx / SQLDB.InTx）。
// 冷却绑定（MarkCoolWithGen）须在事务提交后调用（seclock.Mark 不参与数据库事务）。

  func (svc *Service) BumpAndRevoke(userID, reason string) (*PwGeneration, int, error) {
	  if userID == "" {
		   return nil, 0, ErrUserRequired
	  }
	   // 读当前世代。
	  cur, err := svc.gen.Current(userID)
	  if err != nil {
		   return nil, 0, fmt.Errorf("tradepwgen: BumpAndRevoke.Current: %w", err)
	   }
	   // CAS 抬升世代。
	  ng, err := svc.gen.BumpGeneration(userID, cur.Generation, reason)
	   if err != nil {
		   return nil, 0, fmt.Errorf("tradepwgen: BumpAndRevoke.BumpGeneration: %w", err)
	   }
	  // 原子吊销旧令牌。
	  revoked, err := svc.mutex.RevokeInflight(RevokeInflightReq{
		  UserID: userID,
		   PwGen: ng.Generation,
	  })
	   if err != nil {
		   return nil, 0, fmt.Errorf("tradepwgen: BumpAndRevoke.RevokeInflight: %w", err)
	  }
	  return ng, revoked, nil
   }

// ConfirmCool 确认指定世代冷却已生效（由 tradepw.Book 在 seclock.Mark 成功后调用）。
// gen 须与当前世代一致，否则返回 ErrGenConflict。
   func (svc *Service) ConfirmCool(userID string, gen int64) error {
	  return svc.gen.ConfirmCool(userID, gen)
  }

// ──────────────────────────────────────────────
// 出金核验令牌路径（RequireForWithdraw / 出金提交调用）
// ──────────────────────────────────────────────
// IssueVerifyLease 颁发核验令牌（RequireForWithdraw 通过后调用）。
// 前置条件：交易密码已通过核验（由 tradepw.Book.Verify 保证，调用方须先调用）。
// 漂移案件 open 时返回 ErrDriftOpen；世代不存在时返回错误。
  func (svc *Service) IssueVerifyLease(req IssueLeaseReq) (*VerifyLease, error) {
	   if err := svc.gen.GuardIssueLease(req.UserID); err != nil {
		   return nil, err
	   }
	   return svc.mutex.IssueLease(req)
  }

// ConsumeVerifyLease 消费核验令牌（出金提交时调用）。
// 发现世代漂移时自动创建 PwDriftCase，并返回 ErrLeaseGenMismatch。
// 调用方须在发现漂移后拒绝出金请求。
   func (svc *Service) ConsumeVerifyLease(req ConsumeLeaseReq) (*VerifyLease, *PwDriftCase, error) {
	  lease, ev, err := svc.mutex.ConsumeLease(req)
	   if err == nil {
		   return lease, nil, nil
	   }
	  if err == ErrLeaseGenMismatch && ev != nil {
		   dc, detectErr := svc.detector.Detect(*ev)
		   if detectErr != nil {
			   return nil, nil, fmt.Errorf("tradepwgen: ConsumeVerifyLease.Detect: %w; original: %w", detectErr, err)
		  }
		  return nil, dc, ErrLeaseGenMismatch
	   }
	  return nil, nil, err
  }

// ──────────────────────────────────────────────
// 漂移案件路径
// ──────────────────────────────────────────────
// ProveGenDrift 固化漂移案件证明（open→proven）。
  func (svc *Service) ProveGenDrift(req ProveGenDriftReq) (*PwDriftCase, error) {
	  return svc.detector.ProveCase(req)
   }

// CloseGenDrift 收口漂移案件（proven→closed）。
  func (svc *Service) CloseGenDrift(req CloseDriftReq) (*PwDriftCase, error) {
	  return svc.detector.CloseCase(req)
   }

// ForceCloseGenDrift ops 强制收口漂移案件。
  func (svc *Service) ForceCloseGenDrift(req CloseDriftReq, proof string) (*PwDriftCase, error) {
	  return svc.detector.ForceClose(req, proof)
  }

// ──────────────────────────────────────────────
// 查询 API
// ──────────────────────────────────────────────
// QueryGeneration 查询用户世代详情。
   func (svc *Service) QueryGeneration(userID string) (*GenQueryResp, error) {
	  return svc.gen.QueryResp(userID)
  }

// GetVerifyLease 按 ID 查询核验令牌（供他包只读引用）。
  func (svc *Service) GetVerifyLease(leaseID string) (*VerifyLease, error) {
	   return svc.mutex.GetLease(leaseID)
  }

// GetDriftCase 按 ID 查询漂移案件（供他包只读引用）。

  func (svc *Service) GetDriftCase(caseID string) (*PwDriftCase, error) {
	  return svc.detector.GetCase(caseID)
   }

// ListOpenDriftCases 列出全部未关闭漂移案件（Worker 巡检入口）。

  func (svc *Service) ListOpenDriftCases(limit int) ([]*PwDriftCase, error) {
	  return svc.detector.ListOpenCases(limit)
  }

// ListOpenDriftCasesByUser 列出指定用户未关闭漂移案件。
   func (svc *Service) ListOpenDriftCasesByUser(userID string, limit int) ([]*PwDriftCase, error) {
	   return svc.detector.ListOpenByUser(userID,
		  limit)
   }

// ListInflightLeases 列出指定用户 open 核验令牌（Worker 扫描入口）。
   func (svc *Service) ListInflightLeases(userID string, limit int) ([]*VerifyLease, error) {
	  return svc.mutex.ListOpenLeases(userID, limit)
  }

// InflightCount 返回用户当前在途令牌数量（监控用）。

   func (svc *Service) InflightCount(userID string) (int, error) {
	   return svc.mutex.InflightCount(userID)
  }
