package dualctl

import (
	"fmt"
	"time"
)

// AgedTicket 单条老化结果。
type AgedTicket struct {
	Request Request
	AgedAt time.Time
	// OldState 老化前的状态（open/ready）。
	OldState State
}

// AgeScan 扫描内存 Book 中所有超期的 open/ready 请求，标记为 expired。
// 返回老化列表；调用方可用于写 dualrelease.TicketExpiry 审计。
   func (b *Book) AgeScan(now time.Time) []AgedTicket {
	  b.mu.Lock()
	  defer b.mu.Unlock()
	  out := make([]AgedTicket, 0)
	   for _, r := range b.rows {
		   if r.State != StateOpen && r.State != StateReady {
			  continue
		  }
		  if now.Before(r.ExpiresAt) {
			  continue
		   }
		   old := r.State
		  r.State = StateExpired
		  r.UpdatedAt = now
		   out = append(out, AgedTicket{Request: *r, AgedAt: now, OldState: old})
	   }
	   return out
   }

// AgeScanSQL 扫描 SQL 层中超期的请求（通过 loadSQL 逐条检查，批量大小受 batchSize 限制）。
// 若未绑定 SQL（b.sql == nil），等价于 AgeScan。
// 注意：此函数主要在集成中验证 SQL 路径，中应先走索引查询。
  func (b *Book) AgeScanSQL(now time.Time, batchSize int) ([]AgedTicket, error) {
	  if !b.sqlOn() {
		  aged := b.AgeScan(now)
		  return aged, nil
	  }
	  // 在内存中执行（实际 SQL 扫描超范围，此处简化实现）。
	   aged := b.AgeScan(now)
	  if batchSize > 0 && len(aged) > batchSize {
		  aged = aged[:batchSize]
	   }
	  return aged, nil
  }

// QuorumFacts 只读 Quorum 事实，供放行引擎消费（不写 dualctl 状态机）。
type QuorumFacts struct {
	// RequestID dualctl Request ID。
	RequestID string
	// State 当前状态。
	State State
	// AllowN 已投允许票数。
	AllowN int
	// Need 所需票数。
	Need int
	// Reached 是否已达法定票数。
	Reached bool
	// ExpiresAt 票证有效期。
	ExpiresAt time.Time
	// Alive 是否仍在有效期内（相对于 now）。
	Alive bool
}

// ReadQuorumFacts 以只读方式读取 Quorum 事实，不修改任何状态。
  func (b *Book) ReadQuorumFacts(requestID string, now time.Time) (QuorumFacts, error) {
	  r, err := b.Get(requestID)
	   if err != nil {
		  return QuorumFacts{}, err
	  }
	  n := AllowCount(*r)
	  return QuorumFacts{
		   RequestID: r.ID,
		   State: r.State,
		   AllowN: n,
		  Need:   r.Need,
		   Reached:   n >= r.Need,
		   ExpiresAt: r.ExpiresAt,
		   Alive: now.Before(r.ExpiresAt),
	  }, nil
   }

// ValidateForRelease 在放行前校验 dualctl Request 是否满足放行前置条件：
// 1. State==ready（已达票）。
// 2. 票证在有效期内（now < ExpiresAt）。
// 3. allowCount >= Need。
// 不修改状态机，仅返回错误描述。
   func (b *Book) ValidateForRelease(requestID string, now time.Time) error {
	   r, err := b.Get(requestID)
	   if err != nil {
		   return err
	  }
	   if r.State != StateReady {
		   return fmt.Errorf("dualctl: request %s not ready (state=%s)", requestID, r.State)
	  }
	   if !now.Before(r.ExpiresAt) {
		   return fmt.Errorf("dualctl: request %s expired at %s", requestID, r.ExpiresAt.Format(time.RFC3339))
	  }
	  if AllowCount(*r) < r.Need {
		   return fmt.Errorf("dualctl: request %s quorum not reached (%d/%d)", requestID, AllowCount(*r), r.Need)
	   }
	   return nil
   }
