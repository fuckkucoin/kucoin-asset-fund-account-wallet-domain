package flow

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/destlife"
)

// TestInflightLock_RemoveBlockedByRef 出金在途时 Remove 必须进入 revoking，不得物理删除。
   func TestInflightLock_RemoveBlockedByRef(t *testing.T) {
	  t.Parallel()
	   ctx := context.Background()
	   now := time.Date(2025, 4, 20, 14, 0, 0, 0, time.UTC)
	   clk := &fakeClock{t: now}
	   calStore := destlife.NewMemCalendarStore()
	   auditStore := destlife.NewMemAuditStore()
	  cal := destlife.NewCalendar(calStore, auditStore, nil, 0, clk.Now) // 无冷却窗口
	  // 初始化条目并激活。
	  e, err := cal.InitAdded(ctx, "user10", "BTC", "bc1remove", "label", 1)
	   if err != nil {
		   t.Fatalf("InitAdded: %v", err)
	  }
	  clk.Advance(1 * time.Second)
	  e, err = cal.ActivateCooled(ctx, e.EntryLifeID, clk.Now(), 1)
	  if err != nil {
		   t.Fatalf("ActivateCooled: %v", err)
	  }
	   // 获取在途引用。
	   inflightStore := destlife.NewMemInflightStore()
	  lock := destlife.NewInflightLock(inflightStore,
		  cal, clk.Now)
	   _, err = lock.AcquireRef(ctx, e.EntryLifeID, "user10", "BTC", "bc1remove", "mvt-001")
	   if err != nil {
		   t.Fatalf("AcquireRef: %v", err)
	   }
	   // 此时 ActiveCount 应为 1。
	  cnt, err := lock.ActiveCount(ctx, e.EntryLifeID)
	  if err != nil {
		   t.Fatalf("ActiveCount: %v", err)
	  }
	  if cnt != 1 {
		  t.Errorf("expected active_refs=1, got=%d", cnt)
	   }
	  // 尝试 Remove：应返回 ErrInflightBlock，条目进入 revoking。
	  err = lock.ForceRevokingOnRemove(ctx, e.EntryLifeID, "ops", "remove_requested", e.Generation)
	   if !errors.Is(err, destlife.ErrInflightBlock) {
		  t.Fatalf("expected ErrInflightBlock, got=%v", err)
	  }
	   // 验证条目已进入 revoking 状态。
	   updated, err := cal.GetLifeByID(ctx, e.EntryLifeID)
	  if err != nil {
		   t.Fatalf("GetLifeByID: %v", err)
	  }
	  if updated.State != destlife.StateRevoking {
		   t.Errorf("expected state=revoking after blocked remove, got=%s", updated.State)
	   }
  }

// TestInflightLock_ReleaseTriggersRevoke 引用释放后引用计数归零触发 revoking→revoked。
   func TestInflightLock_ReleaseTriggersRevoke(t *testing.T) {
	   t.Parallel()
	  ctx := context.Background()
	   now := time.Date(2025, 4, 20, 15, 0, 0, 0, time.UTC)
	   clk := &fakeClock{t: now}
	  calStore := destlife.NewMemCalendarStore()
	   auditStore := destlife.NewMemAuditStore()
	  cal := destlife.NewCalendar(calStore, auditStore, nil, 0, clk.Now)
	   e, _ := cal.InitAdded(ctx, "user11", "ETH", "0xRevoke1", "", 1)
	  clk.Advance(1 * time.Second)
	  e,
		   _ = cal.ActivateCooled(ctx, e.EntryLifeID, clk.Now(), 1)
	   inflightStore := destlife.NewMemInflightStore()
	  lock := destlife.NewInflightLock(inflightStore, cal, clk.Now)
	   ref, _ := lock.AcquireRef(ctx, e.EntryLifeID, "user11", "ETH", "0xRevoke1", "mvt-002")
	   // 先触发 revoking（运营强制撤销）。
	  _, err := cal.BeginRevoking(ctx, e.EntryLifeID, "ops", "test", e.Generation)
	  if err != nil {
		   t.Fatalf("BeginRevoking: %v", err)
	  }
	  // 条目处于 revoking，引用仍有 1 个，不应直接变 revoked。
	  e2, _ := cal.GetLifeByID(ctx, e.EntryLifeID)
	   if e2.State != destlife.StateRevoking {
		  t.Fatalf("expected revoking, got %s", e2.State)
	   }
	   // 释放引用：自动触发 FinalizeRevoked。
	  clk.Advance(100 * time.Millisecond)
	  if err := lock.ReleaseRef(ctx, ref.RefID, "worker", e2.Generation); err != nil {
		   t.Fatalf("ReleaseRef: %v", err)
	  }
	   // 验证条目已变为 revoked。
	   final,
		  _ := cal.GetLifeByID(ctx, e.EntryLifeID)
	   if final.State != destlife.StateRevoked {
		   t.Errorf("expected revoked after ref release, got=%s", final.State)
	   }
   }

