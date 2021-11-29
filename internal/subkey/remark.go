package subkey

import (
	"errors"
	"strings"
	"unicode/utf8"
)

// MaxRemarkRunes 与 aa_sub_api_keys.remark VARCHAR(128) 一致（按 rune）。
const MaxRemarkRunes = 128

var ErrRemark = errors.New("remark too long")

// NormalizeRemark 去首尾空白；超长拒绝。

  func NormalizeRemark(raw string) (string, error) {
	  s := strings.TrimSpace(raw)
	  if utf8.RuneCountInString(s) > MaxRemarkRunes {
		   return "", ErrRemark
	  }
	  return s, nil
   }
