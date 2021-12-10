package subkey

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"strings"
	"time"
)

var (
	ErrPermNeed = errors.New("api key missing required permission")
	ErrSecret = errors.New("api key secret unavailable")
	ErrSign   = errors.New("api key signature mismatch")
)

// ResolveSecret 解出明文 secret；不校验口令（HMAC 路径由签名证明持有）。
// 宽限期内先新密，失败再试旧密。
   func (s *Store) ResolveSecret(apiKey string) (Record, string, error) {
	   if s == nil || s.enc == nil {
		   return Record{}, "", ErrEnc
	  }
	  apiKey = strings.TrimSpace(apiKey)
	   if apiKey == "" {
		  return Record{}, "", ErrKey
	   }
	   s.mu.Lock()
	  r, ok := s.by[apiKey]
	  if !ok {
		  s.mu.Unlock()
		   return Record{}, "", ErrGone
	  }
	   enc := append([]byte(nil), r.SecretEnc...)
	   prev := append([]byte(nil), r.PrevSecretEnc...)
	   graceOK := r.withinGrace(time.Now().UTC().UnixMilli())
	   rec := r.Record
	  s.mu.Unlock()
	   if rec.IsRevoked() {
		  return Record{}, "", ErrRevoked
	   }
	   if !rec.IsEnabled() {
		   return Record{}, "", ErrOff
	   }
	   if rec.IsExpired(0) {
		  return Record{}, "", ErrExpired
	   }
	  if rec.IsIdle(0) {
		  return Record{}, "", ErrIdle
	  }
	   if rec.IsNotBefore(0) {
		   return Record{}, "", ErrNotBefore
	   }
	   plain, err := s.enc.Decrypt(enc)
	   if err == nil && len(plain) > 0 {
		   return rec, string(plain), nil
	   }
	   if graceOK && len(prev) > 0 {
		  old,
			   err2 := s.enc.Decrypt(prev)
		   if err2 == nil && len(old) > 0 {
			   return rec, string(old), nil
		  }
	   }
	   return Record{}, "", ErrSecret
   }

// resolveSecretPair 同时取出新/旧明文供签名（宽限双验）。

   func (s *Store) resolveSecretPair(apiKey string) (Record, string, string, error) {
	   if s == nil || s.enc == nil {
		   return Record{}, "", "", ErrEnc
	   }
	  apiKey = strings.TrimSpace(apiKey)
	  if apiKey == "" {
		  return Record{}, "", "", ErrKey
	  }
	  s.mu.Lock()
	  r, ok := s.by[apiKey]
	   if !ok {
		  s.mu.Unlock()
		  return Record{}, "", "", ErrGone
	   }
	   enc := append([]byte(nil), r.SecretEnc...)
	  prev := append([]byte(nil), r.PrevSecretEnc...)
	  graceOK := r.withinGrace(time.Now().UTC().UnixMilli())
	  rec := r.Record
	   s.mu.Unlock()
	   if rec.IsRevoked() {
		   return Record{}, "", "", ErrRevoked
	  }
	  if !rec.IsEnabled() {
		   return Record{}, "", "", ErrOff
	  }
	  if rec.IsExpired(0) {
		   return Record{}, "", "", ErrExpired
	  }
	   if rec.IsIdle(0) {
		   return Record{}, "", "", ErrIdle
	  }
	  if rec.IsNotBefore(0) {
		   return Record{}, "", "", ErrNotBefore
	  }
	  var cur, old string
	  if plain, err := s.enc.Decrypt(enc); err == nil && len(plain) > 0 {
		  cur = string(plain)
	  }
	   if graceOK && len(prev) > 0 {
		   if plain, err := s.enc.Decrypt(prev); err == nil && len(plain) > 0 {
			   old = string(plain)
		   }
	  }
	   if cur == "" && old == "" {
		   return Record{}, "", "", ErrSecret
	  }
	   return rec, cur, old, nil
  }

