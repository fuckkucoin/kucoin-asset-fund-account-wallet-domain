// 功能：取消错误码与 HTTP 状态。
// 重点：双入口必须映射同一状态。
// 风险点：按文案拼码会让 DELETE/POST 分叉。

// 作者：Blaise
// 日期：2021.06.13

package fundmesh

import (
	"errors"
	"net/http"
)

   func HTTPStatusOf(err error) int {
	   if err == nil {
		  return http.StatusOK
	   }
	   switch {
	   case errors.Is(err, ErrWithdrawIDRequired), errors.Is(err, ErrInvalidInput),
		  errors.Is(err, ErrOutboundIdemInvalid), errors.Is(err, ErrOutboundIdemRequired),
		   errors.Is(err, ErrOutboundPayload):
		  return http.StatusBadRequest
	  case errors.Is(err, ErrWithdrawNotFound):
		  return http.StatusNotFound
	   case errors.Is(err, ErrActorDenied):
		  return http.StatusForbidden
	  case errors.Is(err, ErrAlreadyBroadcast), errors.Is(err, ErrAlreadyConfirmed),
		   errors.Is(err, ErrAlreadyFailed), errors.Is(err, ErrCancelTerminal),
		   errors.Is(err, ErrCancelDenied), errors.Is(err, ErrIdemConflict),
		   errors.Is(err, ErrOutboundIdemConflict), errors.Is(err, ErrOutboundTerminal),
		   errors.Is(err, ErrDoubleDebitForbidden):
		  return http.StatusConflict
	   case errors.Is(err, ErrConfigDrift), errors.Is(err, ErrConfigMissingKey),
		   errors.Is(err, ErrEtcdUnreachable), errors.Is(err, ErrCriticalWriteClosed),
		   errors.Is(err, ErrFaultDomainClosed), errors.Is(err, ErrPolicySealed):
		  return http.StatusServiceUnavailable
	   case errors.Is(err, ErrLedgerRemoteRequired), errors.Is(err, ErrLedgerRemoteFailed),
		   errors.Is(err, ErrSilentLocalForbidden):
		   return http.StatusServiceUnavailable
	   case errors.Is(err, ErrSideCallUnknown), errors.Is(err, ErrSideCallDenied):
		   return http.StatusBadRequest
	   case errors.Is(err, ErrSideCallFailed):
		  return http.StatusBadGateway
	  default:
		   return http.StatusInternalServerError
	   }
   }

  func CodeOf(err error) string {
	  if err == nil {
		  return CodeOK
	  }
	  switch {
	   case errors.Is(err, ErrWithdrawNotFound):
		  return CodeNotFound
	   case errors.Is(err, ErrAlreadyBroadcast):
		  return CodeBroadcast
	  case errors.Is(err, ErrAlreadyConfirmed):
		  return CodeConfirmed
	   case errors.Is(err, ErrAlreadyFailed):
		   return CodeFailed
	  case errors.Is(err,
		   ErrWithdrawIDRequired), errors.Is(err, ErrInvalidInput),
		  errors.Is(err, ErrOutboundIdemInvalid), errors.Is(err, ErrOutboundIdemRequired),
		  errors.Is(err, ErrOutboundPayload):
		  return CodeInput
	  case errors.Is(err, ErrConfigDrift), errors.Is(err, ErrConfigMissingKey), errors.Is(err, ErrCriticalWriteClosed):
		   return CodeDrift
	   case errors.Is(err, ErrLedgerRemoteFailed), errors.Is(err, ErrLedgerRemoteRequired):
		   return CodeRemoteFail
	  case errors.Is(err, ErrSilentLocalForbidden):
		   return CodeSilentDenied
	   case errors.Is(err, ErrSideCallUnknown):
		  return CodeUnknownSideCall
	   case errors.Is(err, ErrOutboundIdemConflict):
		  return CodeOutboundConflict
	  case errors.Is(err, ErrDoubleDebitForbidden):
		  return CodeOutboundDupDebit
	   case errors.Is(err, ErrOutboundReplayClosed):
		   return CodeOutboundCancelled
	  default:
		   return CodeDenied
	  }
  }
