package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
)

func decode(r *http.Request, into interface{}) error {
	rawJson, err := io.ReadAll(r.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(rawJson, into)
}

func respond(ctx context.Context, rw http.ResponseWriter, status int, data interface{}) {
	ctx, span := otel.GetTracerProvider().Tracer("").Start(ctx, "handler.respond")
	span.SetAttributes(attribute.Int("http.status", status))
	defer span.End()

	if status == http.StatusNoContent || data == nil {
		rw.WriteHeader(status)
		return
	}

	rawJson, err := json.Marshal(data)
	if err != nil {
		panic("respond-json-marshal:" + err.Error())
	}

	rw.Header().Add("Content-Type", "application-json")
	rw.WriteHeader(status)
	rw.Write(rawJson)
}

func respondErr(ctx context.Context, rw http.ResponseWriter, status int, err error) {
	respond(ctx, rw, status, map[string]string{
		"code":  http.StatusText(status),
		"error": err.Error(),
	})
}
