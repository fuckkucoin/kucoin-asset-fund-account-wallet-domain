package wdrelease

import (
	"fmt"
	"strconv"
	"strings"
)

  func explain(dec Decision) string {
	  if dec.Deny {
		  return "拒绝 " + dec.Code + "，不开可冻结票据"
	  }
	   if dec.Skip {
		   return "小额跳过复核，整段授权，版本 " + dec.Version
	  }
	   ratios := make([]string, len(dec.Ratios))
	  for i, r := range dec.Ratios {
		   ratios[i] = strconv.Itoa(r)
	  }
	   return fmt.Sprintf("版本 %s 深度 %d 分段 %s 时限 %d 秒 超时 %s",
		   dec.Version, dec.Depth, strings.Join(ratios, "/"), dec.SLASec, dec.Timeout)
   }
