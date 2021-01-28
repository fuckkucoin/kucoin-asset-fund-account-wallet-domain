// 功能：deposit/patrol_time.go
// 作者：Talon
// 日期：2021.01.08

package deposit

import "time"

   func seconds(n int) time.Duration {
	   if n <= 0 {
		  return 0
	   }
	  return time.Duration(n) * time.Second
  }
