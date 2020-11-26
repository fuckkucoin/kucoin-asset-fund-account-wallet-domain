package test

import (
	"testing"
	walletv1 "account-assets.kucoin.net/funding/api/proto/wallet/v1"
)

  func TestGetBalancesRequestZero(t *testing.T) {
	  var r walletv1.GetBalancesRequest
	  if r.GetUserId() != "" || r.GetChain() != "" {
		   t.Fatalf("%+v", r)
	   }
   }
