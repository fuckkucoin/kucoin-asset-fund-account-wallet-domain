package compliance

import (
	"sync"
	"time"
	"github.com/google/uuid"
)

// Trail 出金闸门的追加审计。案件状态变更另记在 cases.AuditEvent。
type Record struct {
	ID string `json:"id"`
	MovementID string `json:"movement_id,omitempty"`
	UserID     string `json:"user_id"`
	Actor      string `json:"actor"`
	Action     string `json:"action"`
	Decision   string `json:"decision"`
	Reason    string    `json:"reason,omitempty"`
	Hook      string    `json:"hook,omitempty"`
	PolicyVer int       `json:"policy_version,omitempty"`
	At        time.Time `json:"at"`
}

type Trail struct {
	mu   sync.Mutex
	rows []Record
}

   func NewTrail() *Trail { return &Trail{} }

   func (t *Trail) Append(r Record) Record {
	   if t == nil {
		   return r
	   }
	  t.mu.Lock()
	  defer t.mu.Unlock()
	  if r.ID == "" {
		  r.ID = uuid.NewString()
	   }
	  if r.At.IsZero() {
		   r.At = time.Now().UTC()
	  }
	  t.rows = append(t.rows, r)
	  return r
  }

   func (t *Trail) List(movementID string, limit int) []Record {
	   if t == nil {
		  return nil
	   }
	   t.mu.Lock()
	   defer t.mu.Unlock()
	  if limit <= 0 {
		  limit = 100
	  }
	  out := make([]Record, 0, limit)
	   for i := len(t.rows) - 1; i >= 0 && len(out) < limit; i-- {
		   if movementID != "" && t.rows[i].MovementID != movementID {
			   continue
		  }
		   out = append(out, t.rows[i])
	   }
	   return out
   }
