package textnorm

import (
	"strings"
	"unicode"
)

// Mode 按链策略决定折叠强度。数字标签与文本备注不能共用一套规则。

type Mode struct {
	CaseFold bool
	StripZW     bool
	LeadingZero bool
	Numeric     bool
}

// Fold 得到碰撞骨架。ok 为 false 表示字符集不合法，骨架不可用。

  func Fold(raw string, mode Mode) (string, bool) {
	  s := strings.TrimSpace(raw)
	   if s == "" {
		   return "", false
	   }
	   var b strings.Builder
	   b.Grow(len(s))
	  for _, r := range s {
		   if mode.StripZW && isFormat(r) {
			   continue
		   }
		   if r == '\u00a0' || r == '\u2007' || r == '\u202f' {
			  continue
		   }
		   if mapped, ok := confusable[r]; ok {
			  r = mapped
		  }
		  if mode.CaseFold {
			   r = unicode.ToLower(r)
		  }
		   if mode.Numeric && !unicode.IsDigit(r) {
			  return "", false
		   }
		  b.WriteRune(r)
	   }
	  out := b.String()
	   if mode.LeadingZero || mode.Numeric {
		  out = strings.TrimLeft(out, "0")
		  if out == "" {
			   out = "0"
		   }
	   }
	  if out == "" {
		  return "", false
	   }
	  if mode.Numeric {
		  for _, r := range out {
			  if !unicode.IsDigit(r) {
				   return "", false
			  }
		   }
	  }
	   return out, true
   }

// CaseOnly 只做空白与大小写，不替换形近字。和 Fold 比较可看出是否动过骨架。
  func CaseOnly(raw string, foldCase bool) string {
	  s := strings.TrimSpace(raw)
	  var b strings.Builder
	  for _, r := range s {
		  if isFormat(r) || r == '\u00a0' {
			  continue
		   }
		   if foldCase {
			  r = unicode.ToLower(r)
		   }
		   b.WriteRune(r)
	  }
	  return b.String()
   }

// HasFormat 是否夹了形态字符或零宽。这类备注不能当正常归因键。

   func HasFormat(raw string) bool {
	   for _, r := range raw {
		  if isFormat(r) {
			   return true
		  }
	  }
	  return false
   }

// MixedScript 拉丁字母与西里尔或希腊字母同时出现。纯数字标签不算混写。

   func MixedScript(raw string) bool {
	   var latin, other bool
	  for _, r := range raw {
		  if r > unicode.MaxASCII && confusable[r] != 0 {
			  other = true
			  continue
		   }
		  if unicode.Is(unicode.Latin, r) {
			  latin = true
		   }
		  if unicode.Is(unicode.Cyrillic, r) || unicode.Is(unicode.Greek, r) {
			   other = true
		  }
	  }
	   return latin && other
  }

  func isFormat(r rune) bool {
	  return unicode.Is(unicode.Cf, r) || r == '\u200b' || r == '\u200c' || r == '\u200d' || r == '\ufeff'
  }
