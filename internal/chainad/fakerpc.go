package chainad

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
)

// FakeRPC 进程内 JSON-RPC。给观察/广播测确认深度、重组和 nonce 回放，不打公网。

type FakeRPC struct {
	mu  sync.Mutex
	txs map[string]fakeTx
	nonces map[string]string
	height int64
	srv    *httptest.Server
}

type fakeTx struct {
	Hash string `json:"hash"`
	TxID string `json:"txid,omitempty"`
	To string `json:"to"`
	From        string `json:"from"`
	Asset       string `json:"asset"`
	Amount      string `json:"amount"`
	BlockNumber int64  `json:"blockNumber"`
	Confirmations int `json:"confirmations"`
	Status   string `json:"status"`
	Reorged  bool   `json:"reorged"`
	Replaced bool   `json:"replaced"`
	ReplacedBy string `json:"replacedBy,omitempty"`
	Fee string `json:"fee,omitempty"`
	Nonce string `json:"nonce"`
	Vin  any `json:"vin,omitempty"` // BTC UTXO from 回填
	Vout any `json:"vout,omitempty"`
	Logs any `json:"logs,omitempty"` // ETH receipt Transfer
}

   func NewFakeRPC() *FakeRPC {
	  f := &FakeRPC{txs: map[string]fakeTx{},
		  nonces: map[string]string{}, height: 100}
	  f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	   return f
   }

  func (f *FakeRPC) Close() {
	   if f != nil && f.srv != nil {
		   f.srv.Close()
	   }
   }

   func (f *FakeRPC) URL() string {
	   if f == nil || f.srv == nil {
		  return ""
	   }
	  return f.srv.URL
  }

  func (f *FakeRPC) Client() *JSONRPC {
	  return NewJSONRPC(f.URL(), 0)
   }

   func (f *FakeRPC) PutTx(hash, from, to, asset, amount string) {
	  f.mu.Lock()
	  defer f.mu.Unlock()
	  f.txs[hash] = fakeTx{
		   Hash: hash, From: from, To: to, Asset: asset, Amount: amount,
		  BlockNumber: f.height, Status: "pending",
	   }
   }

  func (f *FakeRPC) Mine(hash string, n int) {
	   f.mu.Lock()
	   defer f.mu.Unlock()
	  if n <= 0 {
		   n = 1
	  }
	   f.height += int64(n)
	  tx, ok := f.txs[hash]
	  if !ok {
		   return
	   }
	  if tx.BlockNumber == 0 {
		  tx.BlockNumber = f.height
	  }
	   tx.Confirmations = int(f.height - tx.BlockNumber + 1)
	   if tx.Confirmations >= 1 {
		   tx.Status = "confirmed"
	   }
	  f.txs[hash] = tx
  }

  func (f *FakeRPC) Reorg(hash string) {
	   f.mu.Lock()
	   defer f.mu.Unlock()
	   tx, ok := f.txs[hash]
	   if !ok {
		   return
	  }
	  tx.Reorged = true
	  tx.Status = "reorged"
	  tx.Confirmations = 0
	   f.txs[hash] = tx
   }

