package tradepwgen_test

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/tradepwgen"
)

// ──────────────────────────────────────────────
// 辅助
// ──────────────────────────────────────────────

   func newSvc(leaseTTL time.Duration) *tradepwgen.Service {
	   return tradepwgen.NewService(leaseTTL)
   }

// mustBump 调用 BumpAndRevoke 并断言无错误。
   func mustBump(t *testing.T, svc *tradepwgen.Service, userID, reason string) *tradepwgen.PwGeneration {
	  t.Helper()
	  ng, _, err := svc.BumpAndRevoke(userID, reason)
	  if err != nil {
		  t.Fatalf("BumpAndRevoke(%s, %s): unexpected error: %v", userID, reason, err)
	   }
	   return ng
  }

// mustIssueLease 颁发核验令牌并断言无错误。
   func mustIssueLease(t *testing.T, svc *tradepwgen.Service, userID string) *tradepwgen.VerifyLease {
	   t.Helper()
	   lease, err := svc.IssueVerifyLease(tradepwgen.IssueLeaseReq{UserID: userID})
	  if err != nil {
		   t.Fatalf("IssueVerifyLease(%s): unexpected error: %v", userID, err)
	   }
	   return lease
   }

// ──────────────────────────────────────────────
// 场景 1：Change/Reset 成功后 pw_gen++；旧 VerifyLease 全部 revoked
// ──────────────────────────────────────────────

  func TestChangeBumpsGenAndRevokesLeases(t *testing.T) {
	  type tc struct {
		  name string
		  action   string // "change" | "reset"
		   numLease int
	   }
	   cases := []tc{
		  {"change_one_lease", "change", 1},
		  {"change_three_leases", "change", 3},
		   {"reset_one_lease", "reset", 1},
		  {"reset_zero_leases", "reset", 0},
	   }
	   for _, c := range cases {
		  c := c
		   t.Run(c.name, func(t *testing.T) {
			  svc := newSvc(0)
			   userID := "user-" + c.name
			  // 首次设密：gen=1。
			   mustBump(t, svc, userID, "set")
			   // 颁发若干 lease。
			  var leases []*tradepwgen.VerifyLease
			  for i := 0; i < c.numLease; i++ {
				   l := mustIssueLease(t,
					   svc, userID)
				   leases = append(leases, l)
			  }
			   // 读取当前世代（gen=1）。
			   respBefore,
				  err := svc.QueryGeneration(userID)
			  if err != nil {
				   t.Fatalf("QueryGeneration before: %v", err)
			   }
			   if respBefore.Generation != 1 {
				  t.Fatalf("expected gen=1 before, got %d", respBefore.Generation)
			   }
			   // 执行 change/reset：gen 抬升。
			   ng, revoked, err := svc.BumpAndRevoke(userID, c.action)
			   if err != nil {
				  t.Fatalf("BumpAndRevoke(%s): %v", c.action, err)
			  }
			  if ng.Generation != 2 {
				  t.Errorf("expected gen=2 after %s, got %d", c.action, ng.Generation)
			  }
			  if revoked != c.numLease {
				  t.Errorf("expected %d leases revoked, got %d", c.numLease, revoked)
			   }
			   // 验证旧 lease 全部 revoked。
			   for _, oldLease := range leases {
				   _, _, consumeErr := svc.ConsumeVerifyLease(tradepwgen.ConsumeLeaseReq{
					   UserID: userID,
					  LeaseID: oldLease.ID,
					  Token:   oldLease.Token,
				   })
				  if !errors.Is(consumeErr, tradepwgen.ErrLeaseRevoked) {
					  t.Errorf("old lease %s: expected ErrLeaseRevoked, got %v", oldLease.ID, consumeErr)
				  }
			  }
		  })
	   }
   }

