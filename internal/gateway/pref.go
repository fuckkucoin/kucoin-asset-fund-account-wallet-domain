package gateway

import (
	"context"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/fundpref"
	"github.com/gin-gonic/gin"
)

  func registerPref(v1 *gin.RouterGroup, rt *app.Runtime) {
	  registerPrefMesh(v1, rt)
   }

   func applyLiteDefault(rt *app.Runtime, ctx context.Context, user string, view *LiteView) error {
	  if view == nil || rt == nil {
		   return nil
	   }
	  book := prefBook(rt)
	   got, err := book.ResolveDefaultView(ctx, user)
	  if err != nil {
		  return err
	  }
	   view.DefaultDeposit = got.AccountType
	  return nil
  }

  func applyValuedOrder(rt *app.Runtime, ctx context.Context, user string, rows []Valued) ([]Valued, error) {
	   if rt == nil || len(rows) == 0 {
		   return rows, nil
	  }
	  p, err := prefBook(rt).Get(ctx, user)
	  if err != nil {
		   return nil, err
	   }
	   if len(p.AssetOrder) == 0 {
		   return rows, nil
	   }
	   return fundpref.ApplyOrder(rows, p.AssetOrder, func(v Valued) string { return v.Asset }), nil
  }
