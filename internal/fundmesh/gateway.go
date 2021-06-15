// - 传输失败返回 error，由 kcfund 决定是否改用本进程 ledger/withdraw；禁止在此处伪造成功响应。
// - 响应体上限 1MiB，防止异常实例拖垮网关内存。
// - Authorization 与 X-User-Id 透传，供下游 IAM 与科目隔离校验。
// 参考：docs.platform.kucoin.local/vendor/etcd/v3.5/learning/api/

// Author: Cason
// Date: 2021.06.14

package fundmesh

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/config"
	"account-assets.kucoin.net/funding/internal/ledger"
	"account-assets.kucoin.net/funding/internal/svcdial"
	"account-assets.kucoin.net/funding/internal/svcreg"
	"github.com/rs/zerolog"
)

const maxResponseBytes = 1 << 20

// Client 通过 etcd/hosts 拨号其他 AA 微服务；不是对外 API 网关。

type Client struct {
	dial *svcdial.Client
	reg svcreg.Registry
	cfg  *config.Config
	log  zerolog.Logger
	http *http.Client
}

// New 构造网关客户端；超时与重试策略与资金域 SLO（P99<2s 读）一致。

  func New(dial *svcdial.Client, reg svcreg.Registry, cfg *config.Config, log zerolog.Logger) *Client {
	   return &Client{
		   dial: dial, reg: reg, cfg: cfg, log: log,
		   http: &http.Client{Timeout: 2 * time.Second},
	   }
   }

// Active 判定当前进程是否启用跨服务编排。profile=mem 为集成剖面，仅本进程账本。
   func (g *Client) Active() bool {
	  if g == nil || g.cfg == nil {
		   return false
	  }
	   if strings.EqualFold(strings.TrimSpace(g.cfg.Profile), "mem") {
		  return false
	  }
	   return g.dial != nil || g.reg != nil
  }

// CallOpts 下游鉴权头；UserID 与公开 REST 的 caller 解析一致。

