package gcp

import (
	"path/filepath"
	"time"

	"github.com/alicebob/sqlittle"
)

// gcpExpiry reads the cached access-token expiry for account from
// <gcDir>/access_tokens.db — the SQLite cache gcloud writes in
// rollback-journal mode with token_expiry as naive-UTC
// "YYYY-MM-DD HH:MM:SS[.ffffff]" text. Best-effort and disk-only: nil on
// any read/parse error, a missing row, or a NULL expiry.
func gcpExpiry(gcDir, account string) *time.Time {
	if account == "" {
		return nil
	}
	db, err := sqlittle.Open(filepath.Join(gcDir, "access_tokens.db"))
	if err != nil {
		return nil
	}
	defer db.Close()
	var out *time.Time
	selErr := db.Select("access_tokens", func(r sqlittle.Row) {
		acct, expiry, err := r.ScanStringString()
		if err != nil || acct != account || expiry == "" {
			return
		}
		// .999999 matches both with- and without-fraction values.
		if t, err := time.ParseInLocation("2006-01-02 15:04:05.999999", expiry, time.UTC); err == nil {
			out = &t
		}
	}, "account_id", "token_expiry")
	if selErr != nil {
		return nil
	}
	return out
}
