package oteldemo

import (
	"context"
	"errors"
	"time"
)

var (
	ErrDuplicatedLead = errors.New("email already in use")
	ErrLeadNotFound   = errors.New("lead not found")
)

type Lead struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Email       string    `json:"email"`
	PhoneNumber string    `json:"phone_number"`
	Address     string    `json:"address"`
	CreatedAt   time.Time `json:"created_at"`
	ModifiedAt  time.Time `json:"modified_at"`
}

type LeadService interface {
	Create(ctx context.Context, newLead Lead) error
	GetByID(ctx context.Context, id string) (Lead, error)
}
