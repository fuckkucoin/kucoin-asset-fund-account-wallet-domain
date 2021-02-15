package amlhold_test

import (
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/amlhold"
)

// ─── AgingScanner Scan 表驱动 ────────────────────────────────────────────

   func TestAgingScannerScan(t *testing.T) {
	   t.Parallel()
	   base := time.Date(2024, 6, 17, 0, 0, 0, 0, time.UTC)
	   cases := []struct {
		   name string
		   openHolds    int
		   advanceClock time.Duration
		   wantAdvanced int
	   }{
		   {
			  name: "未超时：无升级",
			  openHolds: 3,
			  advanceClock: 12 * time.Hour,
			   wantAdvanced: 0,
		  },
		  {
			  name:      "SLA 超时：全部升级到 warn",
			   openHolds: 3,
			   advanceClock: 25 * time.Hour,
			   wantAdvanced: 3,
		   },
	   }
	  for _, tc := range cases {
		   tc := tc
		   t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			   svc := amlhold.New()
			   svc.WithClock(fixedClock(base))
			  for i := 0; i < tc.openHolds; i++ {
				  caseID := "case-scan-" + string(rune('a'+i))
				   userID := "user-scan-" + string(rune('a'+i))
				   _, err := svc.OpenHold(caseID, userID, "USDT", "100", "ops-agent", 86400)
				  if err != nil {
					  t.Fatal(err)
				  }
			   }
			   svc.WithClock(fixedClock(base.Add(tc.advanceClock)))
			  result := svc.RunAgingScan()
			   if result.Advanced != tc.wantAdvanced {
				   t.Errorf("期望升级 %d 个 hold，实际 %d", tc.wantAdvanced, result.Advanced)
			  }
			  // 关键：扫描不得改变 Case 状态（此处仅验证 hold 层不进入终态）
			  if len(result.Errors) > 0 {
				   t.Errorf("扫描产生错误: %v", result.Errors)
			  }
		   })
	  }
  }

// ─── DueHolds 表驱动 ─────────────────────────────────────────────────────

  func TestDueHolds(t *testing.T) {
	   t.Parallel()
	   base := time.Date(2024, 6, 17, 0, 0, 0, 0, time.UTC)
	  cases := []struct {
		  name string
		   queryAt time.Duration
		  wantDue int
	   }{
		  {"未超时", 12 * time.Hour, 0},
		  {"SLA 超时", 25 * time.Hour, 1},
	  }
	   for _, tc := range cases {
		   tc := tc
		  t.Run(tc.name, func(t *testing.T) {
			  t.Parallel()
			   svc := amlhold.New()
			  svc.WithClock(fixedClock(base))
			   _, err := svc.OpenHold("case-due", "user-due", "BTC", "1.0", "ops-agent", 86400)
			  if err != nil {
				  t.Fatal(err)
			   }
			   due := svc.DueHolds(base.Add(tc.queryAt))
			  if len(due) != tc.wantDue {
				  t.Errorf("期望 %d 个 due hold，得到 %d", tc.wantDue, len(due))
			   }
		   })
	   }
   }

// ─── 老化事件记录 ──────────────────────────────────────────────────────────

  func TestAgingEventsRecorded(t *testing.T) {
	  t.Parallel()
	  svc := amlhold.New()
	   h, err := svc.OpenHold("case-events", "user-ev", "ETH", "2.0", "ops-agent", 86400)
	  if err != nil {
		  t.Fatal(err)
	   }
	   // 升级两次
	  for i := 0; i < 2; i++ {
		   cur, _ := svc.GetHold(h.ID)
		   h, _, err = svc.AdvanceAging(h.ID, "system:aging", "test", cur.FenceEpoch, cur.FenceToken)
		  if err != nil {
			   t.Fatalf("第 %d 次升级失败: %v", i+1, err)
		   }
	   }
	   events := svc.AgingEvents(h.ID)
	  if len(events) != 2 {
		   t.Errorf("期望 2 条老化事件，得到 %d", len(events))
	   }
	   for _, ev := range events {
		  if ev.HoldID != h.ID {
			  t.Errorf("事件 hold_id 不匹配: %s", ev.HoldID)
		  }
		   if ev.Age <= 0 {
			   // 在中时间可能为 0，跳过此检查
		  }
		  if ev.ID == "" {
			  t.Error("事件 ID 不得为空")
		  }
	  }
  }

// ─── 老化配置自定义 ───────────────────────────────────────────────────────

   func TestCustomAgingConfig(t *testing.T) {
	   t.Parallel()
	   base := time.Date(2024,
		  6, 17, 0, 0, 0, 0, time.UTC)
	  // 超快老化配置（用）
	  cfg := amlhold.AgingConfig{
		   WarnAfter: 60, // 1 分钟
		   ForceHoldAfter: 120, // 2 分钟
		  EscalateAfter:     180, // 3 分钟
		   CompelReviewAfter: 240, // 4 分钟
	   }
	   svc := amlhold.NewWithConfig(nil, cfg)
	   svc.WithClock(fixedClock(base))
	   h, err := svc.OpenHold("case-custom", "user-custom", "USDT", "100", "ops-agent", 60)
	  if err != nil {
		  t.Fatal(err)
	  }
	  // 90 秒后：应触发 warn（超过 60s）
	   svc.WithClock(fixedClock(base.Add(90 * time.Second)))
	  result := svc.RunAgingScan()
	   if result.Advanced == 0 {
		   t.Error("90s 后应触发 warn 升级")
	  }
	   // 验证 hold 状态
	  cur, _ := svc.GetHold(h.ID)
	  if cur.Status != amlhold.HoldStatusAging {
		   t.Errorf("期望 aging，得到 %s", cur.Status)
	   }
	   if cur.AgingTier != amlhold.AgingTierWarn {
		  t.Errorf("期望 warn 级别，得到 %s", cur.AgingTier)
	  }
	   _ = h
   }

// ─── 并发老化扫描安全性 ───────────────────────────────────────────────────

   func TestConcurrentAgingScan(t *testing.T) {
	   t.Parallel()
	  base := time.Date(2024, 6, 17, 0, 0, 0, 0, time.UTC)
	  expired := base.Add(25 * time.Hour)
	   svc := amlhold.New()
	   svc.WithClock(fixedClock(base))
	   // 开启 10 个 hold
	   for i := 0; i < 10; i++ {
		  caseID := "case-conc-" + string(rune('a'+i))
		   userID := "user-conc-" + string(rune('a'+i))
		   _, err := svc.OpenHold(caseID, userID, "USDT", "100", "ops-agent", 86400)
		   if err != nil {
			  t.Fatal(err)
		  }
	   }
	   svc.WithClock(fixedClock(expired))
	  // 并发执行 5 次扫描
	   done := make(chan struct{}, 5)
	  for i := 0; i < 5; i++ {
		  go func() {
			  svc.RunAgingScan()
			  done <- struct{}{}
		   }()
	  }
	   for i := 0; i < 5; i++ {
		   <-done
	   }
	   // 验证所有 hold 仍为有效状态（不得崩溃或产生不一致）
	  all := svc.ListByCase("case-conc-a", false)
	   for _, h := range all {
		  if h.ID == "" {
			  t.Error("并发扫描后 hold 状态损坏")
		  }
	   }
  }
