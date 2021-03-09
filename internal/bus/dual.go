// 通知先写入库表，再抄一份到 Redis。处理只从库表读。
// 作者: Cason
// 日期: 2021.03.08
package bus

  import (
  	"context"
  	"database/sql"
  	"encoding/base64"
  	"strings"

   	"account-assets.kucoin.net/funding/internal/persist"
   	"github.com/redis/go-redis/v9"
   	"github.com/rs/zerolog"
   )

  // Sealer 在写入通知表之前封存正文，交给处理函数之前还原。
  // 主题和键保持调用方传入的原值。未挂封存器时正文按原字节写入。
  type Sealer interface {
  	Encrypt(plain []byte) ([]byte, error)
  	Decrypt(blob []byte) ([]byte, error)
  }

  const sealPrefix = "e1."

  // Durable 以库表作为通知是否投出的依据。
  // Publish 先封存正文，再写入 bus_events。库表写入失败则本次失败，不得把出箱标成已投。
  // Redis 只保留一份副本。副本写入失败只记日志，不改变本次结果。
  // Subscribe 只按库表里的消费位点往后读，解开后再交给处理函数。副本丢失不影响投递。
  // 处理函数成功之前不得推进位点。同一条可能再次交出，处理函数必须按键去重。
  type Durable struct {
  	sql   *SQL
  	redis *Redis
  	seal  Sealer
  	log   zerolog.Logger
  }

  func OpenDurable(db *sql.DB, d persist.Dialect, rdb *redis.Client, seal Sealer, log zerolog.Logger) (*Durable, error) {
  	sqlBus, err := OpenSQL(db, d, log)
  	if err != nil {
  		return nil, err
  	}
  	return &Durable{sql: sqlBus, redis: OpenRedis(rdb, log), seal: seal, log: log}, nil
  }

func (d *Durable) Kind() string { return "sql" }

   func (d *Durable) Close() error { return nil }

   func (d *Durable) Publish(ctx context.Context, topic string, key, value []byte) error {
   	if d == nil || d.sql == nil {
   		return nil
   	}
   	body, err := d.sealBody(value)
   	if err != nil {
   		return err
   	}
   	if err := d.sql.Publish(ctx, topic, key, body); err != nil {
   		return err
   	}
   	if d.redis != nil {
   		if err := d.redis.Publish(ctx, topic, key, body); err != nil {
   			d.log.Error().Err(err).Str("topic", topic).Msg("bus redis copy")
   		}
   	}
   	return nil
   }

  func (d *Durable) Subscribe(ctx context.Context, topic, group string, h Handler) {
  	if d == nil || d.sql == nil || h == nil {
  		return
  	}
  	d.sql.Subscribe(ctx, topic, group, func(ctx context.Context, key, value []byte) error {
  			plain, err := d.openBody(value)
  			if err != nil {
  				return err
  			}
  			return h(ctx, key, plain)
  		})
  }

  func (d *Durable) sealBody(plain []byte) ([]byte, error) {
  	if d == nil || d.seal == nil || len(plain) == 0 || strings.HasPrefix(string(plain), sealPrefix) {
  		return plain, nil
  	}
  	blob, err := d.seal.Encrypt(plain)
  	if err != nil {
  		return nil, err
  	}
  	return []byte(sealPrefix + base64.RawStdEncoding.EncodeToString(blob)), nil
  }

   func (d *Durable) openBody(stored []byte) ([]byte, error) {
   	if d == nil || d.seal == nil || !strings.HasPrefix(string(stored), sealPrefix) {
   		return stored, nil
   	}
   	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(string(stored), sealPrefix))
   	if err != nil {
   		return stored, nil
   	}
   	plain, err := d.seal.Decrypt(raw)
   	if err != nil {
   		return nil, err
   	}
   	return plain, nil
   }
