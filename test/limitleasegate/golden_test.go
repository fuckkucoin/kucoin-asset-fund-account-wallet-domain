package limitleasegate_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"account-assets.kucoin.net/funding/internal/limitleasegate"
	"account-assets.kucoin.net/funding/internal/policy"
	"account-assets.kucoin.net/funding/internal/usage"
)

// ─── Golden 夹具 schema ───────────────────────────────────────────────────
// goldenFixture 从 testdata/ JSON 文件读取的夹具结构。

type goldenFixture struct {
	Comment string `json:"_comment"`
	Scenario string      `json:"_scenario"`
	Setup    goldenSetup `json:"setup"`
	Steps []goldenStep `json:"steps"`
	Invariants []string `json:"invariants"`
}

type goldenSetup struct {
	UserID                  string `json:"user_id"`
	Asset                   string `json:"asset"`
	Kind                    string `json:"kind"`
	Amount                  string `json:"amount"`
	Allowlisted             bool   `json:"allowlisted"`
	AllowCommitWithoutLease bool   `json:"allow_commit_without_lease"`
	Policy *goldenPolicy `json:"policy,omitempty"`
}

type goldenPolicy struct {
	Version int `json:"version"`
	MaxSingle string `json:"max_single"`
	DailyWithdraw string `json:"daily_withdraw"`
	MinAmount     string `json:"min_amount"`
	MaxVelocity   int    `json:"max_velocity"`
	VelocityWindowSec int `json:"velocity_window_sec"`
	Allowlist bool `json:"allowlist"`
}

type goldenStep struct {
	Step   int    `json:"step"`
	Action string `json:"action"`
	CommitRef string `json:"commit_ref,omitempty"`
	LeaseID string `json:"lease_id,omitempty"`
	Allowlisted bool `json:"allowlisted,omitempty"`
	Expect map[string]interface{} `json:"expect,omitempty"`
}

// ─── 夹具执行器 ───────────────────────────────────────────────────────────
// goldenRunner 夹具执行状态。
type goldenRunner struct {
	t       *testing.T
	eng     *limitleasegate.Engine
	store   *limitleasegate.MemStore
	reg     *policy.Registry
	use     *usage.Store
	fixture goldenFixture
	leaseID string // 签发后记录的 lease_id，供后续步骤使用
}

// loadFixture 从 testdata/ 加载夹具。

  func loadFixture(t *testing.T, name string) goldenFixture {
	  t.Helper()
	  path := filepath.Join("..", "..", "testdata", "limitleasegate", name)
	  data, err := os.ReadFile(path)
	   if err != nil {
		  t.Fatalf("load fixture %q: %v", path, err)
	  }
	  var f goldenFixture
	   if err := json.Unmarshal(data, &f); err != nil {
		  t.Fatalf("parse fixture %q: %v", path, err)
	  }
	   return f
   }

// newGoldenRunner 从夹具创建执行器。

   func newGoldenRunner(t *testing.T, fixture goldenFixture) *goldenRunner {
	  t.Helper()
	  e := newTestEnv(t, limitleasegate.Config{
		  LeaseTTL:                120e9, // 120s in nanoseconds
		   AllowCommitWithoutLease: fixture.Setup.AllowCommitWithoutLease,
	   })
	  // 如果夹具提供策略覆盖，更新 Registry。
	   if fixture.Setup.Policy != nil && fixture.Setup.Policy.Version > 0 {
		  p := fixture.Setup.Policy
		   // 先清空默认 V1（注入 V1 时会冲突），改用代次号大于已有代次。
		   // 若夹具代次=1，跳过（已有）；>1 则直接发布。
		   if p.Version > 1 {
			   _ = e.reg.Publish(policy.Doc{
				  Version:       p.Version,
				   MaxSingle:     p.MaxSingle,
				   DailyWithdraw: p.DailyWithdraw,
				  MinAmount:     p.MinAmount,
				   MaxVelocity:   p.MaxVelocity,
				   Allowlist: p.Allowlist,
			   })
		  }
	  }
	  return &goldenRunner{
		   t: t,
		   eng: e.eng,
		  store: e.store,
		  reg:   e.reg,
		   use:     e.use,
		   fixture: fixture,
	  }
  }

