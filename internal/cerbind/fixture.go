package cerbind

import (
	"context"
	"errors"
	"fmt"
	"time"
	"account-assets.kucoin.net/funding/internal/airgap"
	"account-assets.kucoin.net/funding/internal/custody"
	"account-assets.kucoin.net/funding/internal/keycer"
	"account-assets.kucoin.net/funding/internal/signpipe"
)

// DrillUnsigned 无绑定会话时签播闸拒绝并留下审计。
  func DrillUnsigned() error {
	  ctx := context.Background()
	   eng, err := newDrill()
	  if err != nil {
		  return err
	  }
	  snap, err := eng.Ingest(ctx, snapOf("batch-unsigned", "idem-unsigned", "20", "0xabc"))
	  if err != nil {
		  return err
	  }
	   err = eng.CheckBinding(ctx, Subject{BatchID: snap.BatchID, Chain: "ETH", Amount: "20", Dest: "0xabc"})
	  if !errors.Is(err, ErrDenied) {
		  return fmt.Errorf("unsigned gate: %v", err)
	   }
	   err = eng.CheckBinding(ctx, Subject{Idem: "idem-unsigned", Chain: "ETH", Amount: "20", Dest: "0xabc"})
	  if !errors.Is(err, ErrDenied) {
		  return fmt.Errorf("idem gate: %v", err)
	  }
	   rows, err := eng.AuditsOf(ctx, "", snap.BatchID, 20)
	   if err != nil {
		   return err
	  }
	   if !hasKind(rows, "gate_deny") {
		  return fmt.Errorf("missing gate audit")
	  }
	   if err := eng.CheckBinding(ctx, Subject{BatchID: snap.BatchID, Chain: "ETH", Amount: "999", Dest: "0xabc"}); !errors.Is(err, ErrDigest) {
		  return fmt.Errorf("amount mismatch: %v", err)
	   }
	   return nil
   }

// DrillSignHook 把闸挂到签章管线，未绑定的批次不得签成。
   func DrillSignHook() error {
	   ctx := context.Background()
	   eng, err := newDrill()
	   if err != nil {
		  return err
	  }
	  eng.Arm()
	  defer Detach()
	  if _, err := eng.Ingest(ctx, snapOf("batch-hook", "idem-hook", "20", "0xabc")); err != nil {
		   return err
	  }
	   p := signpipe.New(time.Hour)
	   job, err := p.Open("alice", "mov-hook", "ETH", "0xabc", "20")
	  if err != nil {
		   return err
	   }
	  job, err = p.Approve(job.ID, "bob", job.Fence.Epoch, job.Fence.Token, "mov-hook", "ETH", "0xabc", "20")
	   if err != nil {
		  return err
	   }
	  if err := p.AttachBatch(job.ID, "batch-hook"); err != nil {
		  return err
	  }
	  _, err = p.Sign(job.ID, "cara", job.Fence.Epoch, job.Fence.Token, func(string) (string, error) { return "aa", nil })
	   if err == nil {
		   return fmt.Errorf("sign passed without ceremony")
	   }
	   got, err := p.Get(job.ID)
	  if err != nil {
		  return err
	  }
	   if got.State != signpipe.StateApproved || got.SigHex != "" {
		   return fmt.Errorf("job mutated: %s", got.State)
	   }
	  return nil
   }

// DrillShortQuorum 份额不足时拒绝绑定，也不得进入 sign_ready。
   func DrillShortQuorum() error {
	  ctx := context.Background()
	  eng, err := newDrill()
	   if err != nil {
		   return err
	  }
	  ses, err := planOpen(ctx, eng, "batch-short", "idem-short")
	  if err != nil {
		  return err
	   }
	   ses, err = eng.Receipt(ctx, ses.ID, "alice", "slot-a", "", ses.FenceEpoch, ses.FenceToken)
	  if err != nil {
		   return err
	  }
	  if ses.State != StOpen {
		   return fmt.Errorf("state %s", ses.State)
	  }
	   if _, err := eng.Bind(ctx, ses.ID, "batch-short", "alice", ses.FenceEpoch, ses.FenceToken); !errors.Is(err, ErrQuorum) {
		  return fmt.Errorf("bind short: %v", err)
	  }
	  if _, err := eng.MarkReady(ctx, ses.ID, "alice", ses.FenceEpoch, ses.FenceToken); !errors.Is(err, ErrState) {
		  return fmt.Errorf("ready short: %v", err)
	   }
	  if _, err := eng.Receipt(ctx, ses.ID, "alice", "slot-b", "", ses.FenceEpoch, ses.FenceToken); !errors.Is(err, ErrShare) {
		  return fmt.Errorf("second slot: %v", err)
	   }
	   if err := guardLabel("xprv-1"); err == nil {
		   return fmt.Errorf("key label accepted")
	  }
	  return nil
  }

