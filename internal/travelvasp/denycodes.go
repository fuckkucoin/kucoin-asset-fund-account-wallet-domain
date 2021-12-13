package travelvasp

import (
	"errors"
	"strings"
)

const (
	DenyDesk     = "TRV_DESK_UNAVAILABLE"
	DenyCorridor = "TRV_CORRIDOR_MISSING"
	DenyPeer      = "TRV_PEER_UNRESOLVED"
	DenyDisabled  = "TRV_VASP_DISABLED"
	DenyThreshold = "TRV_THRESHOLD_INVALID"
	DenyEnvelope = "TRV_ENVELOPE_INCOMPLETE"
	DenyProtocol = "TRV_PROTOCOL_MISMATCH"
	DenyPending  = "TRV_CASE_PENDING"
	DenyRejected = "TRV_CASE_REJECTED"
	DenyBroadcast = "TRV_BROADCAST_BLOCKED"
	DenyNetwork = "TRV_NETWORK_INVALID"
	DenyActor   = "TRV_ACTOR_INVALID"
	DenyIdem = "TRV_IDEMPOTENCY"
	DenyAdapter = "TRV_ADAPTER_FAILED"
	DenyHealth = "TRV_PEER_CIRCUIT"
	DenyIVMS   = "TRV_IVMS_INCOMPLETE"
	DenyRetry  = "TRV_RETRY_BUDGET"
)

// CodeOf 从错误映射稳定拒绝码。

   func CodeOf(err error) string {
	   if err == nil {
		  return ""
	   }
	   switch {
	  case errors.Is(err, ErrUnavailable):
		  return DenyDesk
	  case errors.Is(err, ErrCorridor):
		  return DenyCorridor
	  case errors.Is(err, ErrPeer):
		   return DenyPeer
	  case errors.Is(err, ErrDisabled):
		   return DenyDisabled
	  case errors.Is(err,
		  ErrEnvelope):
		  return DenyEnvelope
	  case errors.Is(err, ErrBroadcast):
		   return DenyBroadcast
	  case errors.Is(err, ErrNetwork):
		   return DenyNetwork
	  case errors.Is(err, ErrActor):
		   return DenyActor
	  case errors.Is(err, ErrIdem):
		   return DenyIdem
	   case errors.Is(err, ErrAdapter), errors.Is(err, ErrPoison):
		   return DenyAdapter
	   case errors.Is(err, ErrProtocol):
		   return DenyProtocol
	   case errors.Is(err,
		   ErrPeerHealth):
		  return DenyHealth
	   case errors.Is(err, ErrIVMS):
		   return DenyIVMS
	   case errors.Is(err, ErrRetryBudget):
		   return DenyRetry
	  case errors.Is(err, ErrAmount), errors.Is(err, ErrAsset):
		   return DenyThreshold
	  default:
		  msg := strings.ToLower(err.Error())
		   if strings.Contains(msg, "threshold") {
			   return DenyThreshold
		  }
		  if strings.Contains(msg, "protocol") {
			   return DenyProtocol
		  }
		  return DenyBroadcast
	  }
   }

// StampDeny 给闸结果打稳定拒绝码。
  func StampDeny(r *GateResult, err error) {
	   if r == nil || err == nil {
		   return
	  }
	  r.DenyCode = CodeOf(err)
	  if r.Reason == "" {
		   r.Reason = err.Error()
	  }
   }
