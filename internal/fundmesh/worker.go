// @FileName: worker.go
// @Summary: 配置闸巡检。
// 重点：周期复验 etcd 快照，缺键即关写域。
// 风险点：巡检本身不得过账或取消提现。
// @Author: Likeo
// @Created: 2021.06.15

package fundmesh

import (
	"context"
	"time"
)

  func (e *Engine) TickGate(ctx context.Context) error {
	  if e == nil {
		  return ErrUnavailable
	   }
	  return e.RefreshGate(ctx)
  }

   func GateInterval() time.Duration { return 15 * time.Second }