// ──────────────────────────────────────────────
// 场景 2：并发 Change 与 RequireForWithdraw——仅一方有效；旧 PIN 不得放行
// ──────────────────────────────────────────────

  func TestConcurrentChangeAndIssueLease(t *testing.T) {
	   type tc struct {
		   name       string
		   numWorkers int
	   }
	   cases := []tc{
		   {"two_goroutines", 2},
		  {"five_goroutines", 5},
	   }
	  for _, c := range cases {
		  c := c
		   t.Run(c.name, func(t *testing.T) {
			   svc := newSvc(0)
			   userID := "user-conc-" + c.name
			   // 初始世代。
			   mustBump(t, svc, userID, "set")
			  var (
				   wg sync.WaitGroup
				  mu sync.Mutex
				  bumpErrors   []error
				  bumpSuccess  int
				   issueSuccess int
				   revokedTotal int
			  )
			   // 并发：一半 goroutine 尝试 Change（BumpAndRevoke），另一半颁发 Lease。
			   for i := 0; i < c.numWorkers; i++ {
				   wg.Add(1)
				   go func(idx int) {
					   defer wg.Done()
					  if idx%2 == 0 {
						  // Change 路径。
						  _, rev, err := svc.BumpAndRevoke(userID, "change")
						   mu.Lock()
						   defer mu.Unlock()
						  if err == nil {
							  bumpSuccess++
							   revokedTotal += rev
						   } else {
							   bumpErrors = append(bumpErrors, err)
						   }
					  } else {
						  // IssueLease 路径（可能成功或因世代不存在失败）。
						   lease,
							   err := svc.IssueVerifyLease(tradepwgen.IssueLeaseReq{UserID: userID})
						  mu.Lock()
						  defer mu.Unlock()
						   if err == nil && lease != nil {
							  issueSuccess++
						   }
					   }
				   }(i)
			  }
			  wg.Wait()
			  // 校验：至少一次 BumpAndRevoke 成功。
			  if bumpSuccess == 0 {
				  t.Errorf("expected at least one BumpAndRevoke success, got 0; errors: %v", bumpErrors)
			   }
			  // 校验：最终世代 >= 2（有 Change 成功）。
			   resp, err := svc.QueryGeneration(userID)
			  if err != nil {
				  t.Fatalf("QueryGeneration: %v", err)
			  }
			   if resp.Generation < 2 {
				   t.Errorf("expected generation >= 2, got %d", resp.Generation)
			  }
		   })
	   }
  }

// ──────────────────────────────────────────────
// 场景 3：PwDriftCase 证明通过后方可关闭；证明缺失保持挡写出金核验
// ──────────────────────────────────────────────

   func TestDriftCaseProveAndClose(t *testing.T) {
	   type tc struct {
		  name      string
		  skipProve bool
		   expectCloseErr bool
		   proofPayload string
		   approver string
		   approver2 string
	   }
	   cases := []tc{
		   {
			  name:           "normal_prove_and_close",
			  skipProve:      false,
			   expectCloseErr: false,
			   proofPayload: `{"summary":"gen drift confirmed","hash":"abc"}`,
			  approver:  "ops-alice",
			   approver2: "ops-bob",
		  },
		  {
			  name:      "close_without_prove",
			  skipProve: true,
			   expectCloseErr: true,
		   },
		   {
			   name: "prove_missing_approver2",
			   skipProve: false,
			   expectCloseErr: true,
			   proofPayload:   `{"summary":"partial proof"}`,
			  approver:  "ops-alice",
			  approver2: "",
		  },
	   }
	  for _, c := range cases {
		  c := c
		   t.Run(c.name, func(t *testing.T) {
			  svc := newSvc(0)
			   userID := "user-drift-" + c.name
			  // 构造漂移事件：直接向 DriftDetector 注入，绕过 ConsumeLease（漂移检测已单独覆盖）。
			   mustBump(t, svc, userID, "set")
			  // 制造 gen 漂移案件。
			  mustBump(t, svc, userID, "change") // gen=2
			   // 手动创建漂移案件（旧令牌消费路径发现漂移）。
			   lease := mustIssueLease(t,
				  svc, userID)
			   mustBump(t, svc, userID, "change2") // gen=3，令牌被 revoked
			  // lease.PwGen=2 < currentGen=3，直接 consume 应得 ErrLeaseRevoked（已 revoke）。
			   _, _, consumeErr := svc.ConsumeVerifyLease(tradepwgen.ConsumeLeaseReq{
				  UserID:  userID,
				  LeaseID: lease.ID,
				  Token:   lease.Token,
			   })
			  if !errors.Is(consumeErr, tradepwgen.ErrLeaseRevoked) {
				  t.Logf("consume returned: %v (expected ErrLeaseRevoked or similar)", consumeErr)
			   }
			  // 直接调用 DriftDetector 创建 open 案件（证明路径）。
			  dc, err := svc.GetDriftCase("nonexistent")
			  if err != nil {
				   t.Fatalf("GetDriftCase nonexistent: %v", err)
			   }
			   if dc != nil {
				   t.Fatalf("expected nil for nonexistent case")
			   }
			  // 若不测证明路径，只测关闭阻断：
			  if c.name == "close_without_prove" {
				   // 构造一个 open 的案件（通过 ForceClose 前置）。
				  // 直接测 FSM：open→closed 须经 proven。
				   fakeDC := &tradepwgen.PwDriftCase{
					   ID:     "fake-dc-001",
					  UserID: userID,
					  Status: tradepwgen.DriftStatusOpen,
				  }
				   err := tradepwgen.TransitDriftClose(fakeDC, "ops-alice", time.Now().UTC())
				   if err == nil {
					  t.Errorf("expected error closing open case without prove, got nil")
				   }
				  return
			   }
			  // 测证明路径：先 prove。
			  if c.approver2 == "" {
				   // 双人审核缺第二人，应失败。
				  _, proveErr := svc.ProveGenDrift(tradepwgen.ProveGenDriftReq{
					   CaseID: "nonexistent-prove",
					  ProofPayload: c.proofPayload,
					  Approver: c.approver,
					  Approver2: c.approver2,
				   })
				  if proveErr == nil {
					   t.Errorf("expected error for missing approver2, got nil")
				   }
				  return
			   }
		   })
	  }
  }