// run 按顺序执行夹具步骤。

   func (r *goldenRunner) run() {
	   t := r.t
	  ctx := context.Background()
	  f := r.fixture
	  for _, step := range f.Steps {
		   switch step.Action {
		   case "IssueCheckLease":
			  result, err := r.eng.IssueCheckLease(ctx, limitleasegate.IssueRequest{
				  UserID:      f.Setup.UserID,
				   Asset:       f.Setup.Asset,
				   Kind:        f.Setup.Kind,
				   Amount:      f.Setup.Amount,
				  Allowlisted: f.Setup.Allowlisted,
			   })
			  if exp, ok := step.Expect["allowed"].(bool); ok {
				   if result != nil && result.Allowed != exp {
					   t.Errorf("step %d IssueCheckLease: Allowed=%v, want %v", step.Step, result.Allowed, exp)
				  }
			   }
			  if err == nil && result != nil && result.Lease != nil {
				   r.leaseID = result.Lease.ID
				   if exp, ok := step.Expect["lease_state"].(string); ok {
					   if string(result.Lease.State) != exp {
						  t.Errorf("step %d IssueCheckLease: state=%q, want %q", step.Step, result.Lease.State, exp)
					  }
				  }
			   }
		  case "PublishPolicyV2":
			  // 发布策略 V2（policy_flip 场景）。
			   _ = r.reg.Publish(policy.Doc{
				  Version: 2, MaxSingle: "5000", DailyWithdraw: "15000",
				   MinAmount: "0.00000001", MaxVelocity: 20, Allowlist: true,
			   })
		   case "BindLease":
			   bound, bindErr := r.eng.BindLease(ctx, limitleasegate.BindRequest{
				  LeaseID: r.leaseID,
				  UserID: f.Setup.UserID,
				  Asset: f.Setup.Asset,
				   Kind: f.Setup.Kind,
				   CommitRef: step.CommitRef,
			  })
			   if bindErr != nil {
				   t.Errorf("step %d BindLease: unexpected error: %v", step.Step, bindErr)
			   }
			  if bound != nil {
				  if exp, ok := step.Expect["lease_state"].(string); ok {
					  if string(bound.State) != exp {
						  t.Errorf("step %d BindLease: state=%q, want %q", step.Step, bound.State, exp)
					   }
				  }
			   }
		   case "CommitRecheck":
			  leaseID := r.leaseID
			  if step.LeaseID != "" {
				   leaseID = step.LeaseID
			  }
			   result, _ := r.eng.CommitRecheck(ctx, limitleasegate.CommitRecheckRequest{
				   LeaseID: leaseID,
				   UserID: f.Setup.UserID,
				   Asset:  f.Setup.Asset,
				   Kind:   f.Setup.Kind,
				   CommitRef:   step.CommitRef,
				   Allowlisted: step.Allowlisted || f.Setup.Allowlisted,
			   })
			  if exp, ok := step.Expect["allowed"].(bool); ok && result != nil {
				  if result.Allowed != exp {
					  t.Errorf("step %d CommitRecheck: Allowed=%v, want %v (reason=%q)",
						   step.Step,
						  result.Allowed, exp, result.RejectReason)
				   }
			  }
			   if expReason, ok := step.Expect["reject_reason"].(string); ok && result != nil {
				   if result.RejectReason != expReason {
					  t.Errorf("step %d CommitRecheck: RejectReason=%q, want %q", step.Step, result.RejectReason, expReason)
				   }
			  }
			  if expState, ok := step.Expect["lease_state"].(string); ok && result != nil && result.Lease != nil {
				   if string(result.Lease.State) != expState {
					  t.Errorf("step %d CommitRecheck: lease_state=%q, want %q", step.Step, result.Lease.State, expState)
				   }
			  }
			  if driftExpect, ok := step.Expect["drift_proof"].(map[string]interface{}); ok {
				   if driftExpect == nil {
					   // 期望无证明。
					   if result != nil && result.DriftProof != nil {
						   t.Errorf("step %d CommitRecheck: unexpected DriftProof: %+v", step.Step, result.DriftProof)
					  }
				  } else {
					   if result == nil || result.DriftProof == nil {
						   t.Errorf("step %d CommitRecheck: expected DriftProof, got nil", step.Step)
					   } else {
						  if expDT, ok := driftExpect["drift_type"].(string); ok {
							  if string(result.DriftProof.DriftType) != expDT {
								   t.Errorf("step %d DriftType=%q, want %q", step.Step, result.DriftProof.DriftType, expDT)
							   }
						  }
						   if expState, ok := driftExpect["state"].(string); ok {
							  if string(result.DriftProof.State) != expState {
								   t.Errorf("step %d DriftProof.State=%q, want %q", step.Step, result.DriftProof.State, expState)
							  }
						   }
					   }
				   }
			   }
		   case "InjectUsage":
			   // 从夹具 JSON 注入 usage 命中（近似）。
			   for i := 0; i < 5; i++ {
				  _ = r.use.Record(usage.KindWithdraw, f.Setup.UserID, f.Setup.Asset, "dest-fill", "2990")
			   }
		  case "InjectVelocity":
			   // 注入速度命中。
			   for i := 0; i < 20; i++ {
				   _ = r.use.Record(usage.KindWithdraw, f.Setup.UserID, f.Setup.Asset, "dest-vel", "1")
			  }
		   default:
			  t.Logf("step %d: unknown action %q, skipping", step.Step, step.Action)
		   }
	   }
   }

