package wdpartial

import (
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// IssueLease 颁发 RemReleaseLease。
// 同 planID 已存在 open 租约时返回 ErrLeaseDuplicate。
   func (s *Service) IssueLease(req IssueLeaseReq) (*RemReleaseLease, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	   }
	  if strings.TrimSpace(req.PlanID) == "" {
		  return nil, ErrNoPlan
	  }
	   ttl := req.TTL
	   if ttl <= 0 {
		  ttl = 5 * time.Minute
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   now := s.now()
	   // 防止同 planID 并发双颁。
	   for _, l := range s.remLeases {
		   if l.PlanID == req.PlanID && l.Status == LeaseStatusOpen {
			  return nil, ErrLeaseDuplicate
		  }
	  }
	  l := &RemReleaseLease{
		  ID:     uuid.NewString(),
		   PlanID: strings.TrimSpace(req.PlanID),
		  UserID: strings.TrimSpace(req.UserID),
		   Generation: 1,
		  Status:     LeaseStatusOpen,
		  ExpiresAt: now.Add(ttl),
		   CreatedAt: now,
		   UpdatedAt: now,
	  }
	  s.remLeases[l.ID] = l
	   return l, nil
   }

// GetLease 查询租约（只读）。

   func (s *Service) GetLease(id string) (*RemReleaseLease, error) {
	  if s == nil {
		  return nil, ErrUnavailable
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  l, ok := s.remLeases[id]
	   if !ok {
		   return nil, ErrLeaseNotFound
	  }
	  cp := *l
	  return &cp, nil
   }

// ExportRemReleaseGen 导出当前 rem_release_gen（供他包只读引用）。
// 返回指定 planID 最新 open 租约的 Generation；不存在时返回 0。
   func (s *Service) ExportRemReleaseGen(planID string) (int64, error) {
	   if s == nil {
		   return 0, ErrUnavailable
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  for _, l := range s.remLeases {
		  if l.PlanID == planID && l.Status == LeaseStatusOpen {
			   return l.Generation, nil
		  }
	   }
	   return 0, ErrLeaseNotFound
  }

// RevokeExpiredLeases 吊销过期但未终态的租约（Worker 定期调用）。

  func (s *Service) RevokeExpiredLeases() []string {
	   if s == nil {
		  return nil
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   now := s.now()
	  var revokedIDs []string
	   for _, l := range s.remLeases {
		   if l.Status.IsTerminal() {
			   continue
		   }
		  if !l.ExpiresAt.IsZero() && now.After(l.ExpiresAt) {
			   l.Status = LeaseStatusRevoked
			  t := now
			  l.RevokedAt = &t
			   l.UpdatedAt = now
			   revokedIDs = append(revokedIDs, l.ID)
		  }
	  }
	   return revokedIDs
  }

// ──────────────────────────────────────────────
// FillMutexToken 管理
// ──────────────────────────────────────────────
// IssueFillMutex 颁发 FillMutexToken（ConfirmFill 路径调用）。
// 颁发后同 planID 的 RemReleaseLease 将被阻塞（hasActiveFillMutexLocked 检测）。
   func (s *Service) IssueFillMutex(req IssueFillMutexReq) (*FillMutexToken, error) {
	   if s == nil {
		   return nil, ErrUnavailable
	   }
	  ttl := req.TTL
	   if ttl <= 0 {
		   ttl = 2 * time.Minute
	   }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  now := s.now()
	  t := &FillMutexToken{
		  ID: uuid.NewString(),
		  PlanID: strings.TrimSpace(req.PlanID),
		   FillID:     strings.TrimSpace(req.FillID),
		  Generation: 1,
		   Token:      uuid.NewString(),
		  IssuedAt:  now,
		  ExpiresAt: now.Add(ttl),
	   }
	  s.tokens[t.ID] = t
	   return t, nil
   }

// ConsumeFillMutex 消费 FillMutexToken（ConsumeHold 完成时调用）。
// 消费后同计划下 blocked 的 RemReleaseLease 自动解除 blocked 状态。

   func (s *Service) ConsumeFillMutex(tokenID string) error {
	   if s == nil {
		  return ErrUnavailable
	  }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	  t, ok := s.tokens[tokenID]
	  if !ok {
		  return ErrFillMutexNotFound
	  }
	   if t.ConsumedAt != nil {
		  return ErrFillMutexConsumed
	  }
	   now := s.now()
	   t.ConsumedAt = &now
	   // 解除同 planID 下 blocked 状态的租约。
	   for _, l := range s.remLeases {
		  if l.PlanID == t.PlanID && l.Status == LeaseStatusBlocked {
			   l.Status = LeaseStatusOpen
			  l.BlockedByFillID = ""
			   l.UpdatedAt = now
		  }
	   }
	   return nil
  }

// ValidateFillMutex 核验 FillMutexToken 令牌（按 tokenID 查找，校验 planID/fillID）。

   func (s *Service) ValidateFillMutex(tokenID, planID, fillID string) (*FillMutexToken, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	   now := s.now()
	  t, ok := s.tokens[tokenID]
	   if !ok {
		  return nil, ErrFillMutexNotFound
	  }
	   if t.PlanID != planID || t.FillID != fillID {
		  return nil, ErrFillMutexMismatch
	   }
	   if t.ConsumedAt != nil {
		   return nil, ErrFillMutexConsumed
	   }
	  if !t.IsValid(now) {
		  return nil, ErrFillMutexExpired
	   }
	  cp := *t
	   return &cp, nil
  }

// hasActiveFillMutexLocked 检查 planID 是否有活跃的 FillMutexToken（已持锁调用）。

  func (s *Service) hasActiveFillMutexLocked(planID string, now time.Time) bool {
	   for _, t := range s.tokens {
		   if t.PlanID == planID && t.IsValid(now) {
			  return true
		  }
	  }
	  return false
  }

// ──────────────────────────────────────────────
// 内部 FSM 辅助（已持锁）
// ──────────────────────────────────────────────
// checkLeaseActiveLocked 检查租约是否活跃（open 且未过期），须在锁内调用。

  func checkLeaseActiveLocked(l *RemReleaseLease, now time.Time) error {
	  if l == nil {
		  return ErrLeaseNotFound
	  }
	   if l.Status != LeaseStatusOpen {
		  return fmt.Errorf("%w: status=%s", ErrLeaseNotOpen, l.Status)
	  }
	   if !l.ExpiresAt.IsZero() && now.After(l.ExpiresAt) {
		   return ErrLeaseExpired
	   }
	  return nil
   }
