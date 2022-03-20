package postgres

import (
	"context"
	"database/sql"

	"github.com/lib/pq"
	oteldemo "github.com/phbpx/otel-demo"
)

// lib/pq errorCodeNames
// https://github.com/lib/pq/blob/master/error.go#L178
const uniqueViolation = "23505"

type LeadService struct {
	db *sql.DB
}

func NewLeadService(db *sql.DB) oteldemo.LeadService {
	return &LeadService{
		db: db,
	}
}

func (ls LeadService) Create(ctx context.Context, lead oteldemo.Lead) error {
	tx, err := ls.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}

	query := `
	INSERT INTO leads (
		id, name, email, phone_number, address, created_at, modified_at
	) VALUES (
		$1, $2, $3, $4, $5, $6, $7
	)`

	_, err = tx.ExecContext(ctx, query,
		lead.ID,
		lead.Name,
		lead.Email,
		lead.PhoneNumber,
		lead.Address,
		lead.CreatedAt,
		lead.ModifiedAt,
	)

	if err != nil {
		tx.Rollback()
		if pqerr, ok := err.(*pq.Error); ok && pqerr.Code == uniqueViolation {
			return oteldemo.ErrDuplicatedLead
		}
		return err
	}

	return tx.Commit()
}

func (ls LeadService) GetByID(ctx context.Context, id string) (oteldemo.Lead, error) {
	query := `
	SELECT 
		id, 
		name, 
		email, 
		phone_number, 
		address, 
		created_at, 
		modified_at 
	FROM leads 
	WHERE id=$1`

	row := ls.db.QueryRowContext(ctx, query, id)

	lead := oteldemo.Lead{}
	err := row.Scan(
		&lead.ID,
		&lead.Name,
		&lead.Email,
		&lead.PhoneNumber,
		&lead.Address,
		&lead.CreatedAt,
		&lead.ModifiedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return lead, oteldemo.ErrLeadNotFound
		}
		return lead, err
	}

	return lead, nil
}
