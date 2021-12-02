package subkey

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"account-assets.kucoin.net/funding/internal/cryptoenc"
	"account-assets.kucoin.net/funding/internal/persist"
)

// Record 对客可见字段；永不包含 apiSecret。

type Record struct {
	SubName     string `json:"subName"`
	Remark      string `json:"remark"`
	APIKey      string `json:"apiKey"`
	APIVersion  int    `json:"apiVersion"`
	Permission  string `json:"permission"`
	IPWhitelist string `json:"ipWhitelist"`
	Status      string `json:"status"`
	ExpireAt   int64 `json:"expireAt,omitempty"`
	LastUsedAt int64 `json:"lastUsedAt,omitempty"`
	RotatedAt  int64 `json:"rotatedAt,omitempty"`
	FailCount   int   `json:"failCount,omitempty"`
	LockedUntil int64 `json:"lockedUntil,omitempty"`
	LastUsedIP string `json:"lastUsedIp,omitempty"`
	CallCount  int64  `json:"callCount,omitempty"`
	DayCount   int64  `json:"dayCount,omitempty"`
	DayStamp   int    `json:"dayStamp,omitempty"`
	DayCap     int    `json:"dayCap,omitempty"`
	RateMax    int    `json:"rateMax,omitempty"`
	SecretFP   string `json:"secretFp,omitempty"`
	GraceUntil int64  `json:"graceUntil,omitempty"`
	CreatedIP   string `json:"createdIp,omitempty"`
	PathAllow   string `json:"pathAllow,omitempty"`
	IdleTTL     int64  `json:"idleTTL,omitempty"`
	MethodAllow string `json:"methodAllow,omitempty"`
	NotBefore   int64  `json:"notBefore,omitempty"`
	StickyIP    bool   `json:"stickyIp,omitempty"`
	SiteAllow   string `json:"siteAllow,omitempty"`
	SignVer     int    `json:"signVer,omitempty"`
	Label       string `json:"label,omitempty"`
	RotateCount int    `json:"rotateCount,omitempty"`
	RotatedIP string `json:"rotatedIp,omitempty"`
	LastFailAt int64 `json:"lastFailAt,omitempty"`
	LastFailIP string `json:"lastFailIp,omitempty"`
	DenyCount  int64  `json:"denyCount,omitempty"`
	CreatedAt int64 `json:"createdAt"`
	UpdatedAt int64 `json:"updatedAt,omitempty"`
	UID      string   `json:"uid,omitempty"`
	IsMaster bool     `json:"isMaster"`
	State    KeyState `json:"state,omitempty"`
	RevokedAt int64 `json:"revokedAt,omitempty"`
}

type row struct {
	Record
	Owner string
	PassHash [32]byte
	SecretEnc []byte
	PassEnc   []byte // KMS 信封保存口令明文，供 KC-API-PASSPHRASE 重算
	PrevSecretEnc []byte // 轮换宽限期内旧 secret 信封
	PermBits      Perm
}

// Store 进程内索引 + 可选 SQL 权威落盘。
type Store struct {
	mu  sync.Mutex
	enc *cryptoenc.Envelope
	by  map[string]*row
	sql *persistSQL
	owner    SubOwner
	maxFails int
	lockTTL  time.Duration
	seen    map[string]int64 // apiKey|ts|METHOD|path|bodyHash → 过期 unix 秒
	audits  []AuditEvent
	rateMax int           // store 级；<=0 用默认 30；<0 关
	rateWin time.Duration // 默认 1s
	hits    map[string][]int64
	dayMax int // <0 关；0 默认 100000
	graceTTL   time.Duration // 轮换宽限；0 默认 10m
	rotateCool time.Duration // 轮换冷却；0 默认 5m
	ownerRate int // 母户聚合；<0 关；0 默认 200
	minMax int // 分钟窗；<0 关；0 默认 600
	ownerHits map[string][]int64
	minHits   map[string][]int64
	maxRotate int // 终身轮换上限；<=0 默认 50
	mgmtMax   int // 管理面分钟上限；<0 关
	mgmtHits  map[string][]int64
	kv        persist.KV
	quota     *QuotaGate
	audit     *auditBuf
}

   func New(enc *cryptoenc.Envelope) *Store {
	  return &Store{
		  enc: enc, by: map[string]*row{}, seen: map[string]int64{}, hits: map[string][]int64{},
		   ownerHits: map[string][]int64{}, minHits: map[string][]int64{}, mgmtHits: map[string][]int64{},
	   }
   }

