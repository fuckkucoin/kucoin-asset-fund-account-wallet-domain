package wdpartial_test

import (
	"errors"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/wdpartial"
)

// ─── 表驱动：Acquire + Verify + Release ───────────────────────────────────

   func TestMutexManagerAcquire(t *testing.T) {
	  t.Parallel()
	   base := time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC)
	   cases := []struct {
		   name string
		   batchID string
		  firstHolder string
		  firstTTL    time.Duration
		  secondHolder  string
		   advanceClock  time.Duration
		   wantFirstGen  int64
		   wantSecondErr error
		  wantSecondGen int64
	   }{
		  {
			   name: "验收2: 双worker竞争，B被拒绝",
			  batchID: "batch-mutex-acc-2",
			   firstHolder: "worker-A",
			   firstTTL:      5 * time.Minute,
			  secondHolder:  "worker-B",
			   advanceClock:  0,
			  wantFirstGen:  1,
			   wantSecondErr: wdpartial.ErrLeaseConflict,
		  },
		   {
			  name: "TTL过期后B可抢占，Gen递进",
			   batchID:     "batch-mutex-ttl",
			  firstHolder: "worker-A",
			   firstTTL:    100 * time.Millisecond,
			   secondHolder: "worker-B",
			  advanceClock:  200 * time.Millisecond,
			   wantFirstGen:  1,
			  wantSecondGen: 2,
		   },
		  {
			   name: "同一holder续租，Gen递进",
			  batchID:     "batch-mutex-renew",
			  firstHolder: "worker-A",
			  firstTTL: 5 * time.Minute,
			  secondHolder: "worker-A",
			   advanceClock: 0,
			  wantFirstGen: 1,
			  wantSecondGen: 2,
		   },
	  }
	   for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			  now := base
			   clockFn := func() time.Time { return now }
			   mgr := wdpartial.NewMutexManager(wdpartial.NewMemLeaseBook()).
				  WithClock(clockFn).
				  WithTTL(tc.firstTTL)
			  // 第一个 holder 获取
			  lease1, err := mgr.Acquire(tc.batchID, tc.firstHolder)
			  if err != nil {
				   t.Fatalf("第一次 Acquire 失败: %v", err)
			   }
			   if lease1.Gen != tc.wantFirstGen {
				   t.Errorf("第一次 Gen: want=%d got=%d", tc.wantFirstGen, lease1.Gen)
			  }
			   // 推进时钟
			   now = now.Add(tc.advanceClock)
			  // 第二个 holder 尝试获取
			  lease2, err2 := mgr.Acquire(tc.batchID, tc.secondHolder)
			   if tc.wantSecondErr != nil {
				  if !errors.Is(err2, tc.wantSecondErr) {
					   t.Errorf("期望错误 %v, 得到 %v", tc.wantSecondErr, err2)
				   }
				   return
			   }
			  if err2 != nil {
				  t.Fatalf("第二次 Acquire 失败（意外）: %v", err2)
			  }
			   if tc.wantSecondGen > 0 && lease2.Gen != tc.wantSecondGen {
				   t.Errorf("第二次 Gen: want=%d got=%d", tc.wantSecondGen, lease2.Gen)
			   }
		   })
	   }
   }

// ─── Verify 表驱动 ────────────────────────────────────────────────────────

   func TestMutexManagerVerify(t *testing.T) {
	  t.Parallel()
	   base := time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC)
	  cases := []struct {
		  name string
		   batchID      string
		  holder       string
		   gen          int64
		  advanceClock time.Duration
		   ttl     time.Duration
		   wantErr error
	   }{
		  {
			  name:    "持有有效租约且Gen正确",
			  batchID: "batch-verify-ok",
			  holder:  "worker-A",
			   gen: 1,
			  ttl: 5 * time.Minute,
			   wantErr: nil,
		   },
		   {
			  name: "持有者不匹配",
			  batchID: "batch-verify-holder",
			   holder: "worker-B",
			  gen: 1,
			  ttl: 5 * time.Minute,
			  wantErr: wdpartial.ErrNoLease,
		   },
		  {
			  name:    "租约已过期",
			   batchID: "batch-verify-expire",
			  holder:  "worker-A",
			   gen:          1,
			   ttl:          100 * time.Millisecond,
			   advanceClock: 200 * time.Millisecond,
			  wantErr: wdpartial.ErrLeaseExpired,
		   },
		   {
			   name: "Gen不匹配（旧Gen）",
			   batchID: "batch-verify-stale",
			  holder: "worker-A",
			   gen: 0,
			  ttl:     5 * time.Minute,
			  wantErr: wdpartial.ErrStaleGen,
		   },
	  }
	   for _, tc := range cases {
		  tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			  now := base
			   clockFn := func() time.Time { return now }
			  mgr := wdpartial.NewMutexManager(wdpartial.NewMemLeaseBook()).
				  WithClock(clockFn).
				  WithTTL(tc.ttl)
			   // worker-A 先获取
			   lease, err := mgr.Acquire(tc.batchID, "worker-A")
			   if err != nil {
				   t.Fatalf("Acquire 失败: %v", err)
			   }
			   now = now.Add(tc.advanceClock)
			   verifyGen := tc.gen
			   if tc.gen == 1 {
				   verifyGen = lease.Gen
			  }
			  err = mgr.Verify(tc.batchID, tc.holder, verifyGen)
			  if tc.wantErr != nil {
				  if !errors.Is(err, tc.wantErr) {
					   t.Errorf("期望错误 %v, 得到 %v", tc.wantErr, err)
				  }
				  return
			   }
			  if err != nil {
				  t.Errorf("意外错误: %v", err)
			   }
		   })
	   }
  }

