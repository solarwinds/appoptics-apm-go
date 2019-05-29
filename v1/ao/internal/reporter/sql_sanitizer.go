package reporter

const (
	Disabled = iota
	EnabledAuto
	EnabledDropDoubleQuoted
	EnabledKeepDoubleQuoted
)

// SQLSanitize does the sanitization and removes the literals from the statement.
func SQLSanitize(sql string) string {
	// TODO
	return sql
}
