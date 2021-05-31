package flow

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/reclaim"
)

// ─── 辅助构造 ────────────────────────────────────────────────────────────────

  func newReclaimSvc() *reclaim.Service {
	  occ := reclaim.NewReclaimOccupancy()
	  store := reclaim.NewStore(persist.NewMemKV())
	  return reclaim.NewService(occ, store)
  }

  func newReclaimSvcWithClock(now func() time.Time) *reclaim.Service {
	  svc := newReclaimSvc()
	  svc.SetClock(now)
	   return svc
   }

   // openCase 辅助函数：快速开立案件并返回。
  func openCase(t *testing.T, svc *reclaim.Service, txHash string, vout int, chain, asset, kind, actor string) reclaim.ReclaimCase {
	   t.Helper()
	  c, err := svc.Open(context.Background(), reclaim.OpenReq{
		  IdempotencyKey: txHash + ":" + strings.ReplaceAll(chain+asset, "", "") + actor,
		  TxHash: txHash,
		  Vout:   vout,
		  Chain:  chain,
		  Asset: asset,
		  Amount: "100.0",
		  Kind:     reclaim.ReclaimKind(kind),
		   OpenedBy: actor,
	  })
	   if err != nil {
		  t.Fatalf("open: %v", err)
	   }
	  return c
  }

// ─── 场景 1：完整错链回收流程 ─────────────────────────────────────────────────

  func TestReclaimWrongChainHappyPath(t *testing.T) {
	   svc := newReclaimSvc()
	  ctx := context.Background()
	  txHash := "0xaaaa000000000000000000000000000000000000000000000000000000000001"
	   c, err := svc.Open(ctx, reclaim.OpenReq{
		  IdempotencyKey: "idem-happy-1",
		  TxHash:         txHash,
		  Vout:     0,
		  Chain:    "BSC",
		  Asset:    "USDT",
		  Amount:   "500.0",
		  Kind:     reclaim.KindWrongChain,
		  UserID:   "u-alice",
		   OpenedBy: "ops-1",
	  })
	  if err != nil {
		  t.Fatalf("open: %v", err)
	   }
	  if c.State != reclaim.StateOpen {
		  t.Fatalf("want open, got %s", c.State)
	  }
	  // 预占
	  c2, err := svc.Reserve(ctx, reclaim.ReserveReq{
		   CaseID: c.ID, Actor: "ops-1",
		   FenceEpoch: c.FenceEpoch, FenceToken: c.FenceToken,
	  })
	  if err != nil {
		  t.Fatalf("reserve: %v", err)
	   }
	  if c2.State != reclaim.StateReserved {
		  t.Fatalf("want reserved, got %s", c2.State)
	   }
	   if svc.Occupancy().HolderOf(c2.OutputKey()) != c2.ID {
		   t.Fatal("occupancy not held after reserve")
	  }
	  // 完成回收
	   c3, err := svc.Reclaim(ctx, reclaim.ReclaimReq{
		   CaseID: c2.ID,
		  ReclaimedTxHash: "0xbbbb",
		   Actor:      "ops-1",
		   FenceEpoch: c2.FenceEpoch,
		   FenceToken: c2.FenceToken,
	   })
	  if err != nil {
		   t.Fatalf("reclaim: %v", err)
	  }
	  if c3.State != reclaim.StateReclaimed {
		  t.Fatalf("want reclaimed, got %s", c3.State)
	   }
	   if svc.Occupancy().HolderOf(c3.OutputKey()) != "" {
		  t.Fatal("occupancy still held after reclaim")
	   }
	  // 创建 Sweep legs
	  group, err := svc.CreateSweepLegs(ctx, reclaim.CreateLegsReq{
		   CaseID:    c3.ID,
		   OutputKey: c3.OutputKey(),
		   Chain:     c3.Chain,
		   Asset:     c3.Asset,
		  Amount: c3.Amount,
		   Actor:  "ops-1",
		  Kinds: []reclaim.LegKind{reclaim.LegKindOccupancyRelease, reclaim.LegKindRefundBroadcast},
		  TxHash: "0xbbbb",
	   })
	   if err != nil {
		   t.Fatalf("sweep legs: %v", err)
	   }
	   if len(group.Legs) != 2 {
		   t.Fatalf("want 2 legs, got %d", len(group.Legs))
	   }
	   if group.Legs[0].Kind != reclaim.LegKindOccupancyRelease {
		   t.Fatalf("leg[0] want occupancy_release, got %s", group.Legs[0].Kind)
	   }
	  if group.Legs[1].Kind != reclaim.LegKindRefundBroadcast {
		  t.Fatalf("leg[1] want refund_broadcast, got %s", group.Legs[1].Kind)
	  }
	   // 时间轴长度
	  tl := svc.GetTimeline(c.ID)
	  if len(tl) < 3 {
		   t.Fatalf("want ≥ 3 timeline events, got %d", len(tl))
	  }
   }

