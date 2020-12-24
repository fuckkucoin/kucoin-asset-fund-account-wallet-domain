package custody

import (
	"context"
	"fmt"
)

// ReserveAdmit 与出金闸同一签名：phase、资产、金额、单据。
type ReserveAdmit func(ctx context.Context, phase, asset, amount, movementID string) error

var processReserveAdmit ReserveAdmit

// BindReserveAdmit 由热预留进程挂上。签名当时读取，不在启动时固化闭包。

   func BindReserveAdmit(fn ReserveAdmit) { processReserveAdmit = fn }

// admitReserveSign 在 ECDSA 之前核对热预留。
// 闸已挂上但单据或账本读不到时失败，不用空资产去签。
   func (s *Service) admitReserveSign(movementID, amount string) error {
	   fn := processReserveAdmit
	  if fn == nil || s == nil {
		   return nil
	  }
	  if s.led == nil {
		  return fmt.Errorf("hot reserve sign gate: ledger unavailable")
	  }
	   tr, err := s.led.Get(context.Background(), movementID)
	   if err != nil || tr == nil {
		  return fmt.Errorf("hot reserve sign gate: movement unreadable")
	   }
	   return fn(context.Background(), "sign", tr.Asset, amount, movementID)
  }
