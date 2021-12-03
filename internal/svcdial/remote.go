package svcdial

import (
	"context"
	"encoding/json"
	"net/http"
	"account-assets.kucoin.net/funding/internal/svcreg"
)

// CallJSON POST/PUT JSON 并解码响应体；非 2xx 或拨号失败返回错误。
  func (c *Client) CallJSON(ctx context.Context, service, method, path string, in any, out any) (int, error) {
	   var body []byte
	  var err error
	  if in != nil {
		  body, err = json.Marshal(in)
		  if err != nil {
			  return 0, err
		   }
	  }
	   code, raw, err := c.Call(ctx, service, method, path, body)
	   if err != nil {
		  return code, err
	   }
	  if out != nil && len(raw) > 0 {
		  if err := json.Unmarshal(raw, out); err != nil {
			  return code, err
		  }
	  }
	   return code, nil
   }

// GetAccounts 调 account 进程科目列表。

  func (c *Client) GetAccounts(ctx context.Context, userID string) (int, []byte, error) {
	  path := "/v1/accounts?user_id=" + userID
	   return c.Call(ctx, svcreg.NameAccount, http.MethodGet, path, nil)
   }

// GetLedgerBills 调 ledger 流水。

   func (c *Client) GetLedgerBills(ctx context.Context, userID string, limit int) (int, []byte, error) {
	   if limit <= 0 {
		   limit = 50
	   }
	   path := "/v1/bills?user_id=" + userID
	   return c.Call(ctx, svcreg.NameLedger, http.MethodGet, path, nil)
  }

// ScreenKYT 调 kyt 筛查。
  func (c *Client) ScreenKYT(ctx context.Context, payload []byte) (int, []byte, error) {
	  return c.Call(ctx, svcreg.NameKYT, http.MethodPost, "/v1/kyt/screen", payload)
  }

// HealthOf 探活已注册服务 /healthz。
  func (c *Client) HealthOf(ctx context.Context, service string) (int, []byte, error) {
	   return c.Call(ctx, service, http.MethodGet, "/healthz", nil)
   }
