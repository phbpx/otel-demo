package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	oteldemo "github.com/phbpx/otel-demo"
	"go.uber.org/zap"
)

type LeadHandler struct {
	service oteldemo.LeadService
	log     *zap.SugaredLogger
}

func NewLeadHanlder(service oteldemo.LeadService, log *zap.SugaredLogger) *LeadHandler {
	return &LeadHandler{
		service: service,
		log:     log,
	}
}

func (lh LeadHandler) Create(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var lead oteldemo.Lead

	if err := decode(r, &lead); err != nil {
		lh.log.Errorw("GetByID", "error", err.Error())
		respondErr(ctx, rw, http.StatusBadRequest, err)
		return
	}

	now := time.Now().UTC()

	lead.ID = uuid.NewString()
	lead.CreatedAt = now
	lead.ModifiedAt = now

	if err := lh.service.Create(r.Context(), lead); err != nil {
		lh.log.Errorw("Create", "error", err.Error())
		switch err {
		case oteldemo.ErrDuplicatedLead:
			respondErr(ctx, rw, http.StatusConflict, err)
		default:
			respondErr(ctx, rw, http.StatusInternalServerError, err)
		}
		return
	}

	respond(ctx, rw, http.StatusCreated, lead)
}

func (lh LeadHandler) GetByID(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		lh.log.Errorw("GetByID", "error", err.Error())
		respondErr(ctx, rw, http.StatusBadRequest, errors.New("ID is not in its proper form"))
		return
	}

	lead, err := lh.service.GetByID(ctx, id.String())
	if err != nil {
		lh.log.Errorw("GetByID", "error", err.Error())
		switch err {
		case oteldemo.ErrLeadNotFound:
			respondErr(ctx, rw, http.StatusNotFound, err)
		default:
			respondErr(ctx, rw, http.StatusInternalServerError, err)
		}
		return
	}

	respond(ctx, rw, http.StatusOK, lead)
}