type CreateReq struct {
	Owner string
	SubName    string
	Passphrase string
	Remark     string
	Permission  string
	IPWhitelist string
	ExpireAt    int64
	CreatedIP string
	PathAllow string
	IdleTTL     int64
	MethodAllow string
	NotBefore   int64
	StickyIP bool
	RateMax  int
	DayCap    int
	SiteAllow string
	SignVer int
	Label string
	ActorIP  string
	ClientIP string
}

// RotateReq account-book 轮换入参。

type RotateReq struct {
	Owner string
	SubName    string
	APIKey     string
	Passphrase string
	ClientIP   string
}

// RevokeReq account-book 吊销入参。
type RevokeReq struct {
	Owner string
	SubName    string
	APIKey     string
	Passphrase string
	ClientIP   string
	Reason     string
}

// Created 仅创建响应回显 secret / passphrase 一次。
type Created struct {
	Record
	APISecret string `json:"apiSecret"`
	Passphrase string `json:"passphrase"`
}

type ModifyReq struct {
	Owner      string
	SubName    string
	APIKey     string
	Passphrase string
	Permission string
	IPWhitelist string
	SetIP       bool // true 时应用 IPWhitelist（可空；Transfer 类权限拒空）
	Remark *string // nil 不改；非 nil 覆盖（含空串清空）
	// ExpireAt：SetExpire=false 不改；true 时 -1/负清零，>0 绝对毫秒截止。
	ExpireAt  int64
	SetExpire bool
	// RateMax：SetRate=false 不改；true 时 >=0 写入（0=回退 store 默认）。
	RateMax int
	SetRate bool
	// DayCap：SetDayCap=false 不改；true 时 >=0 写入（0=回退 store 默认）。
	DayCap int
	SetDayCap bool
	// PathAllow：SetPath=false 不改；true 时规范化（可空=不限）。
	PathAllow string
	SetPath   bool
	// IdleTTL：SetIdle=false 不改；true 时 >=0 写入（0=关闭空闲过期）。
	IdleTTL int64
	SetIdle bool
	// MethodAllow：SetMethod=false 不改；true 时规范化（可空=不限）。
	MethodAllow string
	SetMethod   bool
	// NotBefore：SetNotBefore=false 不改；true 时负清零，>=0 写入。
	NotBefore    int64
	SetNotBefore bool
	// StickyIP：SetSticky=false 不改；true 时写入 StickyIP。
	StickyIP  bool
	SetSticky bool
	// SiteAllow：SetSite=false 不改；true 时规范化（可空=不限）。
	SiteAllow string
	SetSite   bool
	// SignVer：SetSignVer=false 不改；true 时规范化 1|2。
	SignVer    int
	SetSignVer bool
	// Label：SetLabel=false 不改；true 时覆盖（含空串清空）。
	Label    string
	SetLabel bool
	ActorIP  string
	ClientIP string
}

   func (s *Store) Create(req CreateReq) (Created, error) {
	  if s == nil || s.enc == nil {
		   return Created{}, ErrEnc
	   }
	   req.SubName = strings.TrimSpace(req.SubName)
	   req.Passphrase = strings.TrimSpace(req.Passphrase)
	   req.Owner = strings.TrimSpace(req.Owner)
	   if req.ActorIP == "" {
		   req.ActorIP = strings.TrimSpace(req.ClientIP)
	  }
	  if req.SubName == "" || req.Passphrase == "" {
		  return Created{}, ErrPass
	  }
	  if err := checkPassphrase(req.Passphrase); err != nil {
		   return Created{}, err
	  }
	   if err := s.assertParent(req.Owner, req.SubName); err != nil {
		  return Created{}, err
	  }
	   if err := s.allowMgmt(req.Owner); err != nil {
		  return Created{}, err
	   }
	   perm, _, err := NormalizePerm(req.Permission)
	  if err != nil {
		  return Created{}, err
	   }
	   ip,
		  err := bindIPForPerm(perm, req.IPWhitelist)
	  if err != nil {
		  return Created{}, err
	   }
	  remark, err := NormalizeRemark(req.Remark)
	   if err != nil {
		  return Created{}, err
	   }
	  now := time.Now().UTC().UnixMilli()
	   exp, err := NormalizeExpire(req.ExpireAt, now)
	   if err != nil {
		  return Created{}, err
	   }
	   pathAllow,
		   err := NormalizePathAllow(req.PathAllow)
	  if err != nil {
		  return Created{}, err
	  }
	   methodAllow, err := NormalizeMethodAllow(req.MethodAllow)
	   if err != nil {
		  return Created{}, err
	  }
	   siteAllow, err := NormalizeSiteAllow(req.SiteAllow)
	   if err != nil {
		  return Created{}, err
	  }
	  signVer, err := NormalizeSignVer(req.SignVer)
	  if err != nil {
		  return Created{}, err
	   }
	  label, err := NormalizeLabel(req.Label)
	  if err != nil {
		  return Created{}, err
	   }
	   idle := req.IdleTTL
	   if idle < 0 {
		  idle = 0
	   }
	   notBefore := NormalizeNotBefore(req.NotBefore)
	  createdIP := strings.TrimSpace(req.CreatedIP)
	   if len(createdIP) > 64 {
		   createdIP = createdIP[:64]
	  }
	  if req.StickyIP && createdIP == "" {
		   return Created{}, ErrStickyIP
	  }
	  rateMax := req.RateMax
	  if rateMax < 0 {
		   rateMax = 0
	  }
	   dayCap := req.DayCap
	  if dayCap < 0 {
		  dayCap = 0
	   }
	   keyID, err := randHex(12)
	  if err != nil {
		   return Created{}, err
	  }
	  secret, err := randHex(32)
	   if err != nil {
		  return Created{}, err
	   }
	   blob,
		   err := s.enc.Encrypt([]byte(secret))
	  if err != nil {
		  return Created{}, err
	  }
	  passEnc,
		   err := s.enc.Encrypt([]byte(req.Passphrase))
	   if err != nil {
		   return Created{}, err
	   }
	   if err := s.checkQuota(req.Owner, req.SubName); err != nil {
		   return Created{}, err
	   }
	   s.mu.Lock()
	  if err := s.assertQuota(req.Owner, req.SubName); err != nil {
		   s.mu.Unlock()
		  return Created{}, err
	  }
	   bits, _ := ParsePermission(perm)
	  rec := Record{
		   SubName: req.SubName, Remark: remark, APIKey: keyID,
		  APIVersion: 3, Permission: perm, IPWhitelist: ip, Status: StatusEnabled,
		  State:    StateActive,
		   ExpireAt: exp, CreatedIP: createdIP, PathAllow: pathAllow, IdleTTL: idle,
		  MethodAllow: methodAllow, NotBefore: notBefore, StickyIP: req.StickyIP,
		  SiteAllow: siteAllow, SignVer: signVer, Label: label,
		  RateMax: rateMax, DayCap: dayCap,
		  CreatedAt: now, UpdatedAt: now, UID: req.Owner, IsMaster: false,
	   }
	   rec.SecretFP = Fingerprint(secret)
	  r := &row{Record: rec,
		   Owner: req.Owner, PassHash: sha256.Sum256([]byte(req.Passphrase)), SecretEnc: blob, PassEnc: passEnc, PermBits: bits}
	   s.by[keyID] = r
	  s.mu.Unlock()
	  if err := s.persistRow(r); err != nil {
		  s.mu.Lock()
		  delete(s.by, keyID)
		  s.mu.Unlock()
		  return Created{}, err
	  }
	  s.noteAuditIP(req.Owner, req.SubName, keyID, "create", perm, req.ActorIP)
	  return Created{Record: rec, APISecret: secret, Passphrase: req.Passphrase}, nil
   }

