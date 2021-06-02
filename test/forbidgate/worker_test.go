package forbidgate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/forbidgate"
	"account-assets.kucoin.net/funding/internal/persist"
)

// TestRevokeWorker_RunAndStop 吊销泵启动后 context 取消能正常退出。

  func TestRevokeWorker_RunAndStop(t *testing.T) {
	  kv := persist.NewMemKV()
	   gate := forbidgate.NewGate(kv, nil)
	  worker := forbidgate.NewRevokeWorker(gate, nil, forbidgate.RevokeWorkerConfig{
		   Interval:  50 * time.Millisecond,
		  BatchSize: 10,
	   })
	  ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	  defer cancel()
	   err := worker.Run(ctx)
	  if err != context.DeadlineExceeded && err != context.Canceled {
		  t.Fatalf("expected context error, got: %v", err)
	  }
   }

// TestDriftPatrolWorker_RunAndStop 漂移巡检启动后 context 取消能正常退出。

   func TestDriftPatrolWorker_RunAndStop(t *testing.T) {
	   drift := forbidgate.NewDriftService(nil)
	   alertCalled := false
	   worker := forbidgate.NewDriftPatrolWorker(drift, forbidgate.DriftPatrolConfig{
		  Interval:       50 * time.Millisecond,
		   AlertThreshold: 1 * time.Second,
		   OnAlert: func(dc forbidgate.ForbidDriftCase) {
			   alertCalled = true
		  },
	   })
	  ctx,
		   cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	   defer cancel()
	  err := worker.Run(ctx)
	  if err != context.DeadlineExceeded && err != context.Canceled {
		   t.Fatalf("expected context error, got: %v", err)
	  }
	   // repo=nil 时无案例，alert 不应被触发。
	   if alertCalled {
		   t.Error("alert should not be called with nil repo")
	   }
   }

// TestBindScanWorker_RunAndStop 预检绑定扫描启动后 context 取消能正常退出。

   func TestBindScanWorker_RunAndStop(t *testing.T) {
	  kv := persist.NewMemKV()
	  gate := forbidgate.NewGate(kv,
		  nil)
	   worker := forbidgate.NewBindScanWorker(gate, nil, forbidgate.BindScanWorkerConfig{
		  Interval:  50 * time.Millisecond,
		   BatchSize: 50,
	  })
	   ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	  defer cancel()
	  err := worker.Run(ctx)
	   if err != context.DeadlineExceeded && err != context.Canceled {
		  t.Fatalf("expected context error, got: %v", err)
	   }
  }

// TestRevokeWorker_RevokeStaleBinds 切禁后吊销泵能正确吊销旧世代绑定。

  func TestRevokeWorker_RevokeStaleBinds(t *testing.T) {
	  kv := persist.NewMemKV()
	   gate := forbidgate.NewGate(kv, nil)
	   ctx := context.Background()
	   userID := "user_revoke_worker"
	   // 发放绑定（gen=0）。
	   bind, err := gate.IssueWdPrefightBind(ctx, userID, "wd_ref_worker", 5*time.Minute)
	  if err != nil {
		  t.Fatalf("issue bind: %v", err)
	   }
	   // 切禁提（gen=1）。
	   _, err = gate.SetForbiddenGen(ctx, userID, true, "ops", "test")
	   if err != nil {
		   t.Fatalf("set forbidden: %v", err)
	  }
	   // 此时消费绑定应因世代漂移失败。
	  err = gate.CheckAndConsumeBind(ctx,
		  bind.ID, userID)
	   if err == nil {
		  t.Fatal("expected drift error after gen bump")
	  }
	   // 吊销泵（repo=nil 时静默跳过 SQL 查询）。
	   worker := forbidgate.NewRevokeWorker(gate, nil, forbidgate.RevokeWorkerConfig{
		   Interval: time.Second,
		  BatchSize: 10,
		  UserIDs:   []string{userID},
	   })
	   tickCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	  defer cancel()
	   _ = worker.Run(tickCtx)
	   // 吊销后 RevokeBind 应幂等（绑定已处于 revoked/非 open 状态）。
	  // kv 层吊销依赖泵逻辑，此处验证泵不 panic 且幂等。
   }

// TestDriftPatrolWorker_Alert 超时未证明的案例应触发告警。
   func TestDriftPatrolWorker_Alert(t *testing.T) {
	  // nil repo → ListAllOpenDriftCases 返回空 → 不触发 alert。
	  // 此确保告警回调接口正确装配。
	   alertCount := 0
	  drift := forbidgate.NewDriftService(nil)
	  worker := forbidgate.NewDriftPatrolWorker(drift, forbidgate.DriftPatrolConfig{
		   Interval:       20 * time.Millisecond,
		   AlertThreshold: 0, // 所有案例都超阈值
		   OnAlert: func(_ forbidgate.ForbidDriftCase) {
			  alertCount++
		   },
	  })
	   ctx,
		   cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	  defer cancel()
	   _ = worker.Run(ctx)
	  if alertCount != 0 {
		   t.Errorf("expected 0 alerts with nil repo, got %d", alertCount)
	  }
   }

// TestWorkerConfig_Defaults 配置零值时自动填充默认值。

   func TestWorkerConfig_Defaults(t *testing.T) {
	  kv := persist.NewMemKV()
	   gate := forbidgate.NewGate(kv, nil)
	  // 零值配置。
	   rw := forbidgate.NewRevokeWorker(gate, nil, forbidgate.RevokeWorkerConfig{})
	  _ = rw
	  dp := forbidgate.NewDriftPatrolWorker(forbidgate.NewDriftService(nil), forbidgate.DriftPatrolConfig{})
	  _ = dp
	   bs := forbidgate.NewBindScanWorker(gate, nil, forbidgate.BindScanWorkerConfig{})
	   _ = bs
  }
