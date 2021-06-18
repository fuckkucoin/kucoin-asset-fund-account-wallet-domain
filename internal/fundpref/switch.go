package fundpref

import (
	"context"
	"fmt"
	"strings"
	"time"
	"github.com/google/uuid"
)

// SwitchManager 管理 DefaultAccount 切换，含在途贷记互斥。
type SwitchManager struct {
	store SwitchStore
	genMgr   *GenerationManager
	inflight InflightChecker
	clock func() time.Time
}

// SwitchStore 切换持久化接口。
type SwitchStore interface {
	GetDefaultPref(ctx context.Context, userID string) (DefaultAccountPref, error)
	SaveDefaultPref(ctx context.Context, p DefaultAccountPref) error
	SavePendingSwitch(ctx context.Context, ps PendingSwitch) error
	GetPendingSwitch(ctx context.Context, id string) (PendingSwitch, error)
	ListPendingSwitches(ctx context.Context, userID string, state PendingSwitchState) ([]PendingSwitch, error)
	UpdatePendingSwitch(ctx context.Context, ps PendingSwitch) error
}

// InflightChecker 检查用户是否存在在途贷记（只读钩子）。
type InflightChecker interface {
	HasInflightCredit(ctx context.Context, userID string) (bool, error)
}

// NewSwitchManager 构造切换管理器。
   func NewSwitchManager(store SwitchStore, genMgr *GenerationManager, inflight InflightChecker) *SwitchManager {
	  return &SwitchManager{
		  store:    store,
		   genMgr:   genMgr,
		  inflight: inflight,
		   clock: func() time.Time { return time.Now().UTC() },
	  }
   }

// WithClock 注入时钟。

  func (m *SwitchManager) WithClock(c func() time.Time) *SwitchManager {
	   if m != nil && c != nil {
		  m.clock = c
	  }
	   return m
  }

  // SwitchResult Put DefaultAccount 的切换结果。
   type SwitchResult struct {
	   Generation PrefGeneration
	  Applied       bool           // true=立即生效；false=进入队列
	  PendingSwitch *PendingSwitch // 非 nil 时表示已进入队列
   }

   // PutDefaultAccount 切换 DefaultAccount。
   // 若存在在途贷记：EnqueueOnInflight=true 则进入 pending 队列，否则返回 ErrInflightCredit。
   func (m *SwitchManager) PutDefaultAccount(ctx context.Context, req PutDefaultAccountReq, enqueueOnInflight bool) (SwitchResult, error) {
	   if m == nil {
		  return SwitchResult{}, fmt.Errorf("%w: switch manager unavailable", ErrInvalidInput)
	  }
	   userID := strings.TrimSpace(req.UserID)
	  if userID == "" {
		   return SwitchResult{}, fmt.Errorf("%w: userID required", ErrInvalidInput)
	  }
	   if req.Book == "" {
		  return SwitchResult{}, fmt.Errorf("%w: book required", ErrInvalidInput)
	   }
	  if _, err := ParseBook(string(req.Book)); err != nil {
		  return SwitchResult{}, err
	  }
	  // 检查在途贷记
	   if m.inflight != nil {
		   has, err := m.inflight.HasInflightCredit(ctx, userID)
		   if err != nil {
			   return SwitchResult{}, fmt.Errorf("inflight check: %w", err)
		  }
		   if has {
			   if !enqueueOnInflight {
				  return SwitchResult{}, ErrInflightCredit
			   }
			  return m.enqueuePendingSwitch(ctx, req)
		  }
	  }
	   return m.applySwitch(ctx, req)
   }

// applySwitch 立即应用切换并递增世代。
  func (m *SwitchManager) applySwitch(ctx context.Context, req PutDefaultAccountReq) (SwitchResult, error) {
	   now := m.clock()
	  // 获取当前偏好（用于 from 记录）
	   cur, _ := m.store.GetDefaultPref(ctx, req.UserID)
	  // 递增世代
	  gen, err := m.genMgr.Bump(ctx, req.UserID, req.Book, "put_default_account", req.ReqID)
	   if err != nil {
		   return SwitchResult{}, fmt.Errorf("bump generation: %w", err)
	  }
	  // 保存 DefaultAccountPref
	   pref := DefaultAccountPref{
		  UserID: req.UserID,
		   Book:   req.Book,
		   Version:   gen.Generation,
		   UpdatedAt: now,
		  UpdatedBy: req.RequestedBy,
	   }
	   _ = cur
	  if err := m.store.SaveDefaultPref(ctx, pref); err != nil {
		  return SwitchResult{}, fmt.Errorf("save default pref: %w", err)
	  }
	  return SwitchResult{
		  Generation: gen,
		  Applied: true,
	   }, nil
   }

// enqueuePendingSwitch 进入待切换队列。
   func (m *SwitchManager) enqueuePendingSwitch(ctx context.Context, req PutDefaultAccountReq) (SwitchResult, error) {
	   now := m.clock()
	   cur, _ := m.store.GetDefaultPref(ctx, req.UserID)
	   ps := PendingSwitch{
		  ID:     uuid.NewString(),
		  UserID: req.UserID,
		   FromBook: cur.Book,
		   ToBook:      req.Book,
		  State:       PendingSwitchQueued,
		  RequestedBy: req.RequestedBy,
		  RequestedAt: now,
		  FenceEpoch:  0,
		   FenceToken:  uuid.NewString(),
	   }
	  if err := m.store.SavePendingSwitch(ctx, ps); err != nil {
		  return SwitchResult{}, fmt.Errorf("save pending switch: %w", err)
	   }
	   return SwitchResult{
		  Applied: false,
		  PendingSwitch: &ps,
	  }, nil
   }

// AdvancePendingSwitch 推进待切换队列（worker 调用）。
// 当用户无在途贷记时，将 queued 项应用。

   func (m *SwitchManager) AdvancePendingSwitch(ctx context.Context, userID string) ([]PendingSwitch, error) {
	   if m == nil {
		   return nil, fmt.Errorf("%w: switch manager unavailable", ErrInvalidInput)
	   }
	   userID = strings.TrimSpace(userID)
	   if userID == "" {
		   return nil, fmt.Errorf("%w: userID required", ErrInvalidInput)
	   }
	   // 检查在途贷记
	   if m.inflight != nil {
		   has,
			   err := m.inflight.HasInflightCredit(ctx, userID)
		  if err != nil {
			  return nil, err
		  }
		  if has {
			  return nil, nil // 仍有在途贷记，接着等待
		  }
	  }
	  items, err := m.store.ListPendingSwitches(ctx, userID, PendingSwitchQueued)
	  if err != nil {
		  return nil, err
	   }
	  applied := make([]PendingSwitch, 0, len(items))
	  now := m.clock()
	  for _, ps := range items {
		  req := PutDefaultAccountReq{
			  UserID: userID,
			  Book:        ps.ToBook,
			  RequestedBy: ps.RequestedBy,
			   ReqID:       ps.ID,
		  }
		   if _, err := m.applySwitch(ctx, req); err != nil {
			  continue
		   }
		   ps.State = PendingSwitchApplied
		   ps.AppliedAt = &now
		   if err := m.store.UpdatePendingSwitch(ctx, ps); err != nil {
			  continue
		  }
		  applied = append(applied, ps)
	   }
	   return applied, nil
  }
