package acctpol

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"time"
)

// EvalResult 单次策略评估结果。

type EvalResult struct {
	// Allow 为 false 时 Reason 说明拒绝原因。
	Allow  bool   `json:"allow"`
	Reason string `json:"reason,omitempty"`
	PolicyVersion int `json:"policy_version"`
	BizKind BizKind `json:"biz_kind"`
}

// EvalRequest 策略评估输入。

type EvalRequest struct {
	BizKind BizKind `json:"biz_kind"`
	// HopFrom/HopTo 请求的科目 hop（可选，若填则 gate 校验 hop 是否允许）。
	HopFrom string `json:"hop_from,omitempty"`
	HopTo string `json:"hop_to,omitempty"`
	// UserID 用于灰度路由（fnv hash % 100 决定是否命中新代次）。
	UserID string `json:"user_id,omitempty"`
	// At 评估时刻（零值用 time.Now）。
	At time.Time `json:"at,omitempty"`
}

// Engine 策略评估引擎（并发安全）。

type Engine struct {
	store *Store
	pins  sync.Map // userID → int (pinned version)
	clock func() time.Time
}

// NewEngine 创建引擎；store 不得为 nil。

  func NewEngine(store *Store) *Engine {
	  return &Engine{
		  store: store,
		   clock: func() time.Time { return time.Now().UTC() },
	  }
  }

// WithClock 替换时钟（用）。

  func (e *Engine) WithClock(c func() time.Time) *Engine {
	  if e != nil && c != nil {
		  e.clock = c
	   }
	   return e
   }

  // PinUser 将指定用户钉在某一代次（先于灰度路由）。
   func (e *Engine) PinUser(userID string, version int) {
	  e.pins.Store(strings.TrimSpace(userID), version)
  }

// UnpinUser 解除用户 Pin（恢复灰度路由）。
  func (e *Engine) UnpinUser(userID string) {
	  e.pins.Delete(strings.TrimSpace(userID))
  }

// Eval 评估请求是否符合当前活跃策略。
// 若 HopFrom/HopTo 非空，还会校验该 hop 是否在规则允许列表中。

  func (e *Engine) Eval(ctx context.Context, req EvalRequest) (EvalResult, error) {
	  at := req.At
	   if at.IsZero() {
		   at = e.clock()
	   }
	  pv, err := e.selectVersion(ctx, req.UserID, at)
	   if err != nil {
		   return EvalResult{}, err
	   }
	  res := EvalResult{PolicyVersion: pv.Version, BizKind: req.BizKind}
	  rule, err := pv.RuleFor(req.BizKind)
	  if err != nil {
		   res.Reason = err.Error()
		   return res, nil
	   }
	   // hop 检查（可选）
	  if req.HopFrom != "" || req.HopTo != "" {
		   if !rule.AllowsHop(req.HopFrom, req.HopTo) {
			   res.Reason = fmt.Sprintf("%s: %s→%s", ErrHopNotAllowed.Error(), req.HopFrom, req.HopTo)
			   return res, nil
		   }
	   }
	   res.Allow = true
	   return res, nil
  }

// selectVersion 按用户 Pin → 灰度路由 → 当前代次的先级选定评估代次。
   func (e *Engine) selectVersion(ctx context.Context, userID string, at time.Time) (PolicyVersion, error) {
	   // 1. 用户 Pin
	  if v, ok := e.pins.Load(strings.TrimSpace(userID)); ok {
		   ver := v.(int)
		  return e.store.Get(ctx, ver)
	   }
	  // 2. 当前代次
	  cur, err := e.store.Current(ctx)
	  if err != nil {
		  return PolicyVersion{}, err
	   }
	  if !cur.IsEffective(at) {
		   // 当前代次窗口未到或已过，尝试找最近一个有效代次
		  pv, err := e.latestEffective(ctx, at)
		  if err != nil {
			   return PolicyVersion{}, fmt.Errorf("no effective policy at %s", at.Format(time.RFC3339))
		   }
		  return pv, nil
	  }
	  // 3. 灰度路由：若 GrayRatio < 100 且用户不在灰度桶内，降级到前一代次
	   if cur.GrayRatio < 100 && userID != "" {
		   if !inGrayBucket(userID,
			   cur.GrayRatio) {
			   prev := cur.Version - 1
			   if prev > 0 {
				   if pv, err2 := e.store.Get(ctx, prev); err2 == nil {
					  return pv, nil
				  }
			  }
		  }
	   }
	   return cur, nil
  }

// latestEffective 遍历所有代次找最近一个 IsEffective 的，用于窗口切换间隙兜底。
   func (e *Engine) latestEffective(ctx context.Context, at time.Time) (PolicyVersion, error) {
	   vers := e.store.Versions(ctx)
	   for i := len(vers) - 1; i >= 0; i-- {
		   pv, err := e.store.Get(ctx, vers[i])
		  if err != nil {
			  continue
		   }
		   if pv.IsEffective(at) {
			  return pv, nil
		   }
	  }
	  return PolicyVersion{}, ErrPolicyVersionNotFound
  }

// inGrayBucket 用 FNV hash 决定用户是否命中灰度百分比。

  func inGrayBucket(userID string, pct int) bool {
	   if pct <= 0 {
		  return false
	   }
	  if pct >= 100 {
		  return true
	   }
	   h := fnv.New32a()
	   _, _ = h.Write([]byte(userID))
	  return int(h.Sum32()%100) < pct
  }
