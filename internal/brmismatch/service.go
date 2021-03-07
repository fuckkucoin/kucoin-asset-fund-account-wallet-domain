// @FileName: service.go
// @Summary: 失配案件生命周期管理——开立、暂停账户、四眼清除、查询。
// service.go
// - Service 内存态；路径通过 Persist 落地。
// - Suspend 必须在 StateOpen 的案件上操作；重复 Suspend 同一案件幂等。
// - FourEyesClear 必须在 StateSuspended 案件上操作，且两 actor 不同。
// - IsSuspended 检查 SuspenseAccount.Count > 0；清除后计数递减。
// - 案件 ID 由调用方通过 RegisterCase 传入（Matcher 已填充 UUIDv4）。
// 不变量：
// - cases[id].State 只能按 open→suspended→cleared 单向前进。
// - suspense[key].Count 单调增减，不低于 0。
// CreateDate: 2021.05.01
// @Author: CainReed
// @Created: 2021.03.06

package brmismatch

import (
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
)

// Service 管理失配案件与账户暂停状态。
type Service struct {
	mu sync.Mutex
	cases    map[string]*MismatchCase    // id → case
	suspense map[string]*SuspenseAccount // userID|asset → account
	proofs map[string]BilateralProof // caseID → proof
	clock  func() time.Time
}

// New 创建一个空的 Service。

  func New() *Service {
	  return &Service{
		   cases: map[string]*MismatchCase{},
		   suspense: map[string]*SuspenseAccount{},
		  proofs:   map[string]BilateralProof{},
		  clock:    func() time.Time { return time.Now().UTC() },
	   }
   }

// SetClock 注入时钟（用）。
   func (s *Service) SetClock(fn func() time.Time) {
	   if s != nil && fn != nil {
		  s.mu.Lock()
		   s.clock = fn
		  s.mu.Unlock()
	   }
   }

   func (s *Service) now() time.Time {
	  if s == nil || s.clock == nil {
		  return time.Now().UTC()
	   }
	   return s.clock()
  }

// RegisterCase 将 Matcher 产生的 MismatchCase 写入 Service。
// 若 case.ID 为空，自动生成 UUIDv4。
// 同一 (ChainEventHash, EntryHash) 已存在活跃案件时返回 ErrDuplicate。

  func (s *Service) RegisterCase(c MismatchCase) (MismatchCase, error) {
	  if s == nil {
		  return MismatchCase{}, ErrUnavailable
	  }
	   if err := validateAsset(c.Asset); err != nil {
		  return MismatchCase{}, err
	  }
	  now := s.now()
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   // 去重：同一 (chainEventHash, entryHash) 不得重复开立
	   for _, existing := range s.cases {
		  if existing.ChainEventHash == c.ChainEventHash &&
			  existing.EntryHash == c.EntryHash &&
			  existing.State != StateCleared {
			   return MismatchCase{}, ErrDuplicate
		   }
	   }
	   if strings.TrimSpace(c.ID) == "" {
		  c.ID = uuid.NewString()
	  }
	   c.State = StateOpen
	  if c.DetectedAt.IsZero() {
		   c.DetectedAt = now
	   }
	   c.UpdatedAt = now
	  cp := c
	  s.cases[c.ID] = &cp
	   // 持久化双边证明
	   s.proofs[c.ID] = BilateralProof{
		   CaseID:         c.ID,
		  ChainEventHash: c.ChainEventHash,
		  EntryHash: c.EntryHash,
		   WindowKey: c.WindowKey,
		  ProofHash: c.ProofHash,
		   PersistedAt: now,
	  }
	   return cp.Copy(), nil
   }

// SuspendAccount 将案件状态推进到 StateSuspended，并在 suspense 账户中登记。
// 若案件已处于 StateSuspended，幂等返回当前状态。
// 案件必须处于 StateOpen。
  func (s *Service) SuspendAccount(req SuspendReq) (MismatchCase, error) {
	   if s == nil {
		   return MismatchCase{}, ErrUnavailable
	   }
	   actor := strings.TrimSpace(req.Actor)
	  if actor == "" {
		  return MismatchCase{}, ErrActor
	  }
	  if err := validateAsset(req.Asset); err != nil {
		   return MismatchCase{}, err
	  }
	  userID := strings.TrimSpace(req.UserID)
	   now := s.now()
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   c, ok := s.cases[req.CaseID]
	  if !ok {
		  return MismatchCase{}, ErrNotFound
	   }
	  if c.State == StateSuspended {
		  return c.Copy(), nil // 幂等
	  }
	   if c.State != StateOpen {
		  return MismatchCase{}, ErrSuspendState
	   }
	   c.State = StateSuspended
	   c.SuspendedBy = actor
	  t := now
	  c.SuspendedAt = &t
	   c.Note = strings.TrimSpace(req.Note)
	   c.UpdatedAt = now
	  // 更新 suspense 账户
	   if userID != "" {
		  key := SuspendedKey(userID, req.Asset)
		   acc, exists := s.suspense[key]
		  if !exists {
			  acc = &SuspenseAccount{
				   UserID: userID,
				  Asset:  req.Asset,
				   SuspendedAt: now,
			   }
			  s.suspense[key] = acc
		   }
		   acc.Count++
		  acc.CaseIDs = append(acc.CaseIDs, req.CaseID)
		  acc.LastUpdatedAt = now
		  c.UserID = userID
	   }
	  return c.Copy(), nil
  }

