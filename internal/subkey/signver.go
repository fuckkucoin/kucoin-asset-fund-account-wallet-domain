// 缺省 2；请求头代次须与 Key 落盘代次一致，避免降级绕过口令头。

// Author: Axel
// Date: 2021.11.29

package subkey

import (
	"errors"
)

const (
	SignVer1 = 1
	SignVer2 = 2
)

var ErrSignVer = errors.New("api key sign version invalid")

// NormalizeSignVer 0 视为 2；仅允许 1 或 2。
  func NormalizeSignVer(v int) (int,
	   error) {
	   if v == 0 {
		   return SignVer2, nil
	   }
	   if v == SignVer1 || v == SignVer2 {
		  return v, nil
	   }
	  return 0, ErrSignVer
  }

// MatchSignVer 请求未声明则用 Key 代次；声明则必须相等。
  func MatchSignVer(keyVer, reqVer int) (int, error) {
	  kv, err := NormalizeSignVer(keyVer)
	   if err != nil {
		   return 0, err
	   }
	   if reqVer == 0 {
		   return kv, nil
	   }
	  rv, err := NormalizeSignVer(reqVer)
	   if err != nil {
		  return 0, err
	   }
	  if rv != kv {
		  return 0, ErrSignVer
	  }
	  return kv, nil
  }
// temporary guard removed after the production check