// List 按母户过滤；可选 subName / apiKey。含停用/过期项，不回显 secret；按创建时间降序。

  func (s *Store) List(owner, subName, apiKey string) []Record {
	  if s == nil {
		  return nil
	  }
	   owner = strings.TrimSpace(owner)
	  subName = strings.TrimSpace(subName)
	  apiKey = strings.TrimSpace(apiKey)
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  out := make([]Record, 0)
	   for _, r := range s.by {
		   if owner != "" && r.Owner != owner {
			   continue
		  }
		   if subName != "" && r.SubName != subName {
			  continue
		  }
		   if apiKey != "" && r.APIKey != apiKey {
			  continue
		   }
		   cp := r.Record
		   if cp.Status == "" {
			   cp.Status = StatusEnabled
		  }
		   out = append(out, cp)
	   }
	   sort.Slice(out, func(i, j int) bool {
		   if out[i].CreatedAt == out[j].CreatedAt {
			   return out[i].APIKey > out[j].APIKey
		  }
		   return out[i].CreatedAt > out[j].CreatedAt
	  })
	   return out
  }

// Get 母户下按 apiKey 取一条；不回显 secret。
   func (s *Store) Get(owner, apiKey string) (Record, error) {
	  if s == nil {
		  return Record{}, ErrGone
	   }
	  owner = strings.TrimSpace(owner)
	  apiKey = strings.TrimSpace(apiKey)
	  if apiKey == "" {
		   return Record{}, ErrKey
	  }
	   if owner == "" {
		  return Record{}, ErrOwner
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   r, ok := s.by[apiKey]
	   if !ok || r.Owner != owner {
		  return Record{}, ErrGone
	  }
	  cp := r.Record
	   if cp.Status == "" {
		  cp.Status = StatusEnabled
	   }
	  return cp, nil
  }

// ModifyReq 兼容 account-book 写面：非空 IP 视为要更新白名单；ClientIP 记入审计。

  func (s *Store) ModifyReq(req ModifyReq) (Record, error) {
	   if !req.SetIP && strings.TrimSpace(req.IPWhitelist) != "" {
		  req.SetIP = true
	  }
	  if req.ActorIP == "" {
		  req.ActorIP = strings.TrimSpace(req.ClientIP)
	   }
	  return s.Modify(req)
  }

  func (s *Store) Modify(req ModifyReq) (Record, error) {
	   if req.ActorIP == "" {
		  req.ActorIP = strings.TrimSpace(req.ClientIP)
	   }
	   if !req.SetIP && strings.TrimSpace(req.IPWhitelist) != "" {
		  req.SetIP = true
	   }
	   r,
		   err := s.lookup(req.Owner, req.SubName, req.APIKey, req.Passphrase)
	  if err != nil {
		   return Record{}, err
	   }
	   if err := s.assertParent(req.Owner, req.SubName); err != nil {
		   return Record{}, err
	  }
	  if err := EnsureMutable(r.State); err != nil {
		   return Record{}, err
	   }
	  if r.Record.IsRevoked() {
		   return Record{}, ErrRevoked
	   }
	  s.mu.Lock()
	   defer s.mu.Unlock()
	   before := r.Record
	   nextPerm := r.Permission
	   if strings.TrimSpace(req.Permission) != "" {
		   p, _, err := NormalizePerm(req.Permission)
		  if err != nil {
			   return Record{}, err
		   }
		   nextPerm = p
	  }
	   nextIP := r.IPWhitelist
	  if req.SetIP {
		  norm,
			   err := bindIPForPerm(nextPerm, req.IPWhitelist)
		   if err != nil {
			  return Record{}, err
		   }
		   nextIP = norm
	   } else if NeedsIPBind(nextPerm) && strings.TrimSpace(nextIP) == "" {
		  return Record{}, ErrIPReq
	   }
	  if req.Remark != nil {
		  rm, err := NormalizeRemark(*req.Remark)
		   if err != nil {
			   return Record{}, err
		   }
		  r.Remark = rm
	   }
	  if req.SetExpire {
		  now := time.Now().UTC().UnixMilli()
		   exp, err := NormalizeExpire(req.ExpireAt, now)
		   if err != nil {
			  return Record{}, err
		   }
		   r.ExpireAt = exp
	  }
	  if req.SetRate {
		  if req.RateMax < 0 {
			   req.RateMax = 0
		   }
		  r.RateMax = req.RateMax
	  }
	   if req.SetDayCap {
		  if req.DayCap < 0 {
			  req.DayCap = 0
		   }
		   r.DayCap = req.DayCap
	   }
	   if req.SetPath {
		   pa, err := NormalizePathAllow(req.PathAllow)
		  if err != nil {
			  return Record{}, err
		   }
		   r.PathAllow = pa
	  }
	   if req.SetIdle {
		  if req.IdleTTL < 0 {
			   req.IdleTTL = 0
		  }
		  r.IdleTTL = req.IdleTTL
	  }
	   if req.SetMethod {
		  ma,
			   err := NormalizeMethodAllow(req.MethodAllow)
		   if err != nil {
			  return Record{}, err
		  }
		  r.MethodAllow = ma
	   }
	   if req.SetNotBefore {
		   r.NotBefore = NormalizeNotBefore(req.NotBefore)
	   }
	   if req.SetSticky {
		  if req.StickyIP && strings.TrimSpace(r.CreatedIP) == "" {
			   return Record{}, ErrStickyIP
		  }
		   r.StickyIP = req.StickyIP
	  }
	  if req.SetSite {
		   sa, err := NormalizeSiteAllow(req.SiteAllow)
		  if err != nil {
			  return Record{}, err
		   }
		  r.SiteAllow = sa
	  }
	   if req.SetSignVer {
		  sv, err := NormalizeSignVer(req.SignVer)
		  if err != nil {
			   return Record{}, err
		   }
		   r.SignVer = sv
	  }
	   if req.SetLabel {
		  lb, err := NormalizeLabel(req.Label)
		  if err != nil {
			  return Record{}, err
		  }
		   r.Label = lb
	   }
	  r.Permission = nextPerm
	  r.IPWhitelist = nextIP
	   if bits, perr := ParsePermission(nextPerm); perr == nil {
		  r.PermBits = bits
	   }
	  r.UpdatedAt = time.Now().UTC().UnixMilli()
	  out := r.Record
	   if err := s.persistRow(r); err != nil {
		   r.Record = before
		   return Record{}, err
	   }
	  detail := modifyDiff(before, out)
	  if detail == "" {
		   detail = nextPerm
	   }
	   s.noteAuditIP(req.Owner, req.SubName, req.APIKey, "modify", detail, req.ActorIP)
	  return out, nil
   }

// SetStatus 启停；需母户 + passphrase。revoked 不可直接 enabled，须 Restore。
   func (s *Store) SetStatus(owner, subName, apiKey, passphrase, status, actorIP string) (Record, error) {
	  st,
		   err := NormalizeStatus(status)
	  if err != nil {
		  return Record{}, err
	   }
	   r,
		   err := s.lookup(owner, subName, apiKey, passphrase)
	   if err != nil {
		  return Record{}, err
	  }
	  if err := s.assertParent(owner, subName); err != nil {
		  return Record{}, err
	   }
	   s.mu.Lock()
	   defer s.mu.Unlock()
	   if r.Record.IsRevoked() && st == StatusEnabled {
		   return Record{}, ErrRevoked
	   }
	   prevStatus, prevState, prevRevoked := r.Status, r.State, r.RevokedAt
	  if st == StatusRevoked {
		   r.PrevSecretEnc = nil
		   r.GraceUntil = 0
		   r.State = StateRevoked
		  if r.RevokedAt == 0 {
			  r.RevokedAt = time.Now().UTC().UnixMilli()
		   }
	  }
	  r.Status = st
	  r.UpdatedAt = time.Now().UTC().UnixMilli()
	   out := r.Record
	  if err := s.persistRow(r); err != nil {
		  r.Status, r.State, r.RevokedAt = prevStatus, prevState, prevRevoked
		  return Record{}, err
	  }
	  s.noteAuditIP(owner, subName, apiKey, "status", st, actorIP)
	   return out, nil
   }

// DisableAll 应急停用该子户全部 Key；仅母户归属，不需逐条 passphrase。
// 误调用会导致该子户调用面全拒；列表仍可见以便逐条恢复。
  func (s *Store) DisableAll(owner, subName, actorIP string) (int, error) {
	   if s == nil {
		   return 0, ErrGone
	  }
	   owner = strings.TrimSpace(owner)
	   subName = strings.TrimSpace(subName)
	   if owner == "" {
		   return 0, ErrOwner
	   }
	  if subName == "" {
		   return 0, ErrSubName
	   }
	   if err := s.assertParent(owner, subName); err != nil {
		  return 0, err
	   }
	  if err := s.allowMgmt(owner); err != nil {
		   return 0, err
	  }
	  now := time.Now().UTC().UnixMilli()
	  s.mu.Lock()
	   n := 0
	  touched := make([]*row, 0)
	  for _, r := range s.by {
		  if r.Owner != owner || r.SubName != subName {
			   continue
		   }
		  if r.Status == StatusDisabled || r.Status == StatusRevoked {
			  continue
		  }
		  r.Status = StatusDisabled
		  r.UpdatedAt = now
		   touched = append(touched, r)
		  n++
	   }
	  s.mu.Unlock()
	   for _, r := range touched {
		   s.persistRow(r)
	  }
	   if n > 0 {
		  s.noteAuditIP(owner, subName, "", "disable-all", "count="+strconv.Itoa(n), actorIP)
	  }
	   return n, nil
   }

   func (s *Store) Delete(owner, subName, apiKey, passphrase, actorIP string) (Record, error) {
	   r, err := s.lookup(owner, subName, apiKey, passphrase)
	   if err != nil {
		  return Record{}, err
	   }
	  if err := s.assertParent(owner, subName); err != nil {
		   return Record{}, err
	  }
	   if r.Record.IsEnabled() && NeedsIPBind(r.Permission) {
		   return Record{}, ErrNeedRevoke
	  }
	   if err := s.allowMgmt(owner); err != nil {
		   return Record{}, err
	   }
	  s.mu.Lock()
	  defer s.mu.Unlock()
	  rec := r.Record
	  delete(s.by, r.APIKey)
	   s.deleteRow(rec.APIKey)
	  s.noteAuditIP(owner, subName, apiKey, "delete", "", actorIP)
	   return rec, nil
  }

   func (s *Store) lookup(owner, subName, apiKey, passphrase string) (*row, error) {
	   if s == nil {
		   return nil, ErrGone
	   }
	  apiKey = strings.TrimSpace(apiKey)
	  subName = strings.TrimSpace(subName)
	  owner = strings.TrimSpace(owner)
	   if apiKey == "" {
		   return nil, ErrKey
	   }
	  if subName == "" {
		  return nil, ErrSubName
	  }
	  if owner == "" {
		   return nil, ErrOwner
	  }
	   sum := sha256.Sum256([]byte(strings.TrimSpace(passphrase)))
	   now := time.Now().UTC().UnixMilli()
	   s.mu.Lock()
	   r, ok := s.by[apiKey]
	   if !ok || r.SubName != subName || r.Owner != owner {
		   s.mu.Unlock()
		  return nil, ErrGone
	  }
	  if r.isLocked(now) {
		   s.mu.Unlock()
		  return nil, ErrLocked
	   }
	   if subtle.ConstantTimeCompare(r.PassHash[:], sum[:]) != 1 {
		   s.noteFailLocked(r, now)
		   locked := r.isLocked(now)
		  s.mu.Unlock()
		   s.persistRow(r)
		  if locked {
			  return nil, ErrLocked
		   }
		  return nil, ErrAuth
	   }
	  dirty := r.FailCount != 0 || r.LockedUntil != 0
	  s.clearFailLocked(r, now)
	   s.mu.Unlock()
	  if dirty {
		   s.persistRow(r)
	  }
	   return r, nil
   }

// CheckClientIP 对指定 Key 校验来源 IP。白名单非空且不匹配则拒绝。

   func (s *Store) CheckClientIP(owner, apiKey, clientIP string) error {
	  if s == nil {
		   return ErrGone
	  }
	   s.mu.Lock()
	   r, ok := s.by[strings.TrimSpace(apiKey)]
	  s.mu.Unlock()
	  if !ok || (owner != "" && r.Owner != owner) {
		  return ErrGone
	   }
	   if err := ClientIPAllowed(clientIP, r.IPWhitelist); err != nil {
		   return err
	  }
	  if !IPAllowed(r.IPWhitelist, clientIP) {
		  return ErrIPDeny
	  }
	  return nil
   }

// FinalizeRotate 结束轮换：丢弃旧 secret，状态回到 active。非轮换中且无宽限则拒绝。
  func (s *Store) FinalizeRotate(owner, subName, apiKey, passphrase string) (Record, error) {
	   r, err := s.lookup(owner, subName, apiKey, passphrase)
	   if err != nil {
		  return Record{}, err
	   }
	  if r.State != StateRotating && r.GraceUntil == 0 && len(r.PrevSecretEnc) == 0 {
		   return Record{}, ErrBadState
	  }
	   now := time.Now().UTC().UnixMilli()
	   s.mu.Lock()
	  r.State = StateActive
	  r.PrevSecretEnc = nil
	   r.GraceUntil = 0
	   r.UpdatedAt = now
	  out := r.Record
	   s.mu.Unlock()
	  s.persistRow(r)
	  s.noteAuditIP(owner, subName, apiKey, "finalize-rotate", "", "")
	   return out, nil
  }

  func randHex(n int) (string, error) {
	  buf := make([]byte, n)
	   if _, err := rand.Read(buf); err != nil {
		  return "", err
	  }
	  return hex.EncodeToString(buf), nil
   }