// ─── 场景 2：未认领充值取消 ───────────────────────────────────────────────────

   func TestReclaimUnclaimedCancel(t *testing.T) {
	  svc := newReclaimSvc()
	  ctx := context.Background()
	  c, _ := svc.Open(ctx, reclaim.OpenReq{
		  IdempotencyKey: "idem-uc-1",
		   TxHash:         "0xcc01",
		   Chain:          "ETH", Asset: "ETH", Amount: "0.5",
		  Kind: reclaim.KindUnclaimed, OpenedBy: "ops-2",
	   })
	  c2, err := svc.Cancel(ctx, reclaim.CancelReq{
		  CaseID:     c.ID,
		   Actor:      "ops-2",
		   Reason:     "memo found, auto-attributed",
		   FenceEpoch: c.FenceEpoch,
		  FenceToken: c.FenceToken,
	  })
	   if err != nil {
		  t.Fatalf("cancel: %v", err)
	  }
	  if c2.State != reclaim.StateCancelled {
		   t.Fatalf("want cancelled, got %s", c2.State)
	  }
	   // 终态不可再取消
	   _, err2 := svc.Cancel(ctx, reclaim.CancelReq{
		  CaseID: c.ID, Actor: "ops-2", Reason: "dup",
	  })
	   if !errors.Is(err2, reclaim.ErrTerminal) {
		   t.Fatalf("want ErrTerminal, got %v", err2)
	   }
   }

// ─── 场景 3：SLA 超时自动升级 ─────────────────────────────────────────────────

  func TestReclaimSLAOpenBreach(t *testing.T) {
	   t0 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	  clock := t0
	   svc := newReclaimSvcWithClock(func() time.Time { return clock })
	  ctx := context.Background()
	   c, _ := svc.Open(ctx, reclaim.OpenReq{
		  IdempotencyKey: "idem-sla-1",
		  TxHash: "0xdd01",
		   Chain:  "TRX", Asset: "USDT", Amount: "1000.0",
		  Kind: reclaim.KindMemoMismatch, UserID: "u-bob", OpenedBy: "ops-1",
	  })
	  if c.State != reclaim.StateOpen {
		   t.Fatal("want open")
	  }
	  // 未超时：tick 无变化
	  n := svc.TickSLA(ctx)
	  if n != 0 {
		   t.Fatalf("want 0 escalated, got %d", n)
	   }
	  // 推进 73h，超过 72h SLA
	   clock = t0.Add(73 * time.Hour)
	  n = svc.TickSLA(ctx)
	  if n != 1 {
		  t.Fatalf("want 1 escalated, got %d", n)
	   }
	   updated, err := svc.GetCase(c.ID)
	  if err != nil {
		   t.Fatal(err)
	   }
	   if updated.State != reclaim.StateEscalated {
		  t.Fatalf("want escalated, got %s", updated.State)
	  }
	  // escalated 案件仍可完成回收
	  c2, err := svc.Reclaim(ctx, reclaim.ReclaimReq{
		  CaseID: updated.ID, Actor: "senior-ops", ReclaimedTxHash: "0xee",
	  })
	   if err != nil {
		   t.Fatalf("reclaim after escalate: %v", err)
	   }
	   if c2.State != reclaim.StateReclaimed {
		   t.Fatalf("want reclaimed, got %s", c2.State)
	  }
  }