type CallOpts struct {
	UserID        string
	Authorization string
}

   func (g *Client) serviceName(kind string) string {
	  if g == nil || g.cfg == nil {
		   return ""
	  }
	  switch kind {
	   case "ledger":
		   return g.cfg.Mesh.Ledger
	  case "address":
		   return g.cfg.Mesh.Address
	   case "deposit":
		  return g.cfg.Mesh.DepositWithdraw
	   case "account":
		   return g.cfg.Mesh.AccountBook
	  case "statement":
		   return g.cfg.Mesh.StatementExport
	   case "convert":
		  return g.cfg.Mesh.Convert
	  case "fund_surface":
		   return g.cfg.Mesh.FundSurface
	   case "travel":
		   return g.cfg.Mesh.KYT
	  case "p2p":
		   return g.cfg.Mesh.P2PFunding
	  case "funding_bff":
		  return g.cfg.Mesh.FundingBFF
	  default:
		   return ""
	  }
  }

   func (g *Client) fallbackBase(kind string) string {
	   raw := g.fallbackHost(kind)
	   if raw == "" {
		   return ""
	  }
	  httpAddr := ""
	   if g != nil && g.cfg != nil {
		  httpAddr = g.cfg.HTTPAddr
	  }
	   return config.ServiceDialURL(raw, httpAddr)
  }

  func (g *Client) fallbackHost(kind string) string {
	  if g != nil && g.cfg != nil && g.cfg.Hosts != nil {
		  switch kind {
		   case "ledger":
			  if v := strings.TrimSpace(g.cfg.Hosts["ledger"]); v != "" {
				   return v
			   }
		   case "address":
			  if v := strings.TrimSpace(g.cfg.Hosts["address"]); v != "" {
				   return v
			   }
		   case "deposit":
			  if v := strings.TrimSpace(g.cfg.Hosts["deposit_withdraw"]); v != "" {
				  return v
			   }
		   case "account":
			   if v := strings.TrimSpace(g.cfg.Hosts["account-book"]); v != "" {
				  return v
			   }
		  case "statement":
			  if v := strings.TrimSpace(g.cfg.Hosts["statement_export"]); v != "" {
				   return v
			  }
		   case "convert":
			   if v := strings.TrimSpace(g.cfg.Hosts["convert"]); v != "" {
				   return v
			  }
		   case "fund_surface":
			  if v := strings.TrimSpace(g.cfg.Hosts["fund_surface"]); v != "" {
				  return v
			  }
		   case "travel":
			  if v := strings.TrimSpace(g.cfg.Hosts["kyt"]); v != "" {
				  return v
			  }
		   case "p2p":
			   if v := strings.TrimSpace(g.cfg.Hosts["p2p_funding"]); v != "" {
				   return v
			   }
		   case "funding_bff":
			   if v := strings.TrimSpace(g.cfg.Hosts["funding_bff"]); v != "" {
				  return v
			  }
		   }
	  }
	  switch kind {
	   case "ledger":
		  return "https://" + config.HostLedger
	  case "address":
		   return "https://" + config.HostAddress
	  case "deposit":
		  return "https://" + config.HostDepositWithdraw
	   case "account":
		  return "https://" + config.HostAccountBook
	   case "statement":
		  return "https://" + config.HostStmtExport
	  case "convert":
		   return "https://" + config.HostConvert
	  case "fund_surface":
		   return "https://" + config.HostFundSurface
	   case "travel":
		   return "https://" + config.HostKYT
	  case "p2p":
		  return "https://" + config.HostP2PFunding
	   case "funding_bff":
		   return "https://" + config.HostFundingBFF
			  continue
		   }
		   url := strings.TrimRight(base, "/") + path
		  req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
		  if err != nil {
			  return 0, nil, err
		  }
		   if len(body) > 0 {
			   req.Header.Set("Content-Type", "application/json")
		  }
		   if opt.Authorization != "" {
			  req.Header.Set("Authorization", opt.Authorization)
		  }
		   if opt.UserID != "" {
			   req.Header.Set("X-User-Id", opt.UserID)
		   }
		  resp, err := g.http.Do(req)
		   if err != nil {
			  g.log.Error().Err(err).Str("url", url).Str("service", svc).Msg("fundmesh dial")
			   last = err
			   time.Sleep(svcreg.SleepBackoff(attempt + 1))
			   continue
		  }
		  raw, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
		  _ = resp.Body.Close()
		   return resp.StatusCode, raw, nil
	   }
	  if last == nil {
		   last = fmt.Errorf("fundmesh exhausted")
	   }
	   return 0, nil, last
   }

  func (g *Client) pickBase(svc, kind string) (string, error) {
	  if g.reg != nil {
		   inst, err := g.reg.Pick(svc)
		  if err == nil && inst.Addr != "" {
			  return strings.TrimRight(inst.Addr, "/"), nil
		   }
	  }
	   fb := g.fallbackBase(kind)
	   if fb != "" {
		  return strings.TrimRight(fb, "/"), nil
	  }
	   return "", fmt.Errorf("no endpoint for %s", svc)
  }

// LedgerAccounts 读取 ledger-service 科目视图；用于公开 GET /api/v1/accounts 与资金页聚合。
  func (g *Client) LedgerAccounts(ctx context.Context, opt CallOpts, userID string) ([]ledger.BookView, error) {
	  q := url.Values{}
	   q.Set("user_id", userID)
	   code, raw, err := g.do(ctx, "ledger", http.MethodGet, "/v1/ledger/accounts?"+q.Encode(), nil, opt)
	  if err != nil {
		   return nil, err
	   }
	   if code < 200 || code >= 300 {
		   return nil, fmt.Errorf("ledger accounts status %d", code)
	   }
	   var wrap struct {
		   Accounts []ledger.BookView `json:"accounts"`
	   }
	  if err := json.Unmarshal(raw, &wrap); err != nil {
		   return nil, err
	   }
	  return wrap.Accounts, nil
  }

