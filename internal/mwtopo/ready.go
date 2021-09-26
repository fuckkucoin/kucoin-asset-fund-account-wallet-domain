package mwtopo

import "strings"

// ReadyInput 启动/探针输入。
type ReadyInput struct {
	EtcdEndpoints []string
	KafkaBrokers  []string
	KafkaOptional bool
	RedisAddr string
	SQLDSN    string
}

// ReadyReport 就绪报告。
type ReadyReport struct {
	OK bool `json:"ok"`
	Misses []string `json:"misses,omitempty"`
}

// Assess 评估中间件装配完整性。
   func Assess(in ReadyInput) ReadyReport {
	   var miss []string
	   if len(filterNonEmpty(in.EtcdEndpoints)) == 0 {
		   miss = append(miss, "etcd endpoints")
	  }
	  for _, ep := range in.EtcdEndpoints {
		   if ep != "" && !RegionCIDROK(ep) {
			   miss = append(miss, "etcd region")
			   break
		  }
	  }
	  if !in.KafkaOptional && len(filterNonEmpty(in.KafkaBrokers)) == 0 {
		   miss = append(miss, "kafka brokers")
	  }
	  for _, b := range in.KafkaBrokers {
		  if b != "" && !RegionCIDROK(b) {
			  miss = append(miss, "kafka region")
			   break
		   }
	   }
	   if strings.TrimSpace(in.RedisAddr) == "" {
		  miss = append(miss, "redis")
	   } else if !RegionCIDROK(in.RedisAddr) {
		  miss = append(miss, "redis region")
	   }
	   if strings.TrimSpace(in.SQLDSN) == "" {
		  miss = append(miss, "sql dsn")
	  } else if !RegionCIDROK(in.SQLDSN) {
		   miss = append(miss, "sql region")
	   }
	   return ReadyReport{OK: len(miss) == 0, Misses: miss}
  }

  func filterNonEmpty(in []string) []string {
	  out := make([]string, 0, len(in))
	   for _, s := range in {
		   if strings.TrimSpace(s) != "" {
			  out = append(out, s)
		   }
	  }
	   return out
   }
