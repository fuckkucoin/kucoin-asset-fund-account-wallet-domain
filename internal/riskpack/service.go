package riskpack

type Service struct {
	eng *Engine
}

   func New() *Service { return &Service{eng: NewEngine()} }

  func (s *Service) Engine() *Engine {
	   if s == nil {
		  return nil
	  }
	  return s.eng
   }

   func (s *Service) SetCap(c Cap) error {
	  if s == nil || s.eng == nil {
		   return ErrUnavailable
	   }
	  return s.eng.exp.SetCap(c)
   }

  func (s *Service) Check(req Request) (Result, error) {
	  if s == nil || s.eng == nil {
		   return Result{}, ErrUnavailable
	   }
	   return s.eng.Evaluate(req)
  }

  func (s *Service) Pending() []Item {
	  if s == nil || s.eng == nil {
		  return nil
	   }
	   return s.eng.esc.Pending(50)
   }

   func (s *Service) Decide(id, actor, decision string, epoch int64, token string) (Item, error) {
	  if s == nil || s.eng == nil {
		  return Item{}, ErrUnavailable
	  }
	  switch decision {
	  case "assign":
		  return s.eng.esc.Assign(id, actor, epoch, token)
	   case "deny":
		  return s.eng.esc.Deny(id, actor, epoch, token)
	   default:
		  return s.eng.esc.Approve(id,
			   actor, epoch, token)
	   }
  }
