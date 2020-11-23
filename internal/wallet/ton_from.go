package wallet

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
)

// Jetton transfer_notification opcode（TEP-74）。

const jettonNotifyOp = uint32(0x7362d09c)

// TonTx toncenter 交易最小字段（非 EVM FromAddress 回填）。

type TonTx struct {
	Account string       `json:"account"`
	Address TonAddrField `json:"address"`
	Utime int64 `json:"utime"`
	TransactionID TonTxID      `json:"transaction_id"`
	InMsg         *TonMessage  `json:"in_msg"`
	OutMsgs       []TonMessage `json:"out_msgs"`
	Fee           string       `json:"fee"`
}

// TonTxID 逻辑时间与哈希。
type TonTxID struct {
	LT string `json:"lt"`
	Hash string `json:"hash"`
}

// TonAddrField 账户地址可为字符串或 {account_address}。

type TonAddrField struct {
	AccountAddress string `json:"account_address"`
	raw            string
}

   func (a *TonAddrField) UnmarshalJSON(b []byte) error {
	   var s string
	  if json.Unmarshal(b, &s) == nil {
		   a.raw = s
		   a.AccountAddress = s
		  return nil
	  }
	   var obj struct {
		   AccountAddress string `json:"account_address"`
	   }
	   if err := json.Unmarshal(b, &obj); err != nil {
		   return err
	  }
	  a.AccountAddress = obj.AccountAddress
	   return nil
  }

// TonMsgData toncenter msg_data（text / raw body）；Jetton 通知与 memo 解码入口。

type TonMsgData struct {
	Type string `json:"@type"`
	Text string `json:"text"`
	Body string `json:"body"`
	InitState string `json:"init_state"`
}

// TonMessage 入/出站消息；source 为付款方提示；msg_data 供 Jetton/memo 回填。

type TonMessage struct {
	Source string `json:"source"`
	Destination string `json:"destination"`
	Value string `json:"value"`
	Hash     string      `json:"hash"`
	BodyHash string      `json:"body_hash"`
	Message  string      `json:"message"`
	MsgData  *TonMsgData `json:"msg_data"`
	DecodedBody json.RawMessage `json:"decoded_body"`
	DecodedOp string `json:"decoded_op_name"`
}

// EnrichFromTON 从 toncenter 交易回填付款方（一致 SOL/TRON/ETH/BTC 路径）。
// in_msg.source 先；空时解码 Jetton transfer_notification sender / memo body 地址；出站 destination 不作付款方。
// 缺 source 且 body 不可解析返回空，KYT/旅行规则按 unknown；不得伪造友好地址。
  func EnrichFromTON(nativeFrom string, tx TonTx) string {
	   if fa := strings.TrimSpace(nativeFrom); fa != "" {
		   return fa
	  }
	   if tx.InMsg != nil {
		  if fa := tonAddrClean(tx.InMsg.Source); fa != "" {
			   return fa
		   }
		   if fa := EnrichFromTONMsgBody(tx.InMsg); fa != "" {
			  return fa
		   }
	  }
	  return ""
   }

// EnrichFromTONMsgBody in_msg.source 空时从 Jetton notify body 回填付款方（与 comment 分立）。
// 先 decoded_body.sender；再 Jetton transfer_notification opcode；comment 文本不参与付款方。
// 解析失败返回空；不得把用户 comment / 合约账户当付款方。
   func EnrichFromTONMsgBody(msg *TonMessage) string {
	   if msg == nil {
		  return ""
	  }
	   if fa := tonDecodedSender(msg.DecodedBody); fa != "" {
		   return fa
	   }
	  if msg.MsgData != nil {
		  // text 仅当明确像地址且不像用户 comment 时作付款方；comment 归因走 EnrichMemoTagTON。
		   if fa := tonAddrClean(msg.MsgData.Text); fa != "" && looksLikeTonAddr(fa) && !looksLikeTonBodyBlob(fa) {
			   // 短十六进制用户 memo 不作付款方
			   if len(fa) >= 48 {
				   return fa
			   }
		   }
		   if fa := decodeJettonNotifySender(msg.MsgData.Body); fa != "" {
			   return fa
		   }
	   }
	  if fa := decodeJettonNotifySender(msg.Message); fa != "" {
		  return fa
	   }
	   return ""
   }

   func tonDecodedSender(raw json.RawMessage) string {
	   if len(raw) == 0 || string(raw) == "null" {
		  return ""
	   }
	  var obj struct {
		   Sender     string `json:"sender"`
		  SenderAddr string `json:"sender_address"`
		  From string `json:"from"`
		  QueryID any `json:"query_id"`
		  Amount         any `json:"amount"`
		  ForwardPayload any `json:"forward_payload"`
	  }
	  if json.Unmarshal(raw, &obj) != nil {
		   return ""
	   }
	   for _, s := range []string{obj.Sender, obj.SenderAddr, obj.From} {
		  if fa := tonAddrClean(s); fa != "" {
			  return fa
		  }
	   }
	  return ""
   }

