// 功能：HMAC 现时窗 + nonce 防重放。同一 key+nonce 只过一次；时钟偏斜超窗拒。
// 作者：TianpengLan
// 日期：2021.07.13

package hmacfence

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	ErrUnavailable = fmt.Errorf("hmac fence unavailable")
	ErrKey         = fmt.Errorf("hmac fence key invalid")
	ErrNonce       = fmt.Errorf("hmac fence nonce invalid")
	ErrSkew        = fmt.Errorf("hmac fence timestamp skew")
	ErrReplay      = fmt.Errorf("hmac fence nonce replay")
	ErrSign = fmt.Errorf("hmac fence signature invalid")
	ErrHalt = fmt.Errorf("hmac fence halted")
)

type Service struct {
	mu sync.Mutex
	halt bool
	seen map[string]int64
	skew time.Duration
	clock func() time.Time
}

  func New(skew time.Duration) *Service {
	  if skew <= 0 {
		  skew = 30 * time.Second
	  }
	   return &Service{seen: map[string]int64{},
		   skew: skew, clock: func() time.Time { return time.Now().UTC() }}
   }

   func (s *Service) SetHalt(on bool) {
	  if s == nil {
		   return
	  }
	   s.mu.Lock()
	  s.halt = on
	   s.mu.Unlock()
   }

  func (s *Service) Halted() bool {
	  if s == nil {
		  return true
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   return s.halt
   }

   func Sign(secret, ts, nonce, method, path string, body []byte) string {
	  mac := hmac.New(sha256.New, []byte(secret))
	  _, _ = mac.Write([]byte(ts + "\n" + nonce + "\n" + method + "\n" + path + "\n"))
	   _, _ = mac.Write(body)
	  return hex.EncodeToString(mac.Sum(nil))
   }

  func (s *Service) Verify(keyID, secret, ts, nonce, method, path, sig string, body []byte) error {
	  if s == nil {
		  return ErrUnavailable
	  }
	  if s.Halted() {
		   return ErrHalt
	   }
	  keyID = strings.TrimSpace(keyID)
	  nonce = strings.TrimSpace(nonce)
	   if keyID == "" || strings.TrimSpace(secret) == "" {
		   return ErrKey
	  }
	  if nonce == "" || len(nonce) < 8 {
		  return ErrNonce
	  }
	  unix,
		   err := strconv.ParseInt(strings.TrimSpace(ts), 10, 64)
	   if err != nil {
		   return ErrSkew
	   }
	   now := s.clock().Unix()
	   delta := now - unix
	  if delta < 0 {
		   delta = -delta
	   }
	   if time.Duration(delta)*time.Second > s.skew {
		  return ErrSkew
	  }
	  want := Sign(secret, ts, nonce, method, path, body)
	  if subtle.ConstantTimeCompare([]byte(strings.ToLower(want)), []byte(strings.ToLower(strings.TrimSpace(sig)))) != 1 {
		   return ErrSign
	  }
	   k := keyID + "|" + nonce
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  if _, ok := s.seen[k]; ok {
		   return ErrReplay
	   }
	   s.seen[k] = unix
	  return nil
  }

  func (s *Service) Seen() int {
	  if s == nil {
		   return 0
	  }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	  return len(s.seen)
   }