// ─── 场景 4：SLA reserve 超时 ──────────────────────────────────────────────────

  func TestReclaimSLAReserveBreach(t *testing.T) {
	  t0 := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	  clock := t0
	   svc := newReclaimSvcWithClock(func() time.Time { return clock })
	   ctx := context.Background()
	  c, _ := svc.Open(ctx, reclaim.OpenReq{
		  IdempotencyKey: "idem-sla-reserve-1",
		   TxHash:         "0xff01",
		   Chain: "ETH", Asset: "USDC", Amount: "2000.0",
		   Kind: reclaim.KindWrongChain, UserID: "u-eve", OpenedBy: "ops-3",
	   })
	   c2, _ := svc.Reserve(ctx, reclaim.ReserveReq{
		   CaseID: c.ID, Actor: "ops-3",
		  FenceEpoch: c.FenceEpoch, FenceToken: c.FenceToken,
	   })
	  if c2.State != reclaim.StateReserved {
		   t.Fatal("want reserved")
	  }
	  // 推进 169h，超过 168h SLA
	   clock = t0.Add(1*time.Hour + 168*time.Hour + 1*time.Minute)
	   n := svc.TickSLA(ctx)
	  if n != 1 {
		  t.Fatalf("want 1 escalated, got %d", n)
	   }
	  updated, _ := svc.GetCase(c.ID)
	   if updated.State != reclaim.StateEscalated {
		  t.Fatalf("want escalated, got %s", updated.State)
	  }
   }

// ─── 场景 5：迟贷记竞态阻塞 ────────────────────────────────────────────────────

  func TestReclaimLateCreditBlocked(t *testing.T) {
	   svc := newReclaimSvc()
	   ctx := context.Background()
	   txHash := "0xrace001"
	  c, _ := svc.Open(ctx, reclaim.OpenReq{
		   IdempotencyKey: "idem-race-1",
		   TxHash:         txHash, Vout: 0,
		  Chain: "ETH", Asset: "USDT", Amount: "200.0",
		   Kind: reclaim.KindWrongChain, UserID: "u-carol", OpenedBy: "ops-1",
	   })
	  c2, _ := svc.Reserve(ctx, reclaim.ReserveReq{
		   CaseID: c.ID, Actor: "ops-1",
		   FenceEpoch: c.FenceEpoch, FenceToken: c.FenceToken,
	  })
	   if c2.State != reclaim.StateReserved {
		   t.Fatal("want reserved")
	   }
	  // 充值路径尝试贷记：应被阻塞
	  m := svc.LateCreditMutex()
	   outputKey := c2.OutputKey()
	   err := m.CheckCredit(outputKey, "ETH", "USDT", "200.0", "tr-late-001")
	   if !errors.Is(err, reclaim.ErrLateCreditBlocked) {
		  t.Fatalf("want ErrLateCreditBlocked, got %v", err)
	  }
	  // 竞态事件已入队
	  if m.RaceEventCount() != 1 {
		  t.Fatalf("want 1 race event, got %d", m.RaceEventCount())
	   }
	  evs := m.DrainRaceEvents()
	  if len(evs) != 1 {
		   t.Fatalf("want 1 event after drain, got %d", len(evs))
	  }
	  if evs[0].TransferID != "tr-late-001" {
		  t.Fatalf("wrong transfer_id: %s", evs[0].TransferID)
	  }
	  if evs[0].Reason != "credit_blocked_by_reclaim" {
		   t.Fatalf("wrong reason: %s", evs[0].Reason)
	   }
	  // 队列已清空
	   if m.RaceEventCount() != 0 {
		   t.Fatal("race event queue not cleared after drain")
	   }
  }

// ─── 场景 6：同链自动贷记终态 ─────────────────────────────────────────────────

  func TestReclaimAutoCreditTerminates(t *testing.T) {
	   svc := newReclaimSvc()
	  ctx := context.Background()
	   txHash := "0xauto001"
	   c, _ := svc.Open(ctx, reclaim.OpenReq{
		   IdempotencyKey: "idem-auto-1",
		   TxHash:         txHash, Vout: 1,
		  Chain: "ETH", Asset: "USDC", Amount: "100.0",
		   Kind: reclaim.KindExpiredAddress, UserID: "u-dave", OpenedBy: "ops-1",
	   })
	   c2, _ := svc.Reserve(ctx, reclaim.ReserveReq{
		   CaseID: c.ID, Actor: "ops-1",
		  FenceEpoch: c.FenceEpoch, FenceToken: c.FenceToken,
	  })
	   if c2.State != reclaim.StateReserved {
		   t.Fatal("want reserved")
	   }
	  // 同链贷记成功
	   c3, err := svc.AutoCredit(ctx, reclaim.AutoCreditReq{
		  CaseID:     c2.ID,
		   TransferID: "tr-auto-001",
		   Actor: "system",
	  })
	  if err != nil {
		  t.Fatalf("auto credit: %v", err)
	  }
	  if c3.State != reclaim.StateAutoCredited {
		  t.Fatalf("want auto_credited, got %s", c3.State)
	  }
	   if c3.AutoCreditRef != "tr-auto-001" {
		   t.Fatalf("wrong auto_credit_ref: %s", c3.AutoCreditRef)
	  }
	   // 占用已释放
	   if svc.Occupancy().HolderOf(c3.OutputKey()) != "" {
		   t.Fatal("occupancy not released after auto_credit")
	  }
	   // 占用释放后，充值路径可正常贷记（CheckCredit 返回 nil）
	  m := svc.LateCreditMutex()
	  err2 := m.CheckCredit(c3.OutputKey(), "ETH", "USDC", "100.0", "tr-second-001")
	  if err2 != nil {
		  t.Fatalf("credit should succeed after auto_credited: %v", err2)
	   }
  }