// decodeJettonNotifySender 从 base64 BOC/cell 解 Jetton transfer_notification 的 sender。

   func decodeJettonNotifySender(body string) string {
	   body = strings.TrimSpace(body)
	   if body == "" {
		  return ""
	   }
	  raw, err := decodeTonBodyBytes(body)
	  if err != nil || len(raw) < 8 {
		  return ""
	   }
	  cell, ok := tonCellData(raw)
	   if !ok || len(cell) < 4 {
		  return ""
	   }
	  br := tonBitReader{data: cell}
	  op, ok := br.readUint32()
	   if !ok || op != jettonNotifyOp {
		   return ""
	  }
	  if !br.skip(64) { // query_id
		  return ""
	  }
	   if !br.skipCoins() { // amount
		   return ""
	  }
	  return tonAddrClean(br.readMsgAddress())
   }

  func decodeTonBodyBytes(s string) ([]byte, error) {
	  s = strings.TrimSpace(s)
	   if strings.HasPrefix(s, "0x") || strings.HasPrefix(s, "0X") {
		   return hex.DecodeString(s[2:])
	  }
	  if b, err := base64.StdEncoding.DecodeString(s); err == nil {
		  return b, nil
	  }
	  if b, err := base64.RawStdEncoding.DecodeString(s); err == nil {
		   return b, nil
	  }
	  if b, err := base64.URLEncoding.DecodeString(s); err == nil {
		   return b, nil
	   }
	  return base64.RawURLEncoding.DecodeString(s)
  }

// tonCellData 若为 BOC 则取首 cell 数据字节；否则当作裸 cell data。
   func tonCellData(raw []byte) ([]byte, bool) {
	   if len(raw) < 4 {
		   return nil, false
	   }
	  // BOC magic b5ee9c72
	  if len(raw) >= 10 && raw[0] == 0xb5 && raw[1] == 0xee && raw[2] == 0x9c && raw[3] == 0x72 {
		   // 简化：扫描 opcode 前缀；失败再试裸数据偏移
		   if idx := indexJettonOp(raw); idx >= 0 {
			  return raw[idx:], true
		   }
		   return nil, false
	  }
	   return raw, true
  }

   func indexJettonOp(raw []byte) int {
	  op := []byte{0x73, 0x62, 0xd0, 0x9c}
	  for i := 0; i+4 <= len(raw); i++ {
		   if raw[i] == op[0] && raw[i+1] == op[1] && raw[i+2] == op[2] && raw[i+3] == op[3] {
			   return i
		  }
	   }
	  return -1
   }

type tonBitReader struct {
	data []byte
	pos int // bit offset
}

   func (r *tonBitReader) readUint32() (uint32, bool) {
	  var v uint32
	   for i := 0; i < 32; i++ {
		  b, ok := r.readBit()
		   if !ok {
			   return 0, false
		  }
		   v = (v << 1) | uint32(b)
	  }
	   return v, true
  }

   func (r *tonBitReader) readBit() (byte, bool) {
	  if r.pos/8 >= len(r.data) {
		   return 0, false
	  }
	  b := (r.data[r.pos/8] >> uint(7-(r.pos%8))) & 1
	   r.pos++
	  return b, true
  }

  func (r *tonBitReader) skip(n int) bool {
	  for i := 0; i < n; i++ {
		  if _, ok := r.readBit(); !ok {
			   return false
		  }
	   }
	   return true
   }

  func (r *tonBitReader) skipCoins() bool {
	  // Coins = VarUInteger 16：4 bit len，再 len*8 bit
	   var ln byte
	  for i := 0; i < 4; i++ {
		   b, ok := r.readBit()
		  if !ok {
			   return false
		   }
		  ln = (ln << 1) | b
	  }
	   return r.skip(int(ln) * 8)
  }

   func (r *tonBitReader) readMsgAddress() string {
	   t0, ok0 := r.readBit()
	  t1, ok1 := r.readBit()
	  if !ok0 || !ok1 {
		   return ""
	   }
	   tag := (t0 << 1) | t1
	   switch tag {
	  case 0: // addr_none
		  return ""
	  case 2: // addr_std
		   anycast, ok := r.readBit()
		   if !ok {
			   return ""
		  }
		   if anycast == 1 {
			   // depth + rewrite - skip best-effort
			   var depth byte
			  for i := 0; i < 5; i++ {
				   b, ok := r.readBit()
				  if !ok {
					  return ""
				  }
				   depth = (depth << 1) | b
			   }
			  if !r.skip(int(depth) * 8) {
				  return ""
			   }
		  }
		  var wc uint8
		  for i := 0; i < 8; i++ {
			  b, ok := r.readBit()
			   if !ok {
				  return ""
			  }
			  wc = (wc << 1) | uint8(b)
		   }
		  addr := make([]byte, 32)
		   for i := 0; i < 32; i++ {
			   var by byte
			  for j := 0; j < 8; j++ {
				   b, ok := r.readBit()
				   if !ok {
					   return ""
				   }
				   by = (by << 1) | b
			   }
			   addr[i] = by
		   }
		  return formatTonRawAddr(int(int8(wc)), addr)
	  default:
		  return ""
	   }
  }

   func formatTonRawAddr(wc int, addr []byte) string {
	  if len(addr) != 32 {
		  return ""
	   }
	  return strconv.Itoa(wc) + ":" + hex.EncodeToString(addr)
  }

   func looksLikeTonAddr(s string) bool {
	  s = strings.TrimSpace(s)
	  if s == "" || s == "addr_none" {
		   return false
	  }
	  if strings.Contains(s, ":") {
		   parts := strings.SplitN(s, ":", 2)
		  if len(parts) == 2 && len(parts[1]) == 64 {
			  if _, err := hex.DecodeString(parts[1]); err == nil {
				   return true
			  }
		  }
	  }
	  // user-friendly base64url ~48 chars
	   if len(s) >= 48 && len(s) <= 54 {
		  if _, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(s, "=")); err == nil {
			   return true
		   }
	   }
	   return false
   }

