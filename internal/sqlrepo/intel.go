package sqlrepo

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"account-assets.kucoin.net/funding/internal/persist"
)

type IntelTag struct {
	Chain string
	Address string
	Tag string
}

type IntelMark struct {
	Chain   string
	Address string
	Kind string
}

type Intel struct{ db *persist.SQLDB }

   func NewIntel(db *persist.SQLDB) *Intel { return &Intel{db: db} }

  func (r *Intel) PutTag(ctx context.Context, t IntelTag) error {
	   t.Chain = strings.ToUpper(strings.TrimSpace(t.Chain))
	   t.Address = strings.ToLower(strings.TrimSpace(t.Address))
	   t.Tag = strings.TrimSpace(t.Tag)
	   if t.Chain == "" || t.Address == "" || t.Tag == "" {
		   return ErrConflict
	  }
	  if !sqlOn(r.db) {
		   return ErrUnavailable
	   }
	   q := `INSERT INTO aa_intel_tags (chain, address, tag) VALUES (?,?,?)
ON DUPLICATE KEY UPDATE tag=VALUES(tag)`
	  if r.db.Dialect == persist.Postgres {
		   q = `INSERT INTO aa_intel_tags (chain, address, tag) VALUES (?,?,?)
ON CONFLICT (chain, address) DO UPDATE SET tag=EXCLUDED.tag`
	   }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, q, t.Chain, t.Address, t.Tag)
	  return err
   }

  func (r *Intel) GetTag(ctx context.Context, chain, addr string) (IntelTag, error) {
	  if !sqlOn(r.db) {
		  return IntelTag{}, ErrUnavailable
	  }
	   var t IntelTag
	   err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT chain, address, tag FROM aa_intel_tags WHERE chain=? AND address=?`,
		   strings.ToUpper(strings.TrimSpace(chain)), strings.ToLower(strings.TrimSpace(addr)),
	  ).Scan(&t.Chain, &t.Address, &t.Tag)
	   if errors.Is(err, sql.ErrNoRows) {
		  return IntelTag{}, ErrNotFound
	   }
	   return t, err
   }

  func (r *Intel) ListTags(ctx context.Context) ([]IntelTag, error) {
	   if !sqlOn(r.db) {
		   return nil, ErrUnavailable
	  }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT chain, address, tag FROM aa_intel_tags ORDER BY tag, chain, address`)
	   if err != nil {
		  return nil, err
	  }
	  defer rows.Close()
	  out := make([]IntelTag, 0)
	  for rows.Next() {
		   var t IntelTag
		   if err := rows.Scan(&t.Chain, &t.Address, &t.Tag); err != nil {
			   return nil, err
		   }
		  out = append(out, t)
	  }
	  return out, rows.Err()
   }

  func (r *Intel) PutMark(ctx context.Context, m IntelMark) error {
	   m.Chain = strings.ToUpper(strings.TrimSpace(m.Chain))
	  m.Address = strings.ToLower(strings.TrimSpace(m.Address))
	   m.Kind = strings.ToLower(strings.TrimSpace(m.Kind))
	  if m.Chain == "" || m.Address == "" || m.Kind == "" {
		  return ErrConflict
	   }
	   if !sqlOn(r.db) {
		   return ErrUnavailable
	   }
	   q := `INSERT INTO aa_intel_marks (chain, address, kind) VALUES (?,?,?)
ON DUPLICATE KEY UPDATE kind=VALUES(kind)`
	   if r.db.Dialect == persist.Postgres {
		  q = `INSERT INTO aa_intel_marks (chain, address, kind) VALUES (?,?,?)
ON CONFLICT (chain, address) DO UPDATE SET kind=EXCLUDED.kind`
	  }
	  _, err := r.db.Conn(ctx).ExecContext(ctx, q, m.Chain, m.Address, m.Kind)
	   return err
  }

   func (r *Intel) GetMark(ctx context.Context, chain, addr string) (IntelMark, error) {
	  if !sqlOn(r.db) {
		  return IntelMark{}, ErrUnavailable
	   }
	   var m IntelMark
	  err := r.db.Conn(ctx).QueryRowContext(ctx,
		  `SELECT chain, address, kind FROM aa_intel_marks WHERE chain=? AND address=?`,
		   strings.ToUpper(strings.TrimSpace(chain)), strings.ToLower(strings.TrimSpace(addr)),
	  ).Scan(&m.Chain, &m.Address, &m.Kind)
	   if errors.Is(err, sql.ErrNoRows) {
		   return IntelMark{}, ErrNotFound
	   }
	  return m, err
   }

   func (r *Intel) ListMarks(ctx context.Context) ([]IntelMark, error) {
	   if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	  }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx, `SELECT chain, address, kind FROM aa_intel_marks ORDER BY chain, address`)
	  if err != nil {
		   return nil, err
	   }
	  defer rows.Close()
	  out := make([]IntelMark, 0)
	  for rows.Next() {
		   var m IntelMark
		   if err := rows.Scan(&m.Chain, &m.Address, &m.Kind); err != nil {
			   return nil, err
		   }
		  out = append(out, m)
	  }
	  return out, rows.Err()
   }
