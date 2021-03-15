package brokerfence_test

import (
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/brokerfence"
)

// makeService 构造一个有预设账本余额的 Service。

  func makeService(totals map[string]string) *brokerfence.Service {
	   reader := &stubBookReader{totals: totals}
	   svc := brokerfence.NewService(reader)
	   return svc
   }

// ──────────────────────────────────────────────
// 验收场景 1：Fenced 子账 Alloc to_sub 失败
// ──────────────────────────────────────────────

   func TestService_FencedAllocBlocked(t *testing.T) {
	   svc := makeService(map[string]string{"master-001|USDT": "1000"})
	  // 激活围栏。
	   _, _, err := svc.FenceActivate(brokerfence.FenceReq{
		   SubID: "sub-001", MasterID: "master-001", Asset: "USDT", Actor: "ops-alice",
	   })
	   if err != nil {
		   t.Fatalf("FenceActivate 失败: %v", err)
	   }
	   // 世代查询：generation=1，fenced=true。
	  gen, err := svc.QueryGeneration("sub-001")
	  if err != nil {
		   t.Fatal(err)
	  }
	   if gen.Generation != 1 || !gen.Fenced {
		   t.Errorf("围栏激活后世代期望 1/fenced=true，实际 gen=%d fenced=%v", gen.Generation, gen.Fenced)
	  }
	  // 无令牌 AllocCheck → ErrMutexDenied。
	   _, err = svc.AllocCheck(brokerfence.AllocCheckReq{
		  SubID: "sub-001", Generation: 1, Token: "",
	  })
	  if err != brokerfence.ErrMutexDenied {
		  t.Errorf("无令牌时期望 ErrMutexDenied，实际 %v", err)
	  }
   }

// ──────────────────────────────────────────────
// 验收场景 2：围栏窗口 CreditMaster → BypassCase
// ──────────────────────────────────────────────

  func TestService_CreditMasterBypassCase(t *testing.T) {
	  svc := makeService(map[string]string{"master-002|USDT": "500"})
	  lease, _, err := svc.FenceActivate(brokerfence.FenceReq{
		   SubID: "sub-002", MasterID: "master-002", Asset: "USDT", Actor: "ops-alice",
	   })
	   if err != nil {
		  t.Fatalf("FenceActivate 失败: %v", err)
	  }
	  // CreditMasterGuard 检测到围栏。
	   bc, err := svc.CreditMasterGuard("sub-002", "master-002", "USDT", "200")
	  if err != brokerfence.ErrMutexDenied {
		  t.Errorf("期望 ErrMutexDenied，实际 %v", err)
	  }
	  if bc == nil {
		   t.Fatal("BypassCase 应当被创建")
	   }
	  if bc.State != brokerfence.BypassStateOpen {
		   t.Errorf("BypassCase 初始状态期望 open，实际 %s", bc.State)
	  }
	   if bc.HangingAmt != "200" {
		  t.Errorf("HangingAmt 期望 200，实际 %s", bc.HangingAmt)
	   }
	   if bc.LeaseID != lease.ID {
		   t.Errorf("BypassCase.LeaseID 期望 %s，实际 %s", lease.ID, bc.LeaseID)
	  }
  }

// ──────────────────────────────────────────────
// 验收场景 3：并发 Fence 与 AllocCheck
// ──────────────────────────────────────────────

   func TestService_ConcurrentFenceAndAllocCheck(t *testing.T) {
	  svc := makeService(map[string]string{"master-003|USDT": "1000"})
	   const goroutines = 10
	  fenceResults := make([]error, goroutines)
	   allocResults := make([]error, goroutines)
	  var wg sync.WaitGroup
	  wg.Add(goroutines * 2)
	  for i := 0; i < goroutines; i++ {
		   go func(idx int) {
			  defer wg.Done()
			  _, _, err := svc.FenceActivate(brokerfence.FenceReq{
				   SubID: "sub-003", MasterID: "master-003", Asset: "USDT",
				  Actor: "ops-alice",
			  })
			   fenceResults[idx] = err
		   }(i)
		  go func(idx int) {
			  defer wg.Done()
			  _, err := svc.AllocCheck(brokerfence.AllocCheckReq{
				   SubID: "sub-003", Generation: 0, Token: "",
			   })
			  allocResults[idx] = err
		  }(i)
	  }
	   wg.Wait()
	  // 至多一个 FenceActivate 成功（世代 0→1 只能 CAS 一次）。
	  fenceSuccess := 0
	   for _, e := range fenceResults {
		   if e == nil {
			   fenceSuccess++
		  }
	   }
	  if fenceSuccess > 1 {
		   t.Errorf("并发 Fence：期望至多 1 成功，实际 %d 成功", fenceSuccess)
	   }
   }

// ──────────────────────────────────────────────
// 验收场景 4：Unfence 前有未关闭 BypassCase → 拒绝
// ──────────────────────────────────────────────

   func TestService_UnfenceBlockedByOpenCase(t *testing.T) {
	  svc := makeService(map[string]string{"master-004|USDT": "300"})
	  lease, _, err := svc.FenceActivate(brokerfence.FenceReq{
		   SubID: "sub-004", MasterID: "master-004", Asset: "USDT", Actor: "ops-alice",
	  })
	   if err != nil {
		  t.Fatal(err)
	   }
	   // 产出一个 BypassCase（不关闭）。
	   _, _ = svc.CreditMasterGuard("sub-004", "master-004", "USDT", "50")
	   // Unfence 应被拒绝。
	   _, _, err = svc.UnfenceRequest(brokerfence.UnfenceReq{
		  SubID: "sub-004", LeaseID: lease.ID, Actor: "ops-alice",
	  })
	  if err != brokerfence.ErrBypassCaseOpen {
		   t.Errorf("期望 ErrBypassCaseOpen，实际 %v", err)
	  }
  }