// ─── 场景 7：双重认领防护 ─────────────────────────────────────────────────────

  func TestReclaimDoubleClaimRejected(t *testing.T) {
	  svc := newReclaimSvc()
	  ctx := context.Background()
	  txHash := "0xdup001"
	   c, _ := svc.Open(ctx, reclaim.OpenReq{
		   IdempotencyKey: "idem-dup-1",
		  TxHash: txHash, Chain: "SOL", Asset: "USDT", Amount: "250.0",
		   Kind: reclaim.KindUnclaimed, OpenedBy: "ops-1",
	   })
	  // 相同 output_key 开第二个案件
	   _, err := svc.Open(ctx, reclaim.OpenReq{
		  IdempotencyKey: "idem-dup-2",
		   TxHash: txHash, Chain: "SOL", Asset: "USDT", Amount: "250.0",
		   Kind: reclaim.KindUnclaimed, OpenedBy: "ops-2",
	  })
	  if !errors.Is(err, reclaim.ErrDuplicate) {
		  t.Fatalf("want ErrDuplicate, got %v", err)
	   }
	   // 预占后再 Occupy 冲突
	  c2, _ := svc.Reserve(ctx, reclaim.ReserveReq{
		   CaseID: c.ID, Actor: "ops-1",
		  FenceEpoch: c.FenceEpoch, FenceToken: c.FenceToken,
	  })
	  err2 := svc.Occupancy().Occupy("other-case-id", c2.OutputKey(), "SOL", "USDT", "250.0")
	   if !errors.Is(err2, reclaim.ErrOccupancyLocked) {
		  t.Fatalf("want ErrOccupancyLocked, got %v", err2)
	  }
  }

// ─── 场景 8：幂等开案 ────────────────────────────────────────────────────────

  func TestReclaimOpenIdempotent(t *testing.T) {
	   svc := newReclaimSvc()
	   ctx := context.Background()
	  req := reclaim.OpenReq{
		  IdempotencyKey: "idem-idem-1",
		   TxHash: "0xiiiii", Chain: "ETH", Asset: "USDT", Amount: "10.0",
		   Kind: reclaim.KindWrongChain, OpenedBy: "ops-1",
	  }
	   c1, err := svc.Open(ctx, req)
	  if err != nil {
		   t.Fatalf("open 1: %v", err)
	  }
	  c2, err := svc.Open(ctx, req)
	  if err != nil {
		   t.Fatalf("open 2 (idem): %v", err)
	  }
	  if c1.ID != c2.ID {
		  t.Fatalf("idempotent open should return same id: %s vs %s", c1.ID, c2.ID)
	  }
  }

// ─── 场景 9：Fence 乐观锁 ─────────────────────────────────────────────────────

  func TestReclaimStaleFenceRejected(t *testing.T) {
	  svc := newReclaimSvc()
	   ctx := context.Background()
	   c, _ := svc.Open(ctx, reclaim.OpenReq{
		   IdempotencyKey: "idem-fence-1",
		   TxHash:         "0xfence01", Chain: "ETH", Asset: "USDT", Amount: "5.0",
		  Kind: reclaim.KindUnclaimed, OpenedBy: "ops-1",
	  })
	   // 用正确的 fence 预占一次
	  c2, err := svc.Reserve(ctx, reclaim.ReserveReq{
		   CaseID: c.ID, Actor: "ops-1",
		   FenceEpoch: c.FenceEpoch, FenceToken: c.FenceToken,
	   })
	   if err != nil {
		  t.Fatalf("reserve: %v", err)
	   }
	  // 重放旧的 fence 应拒绝
	   _, err2 := svc.Cancel(ctx, reclaim.CancelReq{
		  CaseID: c2.ID, Actor: "ops-1", Reason: "test",
		   FenceEpoch: c.FenceEpoch, // 旧 fence
		   FenceToken: c.FenceToken,
	  })
	   if !errors.Is(err2, reclaim.ErrFence) {
		   t.Fatalf("want ErrFence, got %v", err2)
	   }
   }