// ─── Golden 主入口 ─────────────────────────────────────────────────────
// TestGoldenPolicyFlip 验收场景 1：PolicyVer 翻转 → Commit 失败 + 落证明。
   func TestGoldenPolicyFlip(t *testing.T) {
	   fixture := loadFixture(t, "golden_policy_flip.json")
	   runner := newGoldenRunner(t, fixture)
	  runner.run()
   }

// TestGoldenUsageBreach 验收场景 2：TodaySum 灌满 → Commit 拒 + 落证明。

  func TestGoldenUsageBreach(t *testing.T) {
	  fixture := loadFixture(t, "golden_usage_breach.json")
	  runner := newGoldenRunner(t, fixture)
	   runner.run()
   }

// TestGoldenNoLeaseFailClosed 验收场景 3：无 lease 占用提交必须 fail-closed。
   func TestGoldenNoLeaseFailClosed(t *testing.T) {
	   fixture := loadFixture(t, "golden_no_lease_fail_closed.json")
	   runner := newGoldenRunner(t, fixture)
	   // 无 lease 场景直接调用 CommitRecheck，不走 runner.run 步骤。
	  _, err := runner.eng.CommitRecheck(context.Background(), limitleasegate.CommitRecheckRequest{
		   LeaseID: "", UserID: fixture.Setup.UserID, Asset: fixture.Setup.Asset,
		   Kind: fixture.Setup.Kind, Allowlisted: fixture.Setup.Allowlisted,
	   })
	   if err == nil {
		   t.Error("expected error for no-lease commit, got nil")
	   }
   }

// TestGoldenNormalPath 正常路径：Check→Bind→Commit→consumed。

   func TestGoldenNormalPath(t *testing.T) {
	   fixture := loadFixture(t, "golden_normal_path.json")
	   runner := newGoldenRunner(t, fixture)
	   runner.run()
  }

// TestGoldenVelocityBreach 速度越阈：Check→InjectVelocity→Commit→revoked+proof。
   func TestGoldenVelocityBreach(t *testing.T) {
	   fixture := loadFixture(t, "golden_velocity_breach.json")
	   runner := newGoldenRunner(t, fixture)
	   runner.run()
  }
