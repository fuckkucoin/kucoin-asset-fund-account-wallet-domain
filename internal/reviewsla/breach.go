package reviewsla

import (
	"crypto/rand"
	"encoding/hex"
	"time"
)

// BreachLeaseRef 违约世代引用。Scan 后随 Ticket 一起返回，供 breachclosegate 建档。
type BreachLeaseRef struct {
	CaseID string `json:"case_id"`
	MovementID string `json:"movement_id"`
	UserID string `json:"user_id"`
	BreachGen  string    `json:"breach_gen"` // 唯一令牌，32 字节 hex
	Deadline   time.Time `json:"deadline"`
	BreachedAt time.Time `json:"breached_at"`
}

// newBreachGen 生成 breach_gen 令牌：32 字节密码安全随机 hex 串。

  func newBreachGen() string {
	   b := make([]byte, 32)
	  if _, err := rand.Read(b); err != nil {
		  // 降级：用时间戳加固定前缀，保证唯一性但无密码安全属性。
		  // 此分支在中不应触发；依赖者须告警。
		   var fb [32]byte
		   ts := time.Now().UnixNano()
		   for i := 0; i < 8; i++ {
			  fb[i] = byte(ts >> (i * 8))
		   }
		   copy(fb[8:], []byte("breach-gen-fallback-placeholder.."))
		   return hex.EncodeToString(fb[:])
	  }
	  return hex.EncodeToString(b)
   }

// issueBreachGenLocked 在持有 Book.mu 的条件下为 Ticket 签发 breach_gen。
// 若已签发（BreachGen 非空）则幂等返回已有值。
  func issueBreachGenLocked(t *Ticket, now time.Time) string {
	  if t.BreachGen != "" {
		   return t.BreachGen
	  }
	   t.BreachGen = newBreachGen()
	  t.BreachedAt = now
	  return t.BreachGen
  }

// toLeaseRef 将 Ticket 转为 BreachLeaseRef（只读外部引用）。
// 只在 Ticket.BreachGen 非空时有效。
   func toLeaseRef(t *Ticket) BreachLeaseRef {
	  return BreachLeaseRef{
		   CaseID: t.CaseID,
		   MovementID: t.MovementID,
		  UserID: t.UserID,
		  BreachGen: t.BreachGen,
		   Deadline:   t.Deadline,
		   BreachedAt: t.BreachedAt,
	   }
   }
