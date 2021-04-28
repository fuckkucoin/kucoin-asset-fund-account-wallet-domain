package test

import (
	"testing"
	"account-assets.kucoin.net/funding/internal/config"
)

// TestCatalogHostPresent 原分散在各 cmd/*/main_test.go 的目录主机冒烟，合并至此。

   func TestCatalogHostPresent(t *testing.T) {
	   if !config.CatalogHost(config.HostAccountBook) {
		  t.Fatal("catalog host")
	  }
   }
