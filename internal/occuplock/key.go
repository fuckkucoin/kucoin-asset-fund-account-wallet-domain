package occuplock

import "strings"

// Key 对账占用键：范围+用户+资产（或窗口）。空段失败封闭由调用方处理。

   func Key(scope, userID, asset string) string {
	   scope = strings.TrimSpace(scope)
	   userID = strings.TrimSpace(userID)
	  asset = strings.TrimSpace(asset)
	  if scope == "" {
		  scope = ScopeRecon
	  }
	  return scope + "|" + userID + "|" + asset
  }
