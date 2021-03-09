// 通知的 Redis 副本。与库表并用时，丢失副本不影响投递。
// 作者: jakson
// 日期: 2021.03.08
package bus

   import (
   	"context"
   	"time"

   	"github.com/redis/go-redis/v9"
   	"github.com/rs/zerolog"
   )

   // Redis 把通知写入独立的流。本类型本身不写账本库。
   // 与库表并用时，流只是副本。副本被截断或丢失，不得当成通知已经丢失。投递仍以库表为准。
   type Redis struct {
   	rdb *redis.Client
   	log zerolog.Logger
   }

   func OpenRedis(rdb *redis.Client, log zerolog.Logger) *Redis {
   	if rdb == nil {
   		return nil
   	}
   	return &Redis{rdb: rdb, log: log}
   }

  func (r *Redis) Kind() string { return "redis" }

  func (r *Redis) Close() error { return nil }

func (r *Redis) stream(topic string) string { return "aa:bus:" + topic }

  func (r *Redis) Publish(ctx context.Context, topic string, key, value []byte) error {
  	if r == nil || r.rdb == nil {
  		return nil
  	}
  	return r.rdb.XAdd(ctx, &redis.XAddArgs{
  			Stream: r.stream(topic),
  			MaxLen: 200000,
  			Approx: true,
  			Values: map[string]any{"k": string(key), "v": string(value)},
  		}).Err()
  }

  func (r *Redis) Subscribe(ctx context.Context, topic, group string, h Handler) {
  	if r == nil || r.rdb == nil || h == nil {
  		return
  	}
  	if group == "" {
  		group = "default"
  	}
  	stream := r.stream(topic)
  	_ = r.rdb.XGroupCreateMkStream(ctx, stream, group, "0").Err()
  	go func() {
  		consumer := group + ":1"
  		for ctx.Err() == nil {
  			res, err := r.rdb.XReadGroup(ctx, &redis.XReadGroupArgs{
  					Group:    group,
  					Consumer: consumer,
  					Streams:  []string{stream, ">"},
  					Count:    1,
  					Block:    time.Second,
  				}).Result()
  			if err == redis.Nil || len(res) == 0 {
  				continue
  			}
  			if err != nil {
  				if ctx.Err() != nil {
  					return
  				}
  				r.log.Error().Err(err).Str("topic", topic).Msg("bus read")
  				time.Sleep(200 * time.Millisecond)
  				continue
  			}
  			for _, st := range res {
  				for _, msg := range st.Messages {
  					key, _ := msg.Values["k"].(string)
  					body, _ := msg.Values["v"].(string)
  					deliver(ctx, h, []byte(key), []byte(body))
  					if ctx.Err() != nil {
  						return
  					}
  					_ = r.rdb.XAck(ctx, stream, group, msg.ID).Err()
  				}
  			}
  		}
  	}()
  }