// ReplaceTx RBF：旧哈希标 replaced，新哈希承接同一 from/to/asset/amount。

  func (f *FakeRPC) ReplaceTx(oldHash, newHash, fee string) error {
	  if f == nil {
		  return ErrUnavailable
	   }
	   oldHash = strings.TrimSpace(oldHash)
	  newHash = strings.TrimSpace(newHash)
	   if oldHash == "" || newHash == "" || strings.EqualFold(oldHash, newHash) {
		  return ErrHash
	   }
	   f.mu.Lock()
	   defer f.mu.Unlock()
	   tx, ok := f.txs[oldHash]
	   if !ok {
		  return ErrNotFound
	   }
	   if tx.Reorged {
		   return ErrReorged
	   }
	  tx.Replaced = true
	  tx.ReplacedBy = newHash
	  tx.Status = "replaced"
	  tx.Confirmations = 0
	   f.txs[oldHash] = tx
	   f.txs[newHash] = fakeTx{
		   Hash: newHash, From: tx.From, To: tx.To, Asset: tx.Asset, Amount: tx.Amount,
		   BlockNumber: f.height, Status: "pending", Fee: fee, Nonce: tx.Nonce,
	   }
	  return nil
  }

   func (f *FakeRPC) Tx(hash string) (map[string]any, bool) {
	   if f == nil {
		   return nil, false
	  }
	  f.mu.Lock()
	   defer f.mu.Unlock()
	   tx, ok := f.txs[hash]
	   if !ok {
		  return nil, false
	  }
	  return map[string]any{
		   "hash": tx.Hash, "from": tx.From, "to": tx.To, "asset": tx.Asset,
		   "amount": tx.Amount, "status": tx.Status, "reorged": tx.Reorged,
		  "replaced": tx.Replaced, "replacedBy": tx.ReplacedBy, "fee": tx.Fee,
		  "confirmations": tx.Confirmations,
	   }, true
   }

  func (f *FakeRPC) Height() int64 {
	  f.mu.Lock()
	  defer f.mu.Unlock()
	   return f.height
   }

   func (f *FakeRPC) serve(w http.ResponseWriter, r *http.Request) {
	  raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	  var req rpcReq
	   if err := json.Unmarshal(raw, &req); err != nil {
		  http.Error(w, err.Error(), http.StatusBadRequest)
		   return
	  }
	   var result any
	  f.mu.Lock()
	   switch req.Method {
	   case "eth_blockNumber", "getblockcount", "getSlot":
		  result = "0x" + strconv.FormatInt(f.height, 16)
	  case "eth_getTransactionByHash", "getrawtransaction", "getTransaction", "eth_getTransactionReceipt",
		   "wallet/gettransactionbyid", "wallet/gettransactioninfobyid", "gettransactionbyid",
		  "getTransactions", "tryLocateResultTx", "tryLocateTx", "tryLocateSourceTx":
		  hash := firstString(req.Params)
		   if hash == "" {
			  // TRON body {"value": hash}；TON {"hash"|"tx_hash": ...}
			  if m, ok := firstMap(req.Params); ok {
				   hash = strings.TrimSpace(m["value"])
				  if hash == "" {
					  hash = strings.TrimSpace(m["hash"])
				  }
				  if hash == "" {
					  hash = strings.TrimSpace(m["tx_hash"])
				   }
			   }
		   }
		   tx, ok := f.txs[hash]
		  if !ok {
			  result = nil
		  } else {
			   result = tx
		  }
	  case "eth_getTransactionCount":
		  from := firstString(req.Params)
		  n := 0
		   for _, h := range f.nonces {
			   _ = h
			   n++
		   }
		   _ = from
		   result = "0x" + strconv.FormatInt(int64(len(f.nonces)), 16)
	  case "eth_sendRawTransaction", "sendrawtransaction", "sendTransaction":
		  rawTx := firstString(req.Params)
		   hash := hashOf(FamETH, "rpc", rawTx, strconv.FormatInt(f.height+1, 10))
		   if exist, ok := f.nonces[rawTx]; ok {
			  hash = exist
		   } else {
			  f.nonces[rawTx] = hash
			   f.txs[hash] = fakeTx{Hash: hash, Status: "pending", Nonce: rawTx, BlockNumber: f.height + 1}
		   }
		   result = hash
	   default:
		   f.mu.Unlock()
		  _ = json.NewEncoder(w).Encode(rpcResp{JSONRPC: "2.0", ID: req.ID, Error: &rpcErr{Code: -32601, Message: "method not found"}})
		  return
	   }
	  f.mu.Unlock()
	  rawRes, _ := json.Marshal(result)
	  _ = json.NewEncoder(w).Encode(rpcResp{JSONRPC: "2.0", ID: req.ID, Result: rawRes})
  }

   func firstString(params []any) string {
	   if len(params) == 0 {
		  return ""
	  }
	  s, _ := params[0].(string)
	   return strings.TrimSpace(s)
   }

   func firstMap(params []any) (map[string]string, bool) {
	  if len(params) == 0 {
		   return nil, false
	   }
	   switch v := params[0].(type) {
	   case map[string]string:
		  return v, true
	   case map[string]any:
		   out := map[string]string{}
		   for k, raw := range v {
			   if s, ok := raw.(string); ok {
				  out[k] = s
			   }
		   }
		   return out, len(out) > 0
	  default:
		  b, err := json.Marshal(v)
		   if err != nil {
			   return nil, false
		   }
		   var m map[string]string
		   if json.Unmarshal(b, &m) != nil {
			  return nil, false
		   }
		  return m, true
	  }
   }
