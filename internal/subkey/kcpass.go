// @FileName: kcpass.go
// @Summary: 子账户 API Key 调用面口令头。一致 KC-API-PASSPHRASE：base64(hmac-sha256(secret, passphrase))。
// 验签路径除 secret 外还须证明口令持有；PassEnc 经 KMS 信封保存明文口令供重算。PassEnc 为空的旧 Key 在签名路径拒绝，须改口后重签。
// @Author: Kinsley
// @Created: 2021.11.26

package subkey

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
)

var ErrPassHdr = errors.New("api key passphrase header invalid")

// SignPassphrase 生成 KC v2 口令头。
   func SignPassphrase(secret, passphrase string) string {
	   mac := hmac.New(sha256.New, []byte(secret))
	   _, _ = mac.Write([]byte(passphrase))
	  return base64.StdEncoding.EncodeToString(mac.Sum(nil))
   }

// VerifyPassphraseHdr 恒定时间比较口令头。
   func VerifyPassphraseHdr(secret, passphrase, got string) bool {
	  want := SignPassphrase(secret, passphrase)
	   got = strings.TrimSpace(got)
	   if want == "" || got == "" {
		   return false
	   }
	   return subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
   }

// verifyPassEncHdr 解出口令明文并校验头；PassEnc 缺失则失败关闭。
// ver=1 明文恒定时间比较；ver=2 HMAC。
  func (s *Store) verifyPassEncHdr(passEnc []byte, secret, hdr string, ver int) error {
	   if s == nil || s.enc == nil {
		   return ErrEnc
	  }
	  hdr = strings.TrimSpace(hdr)
	   if hdr == "" {
		   return ErrPassHdr
	  }
	   if len(passEnc) == 0 {
		  return ErrPassHdr
	  }
	   plain, err := s.enc.Decrypt(passEnc)
	   if err != nil || len(plain) == 0 {
		  return ErrPassHdr
	  }
	   if ver <= SignVer1 {
		   if subtle.ConstantTimeCompare([]byte(strings.TrimSpace(string(plain))), []byte(hdr)) != 1 {
			   return ErrPassHdr
		  }
		   return nil
	   }
	  if !VerifyPassphraseHdr(secret, string(plain), hdr) {
		  return ErrPassHdr
	   }
	   return nil
  }