// LedgerBills 读取 ledger-service 流水；与 LedgerAccounts 同源，失败不得编造成空成功。
// Classic margin/isolated 账单与资金页 /assets/margin/* 流水走同一 mesh。

   func (g *Client) LedgerBills(ctx context.Context, opt CallOpts, userID string, limit int) ([]ledger.Bill, error) {
	  if limit <= 0 {
		   limit = 200
	  }
	  if limit > 500 {
		  limit = 500
	  }
	  q := url.Values{}
	  q.Set("user_id", userID)
	  q.Set("limit", strconv.Itoa(limit))
	   code, raw, err := g.do(ctx, "ledger", http.MethodGet, "/v1/ledger/bills?"+q.Encode(), nil, opt)
	  if err != nil {
		  return nil, err
	  }
	  if code < 200 || code >= 300 {
		  return nil, fmt.Errorf("ledger bills status %d", code)
	  }
	   var wrap struct {
		   Bills []ledger.Bill `json:"bills"`
	   }
	  if err := json.Unmarshal(raw, &wrap); err != nil {
		   return nil, err
	  }
	   if wrap.Bills == nil {
		  return nil, fmt.Errorf("ledger bills missing")
	   }
	   return wrap.Bills, nil
  }

// DepositAddressView 地址服务返回的充值面；memo/tag 由 fundsurf 规则在 kcfund 层补齐。

type DepositAddressView struct {
	UserID  string `json:"user_id"`
	Chain   string `json:"chain"`
	Address string `json:"address"`
	Memo    string `json:"memo"`
	TagName string `json:"tagName"`
}

// DepositAddress 向 address-service 申请链上充值地址；私钥材料不得经过本包。

   func (g *Client) DepositAddress(ctx context.Context, opt CallOpts, userID, chain string) (DepositAddressView, error) {
	   q := url.Values{}
	  q.Set("user_id", userID)
	  q.Set("chain", chain)
	  code, raw, err := g.do(ctx, "address", http.MethodGet, "/v1/addresses/deposit?"+q.Encode(), nil, opt)
	   if err != nil {
		   return DepositAddressView{}, err
	  }
	  if code < 200 || code >= 300 {
		   return DepositAddressView{}, fmt.Errorf("deposit address status %d", code)
	  }
	   var view DepositAddressView
	  if err := json.Unmarshal(raw, &view); err != nil {
		  return DepositAddressView{}, err
	  }
	   return view, nil
   }

// DepositWithdraw 将已鉴权的出金/取消请求转发至 deposit-withdraw。body 须为 kcfund 校验后的 JSON。
   func (g *Client) DepositWithdraw(ctx context.Context, method, pathWithQuery string, body []byte, opt CallOpts) (int, []byte, error) {
	   if g == nil {
		  return 0, nil, fmt.Errorf("fundmesh nil")
	  }
	   if !strings.HasPrefix(pathWithQuery, "/") {
		  pathWithQuery = "/" + pathWithQuery
	   }
	  return g.do(ctx, "deposit", method, pathWithQuery, body, opt)
  }

// StatementExport 将账单导出任务提交至 statement-export 服务。
  func (g *Client) StatementExport(ctx context.Context, body []byte, opt CallOpts) (int, []byte, error) {
	  if g == nil {
		  return 0, nil, fmt.Errorf("fundmesh nil")
	  }
	  return g.do(ctx, "statement", http.MethodPost, "/v1/ops/exports", body, opt)
  }

// Convert 将牌价/闪兑/限价 REST 转发至 convert 服务；body 须为 kcfund 已鉴权后的 JSON。
// 传输失败返回 error，由调用方回落本进程 cvfund（账本冻结与幂等键不变）。

  func (g *Client) Convert(ctx context.Context, method, pathWithQuery string, body []byte, opt CallOpts) (int, []byte, error) {
	  if g == nil {
		  return 0, nil, fmt.Errorf("fundmesh nil")
	   }
	   if !strings.HasPrefix(pathWithQuery, "/") {
		   pathWithQuery = "/" + pathWithQuery
	  }
	  return g.do(ctx, "convert", method, pathWithQuery, body, opt)
  }

