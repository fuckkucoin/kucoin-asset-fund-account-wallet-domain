package dustguard

import (
	"context"
	"time"
)

// WaterLevelReader 由地址库存提供水位查询接口。
type WaterLevelReader interface {
	// FreeRatio 返回给定链的空闲槽位占比（0.0–1.0）。
	FreeRatio(chain string) float64
	// Chains 返回库存中有条目的所有链。
	Chains() []string
}

// RunDetector 定期把内存攻击窗口分值刷到 Store，并对高分值地址自动开隔离。
// interval 建议 30s。

   func RunDetector(ctx context.Context, detector *Detector, fsm *QuarantineFSM, policy *PolicyManager, interval time.Duration) {
	   ticker := time.NewTicker(interval)
	  defer ticker.Stop()
	  for {
		  select {
		   case <-ctx.Done():
			  return
		  case <-ticker.C:
			   safeRun("detector", func() {
				   detector.FlushScores(ctx)
				   // 自动驱逐老窗口（窗口本身滑动，超出 2× 窗口时长的条目可回收）
				   maxWin := time.Duration(120) * time.Second
				  // 用策略的最大窗口做 GC ；这里取全局默认的 2 倍
				   pol := policy.Active(ctx, "")
				  maxWin = time.Duration(pol.FloodWindowSec*2) * time.Second
				   detector.EvictAll(time.Now().UTC().Add(-maxWin))
				  // 对分值超阈的地址自动开案（已有案件时 Open 幂等）
				  if !pol.AutoQuarantine {
					   return
				  }
				  scores, err := detector.store.ListScores(ctx, "", LevelAttacking)
				  if err != nil {
					   return
				   }
				   for _, sc := range scores {
					  chainPol := policy.Active(ctx, sc.Chain)
					  if sc.Score < chainPol.ScoreAttack {
						  continue
					   }
					  _, _ = fsm.Open(ctx, sc.Chain, sc.Address, "", "system", sc,
						   "auto quarantine from detector flush")
				   }
			   })
		  }
	  }
  }

// RunQuarantineTimer 扫描 holding 状态中已过取证窗口的案件，更新 backlog 指标。
// 人工释放须通过 HTTP ops API；worker 只做指标驱动。
// interval 建议 60s。

  func RunQuarantineTimer(ctx context.Context,
	   fsm *QuarantineFSM, interval time.Duration) {
	  ticker := time.NewTicker(interval)
	   defer ticker.Stop()
	  for {
		  select {
		  case <-ctx.Done():
			   return
		   case <-ticker.C:
			   safeRun("quarantine-timer", func() {
				  expired, err := fsm.ListHoldingExpired(ctx)
				   if err != nil {
					   return
				   }
				  // 只更新指标；实际 release 须 ops 手动确认
				  DustForensicsExpiredBacklog.Set(float64(len(expired)))
				   // 统计各链活跃隔离数
				  all, err := fsm.store.ListCases(ctx, "", "")
				   if err != nil {
					   return
				  }
				  chainActive := map[string]int{}
				  for _, c := range all {
					  if c.IsActive() {
						  chainActive[c.Chain]++
					   }
				   }
				   for chain, n := range chainActive {
					   DustQuarantineActive.WithLabelValues(chain).Set(float64(n))
				   }
			   })
		  }
	  }
   }

// RunWaterLevel 定期检查地址池空闲水位，低于策略阈值时触发轮换计划。
// 只针对 1:1 地址（FamMEMO 共享地址不走此路径）。
// interval 建议 120s。
   func RunWaterLevel(ctx context.Context, rotator *Rotator, policy *PolicyManager, wlr WaterLevelReader, interval time.Duration) {
	   if wlr == nil {
		   return
	  }
	   ticker := time.NewTicker(interval)
	  defer ticker.Stop()
	   for {
		   select {
		  case <-ctx.Done():
			  return
		   case <-ticker.C:
			   safeRun("waterlevel", func() {
				  for _, chain := range wlr.Chains() {
					  pol := policy.Active(ctx,
						   chain)
					  if !pol.Active || pol.WaterLevel <= 0 {
						   continue
					   }
					   ratio := wlr.FreeRatio(chain)
					   // 百分比转换：ratio=0.15 → 15%；WaterLevel=20 → 阈值 20%
					   pct := int(ratio * 100)
					   DustPoolWaterLevel.WithLabelValues(chain).Set(ratio * 100)
					   if pct >= pol.WaterLevel {
						  continue
					  }
					  // 低于水位：建议轮换（AutoRotate=false 时仅创建计划，ops 处理）
					  _, _ = rotator.OpenAutoRotate(ctx, chain, "", "", "", TriggerWaterLevel)
				   }
			   })
		  }
	   }
   }

// safeRun 捕获 worker tick 中的 panic，防止整个 worker 退出。
  func safeRun(name string, fn func()) {
	   defer func() {
		  if r := recover(); r != nil {
			   DustWorkerPanic.WithLabelValues(name).Inc()
		   }
	  }()
	fn()
}
