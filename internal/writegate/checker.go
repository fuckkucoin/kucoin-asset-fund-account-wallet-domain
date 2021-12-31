package writegate

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"
)

// ─── Checker ──────────────────────────────────────────────────────────────
// Checker 写路径联合 Check 核心。
type Checker struct {
	probeReader ProbeMutexReader
	tokenReader GenTokenReader
	poisonReader PoisonProofReader
	clock func() time.Time
}

// NewChecker 初始化 Checker。
// 参数说明：
// - probeReader：探针互斥状态只读钩子（不可为 nil）。
// - tokenReader：令牌世代只读钩子（不可为 nil）。
// - poisonReader：毒价证明只读钩子（可为 nil，nil 时跳过毒价挡写检查）。
  func NewChecker(probeReader ProbeMutexReader, tokenReader GenTokenReader, poisonReader PoisonProofReader) *Checker {
	  return &Checker{
		  probeReader:  probeReader,
		  tokenReader:  tokenReader,
		  poisonReader: poisonReader,
		  clock:        time.Now,
	   }
  }

// WithClock 替换时钟（注入）。

   func (c *Checker) WithClock(fn func() time.Time) *Checker {
	  c.clock = fn
	  return c
  }

  func (c *Checker) now() time.Time {
	   if c.clock != nil {
		  return c.clock()
	   }
	  return time.Now().UTC()
   }

// ─── CheckGate 联合 Check ─────────────────────────────────────────────────
// CheckResult CheckGate 结果。

type CheckResult struct {
	// Snap 联合 Check 快照（含 Allowed 标志）。
	Snap WriteGateSnap
	// Probe 探针状态快照。
	Probe ProbeState
	// Token 令牌世代快照。
	Token GenTokenSnap
}

// CheckGate 对指定写作用域执行联合 Check（探针 + 令牌 + 毒价）。
// 评估顺序（严格执行，先拒后止）：
// 1. 毒价挡写检查（HasActivePoison）→ 拒绝。
// 2. 令牌有效性检查（GenToken.State=active）→ revoked/expired 拒绝。
// 3. 探针挡写检查（IsProbing=true 时挡写）→ 拒绝（注意：探针挡写与令牌互为独立轴）。
// 注意：步骤 2、3 读取之间存在非原子窗口；probetokengate 联合租约负责在写提交时复验。
// 返回：CheckResult（含 Snap.Allowed 布尔与 RejectReason）；
// Allowed=false 时调用方不得签发联合租约。
  func (c *Checker) CheckGate(_ context.Context, scope CheckScope) (*CheckResult, error) {
	  if err := scope.Validate(); err != nil {
		  return nil, fmt.Errorf("%w: %v", ErrScopeInvalid, err)
	   }
	   now := c.now()
	   result := &CheckResult{}
	  // 步骤 1：毒价挡写检查（只读，不改写 PoisonEngine）。
	   if c.poisonReader != nil {
		  active, err := c.poisonReader.HasActivePoison(scope)
		   if err != nil {
			   return nil, fmt.Errorf("writegate: poison check: %w", err)
		   }
		   if active {
			   snap := WriteGateSnap{
				  Scope: scope,
				  Allowed:      false,
				  RejectReason: "poison_block",
				   SnapAt:       now,
			  }
			   return &CheckResult{Snap: snap}, ErrPoisonBlock
		   }
	   }
	  // 步骤 2：令牌世代读取（非原子窗口开始）。
	  tokenSnap, err := c.tokenReader.ReadGenToken(scope)
	  if err != nil {
		   return nil, fmt.Errorf("writegate: read gen token: %w", err)
	   }
	  tokenSnap.SnapAt = now
	   switch tokenSnap.State {
	   case TokenStateRevoked:
		   snap := WriteGateSnap{
			   Scope: scope,
			   Token: tokenSnap,
			  Allowed:      false,
			  RejectReason: "token_revoked",
			  SnapAt:       now,
		  }
		   return &CheckResult{Snap: snap, Token: tokenSnap}, ErrTokenRevoked
	   case TokenStateExpired:
		   snap := WriteGateSnap{
			   Scope: scope,
			   Token: tokenSnap,
			   Allowed:      false,
			   RejectReason: "token_expired",
			  SnapAt:       now,
		  }
		  return &CheckResult{Snap: snap, Token: tokenSnap}, ErrTokenExpired
	   }
	  // 步骤 3：探针互斥状态读取（非原子窗口持续——令牌读取后，探针可能已翻转）。
	  probeState, err := c.probeReader.ReadProbeState(scope)
	  if err != nil {
		   return nil, fmt.Errorf("writegate: read probe state: %w", err)
	   }
	   probeState.SnapAt = now
	  // 计算联合快照指纹。
	  snapFP := computeSnapFingerprint(probeState.StateFingerprint, tokenSnap.TokenHash, now)
	   snap := WriteGateSnap{
		   Scope: scope,
		  Probe: probeState,
		   Token:           tokenSnap,
		  SnapFingerprint: snapFP,
		   Allowed:         true,
		   SnapAt:          now,
	  }
	   result.Snap = snap
	   result.Probe = probeState
	  result.Token = tokenSnap
	  return result, nil
  }

// ReadProbeState 公开探针状态读取（供 probetokengate CommitCheck 复验使用）。

  func (c *Checker) ReadProbeState(_ context.Context, scope CheckScope) (ProbeState, error) {
	  return c.probeReader.ReadProbeState(scope)
   }

// ReadGenToken 公开令牌世代读取（供 probetokengate CommitCheck 复验使用）。
  func (c *Checker) ReadGenToken(_ context.Context, scope CheckScope) (GenTokenSnap, error) {
	   return c.tokenReader.ReadGenToken(scope)
   }

// computeSnapFingerprint 计算联合 Check 快照指纹。
// 输入：探针指纹 + 令牌指纹 + 时刻（纳秒精度）。
  func computeSnapFingerprint(probeFP, tokenHash string, at time.Time) string {
	  raw := fmt.Sprintf("%s|%s|%d", probeFP, tokenHash, at.UnixNano())
	   sum := sha256.Sum256([]byte(raw))
	  return fmt.Sprintf("%x", sum[:])
   }

// ComputeProbeFingerprint 计算探针状态指纹（供 ProbeMutexReader 实现使用）。
// 输入：pairID + isProbing 布尔 + 时刻。

  func ComputeProbeFingerprint(pairID string, isProbing bool, at time.Time) string {
	  prob := "0"
	   if isProbing {
		  prob = "1"
	  }
	  raw := fmt.Sprintf("%s|%s|%d", pairID, prob, at.UnixNano())
	  sum := sha256.Sum256([]byte(raw))
	  return fmt.Sprintf("%x", sum[:])
   }

// ComputeTokenHash 计算令牌世代指纹（供 GenTokenReader 实现使用）。
// 输入：generation + state + revoke reason + 时刻。
   func ComputeTokenHash(generation int64, state TokenState, revokeReason string, at time.Time) string {
	   raw := fmt.Sprintf("%d|%s|%s|%d", generation, state, revokeReason, at.UnixNano())
	  sum := sha256.Sum256([]byte(raw))
	  return fmt.Sprintf("%x", sum[:])
  }
