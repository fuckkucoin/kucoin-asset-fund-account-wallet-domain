package kcfund

import (
	"net/http"
	"github.com/gin-gonic/gin"
)

// sideCall 有注册实例则拨号；拨号失败记日志并返回，不把失败写成成功。无实例则跳过旁路通知。
   func (a *API) sideCall(c *gin.Context, service, method, path string) {
	      if a == nil || a.rt == nil || a.rt.Dial == nil || a.rt.Registry == nil {
	  	   return
	     }
	   if _, err := a.rt.Registry.Pick(service); err != nil {
		   return
	  }
	      ctx := c.Request.Context()
	  code, _, err := a.rt.Dial.Call(ctx, service, method, path, nil)
	  if err != nil {
	  	   a.rt.Log.Error().Err(err).Str("service", service).Str("path", path).Msg("inter-service dial")
		  return
	  }
	   if code >= 500 {
		   a.rt.Log.Error().Int("status", code).Str("service", service).Str("path", path).Msg("inter-service status")
	  }
	      _ = http.StatusOK
   }
