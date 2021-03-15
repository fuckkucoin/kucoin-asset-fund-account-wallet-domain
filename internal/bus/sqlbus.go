// 通知表与消费位点。处理成功后才推进位点。
// 作者: Cason
// 日期: 2021.03.08
package bus

  import (
  	"context"
  	"database/sql"
  	"time"

  	"account-assets.kucoin.net/funding/internal/persist"
  	"github.com/rs/zerolog"
  )

  // SQL 把通知按主题写入共享表，消费组用位点往后读。
  // 位点在处理函数成功之后才推进。处理失败时停在当前条，不得跳过。
  // 进程在写入成功、位点尚未推进时退出，该条会再次交出。
  type SQL struct {
  	db  *sql.DB
  	d   persist.Dialect
  	log zerolog.Logger
  }

   func OpenSQL(db *sql.DB, d persist.Dialect, log zerolog.Logger) (*SQL, error) {
   	s := &SQL{db: db, d: d, log: log}
   	if err := s.ensure(context.Background()); err != nil {
   		return nil, err
   	}
   	return s, nil
   }

  func (s *SQL) Kind() string { return "sql" }

  func (s *SQL) Close() error { return nil }

func (s *SQL) ensure(ctx context.Context) error {
	if s == nil || s.db == nil {
		return nil
	}
	var stmts []string
	if s.d == persist.Postgres {
		stmts = []string{
			`CREATE TABLE IF NOT EXISTS bus_events (
				id BIGSERIAL PRIMARY KEY,
				topic VARCHAR(191) NOT NULL,
				msg_key BYTEA NOT NULL,
				body BYTEA NOT NULL,
				created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
			)`,
			`CREATE INDEX IF NOT EXISTS idx_bus_events_topic ON bus_events (topic, id)`,
			`CREATE TABLE IF NOT EXISTS bus_cursors (
				topic VARCHAR(191) NOT NULL,
				grp VARCHAR(191) NOT NULL,
				pos BIGINT NOT NULL,
				PRIMARY KEY (topic, grp)
			)`,
		}
	} else {
		stmts = []string{
			`CREATE TABLE IF NOT EXISTS bus_events (
				id BIGINT NOT NULL AUTO_INCREMENT PRIMARY KEY,
				topic VARCHAR(191) NOT NULL,
				msg_key VARBINARY(512) NOT NULL,
				body MEDIUMBLOB NOT NULL,
				created_at TIMESTAMP(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
				KEY idx_bus_events_topic (topic, id)
			)`,
			`CREATE TABLE IF NOT EXISTS bus_cursors (
				topic VARCHAR(191) NOT NULL,
				grp VARCHAR(191) NOT NULL,
				pos BIGINT NOT NULL,
				PRIMARY KEY (topic, grp)
			)`,
		}
	}
	for _, stmt := range stmts {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return err
		}
	}
	return nil
}

  func (s *SQL) exec(ctx context.Context, query string, args ...any) (sql.Result, error) {
  	return s.db.ExecContext(ctx, persist.Rewrite(query, s.d), args...)
  }

  func (s *SQL) queryRow(ctx context.Context, query string, args ...any) *sql.Row {
  	return s.db.QueryRowContext(ctx, persist.Rewrite(query, s.d), args...)
  }

   // Publish 追加一条通知。写入失败必须返回错误，调用方不得把它当成已经投出。
   func (s *SQL) Publish(ctx context.Context, topic string, key, value []byte) error {
   	if s == nil || s.db == nil {
   		return nil
   	}
   	_, err := s.exec(ctx, `INSERT INTO bus_events (topic, msg_key, body) VALUES (?,?,?)`, topic, key, value)
   	return err
   }

   func (s *SQL) Subscribe(ctx context.Context, topic, group string, h Handler) {
   	if s == nil || h == nil {
   		return
   	}
   	if group == "" {
   		group = "default"
   	}
   	go func() {
   		for ctx.Err() == nil {
   			ok, err := s.pullOne(ctx, topic, group, h)
   			if err != nil && ctx.Err() == nil {
   				s.log.Error().Err(err).Str("topic", topic).Msg("bus pull")
   			}
   			if ok {
   				continue
   			}
   			t := time.NewTimer(200 * time.Millisecond)
   			select {
   				case <-ctx.Done():
   				t.Stop()
   				return
   				case <-t.C:
   			}
   		}
   	}()
   }

   func (s *SQL) pullOne(ctx context.Context, topic, group string, h Handler) (bool, error) {
   	var pos int64
   	err := s.queryRow(ctx, `SELECT pos FROM bus_cursors WHERE topic=? AND grp=?`, topic, group).Scan(&pos)
   	if err == sql.ErrNoRows {
   		if _, err = s.exec(ctx, `INSERT INTO bus_cursors (topic, grp, pos) VALUES (?,?,0)`, topic, group); err != nil {
   			_ = s.queryRow(ctx, `SELECT pos FROM bus_cursors WHERE topic=? AND grp=?`, topic, group).Scan(&pos)
   		}
   	} else if err != nil {
   		return false, err
   	}
   	var id int64
   	var key, body []byte
   	err = s.queryRow(ctx, `SELECT id, msg_key, body FROM bus_events WHERE topic=? AND id>? ORDER BY id LIMIT 1`, topic, pos).Scan(&id, &key, &body)
   	if err == sql.ErrNoRows {
   		return false, nil
   	}
   	if err != nil {
   		return false, err
   	}
   	deliver(ctx, h, key, body)
   	if ctx.Err() != nil {
   		return false, ctx.Err()
   	}
   	_, err = s.exec(ctx, `UPDATE bus_cursors SET pos=? WHERE topic=? AND grp=? AND pos=?`, id, topic, group, pos)
   	return err == nil, err
   }
