package georebal

import (
	"errors"
	"fmt"
)

const (
	CodeRegionHalted = "region_halted"
	CodeFirewallBlocked = "firewall_blocked"
	CodePlanHalted      = "plan_halted"
	CodeBadTransition   = "bad_transition"
	CodeNotFound = "plan_not_found"
	CodeInvalid  = "plan_invalid"
	CodeConserve = "plan_conserve"
	CodeApproval  = "plan_approval"
	CodeLegFailed = "leg_failed"
	CodeStoreErr = "store_err"
	CodeActor = "actor_required"
	CodeFirewall = "firewall_check"
	CodeIdem = "idem_conflict"
	CodeQuota = "plan_quota"
)

var (
	ErrRegionHalted    = errors.New("georebal: region halted; new plan rejected")
	ErrFirewallBlocked = errors.New("georebal: firewall blocked")
	ErrPlanHalted      = errors.New("georebal: plan halted")
	ErrBadTransition = errors.New("georebal: bad state transition")
	ErrNotFound      = errors.New("georebal: not found")
	ErrInvalid       = errors.New("georebal: invalid")
	ErrConserve = errors.New("georebal: amount not conserved")
	ErrApproval = errors.New("georebal: pending approval")
	ErrLegFailed = errors.New("georebal: leg failed")
	ErrStore = errors.New("georebal: store error")
	ErrActor = errors.New("georebal: actor required")
	ErrFirewall    = errors.New("georebal: firewall check failed")
	ErrIdem        = errors.New("georebal: idempotency conflict")
	ErrQuota       = errors.New("georebal: quota")
	ErrUnavailable = errors.New("georebal: engine unavailable")
	ErrInvalidReq = errors.New("georebal: invalid request")
	ErrPlanNotFound = errors.New("georebal: plan not found")
	ErrLegNotFound    = errors.New("georebal: leg not found")
	ErrRegionNotFound = errors.New("georebal: region not found")
	ErrInsufficientInv = errors.New("georebal: insufficient regional inventory")
	ErrLegNotRunnable = errors.New("georebal: leg not runnable")
	ErrPlanNotOpen    = errors.New("georebal: plan not open")
	ErrEnqueueFailed  = errors.New("georebal: collect intent enqueue failed")
	ErrTransferFailed = errors.New("georebal: regional inventory transfer failed")
	ErrStateUpdate = errors.New("georebal: update leg state failed")
	ErrAlreadyMoved  = errors.New("georebal: inventory already moved for this leg")
	ErrFenceRequired = errors.New("georebal: transfer requires leg_move_fence")
)

   func Wrap(op string, err error) error {
	   if err == nil {
		  return nil
	   }
	  return fmt.Errorf("georebal: %s: %w", op, err)
   }

  func CodeOf(err error) string {
	   switch {
	   case err == nil:
		   return ""
	   case errors.Is(err, ErrRegionHalted):
		  return CodeRegionHalted
	  case errors.Is(err,
		   ErrFirewallBlocked):
		   return CodeFirewallBlocked
	  case errors.Is(err, ErrPlanHalted):
		   return CodePlanHalted
	   case errors.Is(err, ErrBadTransition):
		   return CodeBadTransition
	  case errors.Is(err, ErrNotFound):
		   return CodeNotFound
	   case errors.Is(err, ErrConserve):
		  return CodeConserve
	  case errors.Is(err,
		   ErrApproval):
		  return CodeApproval
	  case errors.Is(err, ErrLegFailed):
		   return CodeLegFailed
	   case errors.Is(err, ErrStore):
		   return CodeStoreErr
	  case errors.Is(err, ErrActor):
		   return CodeActor
	   case errors.Is(err, ErrFirewall):
		  return CodeFirewall
	   case errors.Is(err, ErrIdem):
		   return CodeIdem
	  case errors.Is(err, ErrQuota):
		  return CodeQuota
	  default:
		  return CodeInvalid
	   }
   }
