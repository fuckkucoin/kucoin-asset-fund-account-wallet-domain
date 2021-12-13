package sweeppol

import (
	"context"
	"fmt"
	"strings"
	"account-assets.kucoin.net/funding/internal/coolmarkgate"
	"account-assets.kucoin.net/funding/internal/hotinv"
)

// CoolGenBook 冷却世代装饰器——在 Book.Evaluate 路径中嵌入世代校验。
// 功能：
// Evaluate 调用前先通过 Engine.CheckExecAllowed 验证世代；
// 若冷却窗活跃或证明缺失则返回空计划（拒绝归集），同时落 CoolBreachCase。

type CoolGenBook struct {
	inner  *Book
	engine *coolmarkgate.Engine
}

// WithCoolGenEngine 为 Book 附加冷却世代引擎，返回 CoolGenBook。
// engine 为 nil 时直接返回 nil（调用方须检查）。

  func WithCoolGenEngine(b *Book, engine *coolmarkgate.Engine) *CoolGenBook {
	  if b == nil || engine == nil {
		  return nil
	  }
	  return &CoolGenBook{inner: b, engine: engine}
  }

// EvaluateWithCoolGen 在 Book.Evaluate 前先对每个归集资产执行冷却世代校验。
// 功能：
// 1. 对 inv 中每个资产，通过 engine.GetCoolGen 取当前世代。
// 2. 调用 engine.CheckExecAllowed 复验；若拒绝则从计划中排除该资产。
// 3. 通过校验的资产列表传给 Book.Evaluate 生成最终计划。
// 风险点：
// engine.CheckExecAllowed 内部落 CoolBreachCase；本函数无需重复落库。
// inv 为空时直接返回空计划。

  func (cgb *CoolGenBook) EvaluateWithCoolGen(ctx context.Context, inv []hotinv.Snapshot, execID string) ([]Plan, error) {
	  if cgb == nil || cgb.inner == nil {
		   return nil, fmt.Errorf("sweeppol.EvaluateWithCoolGen: nil book")
	  }
	   if len(inv) == 0 {
		  return nil, nil
	   }
	  allowed := make([]hotinv.Snapshot, 0, len(inv))
	  for _, snap := range inv {
		   asset := strings.ToUpper(strings.TrimSpace(snap.Asset))
		  if asset == "" {
			   continue
		   }
		  // 读取当前 cool_gen。
		   gen, err := cgb.engine.GetCoolGen(ctx, asset)
		   if err != nil {
			   return nil, fmt.Errorf("sweeppol.EvaluateWithCoolGen: get cool_gen for %s: %w", asset, err)
		   }
		  // 复验：世代匹配、冷却窗已结束、证明存在。
		  res, err := cgb.engine.CheckExecAllowed(ctx, coolmarkgate.ExecVerifyInput{
			   Asset: asset,
			   CoolGen: gen,
			  ExecID:  execID,
		  })
		  if err != nil || (res != nil && !res.Allowed) {
			   // 冷却窗活跃或证明缺失：排除该资产，不产生归集计划。
			   continue
		  }
		  allowed = append(allowed, snap)
	  }
	  return cgb.inner.Evaluate(allowed), nil
  }

// CoolGenProof 供调用方在执行前携带的世代证明引用。
// 功能：由 ExecWithCoolGen 返回，调用方须将 ProofID 与 Generation 记录到执行日志。
type CoolGenProof struct {
	Asset string
	ProofID string
	Generation int64
}

// VerifyExecWithCoolGen 执行前显式复验冷却世代，返回通过的资产证明列表。
// 功能：供归集执行层在调用 treasury.Exec 前最后一道验证；
// 拒绝的资产会产生 CoolBreachCase，且该资产不得进入 Exec 流程。
// 风险点：
// 返回的 CoolGenProof 列表仅覆盖通过校验的资产；调用方须检查拒绝资产并停止其 Exec。
// 本函数不执行任何出金动作，仅做校验。
   func VerifyExecWithCoolGen(ctx context.Context, engine *coolmarkgate.Engine, assets []string, execID string) ([]CoolGenProof, []string, error) {
	  if engine == nil {
		  return nil, nil, fmt.Errorf("sweeppol.VerifyExecWithCoolGen: nil engine")
	   }
	   passed := make([]CoolGenProof, 0, len(assets))
	   blocked := make([]string, 0)
	   for _, raw := range assets {
		   asset := strings.ToUpper(strings.TrimSpace(raw))
		  if asset == "" {
			   continue
		  }
		   gen, err := engine.GetCoolGen(ctx, asset)
		  if err != nil {
			   return nil, nil, fmt.Errorf("sweeppol.VerifyExecWithCoolGen: get cool_gen %s: %w", asset, err)
		  }
		   res, err := engine.CheckExecAllowed(ctx, coolmarkgate.ExecVerifyInput{
			  Asset:   asset,
			  CoolGen: gen,
			   ExecID:  execID,
		  })
		   if err != nil || (res != nil && !res.Allowed) {
			   blocked = append(blocked, asset)
			   continue
		  }
		  passed = append(passed, CoolGenProof{
			  Asset: asset,
			   ProofID: res.ProofID,
			   Generation: res.Generation,
		   })
	  }
	   return passed, blocked, nil
  }
// temporary guard removed after the production check
