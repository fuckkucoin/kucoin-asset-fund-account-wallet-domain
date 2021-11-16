package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
)

type PolicyDoc struct {
	Pack    string
	Version int
	Body string
	PublishedAt time.Time
}

type PolicyMeta struct {
	Pack    string
	Version int
	KYC     string
	DestReuseMax   int
	HourlyVelocity int
	DailyVelocity  int
	Checksum       string
	PublishedAt    time.Time
}

type Policy struct{ db *persist.SQLDB }

   func NewPolicy(db *persist.SQLDB) *Policy { return &Policy{db: db} }

  func (r *Policy) Put(ctx context.Context, doc PolicyDoc, meta PolicyMeta) error {
	  if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	  if doc.PublishedAt.IsZero() {
		  doc.PublishedAt = time.Now().UTC()
	   }
	   if meta.PublishedAt.IsZero() {
		   meta.PublishedAt = doc.PublishedAt
	  }
	   return r.db.InTx(ctx, func(ctx context.Context) error {
		  q := `INSERT INTO aa_policy_docs (pack, version, body, published_at) VALUES (?,?,?,?)`
		   if r.db.Dialect == persist.Postgres {
			   q += ` ON CONFLICT (pack, version) DO NOTHING`
		  }
		   if _, err := r.db.Conn(ctx).ExecContext(ctx, q, doc.Pack, doc.Version, doc.Body, doc.PublishedAt); err != nil {
			  if isUnique(err) {
				  return ErrConflict
			   }
			   return err
		  }
		   mq := `INSERT INTO aa_policy_meta (pack, version, kyc, dest_reuse_max, hourly_velocity, daily_velocity, checksum, published_at)
VALUES (?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE kyc=VALUES(kyc), dest_reuse_max=VALUES(dest_reuse_max), hourly_velocity=VALUES(hourly_velocity), daily_velocity=VALUES(daily_velocity), checksum=VALUES(checksum)`
		   if r.db.Dialect == persist.Postgres {
			   mq = `INSERT INTO aa_policy_meta (pack, version, kyc, dest_reuse_max, hourly_velocity, daily_velocity, checksum, published_at)
VALUES (?,?,?,?,?,?,?,?)
ON CONFLICT (pack, version) DO UPDATE SET kyc=EXCLUDED.kyc, dest_reuse_max=EXCLUDED.dest_reuse_max, hourly_velocity=EXCLUDED.hourly_velocity, daily_velocity=EXCLUDED.daily_velocity, checksum=EXCLUDED.checksum`
		  }
		   _, err := r.db.Conn(ctx).ExecContext(ctx, mq, meta.Pack, meta.Version, meta.KYC, meta.DestReuseMax, meta.HourlyVelocity, meta.DailyVelocity, meta.Checksum, meta.PublishedAt)
		   return err
	  })
   }

  func (r *Policy) Get(ctx context.Context, pack string, ver int) (PolicyDoc, PolicyMeta, error) {
	   if !sqlOn(r.db) {
		  return PolicyDoc{},
			  PolicyMeta{}, ErrUnavailable
	  }
	   var d PolicyDoc
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT pack, version, body, published_at FROM aa_policy_docs WHERE pack=? AND version=?`,
		   strings.ToLower(strings.TrimSpace(pack)), ver,
	   ).Scan(&d.Pack,
		  &d.Version, &d.Body, &d.PublishedAt)
	   if errors.Is(err, sql.ErrNoRows) {
		   return PolicyDoc{}, PolicyMeta{}, ErrNotFound
	  }
	   if err != nil {
		  return PolicyDoc{}, PolicyMeta{}, err
	   }
	  var m PolicyMeta
	  _ = r.db.Conn(ctx).QueryRowContext(ctx,
		   `SELECT pack, version, kyc, dest_reuse_max, hourly_velocity, daily_velocity, checksum, published_at FROM aa_policy_meta WHERE pack=? AND version=?`,
		  d.Pack, d.Version,
	   ).Scan(&m.Pack,
		   &m.Version, &m.KYC, &m.DestReuseMax, &m.HourlyVelocity, &m.DailyVelocity, &m.Checksum, &m.PublishedAt)
	   return d, m, nil
  }

  func (r *Policy) Latest(ctx context.Context, pack string) (PolicyDoc, PolicyMeta, error) {
	  if !sqlOn(r.db) {
		   return PolicyDoc{},
			   PolicyMeta{}, ErrUnavailable
	  }
	  var d PolicyDoc
	   err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT pack, version, body, published_at FROM aa_policy_docs WHERE pack=? ORDER BY version DESC LIMIT 1`,
		  strings.ToLower(strings.TrimSpace(pack)),
	   ).Scan(&d.Pack, &d.Version, &d.Body, &d.PublishedAt)
	   if errors.Is(err, sql.ErrNoRows) {
		   return PolicyDoc{}, PolicyMeta{}, ErrNotFound
	  }
	  if err != nil {
		   return PolicyDoc{},
			   PolicyMeta{}, err
	  }
	   var m PolicyMeta
	   _ = r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT pack, version, kyc, dest_reuse_max, hourly_velocity, daily_velocity, checksum, published_at FROM aa_policy_meta WHERE pack=? AND version=?`,
		   d.Pack, d.Version,
	  ).Scan(&m.Pack, &m.Version, &m.KYC, &m.DestReuseMax, &m.HourlyVelocity, &m.DailyVelocity, &m.Checksum, &m.PublishedAt)
	  return d, m, nil
  }
