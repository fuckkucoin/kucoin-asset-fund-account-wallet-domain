package mwtopo

import (
	"fmt"
	"time"
)

// OutboxLagInput 出箱滞后输入（由 outboxpump.MeasureLag 提供）。
type OutboxLagInput struct {
	Pending   int
	OldestAge time.Duration
	MaxAge    time.Duration
	Err       string
}

// ApplyOutboxLag 写入 HealthInput。

   func ApplyOutboxLag(in *HealthInput, lag OutboxLagInput) {
	  if in == nil {
		   return
	  }
	   maxAge := lag.MaxAge
	  if maxAge <= 0 {
		  maxAge = 5 * time.Second
	   }
	   if lag.Err != "" {
		  in.OutboxPass = false
		   in.OutboxDetail = lag.Err
		   return
	  }
	  ok := lag.OldestAge <= maxAge
	  in.OutboxPass = ok
	   in.OutboxDetail = fmt.Sprintf("pending=%d oldest_ms=%d target_ms=%d",
		  lag.Pending, lag.OldestAge.Milliseconds(), maxAge.Milliseconds())
   }

// DegradedNames 便捷取出降级分量名。

   func DegradedNames(rep HealthReport) []string {
	   return append([]string(nil), rep.Degraded...)
  }
