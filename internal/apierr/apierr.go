// 领域错误到 HTTP/gRPC 码。
// 作者: King
// 日期: 2021.02.06
package apierr

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"account-assets.kucoin.net/funding/internal/authn"
	"account-assets.kucoin.net/funding/internal/callerid"
	"account-assets.kucoin.net/funding/internal/cases"
	"account-assets.kucoin.net/funding/internal/command"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/perm"
	"account-assets.kucoin.net/funding/internal/persist"
	"account-assets.kucoin.net/funding/internal/recon"
	"account-assets.kucoin.net/funding/pkg/money"
	"google.golang.org/grpc/codes"
)

type mapped struct {
	err error
	status int
}

var extra []mapped

// Register 给业务包登记哨兵错误。包不得反向 import 本文件里的领域包。
  func Register(err error, status int) {
	   if err == nil || status <= 0 {
		   return
	   }
	   extra = append(extra, mapped{err: err, status: status})
   }

// HTTPStatus 领域错误到传输层状态。handler 不得按文案拼码。
   func HTTPStatus(err error) int {
	  if err == nil {
		   return http.StatusOK
	   }
	   for _, m := range extra {
		  if errors.Is(err, m.err) {
			  return m.status
		  }
	  }
	   switch {
	  case errors.Is(err,
		   context.DeadlineExceeded):
		  return http.StatusGatewayTimeout
	  case errors.Is(err, persist.ErrNotFound):
		   return http.StatusNotFound
	  case errors.Is(err, persist.ErrStaleFence), errors.Is(err, persist.ErrConflict):
		  return http.StatusConflict
	  case errors.Is(err, persist.ErrTxClosed):
		  return http.StatusInternalServerError
	  case errors.Is(err, recon.ErrNotFound):
		  return http.StatusNotFound
	   case errors.Is(err, recon.ErrClosed), errors.Is(err, recon.ErrEmpty):
		   return http.StatusConflict
	  case errors.Is(err, recon.ErrActor):
		   return http.StatusForbidden
	   case errors.Is(err, money.ErrInvalid), errors.Is(err, money.ErrScientific),
		  errors.Is(err,
			   money.ErrPrecision), errors.Is(err, money.ErrNotPositive),
		   errors.Is(err,
			   money.ErrNegative), errors.Is(err, money.ErrOverflow):
		  return http.StatusBadRequest
	   case errors.Is(err, ledger.ErrPending):
		   return http.StatusAccepted
	   case errors.Is(err, ledger.ErrKYTReview):
		  return http.StatusAccepted
	   case errors.Is(err, authn.ErrUnauthorized), errors.Is(err, callerid.ErrMissing):
		  return http.StatusUnauthorized
	  case errors.Is(err, perm.ErrDenied), errors.Is(err, cases.ErrMakerCheck),
		  errors.Is(err, cases.ErrExpired):
		   return http.StatusForbidden
	  case errors.Is(err,
		  ledger.ErrKYTDenied),
		   errors.Is(err, ledger.ErrAccountFrozen),
		  errors.Is(err, ledger.ErrKYCRequired),
		   errors.Is(err, ledger.ErrMakerChecker),
		   errors.Is(err, ledger.ErrWithdrawCooling),
		  errors.Is(err, ledger.ErrOwnInbound),
		   errors.Is(err, ledger.ErrAddressPoison),
		  errors.Is(err, ledger.ErrPassThrough):
		   return http.StatusForbidden
	  case errors.Is(err, ledger.ErrKYTUnavailable):
		   return http.StatusServiceUnavailable
	   case errors.Is(err, ledger.ErrNotFound), errors.Is(err, cases.ErrNotFound):
		   return http.StatusNotFound
	  case errors.Is(err,
		  ledger.ErrBusy):
		  return http.StatusTooManyRequests
	  case errors.Is(err, ledger.ErrIdempotencyConflict),
		  errors.Is(err, command.ErrConflict),
		   errors.Is(err,
			   cases.ErrExists),
		  errors.Is(err, cases.ErrClosed),
		   errors.Is(err, ledger.ErrBooksDrift),
		  errors.Is(err, ledger.ErrSlippage),
		  errors.Is(err, ledger.ErrRolledBack),
		   errors.Is(err, ledger.ErrSubmittedLive),
		  errors.Is(err, ledger.ErrDuplicateHash),
		  errors.Is(err, ledger.ErrStaleFence),
		   errors.Is(err, ledger.ErrFreezeGate),
		   errors.Is(err,
			   ledger.ErrObservationMismatch),
		  errors.Is(err, ledger.ErrKYTNotPending),
		  errors.Is(err, ledger.ErrHotInventory):
		   return http.StatusConflict
	   case errors.Is(err, ledger.ErrInsufficient),
		   errors.Is(err,
			   ledger.ErrRiskRejected),
		   errors.Is(err, ledger.ErrScientific),
		  errors.Is(err, ledger.ErrPrecision),
		   errors.Is(err, ledger.ErrStaleQuote),
		   errors.Is(err, ledger.ErrInvalidAmount),
		   errors.Is(err, ledger.ErrNegative),
		   errors.Is(err, ledger.ErrIllegalTransition),
		  errors.Is(err, ledger.ErrUnknownDirection),
		  errors.Is(err,
			   ledger.ErrMissingChain),
		   errors.Is(err, ledger.ErrMissingAddress),
		   errors.Is(err,
			  ledger.ErrMissingIdempotency),
		   errors.Is(err, ledger.ErrAssetChain),
		   errors.Is(err, ledger.ErrInvalidAddress),
		   errors.Is(err, ledger.ErrSelfTransfer),
		   errors.Is(err, ledger.ErrCounterparty),
		   errors.Is(err, ledger.ErrCannotCancel),
		  errors.Is(err,
			   ledger.ErrInvalidKYC),
		   errors.Is(err, ledger.ErrAmountOverflow),
		  errors.Is(err,
			   ledger.ErrDepositAddress),
		  errors.Is(err, ledger.ErrAddressFormat):
		  return http.StatusBadRequest
	   default:
		   msg := strings.ToLower(err.Error())
		  if strings.Contains(msg, "required") || strings.Contains(msg, "invalid") {
			   return http.StatusBadRequest
		   }
		  return http.StatusInternalServerError
	   }
   }

  func GRPCCode(err error) codes.Code {
	   if err == nil {
		   return codes.OK
	  }
	   switch {
	   case errors.Is(err, ledger.ErrNotFound):
		   return codes.NotFound
	  case errors.Is(err, ledger.ErrBusy):
		   return codes.ResourceExhausted
	   case errors.Is(err, ledger.ErrInvalidAmount),
		   errors.Is(err, ledger.ErrScientific),
		   errors.Is(err, ledger.ErrPrecision),
		  errors.Is(err, ledger.ErrRiskRejected),
		   errors.Is(err, ledger.ErrUnknownDirection),
		  errors.Is(err, ledger.ErrMissingChain),
		   errors.Is(err, ledger.ErrMissingAddress),
		   errors.Is(err, ledger.ErrMissingIdempotency),
		  errors.Is(err,
			   ledger.ErrAssetChain),
		  errors.Is(err, ledger.ErrInvalidAddress):
		   return codes.InvalidArgument
	  case errors.Is(err, ledger.ErrBooksDrift),
		   errors.Is(err, ledger.ErrIdempotencyConflict),
		   errors.Is(err, ledger.ErrSubmittedLive),
		  errors.Is(err, ledger.ErrDuplicateHash):
		  return codes.FailedPrecondition
	  case errors.Is(err, context.DeadlineExceeded):
		   return codes.DeadlineExceeded
	   default:
		  return codes.Internal
	  }
  }
