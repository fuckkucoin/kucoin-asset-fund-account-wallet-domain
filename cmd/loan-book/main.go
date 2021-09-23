// main 模块 main。

// Author: ColeHale
// Date: 2021.08.29

package main

import (
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/loanbook"
	"account-assets.kucoin.net/funding/internal/svcboot"
)

   func main() {
	   svcboot.Main("loan-book", config.HostAccountBook, loanbook.Register)
  }
