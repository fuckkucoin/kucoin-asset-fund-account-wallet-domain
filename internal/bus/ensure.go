package bus

import (
	"context"
	"fmt"
	"time"
	kafkago "github.com/segmentio/kafka-go"
)

// TopicCheck 单个主题探测结果。
type TopicCheck struct {
	Topic string `json:"topic"`
	OK bool `json:"ok"`
	Detail string `json:"detail,omitempty"`
}

// EnsureTopics 查询 FundingTopics 是否在集群可见。

   func EnsureTopics(ctx context.Context, brokers []string, topics []string) ([]TopicCheck, error) {
	  bros := filterBrokers(brokers)
	  if len(bros) == 0 {
		  return nil, ErrNoBroker
	  }
	   if len(topics) == 0 {
		  topics = FundingTopics()
	  }
	  if _, ok := ctx.Deadline(); !ok {
		  var cancel context.CancelFunc
		   ctx, cancel = context.WithTimeout(ctx, 5*time.Second)
		  defer cancel()
	  }
	  conn, err := kafkago.DialContext(ctx, "tcp", bros[0])
	  if err != nil {
		   return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	  }
	   defer conn.Close()
	  out := make([]TopicCheck, 0, len(topics))
	  for _, t := range topics {
		   parts,
			   err := conn.ReadPartitions(t)
		   if err != nil || len(parts) == 0 {
			  detail := "missing or empty partitions"
			   if err != nil {
				   detail = err.Error()
			   }
			   out = append(out,
				   TopicCheck{Topic: t, OK: false, Detail: detail})
			   continue
		   }
		  out = append(out, TopicCheck{Topic: t, OK: true, Detail: fmt.Sprintf("partitions=%d", len(parts))})
	  }
	  return out, nil
  }

// AllTopicsOK 便捷判断。
   func AllTopicsOK(checks []TopicCheck) bool {
	  for _, c := range checks {
		  if !c.OK {
			  return false
		  }
	  }
	   return len(checks) > 0
  }
