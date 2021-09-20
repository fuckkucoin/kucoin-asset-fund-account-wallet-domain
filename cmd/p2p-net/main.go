// 功能：P2P 通道进程入口，挂载点对点资金网络。
// 作者：Blaise
// 日期：2021.09.19

package main

import (
	"github.com/gin-gonic/gin"
	"account-assets.kucoin.net/funding/internal/app"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/p2pgate"
	"account-assets.kucoin.net/funding/internal/p2pnet"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	   svcboot.Main("p2p-net", config.HostAccountBook, func(v1 *gin.RouterGroup, rt *app.Runtime) {
		  p2pnet.Register(v1, rt)
		  p2pgate.Register(v1, rt)
	   })
   }
