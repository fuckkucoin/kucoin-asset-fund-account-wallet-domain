// @FileName: reserve_gate.go
// @Summary: 出金签名与提交前的热预留闸。未挂载时保持原路径；挂载后由 hotreserve 失败即关。
// @Author: Finn
// @Created: 2021.01.13

package withdraw

import "context"

const (
	PhaseSign   = "sign"
	PhaseSubmit = "submit"
)

// ReserveAdmit 签名或提交前核对热预留。phase 为 sign 或 submit。
type ReserveAdmit func(ctx context.Context,
	  phase, asset, amount, movementID string) error

   func (s *Service) BindReserveAdmit(fn ReserveAdmit) *Service {
	  if s != nil {
		  s.reserveAdmit = fn
	   }
	   return s
  }

   func (s *Service) admitReserve(ctx context.Context, phase, asset, amount, movementID string) error {
	  if s == nil || s.reserveAdmit == nil {
		  return nil
	  }
	  return s.reserveAdmit(ctx, phase, asset, amount, movementID)
   }
