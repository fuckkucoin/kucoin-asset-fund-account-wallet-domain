package isodebt

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

// GateAdjudicator 裁决 TransferGate 请求。

type GateAdjudicator struct {
	store    Store
	leaseMgr *LeaseManager
	mutex    *DebitTransferMutex
	clock func() time.Time
}

// NewGateAdjudicator 构造裁决器。
  func NewGateAdjudicator(store Store, leaseMgr *LeaseManager, mutex *DebitTransferMutex) *GateAdjudicator {
	  return &GateAdjudicator{
		  store:    store,
		  leaseMgr: leaseMgr,
		  mutex:    mutex,
		   clock: func() time.Time { return time.Now().UTC() },
	   }
  }

// WithClock 注入时钟。

   func (a *GateAdjudicator) WithClock(c func() time.Time) *GateAdjudicator {
	  if a != nil && c != nil {
		   a.clock = c
	  }
	  return a
   }

   // AdjudicateRequest 单次转入裁决请求。
   type AdjudicateRequest struct {
	  LeaseID  string          `json:"lease_id"`
	  UserID   string          `json:"user_id"`
	  Symbol   string          `json:"symbol"`
	   Currency string          `json:"currency"`
	   Amount   decimal.Decimal `json:"amount"`
	   RequestedBy string `json:"requested_by"`
	  // TransferID 转入幂等键（可选；空则自动生成）
	   TransferID string `json:"transfer_id,omitempty"`
  }

  // AdjudicateTransferIn 裁决一次转入请求。
   // 若租约无效（stale/blocked/expired/世代不符）或存在漂移案例 → rejected。
   // 若存在并发 Debit 互斥冲突 → ErrMutexConflict，fail-closed。
   func (a *GateAdjudicator) AdjudicateTransferIn(ctx context.Context, req AdjudicateRequest) (TransferGate, error) {
	  req.UserID = strings.TrimSpace(req.UserID)
	   req.Symbol = strings.ToUpper(strings.TrimSpace(req.Symbol))
	   req.Currency = strings.ToUpper(strings.TrimSpace(req.Currency))
	   if req.UserID == "" || req.Symbol == "" || req.LeaseID == "" {
		   return TransferGate{}, fmt.Errorf("%w: userID, symbol, leaseID required", ErrInvalidInput)
	  }
	  if req.Amount.IsNegative() || req.Amount.IsZero() {
		  return TransferGate{}, fmt.Errorf("%w: amount must be positive", ErrInvalidInput)
	   }
	   transferID := req.TransferID
	  if transferID == "" {
		  transferID = uuid.NewString()
	  }
	   now := a.clock()
	  // 互斥检查：是否有并发 Debit 持有锁
	   if a.mutex != nil {
		  if err := a.mutex.CheckTransferIn(req.UserID, req.Symbol, transferID); err != nil {
			   rec := MutexRecord{
				   ID: uuid.NewString(),
				   Kind:   MutexKindDebitVsTransfer,
				  UserID: req.UserID,
				   Symbol: req.Symbol,
				  TransferID: transferID,
				  DetectedAt: now,
				   Resolution: "conflict_rejected",
			   }
			  _ = a.store.SaveMutexRecord(ctx, rec)
			  return TransferGate{}, fmt.Errorf("%w: transfer=%s", ErrMutexConflict, transferID)
		   }
	   }
	  // 初始化 gate 记录（pending → adjudicated）
	   gate := TransferGate{
		   ID: uuid.NewString(),
		   LeaseID:  req.LeaseID,
		   UserID:   req.UserID,
		  Symbol:   req.Symbol,
		  Currency: req.Currency,
		  Amount:    req.Amount,
		   State:     GateStatePending,
		   CreatedAt: now,
		   RequestedBy: req.RequestedBy,
	  }
	  // 租约校验
	   _, err := a.leaseMgr.ValidateLease(ctx, req.LeaseID)
	  adjTime := a.clock()
	   gate.AdjudicatedAt = &adjTime
	   if err != nil {
		  gate.State = GateStateRejected
		  gate.RejectReason = err.Error()
		   _ = a.store.SaveGate(ctx, gate)
		  return gate, err
	  }
	   gate.State = GateStateAllowed
	   if err := a.store.SaveGate(ctx, gate); err != nil {
		   return TransferGate{}, fmt.Errorf("isodebt: save gate: %w", err)
	  }
	  return gate, nil
  }

// ─────────────────────────────────────────────
// DebitTransferMutex
// ─────────────────────────────────────────────
// DebitTransferMutex 实现借记↔转入并发互斥。
// 在 Debit 操作开始时调用 LockDebit，结束时调用 UnlockDebit。
// TransferIn 在 Adjudicate 前调用 CheckTransferIn；
// 若有活跃 Debit 锁，立即返回 ErrMutexConflict（fail-closed）。

type DebitTransferMutex struct {
	mu sync.Mutex
	active map[string]debitLock // key=PairKey
}

type debitLock struct {
	debitTxnID string
	lockedAt   time.Time
}

// NewDebitTransferMutex 构造互斥器。
  func NewDebitTransferMutex() *DebitTransferMutex {
	   return &DebitTransferMutex{active: map[string]debitLock{}}
  }

// LockDebit 在 Debit 开始时锁定 Pair（阻断同 Pair 的 TransferIn）。
// 返回 unlock 函数，调用者必须在 defer 中执行。
   func (mx *DebitTransferMutex) LockDebit(userID, symbol, debitTxnID string) (unlock func()) {
	  key := PairKey(userID, symbol)
	   mx.mu.Lock()
	  mx.active[key] = debitLock{debitTxnID: debitTxnID,
		   lockedAt: time.Now().UTC()}
	  mx.mu.Unlock()
	   return func() {
		  mx.mu.Lock()
		  delete(mx.active, key)
		  mx.mu.Unlock()
	   }
}

// CheckTransferIn 检查是否有活跃 Debit 锁；有则返回 ErrMutexConflict。
  func (mx *DebitTransferMutex) CheckTransferIn(userID, symbol, transferID string) error {
	  key := PairKey(userID, symbol)
	   mx.mu.Lock()
	   lock, ok := mx.active[key]
	   mx.mu.Unlock()
	  if ok {
		  return fmt.Errorf("%w: debit=%s vs transfer=%s", ErrMutexConflict, lock.debitTxnID, transferID)
	  }
	  return nil
   }

// ActiveDebits 返回当前活跃 Debit 锁的 PairKey 列表（用于监控）。
  func (mx *DebitTransferMutex) ActiveDebits() []string {
	   mx.mu.Lock()
	   defer mx.mu.Unlock()
	  keys := make([]string, 0, len(mx.active))
	  for k := range mx.active {
		  keys = append(keys, k)
	  }
	  return keys
  }
