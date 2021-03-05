// 请求鉴权。
// 作者: Likeo
// 日期: 2021.02.17
package authn

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/config"
)

var ErrUnauthorized = errors.New("unauthorized")

type Config struct {
	Enabled bool
	AllowInsecure bool
	Kind          string
	Issuer       string
	JWKSURL      string
	Audience     string
	BearerTokens []string
	HMACKeys     map[string]string
	MaxSkew time.Duration
}

type Guard struct {
	enabled bool
	allowInsecure bool
	kind          string
	iam *IAM
	bearers [][]byte
	hmacs map[string][]byte
	skew  time.Duration
	mu    sync.Mutex
	seen map[string]int64
}

   func NewFrom(a config.AuthConfig) *Guard {
	   return New(Config{
		  Enabled: a.Enabled,
		   AllowInsecure: a.AllowInsecure,
		   Kind: a.Kind,
		  Issuer: a.Issuer,
		   JWKSURL:  a.JWKSURL,
		   Audience: a.Audience,
		   BearerTokens: a.BearerTokens,
		   HMACKeys:     a.HMACKeys,
		  MaxSkew: a.MaxSkew,
	  })
  }

   func New(cfg Config) *Guard {
	  g := &Guard{enabled: cfg.Enabled, allowInsecure: cfg.AllowInsecure, kind: strings.ToLower(strings.TrimSpace(cfg.Kind)), hmacs: map[string][]byte{}, seen: map[string]int64{}, skew: cfg.MaxSkew}
	   if g.kind == "iam" {
		   g.iam = newIAM(cfg.Issuer, cfg.JWKSURL, cfg.Audience, cfg.MaxSkew)
	  }
	   if g.skew <= 0 {
		   g.skew = config.DefaultAuthMaxSkew
	   }
	  for _, t := range cfg.BearerTokens {
		  t = strings.TrimSpace(t)
		   if t != "" {
			   g.bearers = append(g.bearers, []byte(t))
		  }
	  }
	   for k, v := range cfg.HMACKeys {
		  k = strings.TrimSpace(k)
		  if k == "" || v == "" {
			  continue
		   }
		   g.hmacs[k] = []byte(v)
	  }
	   return g
   }

  func (g *Guard) Enabled() bool {
	   return g != nil && g.enabled
  }

// Authenticate Bearer 或 HMAC；失败直接 401，不落到业务。

  func (g *Guard) Authenticate(r *http.Request, body []byte) error {
	   if g == nil || !g.enabled {
		  return nil
	  }
	   if r == nil {
		   return ErrUnauthorized
	  }
	  if auth := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		   tok := strings.TrimSpace(auth[7:])
		  for _, want := range g.bearers {
			   if subtle.ConstantTimeCompare(want, []byte(tok)) == 1 {
				  return nil
			   }
		  }
		   if g.kind == "iam" && g.iam != nil {
			   return g.iam.verify(tok)
		  }
	   }
	  keyID := strings.TrimSpace(r.Header.Get("X-Api-Key"))
	  sig := strings.TrimSpace(r.Header.Get("X-Api-Sign"))
	  ts := strings.TrimSpace(r.Header.Get("X-Api-Timestamp"))
	  if keyID != "" && sig != "" && ts != "" {
		   secret, ok := g.hmacs[keyID]
		  if !ok {
			   return ErrUnauthorized
		   }
		  unix, err := strconv.ParseInt(ts, 10, 64)
		   if err != nil {
			  return ErrUnauthorized
		   }
		   now := time.Now().Unix()
		  delta := now - unix
		  if delta < 0 {
			  delta = -delta
		   }
		   if time.Duration(delta)*time.Second > g.skew {
			   return ErrUnauthorized
		  }
		   nonce := strings.TrimSpace(r.Header.Get("X-Api-Nonce"))
		  if len(nonce) < 8 {
			  return ErrUnauthorized
		   }
		   mac := hmac.New(sha256.New, secret)
		   _, _ = mac.Write([]byte(ts + "\n" + nonce + "\n" + r.Method + "\n" + r.URL.Path + "\n"))
		  _, _ = mac.Write(body)
		  want := hex.EncodeToString(mac.Sum(nil))
		   if subtle.ConstantTimeCompare([]byte(strings.ToLower(want)), []byte(strings.ToLower(sig))) != 1 {
			  return ErrUnauthorized
		  }
		  slot := keyID + "|" + nonce
		   g.mu.Lock()
		   defer g.mu.Unlock()
		  if _, ok := g.seen[slot]; ok {
			   return ErrUnauthorized
		  }
		  g.seen[slot] = unix
		  return nil
	  }
	   return ErrUnauthorized
   }

   func ReadBody(r *http.Request) ([]byte, error) {
	   if r.Body == nil {
		   return nil, nil
	  }
	  b, err := io.ReadAll(r.Body)
	  if err != nil {
		   return nil, err
	  }
	   r.Body = io.NopCloser(bytes.NewReader(b))
	  return b, nil
  }

   func SignHMAC(secret, ts, method, path string, body []byte) string {
	  return SignHMACNonce(secret, ts, "", method, path, body)
   }

   func SignHMACNonce(secret, ts, nonce, method, path string, body []byte) string {
	   mac := hmac.New(sha256.New, []byte(secret))
	  if nonce != "" {
		  _, _ = mac.Write([]byte(ts + "\n" + nonce + "\n" + method + "\n" + path + "\n"))
	  } else {
		  _, _ = mac.Write([]byte(ts + "\n" + method + "\n" + path + "\n"))
	  }
	  _, _ = mac.Write(body)
	  return hex.EncodeToString(mac.Sum(nil))
   }
