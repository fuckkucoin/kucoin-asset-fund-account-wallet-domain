// 功能：main 模块 main。
// 作者：Jett
// 日期：2021.12.20

package main

import (
	"account-assets.kucoin.net/funding/internal/batchresiduefence"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

  func main() {
	   svcboot.Main("wdbatch-residue-fence", config.HostDepositWithdraw, batchresiduefence.Register)
   }
