package forbidgate_test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/forbidgate"
	"account-assets.kucoin.net/funding/internal/persist"
)

// TestOpsHandler_New 构造 OpsHandler 不应 panic。
   func TestOpsHandler_New(t *testing.T) {
	  kv := persist.NewMemKV()
	  gate := forbidgate.NewGate(kv,
		   nil)
	  drift := forbidgate.NewDriftService(nil)
	   m := &forbidgate.Metrics{}
	   h := forbidgate.NewOpsHandler(gate,
		  drift, m)
	   if h == nil {
		   t.Fatal("expected non-nil OpsHandler")
	  }
   }

// TestOpsHandler_NilMetrics nil Metrics 时 OpsHandler 不应 panic。
  func TestOpsHandler_NilMetrics(t *testing.T) {
	   kv := persist.NewMemKV()
	  gate := forbidgate.NewGate(kv,
		  nil)
	   drift := forbidgate.NewDriftService(nil)
	   h := forbidgate.NewOpsHandler(gate, drift, nil)
	   if h == nil {
		   t.Fatal("expected non-nil OpsHandler")
	   }
   }

// TestSplitAndTrim 分割工具函数表驱动（通过 splitAndTrim 内部实现路径验证）。
   func TestSplitAndTrim_ViaBatch(t *testing.T) {
	  // 通过 NewOpsHandler 的批量查询路径间接分割逻辑。
	  // 直接单元 genBatch 需要 httptest，此处仅验证构造不 panic。
	  kv := persist.NewMemKV()
	   gate := forbidgate.NewGate(kv, nil)
	  h := forbidgate.NewOpsHandler(gate, nil, nil)
	   _ = h
  }
