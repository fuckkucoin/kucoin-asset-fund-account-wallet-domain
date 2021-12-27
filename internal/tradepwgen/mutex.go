package tradepwgen

import (
	"encoding/json"
	"fmt"
	"time"
)

// InflightMutex 在途核验令牌互斥引擎。
type InflightMutex struct {
	genStore   GenerationStore
	leaseStore LeaseStore
	driftStore DriftStore
	leaseTTL   time.Duration
	clock      func() time.Time
}

// NewInflightMutex 构造互斥引擎。
  func NewInflightMutex(
	  genStore GenerationStore,
	   leaseStore LeaseStore,
	  driftStore DriftStore,
	  leaseTTL time.Duration,
   ) *InflightMutex {
	   return &InflightMutex{
		  genStore:   genStore,
		   leaseStore: leaseStore,
		  driftStore: driftStore,
		   leaseTTL:   leaseTTL,
		  clock:      func() time.Time { return time.Now().UTC() },
	  }
  }

   func (m *InflightMutex) now() time.Time {
	   if m.clock != nil {
		  return m.clock()
	   }
	  return time.Now().UTC()
   }

// IssueLease 颁发核验令牌。
// 前置条件（由 GenerationManager.GuardIssueLease 保证，调用方须先调用）：
// 用户已有 active 世代且无 open 漂移案件。
// 颁发成功后将 verify_lease_id 写入世代 active_lease_id。

  func (m *InflightMutex) IssueLease(req IssueLeaseReq) (*VerifyLease, error) {
	   if req.UserID == "" {
		   return nil, ErrUserRequired
	  }
	   now := m.now()
	  g, err := m.genStore.Get(req.UserID)
	   if err != nil {
		  return nil, fmt.Errorf("tradepwgen: IssueLease.GetGen: %w", err)
	  }
	   if g.Generation == 0 {
		   return nil, fmt.Errorf("tradepwgen: IssueLease: trading password not set")
	  }
	  id, err := newID()
	   if err != nil {
		   return nil, fmt.Errorf("tradepwgen: IssueLease.newID: %w", err)
	  }
	   tok, err := newToken()
	  if err != nil {
		   return nil, fmt.Errorf("tradepwgen: IssueLease.newToken: %w", err)
	  }
	   var expiresAt time.Time
	  if req.TTL > 0 {
		   expiresAt = now.Add(req.TTL)
	  }
	  lease := &VerifyLease{
		  ID:     id,
		   UserID: req.UserID,
		  PwGen:  g.Generation,
		  Token:  tok,
		  Status: LeaseStatusOpen,
		  ExpiresAt: expiresAt,
		  CreatedAt: now,
		  UpdatedAt: now,
	   }
	   if err := m.leaseStore.Insert(lease); err != nil {
		  return nil, fmt.Errorf("tradepwgen: IssueLease.Insert: %w", err)
	  }
	  // 将活跃令牌 ID 写入世代记录，供出金路径快速索引。
	  if err := m.genStore.SetActiveLeaseID(req.UserID, lease.ID, now); err != nil {
		  return nil, fmt.Errorf("tradepwgen: IssueLease.SetActiveLeaseID: %w", err)
	   }
	  return lease, nil
   }