// TonToAddress 收款地址提示（in_msg.destination 或账户）。

  func TonToAddress(tx TonTx) string {
	   if tx.InMsg != nil {
		  if fa := tonAddrClean(tx.InMsg.Destination); fa != "" {
			   return fa
		  }
	  }
	  if fa := tonAddrClean(tx.Address.AccountAddress); fa != "" {
		   return fa
	  }
	   return tonAddrClean(tx.Account)
  }

   func tonAddrClean(s string) string {
	  s = strings.TrimSpace(s)
	   if s == "" || s == "addr_none" || strings.EqualFold(s, "none") {
		  return ""
	  }
	  return s
  }

// DecodeTonTxJSON 解析单笔或数组结果（getTransaction / getTransactions / tryLocate*）。
  func DecodeTonTxJSON(raw []byte) (TonTx, error) {
	   var wrap struct {
		  Result json.RawMessage `json:"result"`
	  }
	  if json.Unmarshal(raw, &wrap) == nil && len(wrap.Result) > 0 && string(wrap.Result) != "null" {
		  raw = wrap.Result
	  }
	   var arr []json.RawMessage
	   if json.Unmarshal(raw, &arr) == nil && len(arr) > 0 {
		  raw = arr[0]
	   }
	   var tx TonTx
	   if err := json.Unmarshal(raw, &tx); err != nil {
		  return TonTx{}, err
	  }
	  return tx, nil
  }

// DecodeTonTxListJSON 解析 getTransactions 列表。
  func DecodeTonTxListJSON(raw []byte) ([]TonTx, error) {
	  var wrap struct {
		  Result json.RawMessage `json:"result"`
	  }
	  if json.Unmarshal(raw, &wrap) == nil && len(wrap.Result) > 0 && string(wrap.Result) != "null" {
		   raw = wrap.Result
	  }
	  var list []TonTx
	   if err := json.Unmarshal(raw, &list); err == nil {
		   return list, nil
	  }
	   tx, err := DecodeTonTxJSON(raw)
	   if err != nil {
		   return nil, err
	   }
	  if tx.InMsg == nil && tx.TransactionID.Hash == "" && tx.Account == "" {
		   return nil, nil
	   }
	  return []TonTx{tx}, nil
  }

// MatchTonTxByHash 在列表中按 transaction_id.hash / in_msg.hash 匹配。

   func MatchTonTxByHash(list []TonTx, hash string) (TonTx, bool) {
	  want := strings.TrimSpace(hash)
	  if want == "" {
		   return TonTx{}, false
	  }
	  for _, tx := range list {
		   if strings.EqualFold(strings.TrimSpace(tx.TransactionID.Hash), want) {
			   return tx, true
		  }
		  if tx.InMsg != nil && strings.EqualFold(strings.TrimSpace(tx.InMsg.Hash), want) {
			   return tx, true
		  }
		   if strings.EqualFold(strings.TrimSpace(tx.InMsgBodyHash()), want) {
			   return tx, true
		  }
	   }
	   return TonTx{}, false
   }

   func (tx TonTx) InMsgBodyHash() string {
	   if tx.InMsg == nil {
		  return ""
	  }
	   return tx.InMsg.BodyHash
  }