// Earn 将理财公开 REST 转发至 account-book；路径 /api/v1/earn 映射为下游 /v1/earn。
// 传输失败由 kcfund 回落本进程 earn.Service（申购冻结 holdcat 与幂等键不变）。
  func (g *Client) Earn(ctx context.Context, method, pathWithQuery string, body []byte, opt CallOpts) (int, []byte, error) {
	   if g == nil {
		   return 0, nil, fmt.Errorf("fundmesh nil")
	   }
	   if !strings.HasPrefix(pathWithQuery, "/") {
		  pathWithQuery = "/" + pathWithQuery
	   }
	   const pub = "/api/v1/earn"
	  if strings.HasPrefix(pathWithQuery, pub) {
		   suffix := strings.TrimPrefix(pathWithQuery, pub)
		   if i := strings.Index(suffix, "?"); i >= 0 {
			  pathWithQuery = "/v1/earn" + suffix[:i] + suffix[i:]
		  } else {
			   pathWithQuery = "/v1/earn" + suffix
		  }
	  }
	  return g.do(ctx, "account", method, pathWithQuery, body, opt)
   }

// FundSurface 将冻结/网络目录公开 REST 转发至 fund-surface；/api/v1/* 映射为 /v1/assets/*。
   func (g *Client) FundSurface(ctx context.Context, method, pathWithQuery string, body []byte, opt CallOpts) (int, []byte, error) {
	   if g == nil {
		   return 0, nil, fmt.Errorf("fundmesh nil")
	   }
	  if !strings.HasPrefix(pathWithQuery, "/") {
		   pathWithQuery = "/" + pathWithQuery
	   }
	  qIdx := strings.Index(pathWithQuery, "?")
	   path := pathWithQuery
	   query := ""
	   if qIdx >= 0 {
		  path = pathWithQuery[:qIdx]
		   query = pathWithQuery[qIdx:]
	   }
	  switch {
	  case path == "/api/v1/accounts/holds":
		  path = "/v1/assets/frozen"
	  }
	  pathWithQuery = path + query
	   return g.do(ctx, "fund_surface", method, pathWithQuery, body, opt)
   }

// UTAAccountBook 将 UTA 借还读面 REST 转发至 account-book；/api/ua/* 映射为 /v1/uta/*。
// 传输失败由 kcfund 回落本进程 utabook（利率未配置时限额为 0）。

   func (g *Client) UTAAccountBook(ctx context.Context, method, pathWithQuery string, body []byte, opt CallOpts) (int, []byte, error) {
	  if g == nil {
		   return 0, nil, fmt.Errorf("fundmesh nil")
	   }
	  if !strings.HasPrefix(pathWithQuery, "/") {
		  pathWithQuery = "/" + pathWithQuery
	  }
	   const pub = "/api/ua"
	   if strings.HasPrefix(pathWithQuery, pub) {
		   suffix := strings.TrimPrefix(pathWithQuery, pub)
		   if i := strings.Index(suffix, "?"); i >= 0 {
			   pathWithQuery = "/v1/uta" + suffix[:i] + suffix[i:]
		  } else {
			   pathWithQuery = "/v1/uta" + suffix
		  }
	  }
	  return g.do(ctx, "account", method, pathWithQuery, body, opt)
  }