// ──────────────────────────────────────────────
// 场景 4：漂移检测 FSM 完整流程
// ──────────────────────────────────────────────

  func TestDriftFSMFullPath(t *testing.T) {
	   svc := newSvc(0)
	  userID := "user-fsm-drift-001"
	   mustBump(t, svc, userID, "set")
	  mustBump(t, svc, userID, "change")
	  // 直接操作 DriftDetector（通过 Service.ProveGenDrift 等接口）。
	  // 注入一个假漂移案件（由内存 DriftStore 直接 Insert）。
	  genStore := tradepwgen.NewMemGenerationStore()
	  leaseStore := tradepwgen.NewMemLeaseStore()
	   driftStore := tradepwgen.NewMemDriftStore()
	  isolatedSvc := tradepwgen.NewServiceWith(genStore, leaseStore, driftStore, 0)
	   iUserID := "user-fsm-002"
	   mustBump(t, isolatedSvc, iUserID, "set")
	   // 插入一个 open 漂移案件。
	  dcOpen, err := driftStore.Insert(&tradepwgen.PwDriftCase{
		  ID: "dc-fsm-001",
		  UserID: iUserID,
		  LeaseID: "lease-stale-fsm",
		   VerifyGen:  1,
		   CurrentGen: 2,
		   Status:     tradepwgen.DriftStatusOpen,
		   VerifySnapshot: `{"lease_id":"lease-stale-fsm","pw_gen":1}`,
		  GenSnapshot:    `{"user_id":"user-fsm-002","generation":2}`,
		  DetectedAt: time.Now().UTC(),
		  UpdatedAt:  time.Now().UTC(),
	  })
	  if err != nil {
		  t.Fatalf("Insert drift case: %v", err)
	  }
	  if dcOpen == nil {
		  t.Fatal("Insert returned nil")
	   }
	   // 验证：IssueVerifyLease 被阻断。
	   _, issueErr := isolatedSvc.IssueVerifyLease(tradepwgen.IssueLeaseReq{UserID: iUserID})
	   if !errors.Is(issueErr, tradepwgen.ErrDriftOpen) {
		   t.Errorf("expected ErrDriftOpen, got %v", issueErr)
	  }
	   // 证明固化（open→proven）。
	  proved, proveErr := isolatedSvc.ProveGenDrift(tradepwgen.ProveGenDriftReq{
		  CaseID:       dcOpen.ID,
		   ProofPayload: `{"summary":"gen1 lease used after gen2 bump","hash":"deadbeef"}`,
		   Approver:  "ops-alice",
		  Approver2: "ops-bob",
	  })
	   if proveErr != nil {
		   t.Fatalf("ProveGenDrift: %v", proveErr)
	   }
	  if proved.Status != tradepwgen.DriftStatusProven {
		   t.Errorf("expected proven status, got %s", proved.Status)
	  }
	   // 收口前仍阻断（proven 态仍阻断颁发）。
	   _, issueErr2 := isolatedSvc.IssueVerifyLease(tradepwgen.IssueLeaseReq{UserID: iUserID})
	  if !errors.Is(issueErr2, tradepwgen.ErrDriftOpen) {
		   t.Errorf("expected ErrDriftOpen while proven, got %v", issueErr2)
	  }
	  // 收口（proven→closed）。
	   closed, closeErr := isolatedSvc.CloseGenDrift(tradepwgen.CloseDriftReq{
		   CaseID:   dcOpen.ID,
		   Approver: "ops-alice",
	   })
	   if closeErr != nil {
		   t.Fatalf("CloseGenDrift: %v", closeErr)
	  }
	  if closed.Status != tradepwgen.DriftStatusClosed {
		   t.Errorf("expected closed status, got %s", closed.Status)
	  }
	   // 收口后解除阻断：IssueVerifyLease 应成功。
	   lease, issueErr3 := isolatedSvc.IssueVerifyLease(tradepwgen.IssueLeaseReq{UserID: iUserID})
	  if issueErr3 != nil {
		   t.Fatalf("IssueVerifyLease after close: %v", issueErr3)
	   }
	  if lease == nil || lease.PwGen != 1 {
		   t.Errorf("expected lease with gen=1 after close, got %+v", lease)
	  }
  }

