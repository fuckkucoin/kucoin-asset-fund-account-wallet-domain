package validate

import (
	"fmt"
	"strings"
	"unicode"
	"account-assets.kucoin.net/funding/pkg/money"
)

// StrictAmount 用户入参金额。科学计数、符号、空白夹杂、控制字符一律拒。

   func StrictAmount(s string) (string, error) {
	  s = Trim(s)
	   if s == "" {
		   return "", ErrAmount
	  }
	  for _, r := range s {
		  if unicode.IsControl(r) || r == '+' || r == ' ' || r == ',' {
			   return "", fmt.Errorf("%w: illegal char", ErrAmount)
		  }
	   }
	   if strings.ContainsAny(s, "eE") {
		   return "", fmt.Errorf("%w: scientific", ErrAmount)
	  }
	  if strings.HasPrefix(s, "-") {
		  return "", fmt.Errorf("%w: negative", ErrAmount)
	  }
	  amt, err := money.Parse(s)
	  if err != nil {
		   return "", err
	  }
	  if !amt.IsPositive() {
		  return "", fmt.Errorf("%w: not positive", ErrAmount)
	   }
	   return amt.String(), nil
   }

// StrictIdem 禁分隔符，避免把 scoped key 拆穿。
   func StrictIdem(s string) (string, error) {
	  s, err := Idem(s)
	  if err != nil {
		   return "", err
	   }
	  if strings.ContainsAny(s, "|\n\r") {
		   return "", fmt.Errorf("%w: illegal char", ErrIdem)
	  }
	  return s, nil
   }

// StrictAmountCap 入参金额不得超过 cap。cap 解析失败失败即关。
   func StrictAmountCap(s, cap string) (string, error) {
	  got, err := StrictAmount(s)
	   if err != nil {
		  return "", err
	   }
	  lim, err := money.Parse(cap)
	   if err != nil {
		  return "", err
	  }
	  amt, err := money.Parse(got)
	   if err != nil {
		  return "", err
	  }
	   if amt.Cmp(lim) > 0 {
		   return "", fmt.Errorf("%w: over cap", ErrAmount)
	  }
	  return got, nil
  }