// CallReq 调用面过闸参数；Weight<=0 按权限推断。
type CallReq struct {
	APIKey string
	ClientIP string
	WantPerm string
	Method string
	Path   string
	Site string
	Weight int
}

// AuthorizeCall 调用面闸门：启停/过期/空闲/未生效/IP/粘滞/站点/路径/方法/权限/秒级+分钟+母户限流/日配额。
   func (s *Store) AuthorizeCall(apiKey, clientIP, wantPerm, method, path string) (Record, error) {
	   return s.AuthorizeCallEx(CallReq{APIKey: apiKey, ClientIP: clientIP, WantPerm: wantPerm, Method: method, Path: path})
  }

   func (s *Store) AuthorizeCallEx(req CallReq) (Record, error) {
	   if s == nil {
		  return Record{}, ErrGone
	   }
	   apiKey := strings.TrimSpace(req.APIKey)
	   if apiKey == "" {
		   return Record{}, ErrKey
	  }
	  weight := req.Weight
	   if weight <= 0 {
		   weight = InferWeight(req.WantPerm)
	  } else {
		  weight = NormalizeWeight(weight)
	  }
	   s.mu.Lock()
	   r, ok := s.by[apiKey]
	  if !ok {
		   s.mu.Unlock()
		  return Record{}, ErrGone
	   }
	  rec := r.Record
	  if rec.IsRevoked() {
		  s.mu.Unlock()
		   return Record{}, ErrRevoked
	  }
	   if !rec.IsEnabled() {
		  s.mu.Unlock()
		  return Record{}, ErrOff
	   }
	   nowWall := time.Now().UTC()
	   now := nowWall.UnixMilli()
	   if rec.IsExpired(now) {
		   s.mu.Unlock()
		  return Record{}, ErrExpired
	  }
	  if rec.IsIdle(now) {
		   s.mu.Unlock()
		  return Record{}, ErrIdle
	  }
	  if rec.IsNotBefore(now) {
		  s.mu.Unlock()
		  return Record{}, ErrNotBefore
	   }
	  if !IPAllowed(rec.IPWhitelist, req.ClientIP) {
		   s.mu.Unlock()
		   return Record{}, ErrIPDeny
	  }
	  if !StickyIPOK(rec.StickyIP, rec.CreatedIP, req.ClientIP) {
		  s.mu.Unlock()
		   return Record{}, ErrStickyIP
	   }
	   if !SiteAllowed(rec.SiteAllow, req.Site, req.ClientIP) {
		   s.mu.Unlock()
		   return Record{}, ErrSiteDeny
	  }
	  if !PathAllowed(rec.PathAllow, req.Path) {
		   s.mu.Unlock()
		   return Record{}, ErrPathDeny
	  }
	   if !MethodAllowed(rec.MethodAllow, req.Method) {
		   s.mu.Unlock()
		  return Record{}, ErrMethodDeny
	   }
	   if want := strings.TrimSpace(req.WantPerm); want != "" && !HasPerm(rec.Permission, want) {
		  s.mu.Unlock()
		  return Record{}, ErrPermNeed
	  }
	  if err := s.allowRateLocked(apiKey, r.RateMax, weight, nowWall); err != nil {
		  s.mu.Unlock()
		   return Record{}, err
	   }
	  if err := s.allowMinuteLocked(apiKey, r.RateMax, weight, nowWall); err != nil {
		   s.mu.Unlock()
		   return Record{}, err
	  }
	   if err := s.allowOwnerRateLocked(r.Owner, weight, nowWall); err != nil {
		   s.mu.Unlock()
		  return Record{}, err
	   }
	   if err := s.allowDayLocked(r, nowWall, weight); err != nil {
		  s.mu.Unlock()
		   return Record{}, err
	  }
	  r.LastUsedAt = now
	  r.UpdatedAt = now
	   r.CallCount++
	  if ip := strings.TrimSpace(req.ClientIP); ip != "" {
		   r.LastUsedIP = ip
	   }
	  out := r.Record
	   s.mu.Unlock()
	   s.touchLastUsed(apiKey, now, req.ClientIP)
	   s.persistRow(r)
	  return out, nil
  }

