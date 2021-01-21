// 单据备注、标签和地址提示的封存与还原。
// 作者: Cason
// 日期: 2021.01.05
package ledger

   import (
   	"encoding/base64"
   	"strings"
   )

  // Sealer 在落库前封存备注、标签和地址提示，读出时还原。
  // 封存结果带前缀 e1.。未挂封存器时按原文字写入。已经带此前缀的内容不再封一次。
  // 对外接口仍使用原来的明文字段。严禁把封存后的文本回写到调用方结构体。
  type Sealer interface {
  	Encrypt(plain []byte) ([]byte, error)
  	Decrypt(blob []byte) ([]byte, error)
  }

   const sealPrefix = "e1."

  func (s *SQLStore) SetSeal(seal Sealer) {
  	if s == nil {
  		return
  	}
  	s.seal = seal
  }

  func (s *SQLStore) sealHints(memo, tag, name string) (string, string, string, error) {
  	var err error
  	if memo, err = s.sealOne(memo); err != nil {
  		return "", "", "", err
  	}
  	if tag, err = s.sealOne(tag); err != nil {
  		return "", "", "", err
  	}
  	if name, err = s.sealOne(name); err != nil {
  		return "", "", "", err
  	}
  	return memo, tag, name, nil
  }

  func (s *SQLStore) sealOne(plain string) (string, error) {
  	if plain == "" || s == nil || s.seal == nil || strings.HasPrefix(plain, sealPrefix) {
  		return plain, nil
  	}
  	blob, err := s.seal.Encrypt([]byte(plain))
  	if err != nil {
  		return "", err
  	}
  	return sealPrefix + base64.RawStdEncoding.EncodeToString(blob), nil
  }

// reveal 把库内备注、标签、转入转出地址提示还原成明文。
// 路由、风控和链上提交读的是还原后的值。无前缀的历史行保持原文字。
// 前缀在而解开失败时，字段仍是库内原文。严禁把这段原文当作链上地址提交。
func (s *SQLStore) reveal(t *Transfer) {
	if t == nil {
		return
	}
	t.MemoHint = s.openOne(t.MemoHint)
	t.TagHint = s.openOne(t.TagHint)
	t.TagNameHint = s.openOne(t.TagNameHint)
	t.ToAddressHint = s.openOne(t.ToAddressHint)
	t.FromAddressHint = s.openOne(t.FromAddressHint)
}

// SealHints 封存备注/标签/名称（调用方明文不变）。
func (s *SQLStore) SealHints(memo, tag, name string) (string, string, string, error) {
	return s.sealHints(memo, tag, name)
}

// OpenHints 将库内封存字段还原到 Transfer（链上/风控读明文）。
func (s *SQLStore) OpenHints(t *Transfer) {
	s.reveal(t)
}

// SealPrefix 返回封存前缀，用于校验落库形态。
func SealPrefix() string {
	return sealPrefix
}

   // openOne 还原单条封存文本。无前缀视为历史明文。解开失败时返回库内原文。
   func (s *SQLStore) openOne(stored string) string {
   	if s == nil || s.seal == nil || !strings.HasPrefix(stored, sealPrefix) {
   		return stored
   	}
   	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(stored, sealPrefix))
   	if err != nil {
   		return stored
   	}
   	plain, err := s.seal.Decrypt(raw)
   	if err != nil {
   		return stored
   	}
   	return string(plain)
   }
