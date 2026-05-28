package ui

import "github.com/jackc/pgx/v5/pgtype"

// pgUUID is a tiny wrapper to keep handler call sites concise.
type pgUUID struct{ v pgtype.UUID }

func (p *pgUUID) scan(s string) error { return p.v.Scan(s) }
