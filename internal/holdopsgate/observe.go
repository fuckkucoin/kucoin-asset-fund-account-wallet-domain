package holdopsgate

import "context"

type ObserveView struct {
	Stats  Stats             `json:"stats"`
	Recent []AuditEvent      `json:"recent"`
	Gen    HoldOpsGeneration `json:"generation"`
}

  func Observe(ctx context.Context, svc *Service) ObserveView {
	  if svc == nil {
		   return ObserveView{}
	   }
	  gen, _ := svc.GetGeneration(ctx)
	   return ObserveView{Stats: svc.Stats(ctx), Recent: svc.AuditRing(), Gen: gen}
  }
