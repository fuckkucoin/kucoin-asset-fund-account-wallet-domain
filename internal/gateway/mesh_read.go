// 剖面先 fundmesh.LedgerAccounts；传输或 5xx 时改用本进程 ledger.Service，
// 避免在网关层合并两套余额。写路径（冻结、划转）不得经过本文件。

// Author: Axel
// Date: 2021.06.29

package gateway

import (
	"context"
	"errors"
	"net/http"
	  "strings"
	    "account-assets.kucoin.net/funding/internal/apierr"
	"account-assets.kucoin.net/funding/internal/app"
	    "account-assets.kucoin.net/funding/internal/fundmesh"
	"account-assets.kucoin.net/funding/internal/ledger"
)

var errMeshDown = errors.New("downstream unavailable")

   func init() {
	   apierr.Register(errMeshDown, http.StatusServiceUnavailable)
}

// ledgerBooks 返回用户科目视图；auth 透传下游 IAM。

  func ledgerBooks(ctx context.Context, rt *app.Runtime, userID, authorization string) ([]ledger.BookView, error) {
	   if rt == nil || rt.Ledger == nil {
	  	   return nil, errBookUnavailable
	  }
	      userID = strings.TrimSpace(userID)
	   if userID == "" {
		   return nil, errBookUnavailable
	   }
	  if rt.FundMesh != nil && rt.FundMesh.Active() {
		   opt := fundmesh.CallOpts{UserID: userID,
			   Authorization: strings.TrimSpace(authorization)}
		  books, err := rt.FundMesh.LedgerAccounts(ctx, opt, userID)
		   if err != nil {
			  rt.Log.Warn().Err(err).Str("user", userID).Msg("gateway ledger mesh")
			  return nil, errMeshDown
		  }
		  return books, nil
	  }
	   return rt.Ledger.Accounts(ctx, userID)
   }
