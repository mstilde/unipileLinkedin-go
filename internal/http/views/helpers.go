package views

import (
	"strconv"

	"github.com/jackc/pgx/v5/pgtype"
)

func itoa(n int) string { return strconv.Itoa(n) }

func strDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func uuidStr(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b, _ := u.MarshalJSON()
	s := string(b)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}
