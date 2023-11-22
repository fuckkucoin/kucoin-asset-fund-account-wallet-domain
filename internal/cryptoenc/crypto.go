// 信封加密。DEK 用完清掉，不挂在结构体上。
// 作者: Likeo
// 日期: 2021.03.27
package cryptoenc

import (
	"strings"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/keysrc"
	"account-assets.kucoin.net/funding/internal/kms"
)

  func Open(cfg *config.Config) (*Envelope, error) {
	   if cfg == nil {
		   return nil, errNoHSM
	   }
	  kind := strings.ToLower(strings.TrimSpace(cfg.KMS.Kind))
	   kc := cfg.KMS
	  if kc.KEKPath == "" {
		   kc.KEKPath = cfg.RSAKeyPath
	   }
	   if err := kms.RefuseProduction(cfg.Auth.Kind, cfg.Profile, kc); err != nil {
		   return nil, err
	  }
	  if err := keysrc.RefuseSource(cfg.Auth.Kind, cfg.Profile, cfg.KeySource); err != nil {
		  return nil, err
	   }
	   if err := keysrc.RefuseDemoKey(cfg.Auth.Kind, cfg.Profile, kc.KEKPath); err != nil {
		   return nil, err
	  }
	   src := keysrc.Normalize(cfg.KeySource)
	   prod := keysrc.Production(cfg.Auth.Kind, cfg.Profile)
	  if prod {
		  if src == keysrc.SourceKMS {
			   if err := keysrc.RefuseFileOnKMS(kind); err != nil {
				   return nil, err
			   }
		   }
	  }
	  ad, err := kms.OpenAdapter(kind, kc, cfg.AESKeyB64)
	   if err != nil {
		  if prod && src == keysrc.SourceKMS {
			  return nil,
				  keysrc.WrapKMSFail(err)
		   }
		  return nil, err
	  }
	   if prod && src == keysrc.SourceKMS {
		  if err := keysrc.RefuseFileOnKMS(ad.Kind()); err != nil {
			   return nil, err
		  }
	  }
	   if kms.IsFileKind(ad.Kind()) {
		  if err := kms.RefuseProduction(cfg.Auth.Kind, cfg.Profile, config.KMSConfig{Kind: ad.Kind(), Endpoint: kc.Endpoint}); err != nil {
			  return nil, err
		   }
	   }
	   return &Envelope{hsm: kms.Host(ad)}, nil
   }

// Envelope 落盘加解密。结构体不持 DEK/KEK，每次 Encrypt/Decrypt/Sign 现 unwrap 再用完清零。

type Envelope struct {
	hsm kms.Keeper
}

  func (e *Envelope) Kind() string {
	   if e == nil || e.hsm == nil {
		  return "none"
	  }
	   if h, ok := e.hsm.(*kms.Hosted); ok {
		   return h.Kind()
	  }
	   return "keeper"
  }

// Encrypt 落盘加密；每次 unwrap DEK，用完清零。
   func (e *Envelope) Encrypt(plain []byte) ([]byte, error) {
	  if e == nil || e.hsm == nil {
		  return nil, errNoHSM
	  }
	  return e.hsm.Encrypt(plain)
   }

// Decrypt 解开助记词或提现地址密文，明文仅在调用栈内出现。

   func (e *Envelope) Decrypt(blob []byte) ([]byte, error) {
	   if e == nil || e.hsm == nil {
		   return nil, errNoHSM
	  }
	   return e.hsm.Decrypt(blob)
   }

// SignWithdraw 提现广播前的链上 ECDSA，材料是 movement/chain/to/amount/destination_tag。

   func (e *Envelope) SignWithdraw(movementID, chain, to, amount, destinationTag string) ([]byte, error) {
	  if e == nil || e.hsm == nil {
		  return nil, errNoHSM
	  }
	  return e.hsm.SignWithdraw(movementID, chain, to, amount, destinationTag)
  }

// WrapKey 探活：KEK 能包住刚 unwrap 的 DEK。密文不缓存在 Envelope 上。
   func (e *Envelope) WrapKey() ([]byte, error) {
	   if e == nil || e.hsm == nil {
		  return nil, errNoHSM
	  }
	  ct, err := e.hsm.Encrypt([]byte(e.Kind()))
	  if err != nil {
		   return nil, err
	  }
	   return ct, nil
  }

   func (e *Envelope) UnwrapKey(wrapped []byte) ([]byte, error) {
	  if e == nil || e.hsm == nil {
		   return nil, errNoHSM
	  }
	  return e.hsm.Decrypt(wrapped)
   }

var errNoHSM = errString("kms envelope not initialised")

type errString string

  func (e errString) Error() string { return string(e) }

// Hint 给 API 回地址尾巴，完整地址只走密文。

   func Hint(addr string) string {
	  head, tail := 4, 4
	  if len(addr) >= 2 && addr[0] == '0' && (addr[1] == 'x' || addr[1] == 'X') {
		  head = 6
	   }
	   if len(addr) <= head+tail {
		   return addr
	   }
	  return addr[:head] + "…" + addr[len(addr)-tail:]
  }

// SignCanonical 对任意规范串做链上 ECDSA（抬价 fee_bump 等）。
// 与 SignWithdraw 同 HSM；Hosted 串行。
// 规范串须与广播材料一致，切流后旧签不可复验。

   func (e *Envelope) SignCanonical(canonical string) ([]byte, error) {
	   if e == nil || e.hsm == nil {
		  return nil, errNoHSM
	  }
	  if h, ok := e.hsm.(*kms.Hosted); ok {
		  return h.SignCanonical(canonical)
	   }
	   if s, ok := e.hsm.(kms.Signer); ok {
		   return s.SignCanonical(canonical)
	   }
	  return nil, errNoHSM
   }