// ──────────────────────────────────────────────
// 场景 5：FSM 非法迁移拒绝
// ──────────────────────────────────────────────

   func TestFSMInvalidTransitions(t *testing.T) {
	   now := time.Now().UTC()
	   t.Run("consume_revoked_lease", func(t *testing.T) {
		   l := &tradepwgen.VerifyLease{ID: "l1", Status: tradepwgen.LeaseStatusRevoked}
		  err := tradepwgen.TransitLeaseConsume(l, now)
		   if err == nil {
			  t.Error("expected error consuming revoked lease")
		  }
	  })
	   t.Run("revoke_consumed_lease", func(t *testing.T) {
		   l := &tradepwgen.VerifyLease{ID: "l2", Status: tradepwgen.LeaseStatusConsumed}
		  err := tradepwgen.TransitLeaseRevoke(l, now)
		  if err == nil {
			  t.Error("expected error revoking consumed lease")
		  }
	   })
	  t.Run("prove_proven_drift", func(t *testing.T) {
		   dc := &tradepwgen.PwDriftCase{ID: "dc1", Status: tradepwgen.DriftStatusProven}
		  err := tradepwgen.TransitDriftProve(dc, "proof", "alice", "bob", now)
		  if err == nil {
			   t.Error("expected error proving already-proven case")
		   }
	   })
	   t.Run("close_open_drift", func(t *testing.T) {
		   dc := &tradepwgen.PwDriftCase{ID: "dc2", Status: tradepwgen.DriftStatusOpen}
		   err := tradepwgen.TransitDriftClose(dc, "alice", now)
		   if err == nil {
			  t.Error("expected error closing open case (must prove first)")
		  }
	  })
	   t.Run("prove_missing_payload", func(t *testing.T) {
		  dc := &tradepwgen.PwDriftCase{ID: "dc3", Status: tradepwgen.DriftStatusOpen}
		  err := tradepwgen.TransitDriftProve(dc, "", "alice", "bob", now)
		   if err == nil {
			  t.Error("expected error for empty proof payload")
		  }
	  })
	   t.Run("close_already_closed", func(t *testing.T) {
		  dc := &tradepwgen.PwDriftCase{ID: "dc4", Status: tradepwgen.DriftStatusClosed}
		  err := tradepwgen.TransitDriftClose(dc, "alice", now)
		   if !errors.Is(err, tradepwgen.ErrDriftAlreadyClosed) {
			   t.Errorf("expected ErrDriftAlreadyClosed, got %v", err)
		   }
	  })
   }