// SignKC 公开 API 签名材料：base64(hmac-sha256(secret, ts+method+path+body))。

   func SignKC(secret, ts, method, path string, body []byte) string {
	  mac := hmac.New(sha256.New, []byte(secret))
	   _, _ = mac.Write([]byte(ts + strings.ToUpper(strings.TrimSpace(method)) + path))
	  if len(body) > 0 {
		  _, _ = mac.Write(body)
	  }
	  return base64.StdEncoding.EncodeToString(mac.Sum(nil))
   }

// VerifyKCSign 恒定时间比较签名；供网关在 AuthorizeCall 之后验持有。

   func VerifyKCSign(secret, ts, method, path, got string, body []byte) bool {
	   want := SignKC(secret, ts, method, path, body)
	  got = strings.TrimSpace(got)
	  if want == "" || got == "" {
		  return false
	   }
	  return subtle.ConstantTimeCompare([]byte(want), []byte(got)) == 1
   }

// SignedReq 签名过闸参数；SignVer=0 用 Key 代次；Weight<=0 按权限推断。

type SignedReq struct {
	APIKey   string
	ClientIP string
	WantPerm string
	TS       string
	Method   string
	Path string
	Sign string
	PassHdr string
	Site    string
	SignVer int
	Weight int
	Body []byte
}