// DrillTimeout 过期后中止并释放绑定，原批次可以重新开仪式。
   func DrillTimeout() error {
	  ctx := context.Background()
	   eng, err := newDrill()
	   if err != nil {
		   return err
	  }
	   cur := time.Date(2026, 4, 1, 8, 0, 0, 0, time.UTC)
	  eng.SetClock(func() time.Time { return cur })
	   ses, err := readySession(ctx, eng, "batch-timeout", "idem-timeout")
	  if err != nil {
		   return err
	   }
	   cur = cur.Add(5 * time.Hour)
	  rep, err := eng.Tick(ctx)
	  if err != nil {
		  return err
	  }
	  if rep.TimedOut != 1 {
		  return fmt.Errorf("timed out %d", rep.TimedOut)
	  }
	  view, err := eng.View(ctx, ses.ID)
	  if err != nil {
		  return err
	  }
	  if view.Session.State != StAborted {
		  return fmt.Errorf("state %s", view.Session.State)
	  }
	  for _, b := range view.Bindings {
		   if b.State == BindActive {
			   return fmt.Errorf("binding still active")
		   }
	  }
	   if err := eng.CheckBinding(ctx, Subject{BatchID: "batch-timeout", Chain: "ETH", Amount: "20", Dest: "0xabc"}); !errors.Is(err, ErrDenied) {
		   return fmt.Errorf("gate after abort: %v", err)
	   }
	   again, err := eng.Plan(ctx, planReq("batch-timeout", "idem-timeout"))
	  if err != nil {
		   return err
	   }
	   if again.ID == ses.ID || again.State != StPlanned {
		   return fmt.Errorf("replan %+v", again.State)
	  }
	  return nil
   }

// DrillDoubleBind one 档拒绝把第二批绑到同一会话，第一批绑定仍在。
  func DrillDoubleBind() error {
	  ctx := context.Background()
	   eng, err := newDrill()
	  if err != nil {
		   return err
	   }
	   ses, err := quorum(ctx, eng, "batch-one", "idem-one")
	  if err != nil {
		  return err
	  }
	  if _, err := eng.Ingest(ctx, snapOf("batch-two", "idem-two", "20", "0xdef")); err != nil {
		   return err
	   }
	  if _, err := eng.Bind(ctx, ses.ID, "batch-one", "bob", ses.FenceEpoch, ses.FenceToken); err != nil {
		  return err
	  }
	  view, err := eng.View(ctx, ses.ID)
	  if err != nil {
		  return err
	   }
	   if _, err := eng.Bind(ctx, ses.ID, "batch-two", "bob", view.Session.FenceEpoch, view.Session.FenceToken); !errors.Is(err, ErrBindMode) {
		  return fmt.Errorf("second bind: %v", err)
	  }
	  if _, err := eng.BindingOf(ctx, "batch-one"); err != nil {
		  return err
	   }
	   if _, err := eng.BindingOf(ctx, "batch-two"); !errors.Is(err, ErrNotFound) {
		  return fmt.Errorf("second binding: %v", err)
	  }
	  return nil
   }

