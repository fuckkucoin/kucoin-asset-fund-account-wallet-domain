// 向索引服务拉地址流水。
// 作者: Likeo
// 日期: 2020.11.18
package wallet

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"github.com/shopspring/decimal"
	"account-assets.kucoin.net/funding/internal/config"
)

type indexerClient struct {
	url string
	client *http.Client
}

   func newIndexerClient(raw string, timeout time.Duration) *indexerClient {
	   raw = strings.TrimSpace(raw)
	   if raw == "" || !config.AllowOutbound(raw) {
		   return nil
	   }
	   if timeout <= 0 {
		  timeout = 2 * time.Second
	   }
	  return &indexerClient{
		  url:    raw,
		   client: &http.Client{Timeout: timeout},
	   }
   }

type indexerTx struct {
	Chain string `json:"chain"`
	Hash  string `json:"hash"`
	Asset string `json:"asset"`
	Amount    string `json:"amount"`
	Direction string `json:"direction"`
	Status    string `json:"status"`
	Address string `json:"address"`
	Height  int64  `json:"height"`
}

  func (c *indexerClient) History(ctx context.Context, chain, address string, limit int) ([]Tx, error) {
	   if c == nil || c.url == "" {
		  return nil, fmt.Errorf("indexer not configured")
	   }
	  if limit <= 0 {
		  limit = 20
	  }
	  u, err := url.Parse(c.url)
	  if err != nil {
		  return nil, err
	   }
	   q := u.Query()
	  q.Set("chain", chain)
	   q.Set("address", address)
	  q.Set("limit", fmt.Sprintf("%d", limit))
	  u.RawQuery = q.Encode()
	  req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	  if err != nil {
		   return nil, err
	  }
	   resp, err := c.client.Do(req)
	  if err != nil {
		   return nil, err
	   }
	   defer resp.Body.Close()
	  raw,
		  err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	  if err != nil {
		  return nil, err
	   }
	  if resp.StatusCode >= 300 {
		  return nil, fmt.Errorf("indexer status %d", resp.StatusCode)
	   }
	  var wrap struct {
		   Items []indexerTx `json:"items"`
	   }
	   if err := json.Unmarshal(raw, &wrap); err != nil {
		   return nil, fmt.Errorf("indexer decode: %w", err)
	   }
	   out := make([]Tx, 0, len(wrap.Items))
	   for _, it := range wrap.Items {
		  if it.Hash == "" {
			  continue
		   }
		  amt, err := decimal.NewFromString(it.Amount)
		  if err != nil {
			   continue
		  }
		   st := TxStatus(it.Status)
		   if st == "" {
			   st = TxConfirmed
		  }
		   out = append(out, Tx{
			  Chain: it.Chain,
			  Hash:      it.Hash,
			  Asset:     it.Asset,
			  Amount:    amt,
			  Direction: it.Direction,
			  Status: st,
			  Address: it.Address,
			   Height:  it.Height,
		  })
	  }
	  return out, nil
   }
