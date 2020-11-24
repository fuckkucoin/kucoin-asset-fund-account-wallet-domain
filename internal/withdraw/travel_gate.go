// @FileName: travel_gate.go
// @Summary: 出金广播前旅行规则闸钩子。仅检查已落案件是否 acknowledged；不改 KYT / 交易密码面。未挂钩子时保持原广播路径，须由 travelvasp.Register 挂上。
// @Author: Draven
// @Created: 2020.11.03

package withdraw

type TravelGate func(movementID string) error

  func (s *Service) BindTravelGate(fn TravelGate) *Service {
	  if s != nil {
		   s.travelGate = fn
	   }
	   return s
   }

  func (s *Service) checkTravelBroadcast(movementID string) error {
	   if s == nil || s.travelGate == nil {
		   return nil
	   }
	  return s.travelGate(movementID)
   }
