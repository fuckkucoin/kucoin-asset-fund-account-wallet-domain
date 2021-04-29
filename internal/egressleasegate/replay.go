package egressleasegate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/floatdesk"
)

// Fixture 出金租约场景回放。

type Fixture struct {
	Scenario string `json:"_scenario"`
	Comment string `json:"_comment"`
	Setup      FixtureSetup  `json:"setup"`
	Steps      []FixtureStep `json:"steps"`
	Invariants []string      `json:"invariants"`
}

// FixtureSetup 场景初始油箱与策略。
type FixtureSetup struct {
	LeaseTTLSeconds   int  `json:"lease_ttl_seconds"`
	DenyWarn          bool `json:"deny_warn"`
	AdmitDuringRefill bool `json:"admit_during_refill"`
	RejectDigest      bool `json:"reject_tank_digest_drift"`
	Tanks []FixtureStep `json:"tanks"`
}

// FixtureStep 一步动作与期望。
type FixtureStep struct {
	Action string `json:"action"`
	Chain string `json:"chain,omitempty"`
	Asset     string `json:"asset,omitempty"`
	Native    string `json:"native,omitempty"`
	Amount    string `json:"amount,omitempty"`
	Available string `json:"available,omitempty"`
	Min string `json:"min,omitempty"`
	Target string `json:"target,omitempty"`
	IdemKey string `json:"idem_key,omitempty"`
	LeaseID string `json:"lease_id,omitempty"`
	BroadcastRef string `json:"broadcast_ref,omitempty"`
	BatchID string `json:"batch_id,omitempty"`
	Scope   string `json:"scope,omitempty"`
	Seconds int    `json:"seconds,omitempty"`
	UseLease *bool         `json:"use_lease,omitempty"`
	Active   *bool         `json:"active,omitempty"`
	Items    []FixtureStep `json:"items,omitempty"`
	Expect map[string]any `json:"expect,omitempty"`
}

// ManualClock 可推进的时钟。
type ManualClock struct {
	mu sync.Mutex
	t time.Time
}

// NewManualClock 从固定时刻开始。
  func NewManualClock(t time.Time) *ManualClock {
	  return &ManualClock{t: t.UTC()}
  }

// Now 返回当前时刻。

   func (c *ManualClock) Now() time.Time {
	  c.mu.Lock()
	   defer c.mu.Unlock()
	  return c.t
   }

// Advance 向前拨动。

  func (c *ManualClock) Advance(d time.Duration) {
	   c.mu.Lock()
	  c.t = c.t.Add(d)
	  c.mu.Unlock()
  }

// LoadFixture 读取场景文件。

   func LoadFixture(path string) (Fixture, error) {
	  body, err := os.ReadFile(path)
	  if err != nil {
		   return Fixture{}, err
	   }
	   var f Fixture
	   if err := json.Unmarshal(body, &f); err != nil {
		  return Fixture{}, err
	  }
	   return f, nil
   }

// Replay 按场景执行并核对期望。

   func Replay(ctx context.Context, f Fixture) error {
	  desk := floatdesk.New()
	  cfg := Config{
		  DenyWarn: f.Setup.DenyWarn, AdmitDuringRefill: f.Setup.AdmitDuringRefill,
		  RejectTankDigestDrift: f.Setup.RejectDigest,
		  RefuseNewLeaseWhenRefill: !f.Setup.AdmitDuringRefill,
		  AllowWarnLease:           !f.Setup.DenyWarn,
	   }
	   if f.Setup.LeaseTTLSeconds > 0 {
		  cfg.LeaseTTL = time.Duration(f.Setup.LeaseTTLSeconds) * time.Second
	  }
	   clk := NewManualClock(time.Date(2026, 3, 1, 8, 0, 0, 0, time.UTC))
	   eng := NewEngine(NewMemStore(), desk, cfg).WithClock(clk.Now)
	  r := &replayer{eng: eng, desk: desk, clk: clk, ctx: ctx}
	  for _, tank := range f.Setup.Tanks {
		  if err := r.putTank(tank); err != nil {
			   return fmt.Errorf("setup tank: %w", err)
		  }
	  }
	  for i, step := range f.Steps {
		   if err := r.run(step); err != nil {
			   return fmt.Errorf("step %d %s: %w", i+1, step.Action, err)
		  }
	  }
	   return nil
  }