// ─── 场景 10：竞态事件队列有界性 ────────────────────────────────────────────────

   func TestReclaimRaceEventQueueBounded(t *testing.T) {
	  svc := newReclaimSvc()
	   ctx := context.Background()
	   // 预占一个输出
	  txHash := "0xqueue001"
	   c, _ := svc.Open(ctx, reclaim.OpenReq{
		  IdempotencyKey: "idem-queue-1",
		  TxHash: txHash, Chain: "ETH", Asset: "USDT", Amount: "1.0",
		  Kind: reclaim.KindWrongChain, OpenedBy: "ops-1",
	   })
	   c2, _ := svc.Reserve(ctx, reclaim.ReserveReq{
		  CaseID: c.ID, Actor: "ops-1",
		  FenceEpoch: c.FenceEpoch, FenceToken: c.FenceToken,
	  })
	   m := svc.LateCreditMutex()
	  outputKey := c2.OutputKey()
	  // 塞满队列（maxRaceQueueSize = 256）
	  for i := 0; i < 300; i++ {
		  _ = m.CheckCredit(outputKey, "ETH", "USDT", "1.0", "tr-queue-"+reclaim.TestItoa(i))
	  }
	  // 前 256 个入队，后 44 个丢弃
	   if m.RaceEventCount() != 256 {
		  t.Fatalf("want 256 queued, got %d", m.RaceEventCount())
	   }
	  if m.RaceDropCount() != 44 {
		   t.Fatalf("want 44 dropped, got %d", m.RaceDropCount())
	  }
   }

// ─── 场景 11：FSM 非法转移拒绝 ───────────────────────────────────────────────

   func TestReclaimFSMIllegalTransitions(t *testing.T) {
	  type tc struct {
		  name string
		  from  reclaim.State
		  event reclaim.Event
	  }
	   // 这些组合应该被 FSM 或 service 拒绝
	   illegal := []tc{
		   {"open+reclaim", reclaim.StateOpen, reclaim.EventReclaim},
		   {"open+auto_credit", reclaim.StateOpen, reclaim.EventAutoCredit},
		  {"reclaimed+cancel", reclaim.StateReclaimed, reclaim.EventCancel},
		  {"cancelled+reserve", reclaim.StateCancelled, reclaim.EventReserve},
		  {"auto_credited+reclaim", reclaim.StateAutoCredited, reclaim.EventReclaim},
	   }
	   for _, c := range illegal {
		  c := c
		   t.Run(c.name, func(t *testing.T) {
			  svc := newReclaimSvc()
			  ctx := context.Background()
			   base, _ := svc.Open(ctx, reclaim.OpenReq{
				  IdempotencyKey: "fsm-test-" + c.name,
				  TxHash:         "0xfsm" + c.name, Chain: "ETH", Asset: "USDT", Amount: "1.0",
				  Kind: reclaim.KindWrongChain, OpenedBy: "ops",
			  })
			   caseID := base.ID
			  // 推进到目标状态
			  switch c.from {
			  case reclaim.StateReclaimed:
				  r1, _ := svc.Reserve(ctx, reclaim.ReserveReq{CaseID: caseID, Actor: "ops",
					   FenceEpoch: base.FenceEpoch, FenceToken: base.FenceToken})
				  svc.Reclaim(ctx, reclaim.ReclaimReq{CaseID: r1.ID, Actor: "ops", ReclaimedTxHash: "0x"})
			  case reclaim.StateCancelled:
				   svc.Cancel(ctx, reclaim.CancelReq{CaseID: caseID, Actor: "ops", Reason: "test",
					  FenceEpoch: base.FenceEpoch, FenceToken: base.FenceToken})
			  case reclaim.StateAutoCredited:
				  r1, _ := svc.Reserve(ctx, reclaim.ReserveReq{CaseID: caseID, Actor: "ops",
					   FenceEpoch: base.FenceEpoch,
					   FenceToken: base.FenceToken})
				  svc.AutoCredit(ctx, reclaim.AutoCreditReq{CaseID: r1.ID, TransferID: "tr", Actor: "sys"})
			   }
			   cur, _ := svc.GetCase(caseID)
			  if cur.ID == "" {
				   // 终态无法获取不代表失败，说明案件已终态
				  cur = base
			   }
			   var err error
			  switch c.event {
			  case reclaim.EventReclaim:
				   _, err = svc.Reclaim(ctx, reclaim.ReclaimReq{CaseID: cur.ID, Actor: "ops"})
			  case reclaim.EventCancel:
				  _, err = svc.Cancel(ctx, reclaim.CancelReq{CaseID: cur.ID, Actor: "ops", Reason: "test"})
			  case reclaim.EventReserve:
				   _, err = svc.Reserve(ctx, reclaim.ReserveReq{CaseID: cur.ID, Actor: "ops"})
			  case reclaim.EventAutoCredit:
				   _, err = svc.AutoCredit(ctx, reclaim.AutoCreditReq{CaseID: cur.ID, TransferID: "tr", Actor: "sys"})
			  default:
				   t.Skipf("no service method for event %s", c.event)
			   }
			   if err == nil {
				  t.Errorf("illegal transition %s+%s should error", c.from, c.event)
			  }
		  })
	   }
   }

