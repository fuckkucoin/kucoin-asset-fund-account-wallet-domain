package stmt

import (
	"fmt"
	"strings"
	"time"
)

// AuditEvent 导出审计。
type AuditEvent struct {
	JobID string
	UserID string
	Format   string
	StartAt  time.Time
	EndAt    time.Time
	Bytes    int
	Checksum string
	Actor string
	CreatedAt time.Time
}

// Validate 必填项。

   func (e AuditEvent) Validate() error {
	  if strings.TrimSpace(e.UserID) == "" {
		  return fmt.Errorf("audit user_id required")
	   }
	  if strings.TrimSpace(e.JobID) == "" {
		   return fmt.Errorf("audit job_id required")
	   }
	  if strings.TrimSpace(e.Format) == "" {
		  return fmt.Errorf("audit format required")
	  }
	   return nil
  }