// SubKeysAccountBook 将子户 API Key REST 转发至 account-book；公开路径映射为 /v1/sub-keys/*。
// 传输失败由 kcfund 回落本进程 subkey（配额闸与 IP 白名单 fail-closed 不变）。
   func (g *Client) SubKeysAccountBook(ctx context.Context, method, pathWithQuery string, body []byte, opt CallOpts) (int, []byte, error) {
	   if g == nil {
		   return 0, nil, fmt.Errorf("fundmesh nil")
	  }
	   if !strings.HasPrefix(pathWithQuery, "/") {
		   pathWithQuery = "/" + pathWithQuery
	   }
	  qIdx := strings.Index(pathWithQuery, "?")
	  path := pathWithQuery
	  query := ""
	   if qIdx >= 0 {
		  path = pathWithQuery[:qIdx]
		   query = pathWithQuery[qIdx:]
	  }
	  mapped, ok := mapSubKeyPublicPath(method, path)
	   if ok {
		   path = mapped
	  }
	  pathWithQuery = path + query
	   return g.do(ctx, "account", method, pathWithQuery, body, opt)
  }

  func mapSubKeyPublicPath(method, path string) (string, bool) {
	   method = strings.ToUpper(strings.TrimSpace(method))
	   switch {
	  case method == "GET" && (path == "/api/v1/sub/api-key" || path == "/api/ua/v1/sub/api-key" || strings.HasSuffix(path, "/get-sub-api-key")):
		  return "/v1/sub-keys", true
	  case method == "POST" && path == "/api/v1/sub/api-key":
		  return "/v1/sub-keys", true
	   case method == "PATCH" && path == "/api/v1/sub/api-key/update":
		   return "/v1/sub-keys", true
	   case method == "POST" && path == "/api/v1/sub/api-key/update":
		   return "/v1/sub-keys", true
	   case method == "DELETE" && (path == "/api/v1/sub/api-key" || strings.HasSuffix(path, "/sub-api-key")):
		   return "/v1/sub-keys", true
	  case method == "POST" && strings.HasSuffix(path, "create-sub-api-key"):
		  return "/v1/sub-keys", true
	   case method == "POST" && strings.HasSuffix(path, "modify-sub-api-key"):
		  return "/v1/sub-keys", true
	  case method == "GET" && path == "/api/v1/sub/api-key/audit":
		  return "/v1/sub-keys/audit", true
	  case method == "GET" && path == "/api/v1/sub/api-key/quota":
		  return "/v1/sub-keys/quota", true
	   case method == "POST" && path == "/api/v1/sub/api-key/rotate":
		   return "/v1/sub-keys/rotate", true
	  case method == "POST" && path == "/api/v1/sub/api-key/revoke":
		  return "/v1/sub-keys/revoke", true
	  default:
		   return "", false
	   }
   }

// Travel 转发对客旅行规则 REST 至 travel-vasp（etcd 注册在 kyt-service 邻域）。
// /api/v1/travel/* 映射为 /v1/travel/customer/* 下游路径。

   func (g *Client) Travel(ctx context.Context, method, pathWithQuery string, body []byte, opt CallOpts) (int, []byte, error) {
	  if g == nil {
		  return 0, nil, fmt.Errorf("fundmesh nil")
	  }
	  if !strings.HasPrefix(pathWithQuery, "/") {
		  pathWithQuery = "/" + pathWithQuery
	  }
	   const pub = "/api/v1/travel"
	  if strings.HasPrefix(pathWithQuery, pub) {
		   suffix := strings.TrimPrefix(pathWithQuery, pub)
		  if i := strings.Index(suffix, "?"); i >= 0 {
			   pathWithQuery = "/v1/travel/customer" + suffix[:i] + suffix[i:]
		   } else {
			   pathWithQuery = "/v1/travel/customer" + suffix
		   }
	   }
	   return g.do(ctx, "travel", method, pathWithQuery, body, opt)
  }

// FundingBFF 将 Java 产品/编排 Classic 路径原样转发至 funding-bff（路径保持 /api/...）。
// 传输失败返回 error，由 kcfund 回落本进程处理器。
  func (g *Client) FundingBFF(ctx context.Context, method, pathWithQuery string, body []byte, opt CallOpts) (int, []byte, error) {
	  if g == nil {
		  return 0, nil, fmt.Errorf("fundmesh nil")
	   }
	  if !strings.HasPrefix(pathWithQuery, "/") {
		   pathWithQuery = "/" + pathWithQuery
	   }
	   return g.do(ctx, "funding_bff", method, pathWithQuery, body, opt)
   }

// NotifySidecar 触发充提观察面心跳；失败不回滚账本事务。

   func (g *Client) NotifySidecar(ctx context.Context, opt CallOpts, kind, path string) {
	  if g == nil {
		  return
	   }
	  svc := g.serviceName("deposit")
	   if svc == "" {
		   return
	  }
	   code, _, err := g.do(ctx, "deposit", http.MethodPost, path, nil, opt)
	   if err != nil {
		   g.log.Error().Err(err).Str("kind", kind).Str("path", path).Msg("fundmesh sidecar")
		   return
	  }
	   if code >= 500 {
		   g.log.Error().Int("status", code).Str("kind", kind).Str("path", path).Msg("fundmesh sidecar")
	  }
  }
