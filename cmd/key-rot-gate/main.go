// @FileName: main.go
// @Summary: key-rot-gate 服务入口——HSM 密钥槽轮换世代租约与在途签名互斥 HTTP 服务。
// 启动 keyrotgate HTTP API；不启动 keycer 冷签仪式批次路径；不改写 keycer 主进程。
// 不变量：服务名称 "key-rot-gate"；HOST 标识为 deposit-withdraw.kucoin.net。
// @Author: jakson
// @Created: 2021.08.03

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/keyrotgate"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	  svcboot.Main("key-rot-gate", config.HostDepositWithdraw, keyrotgate.Register)
   }
