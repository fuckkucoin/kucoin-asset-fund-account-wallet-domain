package wrongcredit

import (
	"sync"
	"time"
)

// ClaimKind 互斥锁归属类别。
type ClaimKind string

const (
	// ClaimKindReorg 由 reorg-reversal 批次持有。
	ClaimKindReorg ClaimKind = "reorg"
	// ClaimKindWrongCredit 由 wrong-credit 案件持有。
	ClaimKindWrongCredit ClaimKind = "wrong_credit"
)

// creditClaim 单条锁记录。
type creditClaim struct {
	CreditRef string `json:"credit_ref"`
	Kind ClaimKind `json:"kind"`
	Owner     string    `json:"owner"` // 持有者 ID（reorg batchID 或 caseID）
	ClaimedAt time.Time `json:"claimed_at"`
}

// CreditMutex 管理所有活跃 credit_ref 的互斥持有记录；进程内单例。

type CreditMutex struct {
	mu    sync.Mutex
	holds map[string]*creditClaim // key = credit_ref
}

// NewCreditMutex 构造新的互斥锁管理器。
  func NewCreditMutex() *CreditMutex {
	   return &CreditMutex{
		   holds: map[string]*creditClaim{},
	  }
  }

// ClaimWrongCredit wrong-credit 编排器在进入 freeze 步骤前锁定 credit_ref。
// 若该 credit_ref 已被 reorg 批次锁定，返回 ErrMutexLocked。
// 若已被另一个 wrong-credit 案件锁定（owner 不同），返回 ErrMutexConflict。
// 若已被同一 caseID 锁定（幂等），返回 nil。
   func (m *CreditMutex) ClaimWrongCredit(creditRef, caseID string) error {
	  if m == nil {
		   return ErrUnavailable
	  }
	  m.mu.Lock()
	   defer m.mu.Unlock()
	  if cur, ok := m.holds[creditRef]; ok {
		  switch cur.Kind {
		   case ClaimKindReorg:
			   return ErrMutexLocked
		   case ClaimKindWrongCredit:
			  if cur.Owner != caseID {
				   return ErrMutexConflict
			   }
			   // 幂等：相同 caseID 已持有。
			  return nil
		   }
	  }
	   m.holds[creditRef] = &creditClaim{
		   CreditRef: creditRef,
		  Kind: ClaimKindWrongCredit,
		   Owner: caseID,
		  ClaimedAt: time.Now().UTC(),
	  }
	  return nil
  }

// ClaimReorg reorg-reversal 批次在执行 reorg unwind 前锁定 credit_ref。
// 若该 credit_ref 已被 wrong-credit 案件锁定，返回 ErrMutexConflict。
// 若已被同一 batchID 锁定（幂等），返回 nil。
   func (m *CreditMutex) ClaimReorg(creditRef, batchID string) error {
	  if m == nil {
		  return ErrUnavailable
	  }
	  m.mu.Lock()
	  defer m.mu.Unlock()
	  if cur, ok := m.holds[creditRef]; ok {
		  switch cur.Kind {
		   case ClaimKindWrongCredit:
			  return ErrMutexConflict
		  case ClaimKindReorg:
			   if cur.Owner != batchID {
				   // 不同批次同时 claim 同一 credit_ref。
				   return ErrMutexConflict
			  }
			   return nil
		  }
	   }
	  m.holds[creditRef] = &creditClaim{
		   CreditRef: creditRef,
		  Kind:      ClaimKindReorg,
		  Owner: batchID,
		  ClaimedAt: time.Now().UTC(),
	  }
	   return nil
  }

// Release 释放 credit_ref 的持有锁；owner 不匹配时静默忽略（防止误释放）。
   func (m *CreditMutex) Release(creditRef, owner string) {
	   if m == nil {
		   return
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   cur, ok := m.holds[creditRef]
	  if !ok {
		   return
	  }
	   if cur.Owner != owner {
		  return
	   }
	   delete(m.holds, creditRef)
  }

// Holder 返回 credit_ref 的当前持有记录；不存在时返回 nil。
// 主要用于 debug 和审计查询，不作为业务判断依据（须持锁后查询）。

  func (m *CreditMutex) Holder(creditRef string) *creditClaim {
	   if m == nil {
		   return nil
	   }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	   cur, ok := m.holds[creditRef]
	   if !ok {
		  return nil
	   }
	   cp := *cur
	   return &cp
  }

// IsLockedByReorg 返回 credit_ref 是否被 reorg 批次锁定。
  func (m *CreditMutex) IsLockedByReorg(creditRef string) bool {
	  if m == nil {
		  return false
	   }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  cur, ok := m.holds[creditRef]
	   return ok && cur.Kind == ClaimKindReorg
   }

// IsLockedByWrongCredit 返回 credit_ref 是否被 wrong-credit 案件锁定。

  func (m *CreditMutex) IsLockedByWrongCredit(creditRef string) bool {
	   if m == nil {
		   return false
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	  cur, ok := m.holds[creditRef]
	   return ok && cur.Kind == ClaimKindWrongCredit
   }

// Snapshot 返回所有活跃锁记录的快照（只读副本）。供运营查询和监控用。

  func (m *CreditMutex) Snapshot() []creditClaim {
	  if m == nil {
		   return nil
	  }
	   m.mu.Lock()
	   defer m.mu.Unlock()
	   out := make([]creditClaim, 0, len(m.holds))
	   for _, v := range m.holds {
		  out = append(out, *v)
	  }
	   return out
  }

// Len 返回当前持有锁数量。

   func (m *CreditMutex) Len() int {
	   if m == nil {
		   return 0
	  }
	   m.mu.Lock()
	  defer m.mu.Unlock()
	  return len(m.holds)
  }
