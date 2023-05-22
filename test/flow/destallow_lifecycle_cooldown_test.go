package flow

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/destlife"
)

// TestCooldownWindow_Reject 新地址添加后冷却窗口内 Allow 检查失败。
  func TestCooldownWindow_Reject(t *testing.T) {
	   t.Parallel()
	  cases := []struct {
		   name string
		   windowSec int64
		  elapsedAfterAdd time.Duration
		   wantBlocked bool
	   }{
		   {
			   name:      "24h_window_immediate_check",
			  windowSec: 86400,
			  elapsedAfterAdd: 0, wantBlocked: true,
		  },
		   {
			   name: "1h_window_30min_elapsed",
			   windowSec: 3600, elapsedAfterAdd: 30 * time.Minute, wantBlocked: true,
		  },
		   {
			   name:      "1h_window_61min_elapsed",
			   windowSec: 3600, elapsedAfterAdd: 61 * time.Minute, wantBlocked: false,
		   },
		   {
			  name: "zero_window_immediate",
			  windowSec: 0, elapsedAfterAdd: 0, wantBlocked: false,
		  },
	   }
	  for _, tc := range cases {
		  tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			   t.Parallel()
			   now := time.Date(2025, 4, 20, 10, 0, 0, 0, time.UTC)
			   clk := &fakeClock{t: now}
			  calStore := destlife.NewMemCalendarStore()
			   auditStore := destlife.NewMemAuditStore()
			  windowStore := destlife.NewMemWindowStore()
			  window := time.Duration(tc.windowSec) * time.Second
			   cal := destlife.NewCalendar(calStore, auditStore, windowStore, window, clk.Now)
			   ctx := context.Background()
			   e, err := cal.InitAdded(ctx, "user1", "BTC", "bc1qtest", "test", 0)
			  if err != nil {
				   t.Fatalf("InitAdded: %v", err)
			   }
			   if e.State != destlife.StateCooling {
				  t.Fatalf("expected state=cooling, got=%s", e.State)
			  }
			   // 推进时钟。
			  clk.Advance(tc.elapsedAfterAdd)
			  remaining, state, err := cal.CheckCooldown(ctx, "user1", "BTC", "bc1qtest")
			  if err != nil {
				  t.Fatalf("CheckCooldown: %v", err)
			   }
			   // blocked 仅依赖 remaining > 0；remaining <= 0 表示冷却已过期（可用），
			  // state 仍可为 StateCooling（Worker 尚未推进激活）属正常。
			  blocked := remaining > 0
			   if blocked != tc.wantBlocked {
				  t.Errorf("wantBlocked=%v got blocked=%v (remaining=%s state=%s)",
					   tc.wantBlocked,
					   blocked, remaining, state)
			   }
		  })
	  }
   }

// TestCooldownWindow_ActivateTransition 冷却到期后 Worker 激活，状态变为 Active。
   func TestCooldownWindow_ActivateTransition(t *testing.T) {
	   t.Parallel()
	  now := time.Date(2025, 4, 20, 10, 0, 0, 0, time.UTC)
	   clk := &fakeClock{t: now}
	   calStore := destlife.NewMemCalendarStore()
	   auditStore := destlife.NewMemAuditStore()
	   windowStore := destlife.NewMemWindowStore()
	   window := 1 * time.Hour
	   cal := destlife.NewCalendar(calStore, auditStore, windowStore, window, clk.Now)
	   ctx := context.Background()
	   e, _ := cal.InitAdded(ctx, "user2", "ETH", "0xABCDEF", "test", 1)
	   if e.State != destlife.StateCooling {
		   t.Fatalf("expected cooling, got %s", e.State)
	  }
	  // 冷却未到期时不可激活。
	  _, err := cal.ActivateCooled(ctx, e.EntryLifeID, clk.Now(), 1)
	   if err == nil {
		   t.Fatal("expected error: cooling not done yet")
	  }
	  // 推进 61 分钟，冷却满足。
	   clk.Advance(61 * time.Minute)
	   activated,
		  err := cal.ActivateCooled(ctx, e.EntryLifeID, clk.Now(), 1)
	   if err != nil {
		   t.Fatalf("ActivateCooled: %v", err)
	   }
	   if activated.State != destlife.StateActive {
		  t.Errorf("expected state=active, got=%s", activated.State)
	  }
	  if activated.ActiveAt.IsZero() {
		  t.Error("expected active_at set")
	  }
	  // 激活后 CheckCooldown 应返回 remaining=0, state=active。
	  remaining, state, err := cal.CheckCooldown(ctx, "user2", "ETH", "0xABCDEF")
	  if err != nil {
		   t.Fatalf("CheckCooldown after activate: %v", err)
	  }
	   if remaining != 0 || state != destlife.StateActive {
		  t.Errorf("expected active with 0 remaining, got remaining=%s state=%s", remaining, state)
	  }
   }

// TestCooldownWindow_AuditTrail 验证 Add→Cooling→Active 全轨迹审计行完整。
   func TestCooldownWindow_AuditTrail(t *testing.T) {
	   t.Parallel()
	  now := time.Date(2025, 4, 20, 12, 0, 0, 0, time.UTC)
	  clk := &fakeClock{t: now}
	   calStore := destlife.NewMemCalendarStore()
	  auditStore := destlife.NewMemAuditStore()
	   cal := destlife.NewCalendar(calStore, auditStore, nil, 1*time.Hour, clk.Now)
	  ctx := context.Background()
	   e, _ := cal.InitAdded(ctx, "user3", "SOL", "SolAddr1", "", 0)
	   // 冷却 audit。
	  rows, err := cal.ListAuditTrail(ctx, e.EntryLifeID, 50)
	   if err != nil {
		  t.Fatalf("ListAuditTrail: %v", err)
	  }
	  if len(rows) == 0 {
		  t.Fatal("expected at least 1 audit row after InitAdded")
	   }
	  if rows[0].ToState != destlife.StateCooling {
		  t.Errorf("first audit row: expected to_state=cooling, got=%s", rows[0].ToState)
	   }
	  // 激活。
	  clk.Advance(61 * time.Minute)
	  _,
		  _ = cal.ActivateCooled(ctx, e.EntryLifeID, clk.Now(), 0)
	   rows, _ = cal.ListAuditTrail(ctx, e.EntryLifeID, 50)
	  if len(rows) < 2 {
		   t.Fatalf("expected >= 2 audit rows, got %d", len(rows))
	   }
	  lastRow := rows[len(rows)-1]
	  if lastRow.ToState != destlife.StateActive {
		  t.Errorf("last audit row: expected to_state=active, got=%s", lastRow.ToState)
	  }
  }

// fakeClock 可控时钟。
type fakeClock struct {
	t time.Time
}

   func (c *fakeClock) Now() time.Time          { return c.t }
   func (c *fakeClock) Advance(d time.Duration) { c.t = c.t.Add(d) }