// ──────────────────────────────────────────────
// 验收场景 5：守恒漂移挡写
// ──────────────────────────────────────────────

  func TestService_ConserveDriftBlocksUnfence(t *testing.T) {
	  totals := map[string]string{"master-005|USDT": "1000"}
	   reader := &stubBookReader{totals: totals}
	  svc := brokerfence.NewService(reader)
	  lease, _, err := svc.FenceActivate(brokerfence.FenceReq{
		   SubID: "sub-005", MasterID: "master-005", Asset: "USDT", Actor: "ops-bob",
	  })
	  if err != nil {
		   t.Fatal(err)
	  }
	   // 账本在围栏期间被外力修改（漂移）。
	   reader.totals["master-005|USDT"] = "1100"
	   // Unfence 时采样 AfterTotal=1100≠1000 → 漂移 → 拒绝。
	   _, _, err = svc.UnfenceRequest(brokerfence.UnfenceReq{
		   SubID: "sub-005", LeaseID: lease.ID, Actor: "ops-bob",
	  })
	   if err != brokerfence.ErrConserveDrift {
		   t.Errorf("漂移时期望 ErrConserveDrift，实际 %v", err)
	   }
	   // lease 仍为 active（不应被强制关闭）。
	  l, _ := svc.GetLease(lease.ID)
	  if l.State != brokerfence.LeaseStateActive {
		  t.Errorf("漂移拒绝后 lease 状态期望 active，实际 %s", l.State)
	   }
  }

// ──────────────────────────────────────────────
// 幂等性：重复 FenceActivate 拒绝（已围栏）
// ──────────────────────────────────────────────

  func TestService_FenceActivate_Idempotent(t *testing.T) {
	  svc := makeService(nil)
	   req := brokerfence.FenceReq{
		  SubID: "sub-idem", MasterID: "master-idem", Asset: "USDT", Actor: "ops-x",
	   }
	   if _, _, err := svc.FenceActivate(req); err != nil {
		  t.Fatal(err)
	   }
	   // 再次围栏应返回 ErrLeaseSealConflict（已围栏态）。
	   _, _, err := svc.FenceActivate(req)
	   if err != brokerfence.ErrLeaseSealConflict {
		  t.Errorf("重复 FenceActivate 期望 ErrLeaseSealConflict，实际 %v", err)
	   }
   }

// ──────────────────────────────────────────────
// 令牌过期
// ──────────────────────────────────────────────

  func TestMutexStore_TokenExpiry(t *testing.T) {
	   ms := brokerfence.NewMutexStore(50 * time.Millisecond)
	  tok, _ := ms.Issue("lease-Z", "sub-Z", "ops-z")
	   // 立即校验通过。
	   if err := ms.ValidateToken("sub-Z", tok.Token, time.Now().UTC()); err != nil {
		  t.Errorf("有效令牌期望通过，实际 %v", err)
	   }
	  // 过期：传入令牌过期后的时间点。
	  expired := tok.ExpiredAt.Add(time.Millisecond)
	   err := ms.ValidateToken("sub-Z", tok.Token, expired)
	   if err != brokerfence.ErrTokenExpired {
		   t.Errorf("过期令牌期望 ErrTokenExpired，实际 %v", err)
	   }
   }

// ──────────────────────────────────────────────
// 正常解围流程（无 BypassCase、无漂移）
// ──────────────────────────────────────────────

  func TestService_UnfenceOK(t *testing.T) {
	   totals := map[string]string{"master-OK|USDT": "800"}
	  reader := &stubBookReader{totals: totals}
	   svc := brokerfence.NewService(reader)
	   lease, _, err := svc.FenceActivate(brokerfence.FenceReq{
		   SubID: "sub-OK", MasterID: "master-OK", Asset: "USDT", Actor: "ops-c",
	   })
	  if err != nil {
		   t.Fatal(err)
	   }
	   // 余额不变（无漂移）。
	   sealedLease, proof, err := svc.UnfenceRequest(brokerfence.UnfenceReq{
		  SubID: "sub-OK", LeaseID: lease.ID, Actor: "ops-c",
	  })
	  if err != nil {
		   t.Fatalf("UnfenceRequest 失败: %v", err)
	   }
	  if sealedLease.State != brokerfence.LeaseStateSealed {
		   t.Errorf("解围后 lease 状态期望 sealed，实际 %s", sealedLease.State)
	  }
	  if proof == nil || proof.IsDrifted {
		  t.Error("解围后守恒证明应存在且无漂移")
	   }
	   // 世代应已升级（fenced=false）。
	  gen, _ := svc.QueryGeneration("sub-OK")
	  if gen.Fenced {
		   t.Error("解围后 fenced 期望 false")
	   }
	  if gen.Generation != 2 {
		  t.Errorf("解围后世代期望 2，实际 %d", gen.Generation)
	  }
   }
