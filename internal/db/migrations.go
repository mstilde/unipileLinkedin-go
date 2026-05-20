package db

import (
	"embed"
	"io/fs"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migrations returns an fs.FS rooted at the migrations directory, suitable for
// goose.SetBaseFS. The returned FS contains entries like "00001_users_accounts.sql".
func Migrations() fs.FS {
	sub, err := fs.Sub(migrationsFS, "migrations")
	if err != nil {
		// Should be impossible — the path is embedded at compile time.
		panic(err)
	}
	return sub
}
