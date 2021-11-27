// @FileName: label.go
// @Summary: subkey 模块 label。
// label.go
// 子账户 API Key 运维标签。对客可见短标签，便于按用途过滤；过长拒收。
// CreateDate: 2024.07.29
// @Author: Kinsley
// @Created: 2021.11.26

package subkey

import (
	"errors"
	"strings"
	"unicode/utf8"
)

const MaxLabelRunes = 64

var ErrLabel = errors.New("api key label too long")

// NormalizeLabel 去首尾空白；超 64 rune 拒绝。
  func NormalizeLabel(raw string) (string, error) {
	   s := strings.TrimSpace(raw)
	   if utf8.RuneCountInString(s) > MaxLabelRunes {
		   return "", ErrLabel
	  }
	  return s, nil
  }