// ─── Golden fixture 驱动 ─────────────────────────────────────────────────
// reclaimFixture golden fixture 形态。
type reclaimFixture struct {
	Name        string        `json:"name"`
	Description string        `json:"description"`
	NowBase     string        `json:"now_base,omitempty"`
	Steps       []reclaimStep `json:"steps"`
}

type reclaimStep struct {
	Op string `json:"op"`
	Description string `json:"description,omitempty"`
	Input           map[string]interface{} `json:"input,omitempty"`
	Actor           string                 `json:"actor,omitempty"`
	Reason          string                 `json:"reason,omitempty"`
	TransferID      string                 `json:"transfer_id,omitempty"`
	ReclaimedTxHash string                 `json:"reclaimed_tx_hash,omitempty"`
	AdvanceHours int `json:"advance_hours,omitempty"`
	Kinds     []string `json:"kinds,omitempty"`
	TxHash    string   `json:"tx_hash,omitempty"`
	OutputKey string   `json:"output_key,omitempty"`
	CaseID string `json:"case_id,omitempty"`
	Chain  string `json:"chain,omitempty"`
	Asset               string `json:"asset,omitempty"`
	Amount              string `json:"amount,omitempty"`
	ExpectState         string `json:"expect_state,omitempty"`
	ExpectOutputKey     string `json:"expect_output_key,omitempty"`
	ExpectOccupancyHeld *bool  `json:"expect_occupancy_held,omitempty"`
	ExpectLegCount int      `json:"expect_leg_count,omitempty"`
	ExpectLegKinds []string `json:"expect_leg_kinds,omitempty"`
	ExpectError    *string  `json:"expect_error,omitempty"` // null = no error
	ExpectEscalated *int `json:"expect_escalated,omitempty"`
	ExpectRaceEventCount *int `json:"expect_race_event_count,omitempty"`
	ExpectEventReason string `json:"expect_event_reason,omitempty"`
	ExpectCount       *int   `json:"expect_count,omitempty"`
}

  func TestReclaimGoldenFixtures(t *testing.T) {
	  dir := filepath.Join("..", "..", "testdata", "reclaim")
	  ents, err := os.ReadDir(dir)
	   if err != nil {
		   t.Skipf("testdata/reclaim not found: %v", err)
	  }
	  if len(ents) == 0 {
		   t.Fatal("no fixtures in testdata/reclaim")
	   }
	  for _, ent := range ents {
		   if ent.IsDir() || !strings.HasSuffix(ent.Name(), ".json") {
			  continue
		   }
		   t.Run(ent.Name(), func(t *testing.T) {
			   raw, err := os.ReadFile(filepath.Join(dir, ent.Name()))
			  if err != nil {
				   t.Fatal(err)
			  }
			   var fx reclaimFixture
			   if err := json.Unmarshal(raw, &fx); err != nil {
				  t.Fatalf("parse fixture: %v", err)
			   }
			  applyReclaimFixture(t, fx)
		  })
	  }
   }

  func applyReclaimFixture(t *testing.T, fx reclaimFixture) {
	  t.Helper()
	   nowBase := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	   if fx.NowBase != "" {
		   if parsed, err := time.Parse(time.RFC3339, fx.NowBase); err == nil {
			  nowBase = parsed
		  }
	  }
	   clock := nowBase
	   occ := reclaim.NewReclaimOccupancy()
	  store := reclaim.NewStore(persist.NewMemKV())
	  svc := reclaim.NewService(occ, store)
	   svc.SetClock(func() time.Time { return clock })
	   var lastCase reclaim.ReclaimCase
	  ctx := context.Background()
	   for stepIdx, step := range fx.Steps {
		   _ = stepIdx
		   var stepErr error
		   switch step.Op {
		  case "open", "open_idem":
			  req := buildOpenReq(t, step.Input)
			   lastCase, stepErr = svc.Open(ctx, req)
		   case "open_duplicate":
			   req := buildOpenReq(t, step.Input)
			  _, stepErr = svc.Open(ctx, req)
		   case "reserve":
			   actor := step.Actor
			   lastCase, stepErr = svc.Reserve(ctx, reclaim.ReserveReq{
				   CaseID: lastCase.ID, Actor: actor,
				  FenceEpoch: lastCase.FenceEpoch, FenceToken: lastCase.FenceToken,
			   })
		  case "reclaim":
			  actor := step.Actor
			   lastCase, stepErr = svc.Reclaim(ctx, reclaim.ReclaimReq{
				  CaseID: lastCase.ID,
				  Actor:  actor,
				   ReclaimedTxHash: step.ReclaimedTxHash,
				  FenceEpoch:      lastCase.FenceEpoch,
				  FenceToken: lastCase.FenceToken,
			  })
		  case "cancel", "cancel_terminal":
			  actor := step.Actor
			   lastCase, stepErr = svc.Cancel(ctx, reclaim.CancelReq{
				   CaseID: lastCase.ID,
				  Actor:      actor,
				  Reason:     step.Reason,
				  FenceEpoch: lastCase.FenceEpoch,
				   FenceToken: lastCase.FenceToken,
			   })
		  case "escalate":
			  actor := step.Actor
			  lastCase, stepErr = svc.Escalate(ctx, reclaim.EscalateReq{
				   CaseID: lastCase.ID,
				  Actor:      actor,
				   Note:       step.Reason,
				   FenceEpoch: lastCase.FenceEpoch,
				  FenceToken: lastCase.FenceToken,
			   })
		  case "auto_credit":
			  lastCase, stepErr = svc.AutoCredit(ctx, reclaim.AutoCreditReq{
				   CaseID: lastCase.ID,
				  TransferID: step.TransferID,
				   Actor:      step.Actor,
			   })
		  case "tick_sla":
			   if step.AdvanceHours > 0 {
				  clock = clock.Add(time.Duration(step.AdvanceHours) * time.Hour)
			   }
			   n := svc.TickSLA(ctx)
			   if step.ExpectEscalated != nil && n != *step.ExpectEscalated {
				   t.Errorf("step %q: want %d escalated, got %d", step.Op, *step.ExpectEscalated, n)
			  }
			   // 刷新 lastCase（TickSLA 可能改变了状态和 fence）
			   if lastCase.ID != "" {
				  if refreshed, err := svc.GetCase(lastCase.ID); err == nil {
					  lastCase = refreshed
				  }
			   }
			   if step.ExpectState != "" {
				  if string(lastCase.State) != step.ExpectState {
					   t.Errorf("step %q: want state %s, got %s", step.Op, step.ExpectState, lastCase.State)
				   }
			  }
			   continue
		  case "sweep_legs":
			   kinds := make([]reclaim.LegKind, len(step.Kinds))
			  for i, k := range step.Kinds {
				   kinds[i] = reclaim.LegKind(k)
			  }
			  group, serr := svc.CreateSweepLegs(ctx, reclaim.CreateLegsReq{
				  CaseID:    lastCase.ID,
				  OutputKey: lastCase.OutputKey(),
				   Chain:     lastCase.Chain,
				  Asset: lastCase.Asset,
				   Amount: lastCase.Amount,
				   Actor:  step.Actor,
				   Kinds:  kinds,
				   TxHash: step.TxHash,
			   })
			   if serr != nil {
				   stepErr = serr
			   } else if step.ExpectLegCount > 0 && len(group.Legs) != step.ExpectLegCount {
				   t.Errorf("step sweep_legs: want %d legs, got %d", step.ExpectLegCount, len(group.Legs))
			  }
		   case "check_credit":
			  m := svc.LateCreditMutex()
			   stepErr = m.CheckCredit(step.OutputKey, step.Chain, step.Asset, step.Amount, step.TransferID)
			   if step.ExpectRaceEventCount != nil {
				  if m.RaceEventCount() != *step.ExpectRaceEventCount {
					  t.Errorf("step check_credit: want %d race events, got %d",
						  *step.ExpectRaceEventCount, m.RaceEventCount())
				   }
			   }
		  case "check_credit_after_terminal":
			   m := svc.LateCreditMutex()
			   stepErr = m.CheckCredit(step.OutputKey, step.Chain, step.Asset, step.Amount, step.TransferID)
		  case "drain_race_events":
			  m := svc.LateCreditMutex()
			  evs := m.DrainRaceEvents()
			   if step.ExpectCount != nil && len(evs) != *step.ExpectCount {
				  t.Errorf("step drain_race_events: want %d, got %d", *step.ExpectCount, len(evs))
			   }
			  if step.ExpectEventReason != "" && len(evs) > 0 && evs[0].Reason != step.ExpectEventReason {
				  t.Errorf("step drain_race_events: want reason %s, got %s", step.ExpectEventReason, evs[0].Reason)
			   }
			  continue
		   case "occupy_duplicate":
			  stepErr = svc.Occupancy().Occupy(step.CaseID, step.OutputKey, "SOL", "USDT", "250.0")
		  default:
			   t.Logf("unknown step op: %s (skipped)", step.Op)
			   continue
		   }
		  // 错误期望校验
		   if step.ExpectError != nil {
			  if *step.ExpectError == "" {
				   if stepErr != nil {
					   t.Errorf("step %q: want no error, got %v", step.Op, stepErr)
				  }
			   } else {
				  if stepErr == nil || !strings.Contains(stepErr.Error(), *step.ExpectError) {
					  t.Errorf("step %q: want error containing %q, got %v", step.Op, *step.ExpectError, stepErr)
				  }
			   }
		   } else {
			   if stepErr != nil {
				  t.Errorf("step %q: unexpected error: %v", step.Op, stepErr)
			  }
		   }
		  // 状态期望校验
		   if step.ExpectState != "" {
			   if string(lastCase.State) != step.ExpectState {
				  t.Errorf("step %q: want state %s, got %s", step.Op, step.ExpectState, lastCase.State)
			  }
		  }
		   // 占用期望校验
		   if step.ExpectOccupancyHeld != nil {
			  holder := svc.Occupancy().HolderOf(lastCase.OutputKey())
			  held := holder != ""
			   if held != *step.ExpectOccupancyHeld {
				  t.Errorf("step %q: want occupancy_held=%v, got %v (holder=%q)",
					   step.Op, *step.ExpectOccupancyHeld, held, holder)
			  }
		  }
	   }
   }