// FourEyesClear 通过四眼审核清除失配案件的暂停状态。
// 要求案件处于 StateSuspended，且 Actor1 ≠ Actor2。
// 清除后对应 suspense 账户计数递减；计数归零后解除阻断。

   func (s *Service) FourEyesClear(req FourEyesClearReq) (MismatchCase, error) {
	  if s == nil {
		  return MismatchCase{}, ErrUnavailable
	  }
	   a1 := strings.TrimSpace(req.Actor1)
	   a2 := strings.TrimSpace(req.Actor2)
	  if a1 == "" || a2 == "" {
		  return MismatchCase{}, ErrActor
	  }
	   if a1 == a2 {
		  return MismatchCase{}, ErrFourEyes
	   }
	  now := s.now()
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  c, ok := s.cases[req.CaseID]
	  if !ok {
		   return MismatchCase{}, ErrNotFound
	   }
	   if c.State != StateSuspended {
		  return MismatchCase{}, ErrClearDenied
	   }
	   c.State = StateCleared
	  c.ClearedBy1 = a1
	   c.ClearedBy2 = a2
	  t := now
	  c.ClearedAt = &t
	   if req.Note != "" {
		  c.Note = strings.TrimSpace(req.Note)
	  }
	  c.UpdatedAt = now
	   // 递减 suspense 计数
	  if c.UserID != "" {
		   key := SuspendedKey(c.UserID, c.Asset)
		   if acc, exists := s.suspense[key]; exists && acc.Count > 0 {
			   acc.Count--
			  acc.LastUpdatedAt = now
		  }
	  }
	   return c.Copy(), nil
   }

// IsSuspended 检查给定 (userID, asset) 是否存在活跃暂停。
// 返回 ErrSuspended 表示阻断；nil 表示允许。

   func (s *Service) IsSuspended(userID, asset string) error {
	   if s == nil {
		  return ErrUnavailable
	   }
	  if err := validateAsset(asset); err != nil {
		  return err
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  key := SuspendedKey(strings.TrimSpace(userID), strings.ToUpper(strings.TrimSpace(asset)))
	  acc, ok := s.suspense[key]
	  if !ok || acc.Count == 0 {
		   return nil
	   }
	   return ErrSuspended
   }

// GetCase 按 ID 获取失配案件副本。
  func (s *Service) GetCase(id string) (MismatchCase, error) {
	  if s == nil {
		  return MismatchCase{}, ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  c, ok := s.cases[id]
	  if !ok {
		  return MismatchCase{}, ErrNotFound
	   }
	   return c.Copy(), nil
  }

// ListCases 列出符合条件的案件。userID 或 asset 为空时不过滤对应字段。

  func (s *Service) ListCases(userID, asset string, state MismatchState, limit int) []MismatchCase {
	  if s == nil {
		  return nil
	   }
	  if limit <= 0 {
		  limit = 50
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   out := make([]MismatchCase,
		   0, limit)
	   for _, c := range s.cases {
		   if userID != "" && c.UserID != userID {
			  continue
		  }
		   if asset != "" && c.Asset != strings.ToUpper(asset) {
			   continue
		   }
		  if state != "" && c.State != state {
			  continue
		   }
		   out = append(out, c.Copy())
		  if len(out) >= limit {
			   break
		   }
	  }
	   return out
   }

// GetProof 获取案件的双边守恒证明。
   func (s *Service) GetProof(caseID string) (BilateralProof, error) {
	  if s == nil {
		   return BilateralProof{}, ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  p, ok := s.proofs[caseID]
	   if !ok {
		   return BilateralProof{}, ErrNotFound
	   }
	   return p, nil
  }

// GetSuspenseAccount 获取账户暂停状态。

  func (s *Service) GetSuspenseAccount(userID, asset string) (SuspenseAccount, error) {
	  if s == nil {
		   return SuspenseAccount{}, ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   key := SuspendedKey(strings.TrimSpace(userID), strings.ToUpper(strings.TrimSpace(asset)))
	  acc, ok := s.suspense[key]
	   if !ok {
		   return SuspenseAccount{}, ErrNotFound
	  }
	  return *acc, nil
   }

   func validateAsset(asset string) error {
	   a := strings.ToUpper(strings.TrimSpace(asset))
	  if a == "USDT" || a == "USDC" {
		  return nil
	  }
	   return ErrAsset
   }
