package sqlrepo

import (
	"context"
	"strings"
	"time"
	"account-assets.kucoin.net/funding/internal/persist"
	"github.com/google/uuid"
)

type TravelVASP struct {
	Code string
	Name    string
	Country string
	Endpoint string
	DID string
	LEI      string
	Protocol string
	Enabled  bool
	Unhosted bool
	UpdatedAt time.Time
}

type TravelCase struct {
	ID         string
	MovementID string
	UserID string
	Status string
	MsgID string
	Chain string
	Asset string
	Amount string
	Dest   string
	PeerVASP string
	Corridor           string
	OriginatorName     string
	OriginatorAccount  string
	OriginatorVASP     string
	BeneficiaryName    string
	BeneficiaryAccount string
	BeneficiaryVASP string
	Checksum     string
	EnvelopeJSON string
	Actor     string
	Note      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type TravelCaseAudit struct {
	ID string
	CaseID string
	Actor string
	From string
	To   string
	Note string
	At   time.Time
}

type TravelOutbound struct{ db *persist.SQLDB }

   func NewTravelOutbound(db *persist.SQLDB) *TravelOutbound { return &TravelOutbound{db: db} }

   func (r *TravelOutbound) PutVASP(ctx context.Context, row TravelVASP) error {
	  if !sqlOn(r.db) {
		   return ErrUnavailable
	  }
	  row.Code = strings.ToUpper(strings.TrimSpace(row.Code))
	   if row.Code == "" {
		  return ErrConflict
	   }
	  if row.UpdatedAt.IsZero() {
		   row.UpdatedAt = time.Now().UTC()
	  }
	  en, uh := 0, 0
	   if row.Enabled {
		   en = 1
	  }
	   if row.Unhosted {
		  uh = 1
	  }
	  stmt := `INSERT INTO aa_travel_vasps (code, name, country, endpoint, did, lei, protocol, enabled, unhosted, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE name=VALUES(name), country=VALUES(country), endpoint=VALUES(endpoint), did=VALUES(did), lei=VALUES(lei), protocol=VALUES(protocol), enabled=VALUES(enabled), unhosted=VALUES(unhosted), updated_at=VALUES(updated_at)`
	   if r.db.Dialect == persist.Postgres {
		  stmt = `INSERT INTO aa_travel_vasps (code, name, country, endpoint, did, lei, protocol, enabled, unhosted, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (code) DO UPDATE SET name=EXCLUDED.name, country=EXCLUDED.country, endpoint=EXCLUDED.endpoint, did=EXCLUDED.did, lei=EXCLUDED.lei, protocol=EXCLUDED.protocol, enabled=EXCLUDED.enabled, unhosted=EXCLUDED.unhosted, updated_at=EXCLUDED.updated_at`
	   }
	   _, err := r.db.Conn(ctx).ExecContext(ctx, stmt, row.Code, row.Name, row.Country, row.Endpoint, row.DID, row.LEI, row.Protocol, en, uh, row.UpdatedAt)
	  return err
  }

   func (r *TravelOutbound) ListVASPs(ctx context.Context) ([]TravelVASP, error) {
	   if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	  }
	   rows, err := r.db.Conn(ctx).QueryContext(ctx,
		  `SELECT code, name, country, endpoint, did, lei, protocol, enabled, unhosted, updated_at FROM aa_travel_vasps ORDER BY code`)
	   if err != nil {
		   return nil, err
	  }
	   defer rows.Close()
	  out := make([]TravelVASP, 0)
	  for rows.Next() {
		  var row TravelVASP
		   var en, uh int
		  if err := rows.Scan(&row.Code, &row.Name, &row.Country, &row.Endpoint, &row.DID, &row.LEI, &row.Protocol, &en, &uh, &row.UpdatedAt); err != nil {
			   return nil, err
		  }
		  row.Enabled = en != 0
		   row.Unhosted = uh != 0
		   out = append(out, row)
	   }
	  return out, rows.Err()
  }

  func (r *TravelOutbound) PutCase(ctx context.Context, row TravelCase) error {
	  if !sqlOn(r.db) {
		  return ErrUnavailable
	  }
	   if strings.TrimSpace(row.ID) == "" || strings.TrimSpace(row.MovementID) == "" {
		  return ErrConflict
	   }
	   if row.UpdatedAt.IsZero() {
		  row.UpdatedAt = time.Now().UTC()
	   }
	   if row.CreatedAt.IsZero() {
		  row.CreatedAt = row.UpdatedAt
	   }
	  stmt := `INSERT INTO aa_travel_cases (id, movement_id, user_id, status, msg_id, chain, asset, amount, dest, peer_vasp, corridor, originator_name, originator_account, originator_vasp, beneficiary_name, beneficiary_account, beneficiary_vasp, checksum, envelope_json, actor, note, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON DUPLICATE KEY UPDATE status=VALUES(status), msg_id=VALUES(msg_id), chain=VALUES(chain), asset=VALUES(asset), amount=VALUES(amount), dest=VALUES(dest), peer_vasp=VALUES(peer_vasp), corridor=VALUES(corridor), originator_name=VALUES(originator_name), originator_account=VALUES(originator_account), originator_vasp=VALUES(originator_vasp), beneficiary_name=VALUES(beneficiary_name), beneficiary_account=VALUES(beneficiary_account), beneficiary_vasp=VALUES(beneficiary_vasp), checksum=VALUES(checksum), envelope_json=VALUES(envelope_json), actor=VALUES(actor), note=VALUES(note), updated_at=VALUES(updated_at)`
	   if r.db.Dialect == persist.Postgres {
		  stmt = `INSERT INTO aa_travel_cases (id, movement_id, user_id, status, msg_id, chain, asset, amount, dest, peer_vasp, corridor, originator_name, originator_account, originator_vasp, beneficiary_name, beneficiary_account, beneficiary_vasp, checksum, envelope_json, actor, note, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT (id) DO UPDATE SET status=EXCLUDED.status, msg_id=EXCLUDED.msg_id, chain=EXCLUDED.chain, asset=EXCLUDED.asset, amount=EXCLUDED.amount, dest=EXCLUDED.dest, peer_vasp=EXCLUDED.peer_vasp, corridor=EXCLUDED.corridor, originator_name=EXCLUDED.originator_name, originator_account=EXCLUDED.originator_account, originator_vasp=EXCLUDED.originator_vasp, beneficiary_name=EXCLUDED.beneficiary_name, beneficiary_account=EXCLUDED.beneficiary_account, beneficiary_vasp=EXCLUDED.beneficiary_vasp, checksum=EXCLUDED.checksum, envelope_json=EXCLUDED.envelope_json, actor=EXCLUDED.actor, note=EXCLUDED.note, updated_at=EXCLUDED.updated_at`
	  }
	  _,
		   err := r.db.Conn(ctx).ExecContext(ctx, stmt,
		   row.ID, row.MovementID, row.UserID, row.Status, row.MsgID, row.Chain, row.Asset, row.Amount, row.Dest,
		  row.PeerVASP, row.Corridor, row.OriginatorName, row.OriginatorAccount, row.OriginatorVASP,
		   row.BeneficiaryName, row.BeneficiaryAccount, row.BeneficiaryVASP, row.Checksum, row.EnvelopeJSON,
		  row.Actor,
		  row.Note, row.CreatedAt, row.UpdatedAt)
	  return err
   }

   func (r *TravelOutbound) ListCases(ctx context.Context, userID, status string, limit int) ([]TravelCase, error) {
	  if !sqlOn(r.db) {
		  return nil, ErrUnavailable
	  }
	   if limit <= 0 {
		  limit = 50
	   }
	   q := `SELECT id, movement_id, user_id, status, msg_id, chain, asset, amount, dest, peer_vasp, corridor, originator_name, originator_account, originator_vasp, beneficiary_name, beneficiary_account, beneficiary_vasp, checksum, envelope_json, actor, note, created_at, updated_at FROM aa_travel_cases`
	  args := make([]any, 0, 3)
	  where := make([]string, 0, 2)
	  if userID != "" {
		  where = append(where, "user_id=?")
		   args = append(args, userID)
	  }
	  if status != "" {
		   where = append(where, "status=?")
		   args = append(args, status)
	  }
	   if len(where) > 0 {
		   q += " WHERE " + strings.Join(where, " AND ")
	   }
	  q += " ORDER BY updated_at DESC LIMIT ?"
	  args = append(args, limit)
	   rows,
		   err := r.db.Conn(ctx).QueryContext(ctx, q, args...)
	   if err != nil {
		  return nil, err
	  }
	   defer rows.Close()
	   out := make([]TravelCase, 0)
	   for rows.Next() {
		  var row TravelCase
		   if err := rows.Scan(&row.ID, &row.MovementID, &row.UserID, &row.Status, &row.MsgID, &row.Chain, &row.Asset, &row.Amount, &row.Dest, &row.PeerVASP, &row.Corridor, &row.OriginatorName, &row.OriginatorAccount, &row.OriginatorVASP, &row.BeneficiaryName, &row.BeneficiaryAccount, &row.BeneficiaryVASP, &row.Checksum, &row.EnvelopeJSON, &row.Actor, &row.Note, &row.CreatedAt, &row.UpdatedAt); err != nil {
			   return nil, err
		   }
		  out = append(out, row)
	  }
	  return out, rows.Err()
  }

  func (r *TravelOutbound) PutCaseAudit(ctx context.Context, a TravelCaseAudit) error {
	  if !sqlOn(r.db) {
		  return ErrUnavailable
	   }
	   if a.ID == "" {
		  a.ID = uuid.NewString()
	   }
	  if a.At.IsZero() {
		  a.At = time.Now().UTC()
	   }
	  _, err := r.db.Conn(ctx).ExecContext(ctx,
		   `INSERT INTO aa_travel_case_audit (id, case_id, actor, from_status, to_status, note, at) VALUES (?,?,?,?,?,?,?)`,
		  a.ID, a.CaseID, a.Actor, a.From, a.To, a.Note, a.At)
	  return err
   }
