package wallet

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MemoTag FamMEMO 链上备注/目的地标签（入金归因用）。
// XLM memo、XRP destination_tag、ATOM memo、EOS memo、TON comment 同面。
// 解析失败返回空，不得伪造备注；漏填不得入账。

type MemoTag struct {
	Memo    string `json:"memo,omitempty"`
	Tag     string `json:"tag,omitempty"`
	TagName string `json:"tag_name,omitempty"` // memo / destination_tag / comment
}

// EnrichMemoTagXLM Horizon 交易回填 memo（一致 destination/memo tag 归因）。
  func EnrichMemoTagXLM(tx XlmTx) MemoTag {
	   memo := strings.TrimSpace(tx.Memo)
	   if memo == "" {
		   return MemoTag{TagName: "memo"}
	   }
	  mt := strings.ToLower(strings.TrimSpace(tx.MemoType))
	   // text/id/hash/return；id 亦作数字 memo
	  if mt == "none" || mt == "" && memo == "" {
		  return MemoTag{TagName: "memo"}
	  }
	  return MemoTag{Memo: memo, Tag: memo, TagName: "memo"}
   }

// EnrichMemoTagXRP rippled Payment DestinationTag 回填。
   func EnrichMemoTagXRP(tx XrpTx) MemoTag {
	  tag := ""
	  if tx.DestinationTag != nil {
		   tag = strings.TrimSpace(fmt.Sprintf("%d", *tx.DestinationTag))
	   }
	  if tag == "" && tx.DestinationTagStr != "" {
		  tag = strings.TrimSpace(tx.DestinationTagStr)
	  }
	   if tag == "" {
		   return MemoTag{TagName: "destination_tag"}
	   }
	  return MemoTag{Memo: tag, Tag: tag, TagName: "destination_tag"}
   }

// EnrichMemoTagATOM Cosmos MsgSend memo / tx body memo 回填。
  func EnrichMemoTagATOM(tx AtomTx) MemoTag {
	   if m := strings.TrimSpace(tx.Memo); m != "" {
		  return MemoTag{Memo: m, Tag: m, TagName: "memo"}
	   }
	  if tx.Tx != nil && tx.Tx.Body != nil {
		   if m := strings.TrimSpace(tx.Tx.Body.Memo); m != "" {
			  return MemoTag{Memo: m, Tag: m, TagName: "memo"}
		  }
	   }
	   return MemoTag{TagName: "memo"}
  }

// EnrichMemoTagEOS EOS transfer data.memo 回填。
  func EnrichMemoTagEOS(tx EosTx) MemoTag {
	   for _, act := range eosActions(tx) {
		  if m := eosActionMemo(act); m != "" {
			  return MemoTag{Memo: m, Tag: m, TagName: "memo"}
		  }
	  }
	  return MemoTag{TagName: "memo"}
   }

  func eosActionMemo(act EosAction) string {
	   if len(act.Data) == 0 || string(act.Data) == "null" {
		  return ""
	  }
	   var data struct {
		  Memo string `json:"memo"`
		  MemoField string `json:"memo_field"`
	   }
	   if json.Unmarshal(act.Data, &data) == nil {
		   if m := strings.TrimSpace(data.Memo); m != "" {
			  return m
		  }
		  if m := strings.TrimSpace(data.MemoField); m != "" {
			  return m
		  }
	   }
	  var asStr string
	   if json.Unmarshal(act.Data, &asStr) == nil && asStr != "" {
		  if json.Unmarshal([]byte(asStr), &data) == nil {
			  if m := strings.TrimSpace(data.Memo); m != "" {
				  return m
			   }
		   }
	  }
	   return ""
   }

// EnrichMemoTagTON TON comment 备注归因（与 Jetton notify 付款方解码分立）。
// 只取 text/comment 备注；不得把 transfer_notification body 当 memo。
// Jetton opcode body / 类地址 forward payload 不作 comment。
   func EnrichMemoTagTON(tx TonTx) MemoTag {
	   mt := MemoTag{TagName: "comment"}
	   if c := TonCommentOnly(tx); c != "" {
		   mt.Memo, mt.Tag = c, c
	   }
	  return mt
  }