// TestInflightLock_Idempotent 同一 movement_id 重复 AcquireRef 返回同一 ref。
  func TestInflightLock_Idempotent(t *testing.T) {
	  t.Parallel()
	  ctx := context.Background()
	  clk := &fakeClock{t: time.Now().UTC()}
	   calStore := destlife.NewMemCalendarStore()
	  auditStore := destlife.NewMemAuditStore()
	  cal := destlife.NewCalendar(calStore, auditStore, nil, 0, clk.Now)
	  e, _ := cal.InitAdded(ctx, "user12", "BTC", "bc1idem", "", 1)
	  inflightStore := destlife.NewMemInflightStore()
	   lock := destlife.NewInflightLock(inflightStore, cal, clk.Now)
	  ref1, err := lock.AcquireRef(ctx, e.EntryLifeID, "user12", "BTC", "bc1idem", "mvt-idem")
	   if err != nil {
		   t.Fatalf("first AcquireRef: %v", err)
	  }
	  ref2, err := lock.AcquireRef(ctx, e.EntryLifeID, "user12", "BTC", "bc1idem", "mvt-idem")
	   if err != nil {
		   t.Fatalf("second AcquireRef: %v", err)
	  }
	  if ref1.RefID != ref2.RefID {
		  t.Errorf("expected same ref_id, got ref1=%s ref2=%s", ref1.RefID, ref2.RefID)
	  }
   }

// TestInflightLock_TableDriven 大表驱动：多个 movement 并发获取/释放引用。
   func TestInflightLock_TableDriven(t *testing.T) {
	   t.Parallel()
	  type step struct {
		   action string // "acquire" | "release" | "check_count"
		   mvtID string
		   wantErr   bool
		   wantCount int
	   }
	  cases := []struct {
		   name  string
		  steps []step
	   }{
		   {
			  name: "single_acquire_release",
			   steps: []step{
				  {action: "acquire", mvtID: "m1", wantErr: false, wantCount: 1},
				  {action: "check_count", wantCount: 1},
				   {action: "release", mvtID: "m1", wantErr: false},
				  {action: "check_count", wantCount: 0},
			  },
		  },
		  {
			   name: "three_acquires_sequential_releases",
			   steps: []step{
				  {action: "acquire", mvtID: "a1", wantCount: 1},
				  {action: "acquire", mvtID: "a2", wantCount: 2},
				  {action: "acquire", mvtID: "a3", wantCount: 3},
				  {action: "release", mvtID: "a1"},
				  {action: "check_count", wantCount: 2},
				  {action: "release", mvtID: "a2"},
				  {action: "check_count", wantCount: 1},
				  {action: "release", mvtID: "a3"},
				   {action: "check_count", wantCount: 0},
			  },
		   },
	  }
	   for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			   ctx := context.Background()
			  clk := &fakeClock{t: time.Now().UTC()}
			   calStore := destlife.NewMemCalendarStore()
			   auditStore := destlife.NewMemAuditStore()
			  cal := destlife.NewCalendar(calStore, auditStore, nil, 0, clk.Now)
			  e, _ := cal.InitAdded(ctx, "user-td", "BTC", fmt.Sprintf("bc1td-%s", tc.name), "", 1)
			  inflightStore := destlife.NewMemInflightStore()
			   lock := destlife.NewInflightLock(inflightStore, cal, clk.Now)
			  refIDs := map[string]string{}
			  for i, step := range tc.steps {
				   switch step.action {
				   case "acquire":
					  ref, err := lock.AcquireRef(ctx, e.EntryLifeID, "user-td", "BTC", e.Address, step.mvtID)
					  if (err != nil) != step.wantErr {
						  t.Errorf("step[%d] acquire %s: wantErr=%v got=%v", i, step.mvtID, step.wantErr, err)
					  }
					  if err == nil {
						   refIDs[step.mvtID] = ref.RefID
					  }
					  if step.wantCount > 0 {
						  cnt, _ := lock.ActiveCount(ctx, e.EntryLifeID)
						   if cnt != step.wantCount {
							   t.Errorf("step[%d] count after acquire: want=%d got=%d", i, step.wantCount, cnt)
						  }
					   }
				   case "release":
					   refID := refIDs[step.mvtID]
					   err := lock.ReleaseRef(ctx, refID, "test", 1)
					   if (err != nil) != step.wantErr {
						  t.Errorf("step[%d] release %s: wantErr=%v got=%v", i, step.mvtID, step.wantErr, err)
					   }
				   case "check_count":
					  cnt, _ := lock.ActiveCount(ctx, e.EntryLifeID)
					   if cnt != step.wantCount {
						   t.Errorf("step[%d] check_count: want=%d got=%d", i, step.wantCount, cnt)
					   }
				   }
			  }
		   })
	  }
  }