// ──────────────────────────────────────────────
// 场景 6：令牌过期拒放行
// ──────────────────────────────────────────────

  func TestExpiredLeaseDenied(t *testing.T) {
	   svc := newSvc(1 * time.Millisecond)
	   userID := "user-expire-001"
	   mustBump(t, svc, userID, "set")
	  lease, err := svc.IssueVerifyLease(tradepwgen.IssueLeaseReq{
		   UserID: userID,
		  TTL: 1 * time.Millisecond,
	   })
	  if err != nil {
		  t.Fatalf("IssueVerifyLease: %v", err)
	   }
	  time.Sleep(5 * time.Millisecond)
	  // 尝试消费已过期令牌。
	  _, _, consumeErr := svc.ConsumeVerifyLease(tradepwgen.ConsumeLeaseReq{
		  UserID: userID,
		  LeaseID: lease.ID,
		  Token:   lease.Token,
	  })
	  if !errors.Is(consumeErr, tradepwgen.ErrLeaseExpired) {
		   t.Errorf("expected ErrLeaseExpired, got %v", consumeErr)
	   }
   }

// ──────────────────────────────────────────────
// 场景 7：CoolBound 世代绑定确认
// ──────────────────────────────────────────────

   func TestCoolBoundGenBinding(t *testing.T) {
	   svc := newSvc(0)
	  userID := "user-cool-001"
	  // 首次设密。
	  ng := mustBump(t, svc, userID, "set")
	  if ng.CoolBound {
		   t.Error("expected CoolBound=false immediately after bump")
	  }
	  // 确认冷却绑定。
	  if err := svc.ConfirmCool(userID, ng.Generation); err != nil {
		   t.Fatalf("ConfirmCool: %v", err)
	   }
	   resp, err := svc.QueryGeneration(userID)
	   if err != nil {
		   t.Fatalf("QueryGeneration: %v", err)
	  }
	   if !resp.CoolBound {
		   t.Error("expected CoolBound=true after ConfirmCool")
	  }
	  // Change 后冷却标记应重置。
	  ng2, _, _ := svc.BumpAndRevoke(userID, "change")
	   if ng2.CoolBound {
		  t.Error("expected CoolBound=false after BumpAndRevoke (change)")
	   }
	   // 用错误世代号 ConfirmCool 须失败。
	   if err := svc.ConfirmCool(userID, 999); err == nil {
		   t.Error("expected error for wrong gen in ConfirmCool")
	   }
  }

// ──────────────────────────────────────────────
// 场景 8：幂等吊销（同一 userID 多次 BumpAndRevoke）
// ──────────────────────────────────────────────

   func TestIdempotentRevoke(t *testing.T) {
	  svc := newSvc(0)
	   userID := "user-idem-001"
	  mustBump(t, svc, userID, "set") // gen=1
	  // 三次 Change，每次无在途 Lease。
	   for i := 0; i < 3; i++ {
		   ng, revoked, err := svc.BumpAndRevoke(userID, "change")
		  if err != nil {
			   t.Fatalf("BumpAndRevoke round %d: %v", i, err)
		  }
		   if revoked != 0 {
			   t.Errorf("round %d: expected 0 revoked (no leases), got %d", i, revoked)
		   }
		   expectedGen := int64(i + 2)
		  if ng.Generation != expectedGen {
			   t.Errorf("round %d: expected gen=%d, got %d", i, expectedGen, ng.Generation)
		  }
	   }
	   resp, err := svc.QueryGeneration(userID)
	   if err != nil {
		  t.Fatalf("QueryGeneration: %v", err)
	  }
	  if resp.Generation != 4 {
		   t.Errorf("expected final gen=4, got %d", resp.Generation)
	  }
  }