// ConsumeLease 消费核验令牌（出金提交时调用）。
// 校验链：
// 1. 令牌存在且属于 userID。
// 2. 令牌未过期。
// 3. 令牌未吊销/消费（终态检测）。
// 4. 令牌世代 == 账本当前世代（漂移检测）。
// 5. Token 字符串匹配。
// 发现世代漂移时产出 DriftEvent，由调用方（Service）交给 DriftDetector 创建案件。
  func (m *InflightMutex) ConsumeLease(req ConsumeLeaseReq) (*VerifyLease, *DriftEvent, error) {
	  if req.UserID == "" {
		   return nil, nil, ErrUserRequired
	  }
	   now := m.now()
	   lease, err := m.leaseStore.Get(req.LeaseID)
	   if err != nil {
		  return nil, nil, fmt.Errorf("tradepwgen: ConsumeLease.Get: %w", err)
	   }
	  if lease.UserID != req.UserID {
		  return nil, nil, ErrLeaseNotFound
	  }
	  if err := CheckLeaseNotExpired(lease, now); err != nil {
		   return nil, nil, err
	   }
	   if lease.Status == LeaseStatusRevoked {
		   return nil, nil, ErrLeaseRevoked
	   }
	   if lease.Status == LeaseStatusConsumed {
		  return nil, nil, ErrLeaseConsumed
	   }
	  if lease.Status != LeaseStatusOpen {
		   return nil, nil, fmt.Errorf("%w: lease %s status %s", ErrInvalidStatus, lease.ID, lease.Status)
	  }
	  if lease.Token != req.Token {
		   return nil, nil, ErrLeaseNotFound
	   }
	   // 读取账本当前世代，检查漂移。
	  g, err := m.genStore.Get(req.UserID)
	   if err != nil {
		   return nil, nil, fmt.Errorf("tradepwgen: ConsumeLease.GetGen: %w", err)
	   }
	   if lease.PwGen != g.Generation {
		   // 世代漂移：产出 DriftEvent，拒绝消费。
		   verifySnap, _ := json.Marshal(map[string]interface{}{
			  "lease_id": lease.ID,
			  "pw_gen":   lease.PwGen,
			   "created_at": lease.CreatedAt,
		  })
		  genSnap, _ := json.Marshal(map[string]interface{}{
			   "user_id":    g.UserID,
			  "generation": g.Generation,
			   "updated_at": g.UpdatedAt,
		  })
		   ev := &DriftEvent{
			   UserID: req.UserID,
			  LeaseID:   lease.ID,
			  VerifyGen: lease.PwGen,
			  CurrentGen:     g.Generation,
			   VerifySnapshot: string(verifySnap),
			   GenSnapshot:    string(genSnap),
		   }
		   return nil, ev, ErrLeaseGenMismatch
	  }
	   // 消费令牌。
	   if err := TransitLeaseConsume(lease, now); err != nil {
		   return nil, nil, err
	   }
	   if err := m.leaseStore.Update(lease); err != nil {
		   return nil, nil, fmt.Errorf("tradepwgen: ConsumeLease.Update: %w", err)
	   }
	   // 清空世代 active_lease_id。
	  _ = m.genStore.SetActiveLeaseID(req.UserID, "", now)
	   return lease, nil, nil
  }

// RevokeInflight 吊销用户全部在途核验令牌（Change/Reset 触发）。
// newGen 为已抬升的新世代号（BumpGeneration 返回值）。
// 吊销 PwGen < newGen 的全部 open 令牌，返回吊销数量。
  func (m *InflightMutex) RevokeInflight(req RevokeInflightReq) (int, error) {
	   if req.UserID == "" {
		   return 0, ErrUserRequired
	  }
	  now := m.now()
	  count, err := m.leaseStore.RevokeByUser(req.UserID, req.PwGen, now)
	   if err != nil {
		   return 0, fmt.Errorf("tradepwgen: RevokeInflight.RevokeByUser: %w", err)
	  }
	  // 清空世代 active_lease_id。
	  _ = m.genStore.SetActiveLeaseID(req.UserID, "", now)
	  return count, nil
   }

// InflightCount 返回用户当前 open 核验令牌数量（互斥检查，非原子）。
// 仅用于日志/监控；实际互斥由 RevokeInflight 原子完成。
  func (m *InflightMutex) InflightCount(userID string) (int, error) {
	  if userID == "" {
		  return 0, ErrUserRequired
	   }
	  return m.leaseStore.CountOpen(userID)
   }

// GetLease 按 ID 查询核验令牌（供他包只读引用）。
   func (m *InflightMutex) GetLease(leaseID string) (*VerifyLease, error) {
	  return m.leaseStore.Get(leaseID)
   }

// ListOpenLeases 列出指定用户全部 open 令牌（Worker 扫描入口）。
  func (m *InflightMutex) ListOpenLeases(userID string, limit int) ([]*VerifyLease, error) {
	   return m.leaseStore.ListOpenByUser(userID,
		  limit)
  }