// ─── Release ─────────────────────────────────────────────────────────────

   func TestMutexManagerRelease(t *testing.T) {
	   t.Parallel()
	  base := time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC)
	  now := base
	   clockFn := func() time.Time { return now }
	  mgr := wdpartial.NewMutexManager(wdpartial.NewMemLeaseBook()).
		  WithClock(clockFn).
		  WithTTL(5 * time.Minute)
	   batchID := "batch-release-001"
	   // 获取
	  lease, err := mgr.Acquire(batchID, "worker-A")
	  if err != nil {
		   t.Fatalf("Acquire: %v", err)
	   }
	   // worker-B 无法获取（A 未释放）
	   _, err = mgr.Acquire(batchID, "worker-B")
	   if !errors.Is(err, wdpartial.ErrLeaseConflict) {
		   t.Errorf("期望 ErrLeaseConflict, 得到 %v", err)
	   }
	   // A 主动释放
	   if err := mgr.Release(batchID, "worker-A", lease.Gen); err != nil {
		   t.Fatalf("Release: %v", err)
	   }
	   // B 此后可获取，Gen 递进
	   lease2, err := mgr.Acquire(batchID, "worker-B")
	   if err != nil {
		   t.Fatalf("Release 后 B Acquire: %v", err)
	   }
	  if lease2.Gen != 2 {
		   t.Errorf("Release 后 Gen 应为 2, got %d", lease2.Gen)
	   }
	  // A 用旧 Gen 提交应 ErrNoLease（holder 不匹配）
	   err = mgr.Verify(batchID, "worker-A", lease.Gen)
	  if !errors.Is(err, wdpartial.ErrNoLease) {
		  t.Errorf("A 旧 Gen Verify 期望 ErrNoLease, 得到 %v", err)
	   }
  }

// ─── 验收 2 完整场景 ──────────────────────────────────────────────────────
// TestDualWorkerResumeMutex 验收 2：双进程 Resume，仅一方获锁，另一方 ErrStaleGen。
   func TestDualWorkerResumeMutex(t *testing.T) {
	  t.Parallel()
	   base := time.Date(2024, 9, 1, 0, 0, 0, 0, time.UTC)
	  now := base
	   clockFn := func() time.Time { return now }
	   mgr := wdpartial.NewMutexManager(wdpartial.NewMemLeaseBook()).
		   WithClock(clockFn).
		   WithTTL(5 * time.Minute)
	  batchID := "batch-dual-resume"
	   // Step 1: worker-A 获得世代锁 Gen=1
	  leaseA, err := mgr.Acquire(batchID, "worker-A")
	  if err != nil {
		   t.Fatalf("worker-A Acquire: %v", err)
	   }
	   if leaseA.Gen != 1 {
		   t.Errorf("worker-A Gen want=1, got=%d", leaseA.Gen)
	   }
	   // Step 2: worker-B 被拒（ErrLeaseConflict）
	   _, err = mgr.Acquire(batchID, "worker-B")
	  if !errors.Is(err, wdpartial.ErrLeaseConflict) {
		   t.Errorf("worker-B 应被 ErrLeaseConflict 拒绝, 得到 %v", err)
	  }
	   // Step 3: worker-A 完成操作后释放
	   if err := mgr.Release(batchID, "worker-A", leaseA.Gen); err != nil {
		  t.Fatalf("worker-A Release: %v", err)
	   }
	   // Step 4: worker-B 抢占，Gen 递进为 2
	  leaseB, err := mgr.Acquire(batchID, "worker-B")
	  if err != nil {
		  t.Fatalf("worker-B 抢占 Acquire: %v", err)
	   }
	   if leaseB.Gen != 2 {
		  t.Errorf("worker-B Gen want=2, got=%d", leaseB.Gen)
	  }
	   // Step 5: worker-A 用旧 Gen=1 提交 → ErrNoLease（holder 不匹配）
	  err = mgr.Verify(batchID, "worker-A", leaseA.Gen)
	  if !errors.Is(err, wdpartial.ErrNoLease) {
		  t.Errorf("worker-A 旧 Gen Verify 期望 ErrNoLease, 得到 %v", err)
	   }
	  // Step 6: worker-B 用正确 Gen=2 Verify 成功
	   if err := mgr.Verify(batchID, "worker-B", leaseB.Gen); err != nil {
		  t.Errorf("worker-B Verify Gen=2 应成功, 得到 %v", err)
	   }
   }