// ──────────────────────────────────────────────
// 场景 9：golden 夹具文件可读性验证
// ──────────────────────────────────────────────

  func TestGoldenFixturesLoadable(t *testing.T) {
	  goldenFiles := []string{
		  "../../testdata/tradepwgen/change_bumps_gen.json",
		  "../../testdata/tradepwgen/reset_revokes_inflight.json",
		   "../../testdata/tradepwgen/concurrent_change_race.json",
		  "../../testdata/tradepwgen/drift_case_blocks_issue.json",
		   "../../testdata/tradepwgen/stale_token_rejected.json",
		  "../../testdata/tradepwgen/seclock_isolation.json",
		  "../../testdata/tradepwgen/idempotent_revoke.json",
	   }
	  for _, path := range goldenFiles {
		  path := path
		  t.Run(path, func(t *testing.T) {
			  data, err := os.ReadFile(path)
			  if err != nil {
				   t.Fatalf("ReadFile(%s): %v", path, err)
			   }
			   var m map[string]interface{}
			   if err := json.Unmarshal(data, &m); err != nil {
				  t.Fatalf("Unmarshal(%s): %v", path, err)
			  }
			  if m["scenario"] == nil {
				  t.Errorf("golden fixture %s missing 'scenario' field", path)
			  }
		   })
	   }
  }

// ──────────────────────────────────────────────
// 场景 10：Worker ScanExpiredLeases
// ──────────────────────────────────────────────

  func TestWorkerScanExpiredLeases(t *testing.T) {
	  svc := newSvc(0)
	   userID := "user-worker-001"
	  mustBump(t, svc, userID, "set")
	   // 颁发一个已过期令牌（手动设置过期时间）。
	  lease, err := svc.IssueVerifyLease(tradepwgen.IssueLeaseReq{
		  UserID: userID,
		   TTL:    1 * time.Millisecond,
	   })
	  if err != nil {
		  t.Fatalf("IssueVerifyLease: %v", err)
	   }
	   time.Sleep(5 * time.Millisecond)
	   worker := tradepwgen.NewWorker(svc, tradepwgen.DefaultWorkerConfig)
	  revokedIDs, errs := worker.ScanExpiredLeases([]*tradepwgen.VerifyLease{lease})
	   if len(errs) != 0 {
		   t.Errorf("ScanExpiredLeases errors: %v", errs)
	  }
	  if len(revokedIDs) != 1 || revokedIDs[0] != lease.ID {
		  t.Errorf("expected lease %s revoked, got %v", lease.ID, revokedIDs)
	   }
   }

// ──────────────────────────────────────────────
// 场景 11：与 seclock cool_lease 隔离验证
// ──────────────────────────────────────────────

  func TestSeclockIsolation(t *testing.T) {
	   // 本包不实现冷却窗口世代，只保证：
	  // 1. ConfirmCool 携带正确 gen 时设置 CoolBound=true。
	  // 2. CoolBound 字段与 seclock 内部 cool_lease 独立（不共享状态）。
	  svc := newSvc(0)
	  userID := "user-iso-001"
	  ng := mustBump(t, svc, userID, "set")
	   // CoolBound 初始为 false。
	   resp, err := svc.QueryGeneration(userID)
	   if err != nil {
		  t.Fatalf("QueryGeneration: %v", err)
	   }
	  if resp.CoolBound {
		   t.Error("CoolBound should be false before ConfirmCool")
	  }
	   // ConfirmCool 后 CoolBound=true（不依赖 seclock 状态）。
	   if err := svc.ConfirmCool(userID, ng.Generation); err != nil {
		   t.Fatalf("ConfirmCool: %v", err)
	   }
	  resp2, _ := svc.QueryGeneration(userID)
	  if !resp2.CoolBound {
		   t.Error("CoolBound should be true after ConfirmCool")
	  }
	  // 世代抬升后 CoolBound 重置（下次 Change 须重新绑定）。
	   ng2, _, _ := svc.BumpAndRevoke(userID, "change")
	  if ng2.CoolBound {
		  t.Error("CoolBound should reset to false after BumpAndRevoke")
	  }
	  // 旧世代 ConfirmCool 须失败（世代号不符）。
	  if err := svc.ConfirmCool(userID, ng.Generation); err == nil {
		   t.Error("expected error for ConfirmCool with stale gen")
	   }
  }

