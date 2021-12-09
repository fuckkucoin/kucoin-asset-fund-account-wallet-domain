package tradepwgen

import (
	"fmt"
	"time"
)

// GenerationManager 密码世代管理器。

type GenerationManager struct {
	genStore GenerationStore
	driftStore DriftStore
	clock func() time.Time
}

// NewGenerationManager 构造管理器（模式注入 SQL 实现）。
   func NewGenerationManager(genStore GenerationStore, driftStore DriftStore) *GenerationManager {
	   return &GenerationManager{
		  genStore:   genStore,
		   driftStore: driftStore,
		   clock:      func() time.Time { return time.Now().UTC() },
	  }
  }

   func (m *GenerationManager) now() time.Time {
	   if m.clock != nil {
		   return m.clock()
	  }
	  return time.Now().UTC()
  }

// Current 读取用户当前世代。
// 用户尚未设置交易密码时 Generation=0。

  func (m *GenerationManager) Current(userID string) (*PwGeneration, error) {
	  if userID == "" {
		  return nil, ErrUserRequired
	  }
	  return m.genStore.Get(userID)
   }

// BumpGeneration 抬升世代（Set/Change/Reset 核心写路径）。
// 参数：
// userID 目标用户
// expectedGen 调用方读到的当前世代号（首次设密传 0）
// reason 变更原因（set / change / reset）
// 返回新世代记录；CAS 冲突时返回 ErrGenConflict。
   func (m *GenerationManager) BumpGeneration(userID string, expectedGen int64, reason string) (*PwGeneration, error) {
	  if userID == "" {
		   return nil, ErrUserRequired
	   }
	  now := m.now()
	   newGen := expectedGen + 1
	   ng, err := m.genStore.BumpGeneration(userID, expectedGen, newGen, reason, now)
	   if err != nil {
		   return nil, fmt.Errorf("tradepwgen: BumpGeneration: %w", err)
	   }
	  return ng, nil
   }

// ConfirmCool 确认 markCool 已生效（由 Worker 或同步路径调用）。
// gen 必须与当前世代一致，否则返回 ErrGenConflict。

  func (m *GenerationManager) ConfirmCool(userID string, gen int64) error {
	   if userID == "" {
		   return ErrUserRequired
	   }
	   return m.genStore.SetCoolBound(userID,
		   gen, m.now())
   }

// SetActiveLeaseID 更新世代记录中的活跃核验令牌 ID。
// 颁发令牌后调用（leaseID 非空）；令牌消费/吊销后置空（leaseID=""）。
  func (m *GenerationManager) SetActiveLeaseID(userID, leaseID string) error {
	  if userID == "" {
		  return ErrUserRequired
	  }
	   return m.genStore.SetActiveLeaseID(userID, leaseID, m.now())
   }

// GuardIssueLease 颁发核验令牌前置校验。
// 1. 用户须已有世代（Generation>0）。
// 2. 不得存在 open 漂移案件。
   func (m *GenerationManager) GuardIssueLease(userID string) error {
	  if userID == "" {
		  return ErrUserRequired
	   }
	   g, err := m.genStore.Get(userID)
	  if err != nil {
		   return fmt.Errorf("tradepwgen: GuardIssueLease.Get: %w", err)
	  }
	   if g.Generation == 0 {
		  return fmt.Errorf("tradepwgen: GuardIssueLease: trading password not set for user %s", userID)
	  }
	  open, err := m.driftStore.HasOpen(userID)
	  if err != nil {
		   return fmt.Errorf("tradepwgen: GuardIssueLease.HasOpenDrift: %w", err)
	  }
	  if open {
		  return ErrDriftOpen
	  }
	  return nil
   }

// QueryResp 组装世代查询响应。
  func (m *GenerationManager) QueryResp(userID string) (*GenQueryResp, error) {
	  if userID == "" {
		   return nil, ErrUserRequired
	  }
	  g, err := m.genStore.Get(userID)
	  if err != nil {
		   return nil, fmt.Errorf("tradepwgen: QueryResp.Get: %w", err)
	   }
	  open, err := m.driftStore.HasOpen(userID)
	   if err != nil {
		  return nil, fmt.Errorf("tradepwgen: QueryResp.HasOpenDrift: %w", err)
	   }
	   return &GenQueryResp{
		  UserID:     userID,
		  Generation: g.Generation,
		  Status:     g.Status,
		  CoolBound:  g.CoolBound,
		   HasOpenDrift: open,
		   ActiveLeaseID: g.ActiveLeaseID,
		   UpdatedAt:     g.UpdatedAt,
	   }, nil
  }
