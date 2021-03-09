package callerid

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"account-assets.kucoin.net/funding/internal/authn"
	"github.com/gin-gonic/gin"
)

type Opt struct {
	Mem     bool
	Seed    string
	Kind    string
	Profile string
}

func FromRuntime(mem bool, seed, kind, profile string) Opt {
	return Opt{Mem: mem, Seed: seed, Kind: kind, Profile: profile}
}

// Prepare 把查询串或 JSON 里的账户号收到 X-User-Id。
// 请求头里已经有账户号时保持不动。不填种子用户。
func Prepare(c *gin.Context) {
	if c == nil || c.Request == nil {
		return
	}
	if strings.TrimSpace(c.GetHeader("X-User-Id")) != "" {
		return
	}
	if u := strings.TrimSpace(c.GetHeader("X-UID")); u != "" {
		c.Request.Header.Set("X-User-Id", u)
		return
	}
	if u := queryUser(c); u != "" {
		c.Request.Header.Set("X-User-Id", u)
		return
	}
	if u := bodyUser(c); u != "" {
		c.Request.Header.Set("X-User-Id", u)
	}
}

// Resolve 只认请求里的账户号。
// 内存装配允许回落到配置种子用户，供既有单测沿用；与 SQL 进程必须由调用方给出身份。
func Resolve(c *gin.Context, opt Opt) string {
	if c != nil {
		if u := headerUser(c); u != "" {
			return u
		}
		if u := queryUser(c); u != "" {
			return u
		}
	}
	if allowSeedFallback(opt) {
		return strings.TrimSpace(opt.Seed)
	}
	return ""
}

func headerUser(c *gin.Context) string {
	if u := strings.TrimSpace(c.GetHeader("X-User-Id")); u != "" {
		return u
	}
	return strings.TrimSpace(c.GetHeader("X-UID"))
}

func queryUser(c *gin.Context) string {
	for _, k := range []string{"user_id", "userId", "uid"} {
		if u := strings.TrimSpace(c.Query(k)); u != "" {
			return u
		}
	}
	return ""
}

func bodyUser(c *gin.Context) string {
	r := c.Request
	if r.Body == nil || r.Method == http.MethodGet || r.Method == http.MethodHead {
		return ""
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		r.Body = io.NopCloser(bytes.NewReader(nil))
		return ""
	}
	r.Body = io.NopCloser(bytes.NewReader(raw))
	if len(raw) == 0 || raw[0] != '{' {
		return ""
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return ""
	}
	for _, k := range []string{"userId", "user_id", "uid"} {
		switch v := m[k].(type) {
		case string:
			if u := strings.TrimSpace(v); u != "" {
				return u
			}
		case json.Number:
			if u := strings.TrimSpace(v.String()); u != "" {
				return u
			}
		}
	}
	return ""
}

func allowSeedFallback(opt Opt) bool {
	if !opt.Mem {
		return false
	}
	return !authn.ProductionProfile(opt.Kind, opt.Profile)
}

// Require 写路径用。身份为空直接失败，不得用种子用户顶替。
func Require(c *gin.Context, opt Opt) (string, error) {
	uid := Resolve(c, opt)
	if strings.TrimSpace(uid) == "" {
		return "", ErrMissing
	}
	return uid, nil
}

// Bind 先采用调用方已给出的 user_id；缺省再解析请求头。档不得回落种子用户。
func Bind(c *gin.Context, existing string, opt Opt) (string, error) {
	existing = strings.TrimSpace(existing)
	if existing != "" {
		return existing, nil
	}
	return Require(c, opt)
}

// ErrMissing 请求未携带调用方身份。
var ErrMissing = errStr("caller user_id required")

type errStr string

func (e errStr) Error() string { return string(e) }
// temporary guard removed after the production check