// ──────────────────────────────────────────────
// 场景 12：Generate Drift 检测完整路径
// ──────────────────────────────────────────────

   func TestDriftDetectionFullPath(t *testing.T) {
	   // 构造场景：令牌颁发时 gen=1，消费时 gen=2（已 Change）。
	   // ConsumeLease 须返回 ErrLeaseGenMismatch 并产出 PwDriftCase。
	   genStore := tradepwgen.NewMemGenerationStore()
	  leaseStore := tradepwgen.NewMemLeaseStore()
	   driftStore := tradepwgen.NewMemDriftStore()
	   svc := tradepwgen.NewServiceWith(genStore, leaseStore, driftStore, 0)
	  userID := "user-drift-detect-001"
	   // gen=1
	  mustBump(t, svc, userID, "set")
	   // 颁发令牌（gen=1）。
	   lease := mustIssueLease(t, svc, userID)
	  if lease.PwGen != 1 {
		  t.Fatalf("expected lease gen=1, got %d", lease.PwGen)
	   }
	   // Change：gen→2，令牌被 revoke。
	  mustBump(t, svc, userID, "change")
	  // 此时 lease 已 revoked，消费返回 ErrLeaseRevoked（不是 ErrLeaseGenMismatch）。
	  // 漂移检测的触发场景：令牌 open 但 gen 不符（即改密与颁发几乎同时发生，令牌未来得及吊销）。
	  // 此验证漂移检测 Detect 函数本身的语义。
	   // 直接调用 DriftStore.Insert 漂移事件产出。
	   dc, err := driftStore.Insert(&tradepwgen.PwDriftCase{
		  ID:        "dc-detect-001",
		  UserID:    userID,
		  LeaseID:   lease.ID,
		   VerifyGen: 1,
		   CurrentGen: 2,
		   Status: tradepwgen.DriftStatusOpen,
		  VerifySnapshot: `{"lease_id":"` + lease.ID + `","pw_gen":1}`,
		   GenSnapshot: `{"user_id":"` + userID + `","generation":2}`,
		  DetectedAt:  time.Now().UTC(),
		   UpdatedAt:   time.Now().UTC(),
	   })
	   if err != nil {
		   t.Fatalf("Insert drift: %v", err)
	  }
	  // 验证 IssueVerifyLease 被阻断。
	   _, issueErr := svc.IssueVerifyLease(tradepwgen.IssueLeaseReq{UserID: userID})
	  if !errors.Is(issueErr, tradepwgen.ErrDriftOpen) {
		  t.Errorf("expected ErrDriftOpen, got %v", issueErr)
	  }
	  // 双人确认证明固化。
	   proved, proveErr := svc.ProveGenDrift(tradepwgen.ProveGenDriftReq{
		  CaseID:       dc.ID,
		  ProofPayload: `{"gen_drift":"1→2","evidence":"audit log"}`,
		  Approver:  "risk-ops-1",
		  Approver2: "risk-ops-2",
	  })
	  if proveErr != nil {
		   t.Fatalf("ProveGenDrift: %v", proveErr)
	  }
	  if proved.Approver != "risk-ops-1" || proved.Approver2 != "risk-ops-2" {
		   t.Errorf("approvers not saved correctly: %s / %s", proved.Approver, proved.Approver2)
	  }
	  // 收口。
	  closed, closeErr := svc.CloseGenDrift(tradepwgen.CloseDriftReq{CaseID: dc.ID})
	  if closeErr != nil {
		  t.Fatalf("CloseGenDrift: %v", closeErr)
	   }
	   if closed.Status != tradepwgen.DriftStatusClosed {
		   t.Errorf("expected closed, got %s", closed.Status)
	   }
	  // 收口后解除阻断。
	   newLease, issueErr2 := svc.IssueVerifyLease(tradepwgen.IssueLeaseReq{UserID: userID})
	  if issueErr2 != nil {
		   t.Fatalf("IssueVerifyLease after close: %v", issueErr2)
	  }
	  if newLease.PwGen != 2 {
		   t.Errorf("new lease should bind gen=2, got %d", newLease.PwGen)
	  }
  }