// buildOpenReq 从 fixture map 构造 OpenReq。
   func buildOpenReq(t *testing.T, m map[string]interface{}) reclaim.OpenReq {
	   t.Helper()
	  req := reclaim.OpenReq{}
	  if v, ok := m["idempotency_key"].(string); ok {
		  req.IdempotencyKey = v
	   }
	  if v, ok := m["tx_hash"].(string); ok {
		   req.TxHash = v
	   }
	   if v, ok := m["vout"].(float64); ok {
		  req.Vout = int(v)
	   }
	   if v, ok := m["chain"].(string); ok {
		  req.Chain = v
	  }
	  if v, ok := m["asset"].(string); ok {
		   req.Asset = v
	  }
	   if v, ok := m["amount"].(string); ok {
		   req.Amount = v
	   }
	   if v, ok := m["kind"].(string); ok {
		  req.Kind = reclaim.ReclaimKind(v)
	  }
	  if v, ok := m["user_id"].(string); ok {
		   req.UserID = v
	  }
	   if v, ok := m["refund_address"].(string); ok {
		  req.RefundAddress = v
	  }
	   if v, ok := m["opened_by"].(string); ok {
		   req.OpenedBy = v
	  }
	  if v, ok := m["note"].(string); ok {
		  req.Note = v
	   }
	   return req
  }