// TonCommentOnly 仅提取用户 comment 文本；排除 Jetton notify。
  func TonCommentOnly(tx TonTx) string {
	  if tx.InMsg == nil {
		  return ""
	  }
	   msg := tx.InMsg
	   // decoded_op_name 明示 jetton notify 时备注不从 body 取
	   op := strings.ToLower(strings.TrimSpace(msg.DecodedOp))
	   if strings.Contains(op, "transfer_notification") || strings.Contains(op, "jetton") {
		   // Jetton 通知：comment 只认 forward_payload / decoded comment 字段，不认 raw body
		  return tonDecodedComment(msg.DecodedBody)
	  }
	  // 明文 message（短文本）先；排除看起来像 base64 BOC 的 Jetton body
	  if c := strings.TrimSpace(msg.Message); c != "" && len(c) <= 64 && !looksLikeTonBodyBlob(c) {
		   return c
	  }
	   if msg.MsgData != nil {
		  if c := strings.TrimSpace(msg.MsgData.Text); c != "" && len(c) <= 64 && !looksLikeTonBodyBlob(c) {
			  return c
		  }
		   // raw body 若是 Jetton opcode 则不当 comment
		  if decodeJettonNotifySender(msg.MsgData.Body) != "" {
			   return tonDecodedComment(msg.DecodedBody)
		  }
	  }
	  if c := tonDecodedComment(msg.DecodedBody); c != "" {
		  return c
	   }
	   return ""
  }

   func tonDecodedComment(raw json.RawMessage) string {
	   if len(raw) == 0 || string(raw) == "null" {
		  return ""
	  }
	  var body struct {
		  Comment string `json:"comment"`
		   Text    string `json:"text"`
		  Message string `json:"message"`
		  ForwardPayload any `json:"forward_payload"`
	  }
	   if json.Unmarshal(raw, &body) != nil {
		  return ""
	   }
	   for _, cand := range []string{body.Comment, body.Text, body.Message} {
		   c := strings.TrimSpace(cand)
		   if c != "" && len(c) <= 64 && !looksLikeTonBodyBlob(c) {
			  return c
		   }
	   }
	   // forward_payload 可能嵌套 {"comment":"..."} / 字符串
	  switch v := body.ForwardPayload.(type) {
	   case string:
		   c := strings.TrimSpace(v)
		  if c != "" && len(c) <= 64 && !looksLikeTonBodyBlob(c) && !looksLikeTonAddr(c) {
			  return c
		  }
	  case map[string]any:
		  for _, k := range []string{"comment", "text", "message"} {
			   if s, ok := v[k].(string); ok {
				   c := strings.TrimSpace(s)
				   if c != "" && len(c) <= 64 && !looksLikeTonBodyBlob(c) {
					  return c
				  }
			  }
		  }
	  }
	   return ""
   }

  func looksLikeTonBodyBlob(s string) bool {
	   s = strings.TrimSpace(s)
	  if len(s) >= 24 && !strings.ContainsAny(s, " \t\n") {
		   // base64-ish long blob without spaces → 可能是 BOC/cell，不作 comment
		   ok := true
		  for _, r := range s {
			  if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '+' || r == '/' || r == '=' || r == '-' || r == '_' {
				   continue
			  }
			   ok = false
			   break
		  }
		   if ok {
			  return true
		  }
	   }
	  return false
  }

// FamMemoTagName 按链返回公开 Tag 名（与 chainad.MemoSpec 一致）。
  func FamMemoTagName(chain string) string {
	  switch strings.ToUpper(strings.TrimSpace(chain)) {
	  case "XRP":
		   return "destination_tag"
	   case "TON":
		   return "comment"
	   case "XLM", "ATOM", "EOS":
		  return "memo"
	   default:
		   return ""
	  }
   }

// RequiresFamMemo 是否备注链（漏填不入账）。
   func RequiresFamMemo(chain string) bool {
	  return FamMemoTagName(chain) != ""
  }

// FamMemoChains 平台 FamMEMO 备注链清单（共享地址覆盖面）。
// 与 RequiresFamMemo / FamMemoTagName 一致；新增备注链须同步。
// 清单漏链会导致就绪检查放过未登记共享地址。
   func FamMemoChains() []string {
	  return []string{"XRP", "TON", "XLM", "ATOM", "EOS"}
  }
