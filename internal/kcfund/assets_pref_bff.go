// @FileName: assets_pref_bff.go
// @Summary: 资产展示偏好 BFF query 并入 body。
// @Author: Cason
// @Created: 2021.08.01

package kcfund

import (
	   "strings"

	      "account-assets.kucoin.net/funding/internal/fundpref"
	   "github.com/gin-gonic/gin"
)

// mergePreferenceWriteQuery 将 query 中的 quote、hideSmall 并入 PUT body。
   func mergePreferenceWriteQuery(c *gin.Context, req fundpref.WriteReq) fundpref.WriteReq {
	  if strings.TrimSpace(req.DisplayCurrency) == "" {
	  req.DisplayCurrency = strings.TrimSpace(bffParam(c, "quote", "quoteCurrency", "display_currency", "currency"))
	  }
	if req.HideZero == nil && req.HideSmallAlt == nil && req.HideSmall == nil {
		if v, ok := bffParamBool(c, "hideSmall", "hide_small", "hideZero"); ok {
	      	   req.HideZero = &v
		  req.HideSmallAlt = &v
		}
	}
	      return req
}
