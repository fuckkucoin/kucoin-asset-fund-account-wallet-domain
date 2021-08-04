// 功能：子户 API Key 编排辅助：权限目录与来源 IP。写路径在 extras.go，mesh 失败时回落本进程。
// 作者：CainReed
// 日期：2021.08.03

package kcfund

import (
	  "account-assets.kucoin.net/funding/internal/subkey"
	"github.com/gin-gonic/gin"
)

   func (a *API) subkeyClientIP(c *gin.Context) string {
	  if c == nil || c.Request == nil {
		  return ""
	  }
	  return subkey.NormalizeClientIP(c.Request.RemoteAddr, c.GetHeader("X-Forwarded-For"))
  }