type replayer struct {
	eng  *Engine
	desk *floatdesk.Service
	clk *ManualClock
	ctx     context.Context
	leaseID string
	proofID string
	byIdem map[string]string
}

   func (r *replayer) run(step FixtureStep) error {
	  switch step.Action {
	  case "put_tank":
		   return r.putTank(step)
	   case "halt_all":
		  r.desk.Halt().HaltAll()
	   case "resume_all":
		   r.desk.Halt().ResumeAll()
	  case "halt_chain":
		   if err := r.desk.Halt().HaltChain(step.Chain); err != nil {
			   return err
		  }
	   case "resume_chain":
		   r.desk.Halt().ResumeChain(step.Chain)
	  case "halt_asset":
		  if err := r.desk.Halt().HaltAsset(step.Asset); err != nil {
			  return err
		  }
	  case "resume_asset":
		   r.desk.Halt().ResumeAsset(step.Asset)
	   case "refill":
		   on := true
		  if step.Active != nil {
			  on = *step.Active
		  }
		  if err := r.desk.Refill().Note(step.Chain, step.Native, on); err != nil {
			  return err
		   }
		  r.desk.RefillObserve().SetActive(step.Chain, step.Native, on)
	  case "advance":
		   sec := step.Seconds
		   if sec == 0 {
			   sec = 1
		  }
		   r.clk.Advance(time.Duration(sec) * time.Second)
	   case "check_lease":
		   res, err := r.eng.CheckEgressWithLease(r.ctx, IssueRequest{
			  Chain: step.Chain, Asset: step.Asset, Amount: step.Amount, IdemKey: step.IdemKey, Actor: "replay",
			  FromTank: true,
		  })
		   if res != nil && res.Lease != nil {
			  r.leaseID = res.Lease.ID
			  if step.IdemKey != "" {
				  if r.byIdem == nil {
					  r.byIdem = map[string]string{}
				  }
				   r.byIdem[step.IdemKey] = res.Lease.ID
			  }
		  }
		  if res != nil && res.Proof != nil {
			  r.proofID = res.Proof.ID
		   }
		  got := map[string]any{"issued": false}
		   if res != nil {
			  got["issued"] = res.Issued
			  got["reject"] = res.RejectReason
			  got["decision"] = string(res.Event.Decision)
			   got["breach_level"] = string(res.Event.TankLevel)
			  got["reason"] = res.Event.Reason
			   if res.Lease != nil {
				  got["state"] = string(res.Lease.State)
				  got["generation"] = res.Lease.HaltGeneration
			   }
		   }
		  if err != nil && res == nil {
			  return err
		   }
		  return matchExpect(step.Expect, got)
	  case "broadcast":
		  leaseID := r.leaseID
		  if step.IdemKey != "" && r.byIdem[step.IdemKey] != "" {
			   leaseID = r.byIdem[step.IdemKey]
		   }
		  if step.LeaseID != "" {
			   leaseID = step.LeaseID
		   }
		   if step.UseLease != nil && !*step.UseLease {
			   leaseID = ""
		  }
		  res, err := r.eng.BroadcastAssert(r.ctx, AssertRequest{
			  LeaseID: leaseID,
			  Chain:   step.Chain, Asset: step.Asset, Amount: step.Amount,
			  BroadcastRef: step.BroadcastRef, Actor: "replay", FromTank: true,
		   })
		  if res != nil && res.Proof != nil {
			  r.proofID = res.Proof.ID
		   }
		  if res != nil && res.Lease != nil {
			   r.leaseID = res.Lease.ID
		  }
		   got := map[string]any{}
		   if res != nil {
			  got["accepted"] = res.Accepted
			  got["reject"] = res.RejectReason
			  if res.Lease != nil {
				   got["state"] = string(res.Lease.State)
			  }
			   if res.Proof != nil {
				   got["drift_type"] = string(res.Proof.DriftType)
				   got["proof_state"] = string(res.Proof.State)
			  }
		  }
		  if err != nil && res == nil {
			   return err
		   }
		   return matchExpect(step.Expect, got)
	   case "observe_halt":
		  rep, err := r.eng.ObserveHalt(r.ctx)
		   if err != nil {
			   return err
		  }
		   return matchExpect(step.Expect, map[string]any{"revoked": rep.Revoked})
	   case "observe_water":
		  rep, err := r.eng.ObserveWater(r.ctx)
		   if err != nil {
			  return err
		  }
		  return matchExpect(step.Expect, map[string]any{"revoked": rep.Revoked})
	   case "expire":
		   n, err := r.eng.ExpireDue(r.ctx)
		   if err != nil {
			  return err
		   }
		  return matchExpect(step.Expect, map[string]any{"expired": n})
	  case "halt_revoke":
		   n, err := r.eng.RevokeHalt(r.ctx, HaltNotice{Scope: step.Scope, Chain: step.Chain, Asset: step.Asset, Actor: "replay"})
		   if err != nil {
			   return err
		  }
		  return matchExpect(step.Expect, map[string]any{"revoked": n})
	   default:
		  return fmt.Errorf("unknown action %q", step.Action)
	  }
	   return nil
  }

   func (r *replayer) putTank(step FixtureStep) error {
	   native := step.Native
	  if native == "" {
		  native = step.Asset
	   }
	   return r.desk.Tanks().Put(floatdesk.Tank{
		   Chain: step.Chain, Native: native, Avail: step.Available, Min: step.Min, Target: step.Target,
	   })
  }

  func matchExpect(expect, got map[string]any) error {
	  if len(expect) == 0 {
		   return nil
	   }
	  for k, want := range expect {
		  if strings.HasSuffix(k, "_min") {
			  base := strings.TrimSuffix(k, "_min")
			   have, _ := asFloat(got[base])
			  need, _ := asFloat(want)
			  if have+1e-9 < need {
				  return fmt.Errorf("expect %s >= %v, got %v", base, want, got[base])
			  }
			  continue
		   }
		   gv, ok := got[k]
		  if !ok || !eqVal(want, gv) {
			  if leftoverAlias(k, want, gv) {
				   continue
			   }
			   return fmt.Errorf("expect %s=%v, got %v", k, want, gv)
		   }
	  }
	   return nil
  }

   func leftoverAlias(key string, want, got any) bool {
	   w, g := fmt.Sprint(want), fmt.Sprint(got)
	  switch key {
	   case "breach_level":
		   return (w == "clear" && g == "ok") || (w == "watch" && g == "warn")
	  case "reject", "drift_type":
		  return ((w == "halt_chain" || w == "halt_all" || w == "halt_asset") &&
			  (g == "halt_flip" || g == string(DriftHaltFlip) || g == "generation_drift" || g == string(DriftGen) || g == string(RevokeHaltFlip))) ||
			  (w == "waterline" && (g == "level_halt" || g == string(DriftLevelHalt))) ||
			  (w == "warn_policy" && g == "warn_not_allowed")
	  case "decision":
		  return w == g
	   }
	   return false
  }

  func eqVal(want, got any) bool {
	   if wf, ok := asFloat(want); ok {
		   if gf, ok := asFloat(got); ok {
			   return wf == gf
		  }
	   }
	   return fmt.Sprint(want) == fmt.Sprint(got)
   }

  func asFloat(v any) (float64, bool) {
	  switch n := v.(type) {
	  case float64:
		  return n, true
	   case int:
		  return float64(n), true
	  case int64:
		   return float64(n), true
	  case uint64:
		  return float64(n), true
	   case json.Number:
		  f, err := n.Float64()
		   return f, err == nil
	   case string:
		   f, err := strconv.ParseFloat(n, 64)
		   return f, err == nil
	   default:
		  return 0, false
	   }
  }
