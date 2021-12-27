package wdpartial

import (
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// newGuardGen 内部创建 BroadcastGuardGen（OpenHold skip 腿专用，须在锁内调用）。

   func newGuardGen(planID, liveHash string, now time.Time) *BroadcastGuardGen {
	   return &BroadcastGuardGen{
		  ID: uuid.NewString(),
		   PlanID: planID,
		   LiveHash: liveHash,
		  Generation: 1,
		   Status:     GuardGenActive,
		   CreatedAt: now,
		   UpdatedAt: now,
	  }
  }

// InvalidateGuardGen 使守卫世代失效（active→invalidated）。
// live-hash 变更或广播主链 FSM 迁移时调用；须提供失效原因。

   func (s *Service) InvalidateGuardGen(req InvalidateGuardGenReq) (*BroadcastGuardGen, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	  }
	  if strings.TrimSpace(req.GuardGenID) == "" {
		  return nil, ErrGuardGenNotFound
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	  g, ok := s.remGuards[req.GuardGenID]
	  if !ok {
		   return nil, ErrGuardGenNotFound
	  }
	   if g.Status != GuardGenActive {
		   return nil, fmt.Errorf("%w: guard_gen %s is %s, expected active", ErrGuardGenInvalid, g.ID, g.Status)
	  }
	   now := s.now()
	   reason := strings.TrimSpace(req.Reason)
	   if reason == "" {
		  reason = "live-hash invalidated"
	   }
	   g.Status = GuardGenInvalidated
	   g.InvalidatedReason = reason
	   t := now
	  g.InvalidatedAt = &t
	  g.UpdatedAt = now
	   cp := *g
	   return &cp, nil
  }

// GetGuardGen 查询守卫世代（只读）。
   func (s *Service) GetGuardGen(id string) (*BroadcastGuardGen, error) {
	   if s == nil {
		  return nil, ErrUnavailable
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   g, ok := s.remGuards[id]
	   if !ok {
		   return nil, ErrGuardGenNotFound
	  }
	   cp := *g
	   return &cp, nil
  }

// ListActiveGuardGens 查询全部 active 守卫世代（Worker 巡检用）。
   func (s *Service) ListActiveGuardGens(limit int) []*BroadcastGuardGen {
	   if s == nil {
		   return nil
	  }
	   s.mu.Lock()
	  defer s.mu.Unlock()
	   var out []*BroadcastGuardGen
	   for _, g := range s.remGuards {
		   if g.Status == GuardGenActive {
			  cp := *g
			   out = append(out, &cp)
			   if limit > 0 && len(out) >= limit {
				   break
			  }
		   }
	  }
	  return out
  }

// checkGuardActiveLocked 检查守卫世代是否活跃（须在锁内调用）。
// invalidated → ErrGuardGenInvalid（禁止静默跳过）。

   func checkGuardActiveLocked(g *BroadcastGuardGen) error {
	   if g == nil {
		   return ErrGuardGenNotFound
	   }
	   switch g.Status {
	   case GuardGenActive:
		  return nil
	   case GuardGenInvalidated:
		  return ErrGuardGenInvalid
	   case GuardGenConsumed:
		  return ErrGuardGenConsumed
	  default:
		   return fmt.Errorf("%w: unknown status %s", ErrGuardGenInvalid, g.Status)
	  }
   }
