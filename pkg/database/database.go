package database

import (
	"context"
	"database/sql"
	"net/url"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/nhatthm/otelsql"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

// Config is the required properties to use the database.
type Config struct {
	User         string
	Password     string
	Host         string
	Name         string
	MaxIdleConns int
	MaxOpenConns int
	DisableTLS   bool
}

// Open knows how to open a database connection based on the configuration.
func Open(cfg Config) (*sqlx.DB, error) {
	sslMode := "require"
	if cfg.DisableTLS {
		sslMode = "disable"
	}

	q := make(url.Values)
	q.Set("sslmode", sslMode)
	q.Set("timezone", "utc")

	u := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(cfg.User, cfg.Password),
		Host:     cfg.Host,
		Path:     cfg.Name,
		RawQuery: q.Encode(),
	}

	// Register the otelsql wrapper for the provided postgres driver.
	driverName, err := otelsql.Register("postgres",
		otelsql.AllowRoot(),
		otelsql.TraceQueryWithoutArgs(),
		otelsql.TraceRowsClose(),
		otelsql.TraceRowsAffected(),
		otelsql.WithDatabaseName(cfg.Name),
		otelsql.WithSystem(semconv.DBSystemPostgreSQL),
	)
	if err != nil {
		return nil, err
	}

	// Connect to a Postgres database using the postgres driver wrapper.
	db, err := sql.Open(driverName, u.String())
	if err != nil {
		return nil, err
	}

	// Database metrics
	if err := otelsql.RecordStats(db); err != nil {
		return nil, err
	}

	return sqlx.NewDb(db, "postgres"), nil
}

// StatusCheck returns nil if it can successfully talk to the database. It
// returns a non-nil error otherwise.
func StatusCheck(ctx context.Context, db *sqlx.DB) error {

	// First check we can ping the database.
	var pingError error
	for attempts := 1; ; attempts++ {
		pingError = db.Ping()
		if pingError == nil {
			break
		}
		time.Sleep(time.Duration(attempts) * 100 * time.Millisecond)
		if ctx.Err() != nil {
			return ctx.Err()
		}
	}

	// Make sure we didn't timeout or be cancelled.
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Run a simple query to determine connectivity. Running this query forces a
	// round trip through the database.
	const q = `SELECT true`
	var tmp bool
	return db.QueryRowContext(ctx, q).Scan(&tmp)
}