// DrillManyBind n:1 策略允许同一会话绑定两批，两批都要到 sign_ready 才放行。
   func DrillManyBind() error {
	  ctx := context.Background()
	   eng, err := newDrill()
	  if err != nil {
		   return err
	  }
	  if _, err := eng.PutPolicy(ctx, manyPolicy(), "dave"); err != nil {
		  return err
	   }
	   ses, err := quorum(ctx, eng, "batch-m1", "idem-m1")
	  if err != nil {
		   return err
	  }
	   if ses.BindMode != ModeMany {
		  return fmt.Errorf("mode %s", ses.BindMode)
	  }
	   if _, err := eng.Ingest(ctx, snapOf("batch-m2", "idem-m2", "20", "0xdef")); err != nil {
		  return err
	  }
	   if _, err := eng.Bind(ctx, ses.ID, "batch-m1", "bob", ses.FenceEpoch, ses.FenceToken); err != nil {
		  return err
	  }
	   view, err := eng.View(ctx, ses.ID)
	   if err != nil {
		  return err
	  }
	  if _, err := eng.Bind(ctx, ses.ID, "batch-m2", "bob", view.Session.FenceEpoch, view.Session.FenceToken); err != nil {
		  return err
	   }
	  view, err = eng.View(ctx, ses.ID)
	   if err != nil {
		  return err
	  }
	  ready, err := eng.MarkReady(ctx, ses.ID, "alice", view.Session.FenceEpoch, view.Session.FenceToken)
	  if err != nil {
		  return err
	   }
	  if ready.State != StReady {
		  return fmt.Errorf("state %s", ready.State)
	  }
	  for _, id := range []string{"batch-m1", "batch-m2"} {
		   if err := eng.CheckBinding(ctx, Subject{BatchID: id, Chain: "ETH", Amount: "20", Dest: destOf(id)}); err != nil {
			  return fmt.Errorf("%s: %v", id, err)
		  }
	   }
	  return nil
   }

  func destOf(batch string) string {
	   if batch == "batch-m2" {
		   return "0xdef"
	  }
	   return "0xabc"
  }

// DrillAbort 中止后不留半绑，签播拒绝，批次可再计划。

  func DrillAbort() error {
	   ctx := context.Background()
	   eng, err := newDrill()
	   if err != nil {
		   return err
	   }
	  ses, err := readySession(ctx, eng, "batch-abort", "idem-abort")
	  if err != nil {
		  return err
	   }
	   if err := eng.CheckBinding(ctx, Subject{BatchID: "batch-abort", Chain: "ETH", Amount: "20", Dest: "0xabc"}); err != nil {
		  return err
	  }
	   got, err := eng.Abort(ctx, ses.ID, "dave", "desk")
	   if err != nil {
		  return err
	  }
	   if got.State != StAborted {
		   return fmt.Errorf("state %s", got.State)
	   }
	  if _, err := eng.BindingOf(ctx, "batch-abort"); !errors.Is(err, ErrNotFound) {
		  return fmt.Errorf("binding left: %v", err)
	   }
	   if err := eng.CheckBinding(ctx, Subject{BatchID: "batch-abort", Chain: "ETH", Amount: "20", Dest: "0xabc"}); !errors.Is(err, ErrDenied) {
		  return fmt.Errorf("gate: %v", err)
	  }
	   again, err := eng.Plan(ctx, planReq("batch-abort", "idem-abort"))
	   if err != nil {
		   return err
	   }
	   if again.State != StPlanned || again.ID == ses.ID {
		   return fmt.Errorf("replan failed")
	   }
	  return nil
  }

// DrillExec 打开仪式时把气隙、解锁工单和相位簿接上，且不写入密钥字节。
  func DrillExec() error {
	  ctx := context.Background()
	  eng, err := newDrill()
	  if err != nil {
		  return err
	   }
	   air := airgap.New()
	  key := keycer.New()
	   book := custody.NewCeremonyBook()
	   eng.UseExec(Exec{Air: air, Key: key, Cus: book})
	  ses, err := eng.Plan(ctx, planReq("batch-exec", "idem-exec"))
	  if err != nil {
		   return err
	  }
	   opened, err := eng.Open(ctx, ses.ID, "alice", ses.FenceEpoch, ses.FenceToken)
	   if err != nil {
		   return err
	  }
	   if opened.State != StOpen || opened.AirgapID == "" || opened.KeycerTicket == "" || opened.CustodyID == "" {
		   return fmt.Errorf("exec ids missing")
	  }
	   ag, err := air.Get(opened.AirgapID)
	   if err != nil {
		   return err
	  }
	   if ag.ExternalRef != "batch-exec" {
		  return fmt.Errorf("airgap ref %s", ag.ExternalRef)
	   }
	   return nil
   }

