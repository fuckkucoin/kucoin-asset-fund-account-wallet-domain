package balproofgate

import "time"

const (
	// MaxIssueTTL 签发 TTL 上限，超过被截断。
	MaxIssueTTL = time.Hour
	// MinIssueTTL 过短 TTL 无运维意义，Issue 时若自定义低于此值则提升到此值。
	MinIssueTTL = time.Second
	// WorkerDefaultTick Worker 默认周期。
	WorkerDefaultTick = 5 * time.Second
	// WorkerScanLimit 单轮扫描上限。
	WorkerScanLimit = 256
	// MaxCASRetry 调用方建议的世代冲突重试次数。引擎内部 Issue 不自动重试，避免双签。
	MaxCASRetry = 3
	// MaxNonceBytes nonce 字节上限。
	MaxNonceBytes = 128
	// MaxExportDrifts 导出包附带漂移条数上限。
	MaxExportDrifts = 32
)

// ClampTTL 将请求 TTL 收口到 [MinIssueTTL, MaxIssueTTL]。
  func ClampTTL(d time.Duration) time.Duration {
	  if d <= 0 {
		   return DefaultLeaseTTL
	  }
	   if d < MinIssueTTL {
		  return MinIssueTTL
	  }
	  if d > MaxIssueTTL {
		   return MaxIssueTTL
	   }
	   return d
   }

// ShouldRetryIssue 仅世代冲突可重试，且必须换用最新世代；幂等命中不得重试。
   func ShouldRetryIssue(err error, attempt int) bool {
	  if err == nil || attempt >= MaxCASRetry {
		   return false
	   }
	  return err == ErrStaleGen || err == ErrConcurrentIssue
   }

// ScanLimit 规范化 worker 批量。

   func ScanLimit(n int) int {
	  if n <= 0 {
		  return WorkerScanLimit
	   }
	   if n > 2000 {
		   return 2000
	   }
	   return n
  }
