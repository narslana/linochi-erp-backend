package db_test

import (
	"context"
	"strings"
	"testing"

	"gitea.lab.internal/linochi/linochi-erp-backend/internal/platform/db"
)

// pgx puts the connection string into its parse errors. It contains the
// password, and errors end up in logs (NFK-14).
func TestNFK14_OpenErrorsNeverContainThePassword(t *testing.T) {
	for _, dsn := range []string{
		"postgres://linochi:s3cr3t-pw@db:notaport/erp",
		"postgres://linochi:s3cr3t-pw@db:5432/erp?sslmode=bogus",
		"postgres://linochi:s3cr3t-pw@db:5432/erp?connect_timeout=soon",
		"postgres://linochi:s3cr3t-pw@db:5432/erp?pool_max_conns=many",
		"host=db user=linochi password=s3cr3t-pw port=oops",
	} {
		_, err := db.Open(context.Background(), db.Options{URL: dsn, MaxConns: 1})
		if err == nil {
			t.Errorf("%q: expected an error", dsn)
			continue
		}
		if strings.Contains(err.Error(), "s3cr3t-pw") {
			t.Errorf("%q: the error leaks the password: %v", dsn, err)
		}
	}
}
