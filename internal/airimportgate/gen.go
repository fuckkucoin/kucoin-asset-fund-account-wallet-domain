package airimportgate

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	defaultGenTTL = 300 * time.Second
	maxGenTTL     = 3600 * time.Second
	genPrefix     = "ig-"
)

// ParsedGen 解析后的导入世代令牌。

type ParsedGen struct {
	Raw      string
	UUID     string
	IssuedAt time.Time
}

// ParseImportGen 解析 ImportGen 字符串；形态为 "ig-{uuid}-{unix_sec}"。
   func ParseImportGen(raw string) (ParsedGen, error) {
	   raw = strings.TrimSpace(raw)
	   if raw == "" {
		   return ParsedGen{},
			  ErrImportGenInvalid
	  }
	  if !strings.HasPrefix(raw, genPrefix) {
		  return ParsedGen{}, ErrImportGenInvalid
	   }
	   // 形态：ig-<uuid>-<unix_sec>；uuid 本身含 4 个 '-'，所以从末尾分割
	   idx := strings.LastIndex(raw, "-")
	  if idx < 0 || idx == len(raw)-1 {
		  return ParsedGen{}, ErrImportGenInvalid
	  }
	  uuidPart := raw[len(genPrefix):idx]
	  tsPart := raw[idx+1:]
	  if len(uuidPart) < 8 {
		   return ParsedGen{}, ErrImportGenInvalid
	  }
	  ts, err := strconv.ParseInt(tsPart, 10, 64)
	  if err != nil || ts <= 0 {
		   return ParsedGen{}, ErrImportGenInvalid
	  }
	  return ParsedGen{
		  Raw: raw,
		  UUID: uuidPart,
		   IssuedAt: time.Unix(ts, 0).UTC(),
	  }, nil
  }

// CheckGenExpiry 检测 ImportGen 是否在有效期内。

  func CheckGenExpiry(gen ParsedGen, ttl time.Duration, now time.Time) error {
	   if ttl <= 0 {
		  ttl = defaultGenTTL
	  }
	   if ttl > maxGenTTL {
		   ttl = maxGenTTL
	  }
	  if now.Sub(gen.IssuedAt) > ttl {
		  return ErrImportGenExpired
	  }
	   return nil
   }

// MakeImportGen 构造一个有效的 ImportGen 令牌（供与 ops 工具使用）。

  func MakeImportGen(id string, issuedAt time.Time) string {
	   return fmt.Sprintf("%s%s-%d", genPrefix, id, issuedAt.Unix())
  }