// DrillFloor 出厂底档对大额 ETH 冷钱包把阈值抬到 3；热钱包豁免；制裁地址强制 3。
   func DrillFloor() error {
	  ctx := context.Background()
	   eng := New(NewMem())
	  dec, err := eng.Eval(ctx, PlanReq{
		   BatchID: "batch-eth", Asset: "USDT", Chain: "ETH", Amount: "10000", Dest: "0xabc", Side: SideCold,
	  })
	  if err != nil {
		  return err
	  }
	  if !dec.Required || dec.Threshold != 3 || dec.PolicyID != polFloor {
		   return fmt.Errorf("floor %+v", dec)
	  }
	   hot, err := eng.Eval(ctx, PlanReq{
		  BatchID: "batch-hot", Asset: "USDT", Chain: "ETH", Amount: "1000000", Dest: "0xabc", Side: SideHot,
	   })
	  if err != nil {
		  return err
	   }
	   if hot.Required {
		   return fmt.Errorf("hot required %+v", hot)
	   }
	  crit, err := eng.Eval(ctx, PlanReq{
		  BatchID: "batch-sanc", Asset: "USDT", Chain: "ETH", Amount: "1", Dest: "sanc:addr", Side: SideCold,
	   })
	   if err != nil {
		  return err
	   }
	   if !crit.Required || crit.Threshold != 3 || crit.PolicyID != polCritical {
		   return fmt.Errorf("critical %+v", crit)
	  }
	   return nil
   }

   func newDrill() (*Engine, error) {
	  eng := New(NewMem())
	  _, err := eng.PutPolicy(context.Background(), drillPolicy(ModeOne, 500), "dave")
	  return eng, err
   }

   func drillPolicy(mode string, pri int) Policy {
	  return Policy{
		   ID: "cer-pol-drill", Name: "drill cold", Asset: "*", Chain: "*", Purpose: PurposeWithdraw,
		   MinAmount: "0.01", DestRisk: RiskAny, Side: SideCold, ForceCold: true,
		  Threshold: 2, Shares: 3, BindMode: mode, Priority: pri, Enabled: true,
	  }
  }

   func manyPolicy() Policy {
	   p := drillPolicy(ModeMany, 600)
	   p.ID = "cer-pol-many"
	   p.Name = "drill many"
	  return p
  }

   func planReq(batch, idem string) PlanReq {
	  return PlanReq{
		  Actor: "alice", BatchID: batch, Asset: "USDT", Chain: "ETH", Amount: "20",
		  Dest: "0xabc", Side: SideCold, Purpose: PurposeWithdraw, ItemRefs: []string{idem},
	  }
   }

   func snapOf(batch, idem, amount, dest string) Snapshot {
	  return Snapshot{BatchID: batch, Lines: []Line{{
		   Asset: "USDT", Chain: "ETH", Amount: amount, Dest: dest, Side: SideCold,
		   Purpose: PurposeWithdraw, ItemRefs: []string{idem},
	  }}}
   }

   func planOpen(ctx context.Context, eng *Engine, batch, idem string) (Session, error) {
	  ses, err := eng.Plan(ctx, planReq(batch, idem))
	   if err != nil {
		  return Session{}, err
	   }
	   return eng.Open(ctx, ses.ID, "alice", ses.FenceEpoch, ses.FenceToken)
   }

   func quorum(ctx context.Context, eng *Engine, batch, idem string) (Session, error) {
	   ses, err := planOpen(ctx, eng, batch, idem)
	   if err != nil {
		  return Session{}, err
	  }
	   ses, err = eng.Receipt(ctx, ses.ID, "alice", "slot-a", "", ses.FenceEpoch, ses.FenceToken)
	   if err != nil {
		   return Session{}, err
	  }
	   return eng.Receipt(ctx, ses.ID, "bob", "slot-b", "", ses.FenceEpoch, ses.FenceToken)
   }

  func readySession(ctx context.Context, eng *Engine, batch, idem string) (Session, error) {
	   ses,
		   err := quorum(ctx, eng, batch, idem)
	   if err != nil {
		   return Session{}, err
	  }
	  if _, err := eng.Bind(ctx, ses.ID, batch, "bob", ses.FenceEpoch, ses.FenceToken); err != nil {
		   return Session{}, err
	  }
	   view, err := eng.View(ctx, ses.ID)
	   if err != nil {
		   return Session{}, err
	  }
	  return eng.MarkReady(ctx, ses.ID, "alice", view.Session.FenceEpoch, view.Session.FenceToken)
  }

  func hasKind(rows []Audit, kind string) bool {
	  for _, a := range rows {
		  if a.Kind == kind {
			  return true
		   }
	  }
	  return false
  }
