// 功能：invcheck 偏离钩子：为 invdrift 管线提供快速预检接口。
// DriftGate.Check 在签出金/超额归集预检时查询活跃封堵建议；
// 存在 proposed 建议时返回非 nil error，调用方决定是否挡写。
// 本文件只做只读查询，不修改任何账本状态。

// 作者：jakson
// 日期：2021.07.27

package invcheck

import (
	"context"
	"fmt"
	"strings"
)

// DriftChecker 封堵建议查询接口（由 invdrift.Service 实现）
type DriftChecker interface {
	// HasActiveAdvice 返回指定资产当前是否存在 proposed 封堵建议。
	HasActiveAdvice(ctx context.Context, asset string) (bool, error)
}

// DriftGate 偏离封堵检查闸；嵌入 Service 以支持出金预检路径。

type DriftGate struct {
	checker DriftChecker
}

// NewDriftGate 构造 DriftGate；checker 为 nil 时 Check 始终放行（用）。
  func NewDriftGate(checker DriftChecker) *DriftGate {
	  return &DriftGate{checker: checker}
   }

// ErrSealAdviceActive 封堵建议活跃时返回此错误

var ErrSealAdviceActive = fmt.Errorf("invcheck: active seal advice for asset, write path blocked")

// ErrCheckerUnavailable 封堵检查器不可用时返回此错误（fail-closed）
var ErrCheckerUnavailable = fmt.Errorf("invcheck: seal advice checker unavailable, write path blocked")

// Check 预检指定资产是否有活跃封堵建议。
// 存在建议或查询失败均返回非 nil error（fail-closed）；checker 为 nil 放行（仅）。

   func (g *DriftGate) Check(ctx context.Context, asset string) error {
	  if g == nil || g.checker == nil {
		   return nil
	  }
	   asset = strings.ToUpper(strings.TrimSpace(asset))
	  if asset == "" {
		   return nil
	  }
	   has, err := g.checker.HasActiveAdvice(ctx, asset)
	   if err != nil {
		   // 查询失败时 fail-closed：封堵检查器不可用视为存在风险。
		   return fmt.Errorf("%w: %v", ErrCheckerUnavailable, err)
	   }
	  if has {
		   return ErrSealAdviceActive
	  }
	  return nil
  }
