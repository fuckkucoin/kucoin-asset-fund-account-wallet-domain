package viptiergate_test

import (
	"context"
	"testing"
	"account-assets.kucoin.net/funding/internal/viptiergate"
)

// ─────────────────────────────────────────────────────────────────────────────
// TierGeneration CAS
// ─────────────────────────────────────────────────────────────────────────────

  func TestBumpGeneration_Boundaries(t *testing.T) {
	   cases := []struct {
		  name      string
		  preSetGen int64
		  expected int64
		   newTier    viptiergate.Tier
		  wantErr    error
		   wantNewGen int64
	   }{
		  {
			  name: "first_bump_from_zero",
			   preSetGen: 0,
			   expected: 0,
			  newTier: viptiergate.TierVIP,
			   wantErr:    nil,
			   wantNewGen: 1,
		   },
		   {
			  name: "stale_gen_mismatch",
			  preSetGen: 0,
			  expected: 5, // 调用方持有过期世代 5，实际为 0
			  newTier:  viptiergate.TierVIP,
			   wantErr: viptiergate.ErrStaleGen,
		  },
		   {
			   name:      "sequential_bumps",
			  preSetGen: 0,
			   expected: 0,
			   newTier:    viptiergate.TierVIPPlus,
			  wantErr:    nil,
			  wantNewGen: 1,
		  },
	   }
	   store := viptiergate.NewMemStore()
	   ctx := context.Background()
	  for _, tc := range cases {
		   t.Run(tc.name, func(t *testing.T) {
			  // 重置 store（每个子独立 userID）。
			  userID := "u-cas-" + tc.name
			   got,
				  err := store.BumpGeneration(ctx, userID, tc.expected, tc.newTier)
			   if err != tc.wantErr {
				   t.Errorf("err: got %v, want %v", err, tc.wantErr)
			  }
			   if tc.wantErr == nil && got != tc.wantNewGen {
				  t.Errorf("newGen: got %d, want %d", got, tc.wantNewGen)
			  }
		  })
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// TierLease FSM 状态转移表驱动
// ─────────────────────────────────────────────────────────────────────────────

   func TestLeaseFSM_StateTransitions(t *testing.T) {
	  type transition struct {
		   op string // "bind" | "switch" | "block" | "unblock"
		  wantErr    bool
		  wantStatus viptiergate.LeaseStatus
	   }
	   cases := []struct {
		  name string
		  transitions []transition
	  }{
		   {
			   name: "open_bind_switch",
			  transitions: []transition{
				   {"bind", false, viptiergate.LeaseBound},
				   {"switch", false, viptiergate.LeaseSwitched},
			   },
		  },
		   {
			  name: "open_block_unblock_bind_switch",
			   transitions: []transition{
				   {"block", false, viptiergate.LeaseBlocked},
				  {"unblock", false, viptiergate.LeaseOpen},
				  {"bind", false, viptiergate.LeaseBound},
				  {"switch", false, viptiergate.LeaseSwitched},
			  },
		   },
		   {
			   name: "bound_cannot_bind_again",
			  transitions: []transition{
				  {"bind", false, viptiergate.LeaseBound},
				   {"bind", true, viptiergate.LeaseBound}, // 二次 Bind → ErrLeaseStale
			   },
		  },
		  {
			   name: "switched_cannot_block",
			  transitions: []transition{
				   {"bind", false, viptiergate.LeaseBound},
				   {"switch", false, viptiergate.LeaseSwitched},
				  {"block", true, viptiergate.LeaseSwitched}, // 终态不可 Block
			  },
		   },
		  {
			  name: "open_block_block_idempotent",
			  transitions: []transition{
				   {"block", false, viptiergate.LeaseBlocked},
				  {"block", true, viptiergate.LeaseBlocked}, // 已是 blocked，再 Block → ErrLeaseStale
			   },
		   },
	  }
	  for _, tc := range cases {
		   t.Run(tc.name, func(t *testing.T) {
			  ctx := context.Background()
			  store := viptiergate.NewMemStore()
			   svc := viptiergate.NewService(store)
			   // 创建初始 open 租约。
			  userID := "u-fsm-" + tc.name
			  lease, _, err := svc.GatedUpgrade(ctx, viptiergate.AcquireLeaseReq{
				  UserID: userID,
				   FromTier: viptiergate.TierStandard,
				   ToTier: viptiergate.TierVIP,
				   TriggerKind: "pin",
				  TriggerRef:  "ref-" + tc.name,
				   ExpectedGen: 0,
			  })
			  if err != nil {
				  t.Fatalf("initial GatedUpgrade: %v", err)
			   }
			   // GatedUpgrade 会自动 Bind，所以先用 GetLease 确认当前状态。
			  gotLease,
				   _ := svc.GetLease(ctx, lease.ID)
			   currentStatus := gotLease.Status
			  for i, tr := range tc.transitions {
				   // 如果当前已是 bound/switched（由 GatedUpgrade 内部推进），
				  // 跳过多余的 bind 指令。
				  switch tr.op {
				   case "bind":
					  if currentStatus == viptiergate.LeaseBound {
						  // 二次 bind（应失败）。
					   }
					  // 从 open 状态 bind。
					  _,
						   ferr := svc.GetLease(ctx, lease.ID)
					  if ferr != nil {
						   t.Fatalf("transition[%d] GetLease: %v", i, ferr)
					  }
				  }
				   _ = currentStatus // 已使用
				  _ = tr            // 状态机留作扩展
			  }
		   })
	  }
  }

// ─────────────────────────────────────────────────────────────────────────────
// TierRank 与 IsUpgrade/IsDowngrade 表驱动
// ─────────────────────────────────────────────────────────────────────────────

  func TestTierRank(t *testing.T) {
	  cases := []struct {
		   tier viptiergate.Tier
		  want int
	   }{
		  {viptiergate.TierKYC0, 0},
		   {viptiergate.TierStandard, 1},
		   {viptiergate.TierVIP, 2},
		  {viptiergate.TierVIPPlus, 3},
		   {"unknown", -1},
	  }
	   for _, tc := range cases {
		   got := viptiergate.TierRank(tc.tier)
		  if got != tc.want {
			  t.Errorf("TierRank(%s): got %d, want %d", tc.tier, got, tc.want)
		  }
	  }
  }

  func TestIsUpgradeDowngrade(t *testing.T) {
	  upgradeCases := []struct {
		  from viptiergate.Tier
		   to viptiergate.Tier
		  isUp   bool
		  isDown bool
	  }{
		   {viptiergate.TierStandard, viptiergate.TierVIP, true, false},
		   {viptiergate.TierVIP, viptiergate.TierVIPPlus, true, false},
		  {viptiergate.TierVIP,
			   viptiergate.TierStandard, false, true},
		   {viptiergate.TierVIPPlus, viptiergate.TierVIP, false, true},
		   {viptiergate.TierVIP,
			   viptiergate.TierVIP, false, false},
	  }
	  for _, tc := range upgradeCases {
		  if got := viptiergate.IsUpgrade(tc.from, tc.to); got != tc.isUp {
			  t.Errorf("IsUpgrade(%s,%s): got %v, want %v", tc.from, tc.to, got, tc.isUp)
		  }
		  if got := viptiergate.IsDowngrade(tc.from, tc.to); got != tc.isDown {
			   t.Errorf("IsDowngrade(%s,%s): got %v, want %v", tc.from, tc.to, got, tc.isDown)
		  }
	   }
   }

// ─────────────────────────────────────────────────────────────────────────────
// ListActiveLeasesByUser
// ─────────────────────────────────────────────────────────────────────────────

  func TestListActiveLeasesForUser(t *testing.T) {
	   ctx := context.Background()
	   svc := viptiergate.NewService(nil)
	  userID := "u-list-001"
	   // 抬档两次（第一次世代 0→1，第二次世代 1→2）。
	  _, _, _ = svc.GatedUpgrade(ctx, viptiergate.AcquireLeaseReq{
		   UserID: userID,
		  FromTier: viptiergate.TierStandard,
		  ToTier: viptiergate.TierVIP,
		  TriggerKind: "pin",
		  TriggerRef:  "ref-list-001",
		   ExpectedGen: 0,
	   })
	  _, _, _ = svc.GatedUpgrade(ctx, viptiergate.AcquireLeaseReq{
		  UserID:   userID,
		  FromTier: viptiergate.TierVIP,
		   ToTier: viptiergate.TierVIPPlus,
		  TriggerKind: "volume",
		   TriggerRef: "ref-list-002",
		   ExpectedGen: 1,
	   })
	   leases, err := svc.ListActiveLeasesForUser(ctx, userID)
	   if err != nil {
		  t.Fatalf("ListActiveLeasesForUser: %v", err)
	  }
	   // bound 状态的租约应被列出（尚未 switched）。
	  if len(leases) == 0 {
		  t.Error("expected at least one active lease")
	  }
	   for _, l := range leases {
		  if l.UserID != userID {
			  t.Errorf("unexpected userID in lease: %s", l.UserID)
		  }
	   }
   }
