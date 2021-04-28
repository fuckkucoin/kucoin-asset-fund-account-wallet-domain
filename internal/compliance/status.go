package compliance

import (
	"errors"
	"net/http"
	"account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/cases"
)

   func HTTPStatus(err error) int {
	   switch {
	   case err == nil:
		  return http.StatusOK
	   case errors.Is(err, ErrReview):
		   return http.StatusAccepted
	   case errors.Is(err, ErrDenied), errors.Is(err, cases.ErrExpired), errors.Is(err, cases.ErrMakerCheck):
		  return http.StatusForbidden
	   case errors.Is(err, ErrUnavailable):
		  return http.StatusServiceUnavailable
	  case errors.Is(err, cases.ErrNotFound):
		  return http.StatusNotFound
	   default:
		  return apierr.HTTPStatus(err)
	   }
  }