// AuthorizeSigned 启停/过期/空闲/未生效/IP/粘滞/站点/路径/方法/权限 + KC 签名（含宽限旧密）+ 口令头一次过闸；
// 验签失败计入错密锁；成功则秒级+分钟+母户限流/日配额/重放槽(含 body hash)/callCount 并刷新 lastUsedAt/Ip。
   func (s *Store) AuthorizeSigned(apiKey, clientIP, wantPerm, ts, method, path, sign, passHdr string, body []byte) (Record, error) {
	   return s.AuthorizeSignedEx(SignedReq{
		   APIKey: apiKey, ClientIP: clientIP, WantPerm: wantPerm, TS: ts,
		   Method: method, Path: path, Sign: sign, PassHdr: passHdr, Body: body,
	   })
  }

  func (s *Store) AuthorizeSignedEx(req SignedReq) (Record, error) {
	  req.Path = NormalizeSignPath(req.Path)
	   nowWall := time.Now().UTC()
	   if err := CheckSignSkew(req.TS, nowWall); err != nil {
		   return Record{}, err
	   }
	   rec, cur, old, err := s.resolveSecretPair(req.APIKey)
	  if err != nil {
		   return Record{}, err
	  }
	   if rec.IsRevoked() {
		  return Record{}, ErrRevoked
	   }
	  if rec.IsNotBefore(nowWall.UnixMilli()) {
		  return Record{}, ErrNotBefore
	  }
	  if !IPAllowed(rec.IPWhitelist, req.ClientIP) {
		  s.noteDeny(req.APIKey, req.ClientIP, ErrIPDeny)
		   return Record{}, ErrIPDeny
	  }
	   if !StickyIPOK(rec.StickyIP, rec.CreatedIP, req.ClientIP) {
		  s.noteDeny(req.APIKey, req.ClientIP, ErrStickyIP)
		  return Record{}, ErrStickyIP
	   }
	  if !SiteAllowed(rec.SiteAllow, req.Site, req.ClientIP) {
		  s.noteDeny(req.APIKey, req.ClientIP, ErrSiteDeny)
		   return Record{}, ErrSiteDeny
	   }
	  if !PathAllowed(rec.PathAllow, req.Path) {
		   return Record{}, ErrPathDeny
	   }
	   if !MethodAllowed(rec.MethodAllow, req.Method) {
		   return Record{}, ErrMethodDeny
	  }
	  if want := strings.TrimSpace(req.WantPerm); want != "" && !HasPerm(rec.Permission, want) {
		   return Record{}, ErrPermNeed
	  }
	  ver, err := MatchSignVer(rec.SignVer, req.SignVer)
	  if err != nil {
		  s.noteDeny(req.APIKey, req.ClientIP, err)
		  return Record{}, err
	   }
	   var secret string
	  switch {
	  case cur != "" && VerifyKCSign(cur, req.TS, req.Method, req.Path, req.Sign, req.Body):
		  secret = cur
	  case old != "" && VerifyKCSign(old, req.TS, req.Method, req.Path, req.Sign, req.Body):
		  secret = old
	   default:
		   s.noteCallFail(req.APIKey, nowWall.UnixMilli(), req.ClientIP)
		   s.noteDeny(req.APIKey, req.ClientIP, ErrSign)
		   return Record{}, ErrSign
	   }
	   weight := req.Weight
	  if weight <= 0 {
		   weight = InferWeight(req.WantPerm)
	  } else {
		   weight = NormalizeWeight(weight)
	   }
	   now := nowWall.UnixMilli()
	   s.mu.Lock()
	  r, ok := s.by[req.APIKey]
	   if !ok {
		   s.mu.Unlock()
		   return Record{}, ErrGone
	   }
	  if r.isLocked(now) {
		  s.mu.Unlock()
		  s.noteDeny(req.APIKey, req.ClientIP, ErrLocked)
		   return Record{}, ErrLocked
	  }
	   if err := s.verifyPassEncHdr(r.PassEnc, secret, req.PassHdr, ver); err != nil {
		   s.noteFailAt(r, now, req.ClientIP)
		   locked := r.isLocked(now)
		  s.mu.Unlock()
		   s.persistRow(r)
		  s.noteDeny(req.APIKey, req.ClientIP, err)
		  if locked {
			  return Record{}, ErrLocked
		   }
		   return Record{}, err
	   }
	  if err := s.allowRateLocked(req.APIKey, r.RateMax, weight, nowWall); err != nil {
		  s.mu.Unlock()
		  return Record{}, err
	   }
	  if err := s.allowMinuteLocked(req.APIKey, r.RateMax, weight, nowWall); err != nil {
		   s.mu.Unlock()
		   return Record{}, err
	  }
	   if err := s.allowOwnerRateLocked(r.Owner, weight, nowWall); err != nil {
		   s.mu.Unlock()
		   return Record{}, err
	   }
	  if err := s.noteSignSeenLocked(req.APIKey, req.TS, req.Method, req.Path, req.Body, nowWall); err != nil {
		  s.mu.Unlock()
		   s.noteDeny(req.APIKey, req.ClientIP, err)
		   return Record{}, err
	   }
	  if err := s.allowDayLocked(r, nowWall, weight); err != nil {
		   s.mu.Unlock()
		  return Record{}, err
	  }
	   s.clearFailLocked(r, now)
	  r.LastUsedAt = now
	   r.UpdatedAt = now
	  r.CallCount++
	   if ip := strings.TrimSpace(req.ClientIP); ip != "" {
		   r.LastUsedIP = ip
	   }
	   rec = r.Record
	  s.mu.Unlock()
	   s.touchLastUsed(req.APIKey, now, req.ClientIP)
	   s.persistRow(r)
	   return rec, nil
  }

// noteCallFail 验签失败累计错密锁；独立取锁，避免与调用方锁重入。
   func (s *Store) noteCallFail(apiKey string, nowMs int64, clientIP string) {
	   if s == nil {
		  return
	  }
	   apiKey = strings.TrimSpace(apiKey)
	   if apiKey == "" {
		   return
	   }
	   s.mu.Lock()
	   r, ok := s.by[apiKey]
	  if !ok {
		   s.mu.Unlock()
		   return
	  }
	   s.noteFailAt(r, nowMs, clientIP)
	  s.mu.Unlock()
	  s.persistRow(r)
  }
