package reclaim

import (
	"strings"
	"github.com/google/uuid"
)

// newID 生成新的 UUID 字符串（无横线）。

   func newID() string {
	   return strings.ReplaceAll(uuid.NewString(), "-", "")
  }

// newToken 生成新的围栏 token（UUID）。

   func newToken() string {
	   return uuid.NewString()
   }

// nonEmpty 返回字符串去除空白后是否非空。
  func nonEmpty(s string) bool {
	   return strings.TrimSpace(s) != ""
   }

// TestItoa 辅助：将整数转为字符串（供外部调用）。
  func TestItoa(n int) string {
	   return itoa(n)
   }
