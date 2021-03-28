// @FileName: fill_ident.go
// @Summary: cvfund 模块 fill_ident。
// @Author: Kian
// @Created: 2021.03.27

package cvfund

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrFillIdent = fmt.Errorf("convert fill identity refused")
	ErrFillSig   = fmt.Errorf("convert fill signature invalid")
	ErrFillMTLS  = fmt.Errorf("convert fill mtls identity refused")
	ErrFillNonce = fmt.Errorf("convert fill nonce invalid")
	ErrFillSkew   = fmt.Errorf("convert fill timestamp skew")
	ErrFillReplay = fmt.Errorf("convert fill nonce replay")
)

// FillIdentity 限价闪兑部分成交回调身份（超越 X-Role）。
// HMAC 签名 maker 回调或 mTLS CN 白名单；两者任一通过即可（若已配置）。
// 未配置密钥且未配 mTLS 时仅依赖角色闸，须注入密钥。

type FillIdentity struct {
	mu      sync.Mutex
	secret  string
	mtlsCNs map[string]struct{}
	seen map[string]int64
	skew    time.Duration
	require bool
	clock   func() time.Time
}

  func NewFillIdentity(secret string, mtlsCNs []string) *FillIdentity {
	  if secret == "" {
		   secret = strings.TrimSpace(os.Getenv("CONVERT_FILL_HMAC_SECRET"))
	  }
	  set := map[string]struct{}{}
	  raw := strings.TrimSpace(os.Getenv("CONVERT_FILL_MTLS_CNS"))
	   for _, cn := range append(mtlsCNs, strings.Split(raw, ",")...) {
		  cn = strings.TrimSpace(cn)
		  if cn != "" {
			  set[cn] = struct{}{}
		   }
	  }
	   return &FillIdentity{
		   secret: secret,
		   mtlsCNs: set,
		  seen: map[string]int64{},
		   skew: 30 * time.Second,
		   require: strings.EqualFold(strings.TrimSpace(os.Getenv("CONVERT_FILL_REQUIRE_IDENT")), "1") ||
			   strings.EqualFold(strings.TrimSpace(os.Getenv("CONVERT_FILL_REQUIRE_IDENT")), "true"),
		  clock: func() time.Time { return time.Now().UTC() },
	   }
   }

  func (f *FillIdentity) Configured() bool {
	  return f != nil && (strings.TrimSpace(f.secret) != "" || len(f.mtlsCNs) > 0)
   }

   func (f *FillIdentity) Require() bool {
	   return f != nil && f.require
   }

  func (f *FillIdentity) RequireIdent(on bool) {
	  if f != nil && on {
		  f.require = true
	  }
  }

// VerifyRequest 校验 maker 回调 HMAC 或 mTLS 身份钩子。
// Header X-KC-FILL-SIGN / X-KC-TIMESTAMP / X-KC-NONCE；mTLS 读 X-Client-CN 或 TLS PeerCertificates。
// 签名与 nonce 防重放；缺身份且 Require 时拒绝。

   func (f *FillIdentity) VerifyRequest(r *http.Request, body []byte) (method string, err error) {
	  if f == nil {
		  return "", ErrFillIdent
	   }
	   if cn := mtlsCN(r); cn != "" && f.allowCN(cn) {
		  return "mtls:" + cn, nil
	   }
	   if strings.TrimSpace(f.secret) != "" {
		   ts := strings.TrimSpace(r.Header.Get("X-KC-TIMESTAMP"))
		   nonce := strings.TrimSpace(r.Header.Get("X-KC-NONCE"))
		   sig := strings.TrimSpace(r.Header.Get("X-KC-FILL-SIGN"))
		   if ts == "" {
			  ts = strings.TrimSpace(r.Header.Get("X-KC-TS"))
		   }
		  if sig == "" {
			   sig = strings.TrimSpace(r.Header.Get("X-Signature"))
		   }
		   path := r.URL.Path
		   if err := f.verifyHMAC(ts, nonce, r.Method, path, sig, body); err != nil {
			   return "", err
		   }
		  return "hmac", nil
	  }
	  if f.require {
		  return "", ErrFillIdent
	   }
	  return "role-only", nil
   }

   func (f *FillIdentity) verifyHMAC(ts, nonce, method, path, sig string, body []byte) error {
	   nonce = strings.TrimSpace(nonce)
	   if nonce == "" || len(nonce) < 8 {
		   return ErrFillNonce
	  }
	  unix, err := strconv.ParseInt(strings.TrimSpace(ts), 10, 64)
	  if err != nil {
		   return ErrFillSkew
	  }
	  now := f.clock().Unix()
	   delta := now - unix
	   if delta < 0 {
		   delta = -delta
	   }
	   if time.Duration(delta)*time.Second > f.skew {
		   return ErrFillSkew
	   }
	  want := SignFill(f.secret,
		   ts, nonce, method, path, body)
	  if subtle.ConstantTimeCompare([]byte(strings.ToLower(want)), []byte(strings.ToLower(strings.TrimSpace(sig)))) != 1 {
		  return ErrFillSig
	   }
	  f.mu.Lock()
	  defer f.mu.Unlock()
	   if _, ok := f.seen[nonce]; ok {
		  return ErrFillReplay
	  }
	  f.seen[nonce] = unix
	  return nil
   }

  func (f *FillIdentity) allowCN(cn string) bool {
	   if f == nil || len(f.mtlsCNs) == 0 {
		  return false
	  }
	   _, ok := f.mtlsCNs[strings.TrimSpace(cn)]
	  return ok
   }

   func mtlsCN(r *http.Request) string {
	   if r == nil {
		   return ""
	   }
	   if v := strings.TrimSpace(r.Header.Get("X-Client-CN")); v != "" {
		  return v
	   }
	   if v := strings.TrimSpace(r.Header.Get("X-Forwarded-Client-Cert-CN")); v != "" {
		   return v
	  }
	   if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
		   return strings.TrimSpace(r.TLS.PeerCertificates[0].Subject.CommonName)
	   }
	   return ""
   }

// SignFill maker 侧生成回调签名（/夹具）。
  func SignFill(secret, ts, nonce, method, path string, body []byte) string {
	  mac := hmac.New(sha256.New, []byte(secret))
	  _, _ = mac.Write([]byte(ts + "\n" + nonce + "\n" + method + "\n" + path + "\n"))
	   _, _ = mac.Write(body)
	   return hex.EncodeToString(mac.Sum(nil))
  }

// ReadBody 读取并还原请求体，供验签。

  func ReadBody(r *http.Request) ([]byte, error) {
	   if r == nil || r.Body == nil {
		   return nil, nil
	   }
	   raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	   if err != nil {
		   return nil, err
	  }
	   r.Body = io.NopCloser(strings.NewReader(string(raw)))
	   return raw, nil
  }
