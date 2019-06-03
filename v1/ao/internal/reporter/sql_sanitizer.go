package reporter

import (
	"bytes"
	"strings"
	"unicode"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/config"
)

// The SQL sanitization mode
const (
	// Disabled - disable SQL sanitizing (the default).
	Disabled = iota
	// EnabledAuto - enable SQL sanitizing and attempt to automatically determine
	// which quoting form to use.
	EnabledAuto
	// EnabledDropDoubleQuoted - enable SQL sanitizing and force dropping double-
	// quoted runes.
	EnabledDropDoubleQuoted
	// EnabledKeepDoubleQuoted enable SQL sanitizing and force retaining double-
	// quoted runes.
	EnabledKeepDoubleQuoted
)

// The database types
const (
	PostgreSQL = "postgresql"
	Oracle     = "oracle"
	MySQL      = "mysql"
	Sybase     = "sybase"
	SQLServer  = "sqlserver"
	Default    = "default"
)

// FSMState defines the states of a FSM
type FSMState int

// The states for the SQL sanitization FSM
const (
	Uninitialized = iota
	Copy
	CopyEscape
	StringStart
	StringBody
	StringEscape
	StringEnd
	Number
	NumericExtension
	Identifier
	QuotedIdentifier
	QuotedIdentifierEscape
)

const (
	// ReplacementRune is the rune to replace the removed literals
	ReplacementRune = '?'
	// EscapeRune is the SQL escape rune
	EscapeRune = '\\'
	// MaxSQLLen defines the maximum number of runes after truncation.
	MaxSQLLen = 2048
	// Ellipsis is appended to the truncated SQL statement
	Ellipsis = "..."
)

// SQLOperatorRunes defines the list of SQL operators
var SQLOperatorRunes = map[rune]bool{
	'.': true,
	';': true,
	'(': true,
	')': true,
	',': true,
	'+': true,
	'-': true,
	'*': true,
	'/': true,
	'|': true,
	'=': true,
	'!': true,
	'^': true,
	'>': true,
	'<': true,
	'[': true,
	']': true,
}

// SQLSanitizer sanitizes the SQL statement by removing literals from it.
type SQLSanitizer struct {
	// database types
	dbType string
	// the quotes surrounding literals
	literalQuotes map[rune]rune
	// the quotes surrounding identifiers, e.g., column names
	identifierQuotes map[rune]rune
}

// the sanitizers for various database types, which is initialized in the init
// function.
var sanitizers map[string]*SQLSanitizer

func init() {
	sanitizers = initSanitizersMap()
}

func initSanitizersMap() map[string]*SQLSanitizer {
	sanitizeFlag := config.GetSQLSanitize()
	if sanitizeFlag == Disabled {
		return nil
	}

	ss := make(map[string]*SQLSanitizer)
	for _, t := range []string{PostgreSQL, Oracle, MySQL, Sybase, SQLServer, Default} {
		ss[t] = NewSQLSanitizer(t)
	}
	return ss
}

// NewSQLSanitizer returns the pointer of a new SQLSanitizer.
func NewSQLSanitizer(dbType string) *SQLSanitizer {
	sanitizer := SQLSanitizer{
		dbType:           strings.ToLower(dbType),
		literalQuotes:    make(map[rune]rune),
		identifierQuotes: make(map[rune]rune),
	}

	sanitizeFlag := config.GetSQLSanitize()

	sanitizer.literalQuotes['\''] = '\''

	if sanitizeFlag == EnabledDropDoubleQuoted {
		sanitizer.literalQuotes['"'] = '"'
	} else if sanitizeFlag == EnabledKeepDoubleQuoted {
		sanitizer.identifierQuotes['"'] = '"'
	} else {
		if dbType == PostgreSQL || dbType == Oracle {
			sanitizer.identifierQuotes['"'] = '"'
		} else {
			sanitizer.literalQuotes['"'] = '"'
		}
	}

	if dbType == MySQL {
		sanitizer.identifierQuotes['`'] = '`'
	} else if dbType == Sybase || dbType == SQLServer {
		sanitizer.identifierQuotes['['] = ']'
	}

	return &sanitizer
}

// Sanitize does the SQL sanitization by removing literals from the statement, it
// also truncates the statement after sanitization if it's longer than MaxSQLLen.
func (s *SQLSanitizer) Sanitize(sql string) string {
	var buffer bytes.Buffer

	currState := Copy
	prevState := Uninitialized

	var closingQuoteChar rune

	runeCnt := 0 // should not exceed MaxSQLLen
	WriteRuneAndInc := func(r rune) {
		buffer.WriteRune(r)
		runeCnt++
	}

	maxRuneCnt := MaxSQLLen - 3 // len("...")

	for _, currRune := range sql {
		if runeCnt >= maxRuneCnt {
			buffer.WriteString(Ellipsis)
			break
		}

		if currState != prevState {
			prevState = currState
		}

		switch currState {
		case StringStart:
			WriteRuneAndInc(ReplacementRune)

			if currRune == closingQuoteChar {
				currState = StringEnd
			} else if currRune == EscapeRune {
				currState = StringEscape
			} else {
				currState = StringBody
			}

		case StringBody:
			if currRune == closingQuoteChar {
				currState = StringEnd
			} else if currRune == EscapeRune {
				currState = StringEscape
			}

		case StringEscape:
			currState = StringBody

		case StringEnd:
			if currRune == closingQuoteChar {
				currState = StringBody
			} else {
				WriteRuneAndInc(currRune)
				currState = Copy
			}

		case CopyEscape:
			WriteRuneAndInc(currRune)
			currState = Copy

		case Number:
			if !unicode.IsDigit(currRune) {
				if currRune == '.' || currRune == 'E' {
					currState = NumericExtension
				} else {
					WriteRuneAndInc(currRune)
					currState = Copy
				}
			}

		case NumericExtension:
			currState = Number

		case Identifier:
			WriteRuneAndInc(currRune)
			if c, ok := s.literalQuotes[currRune]; ok {
				closingQuoteChar = c
				currState = StringStart
			} else if unicode.IsSpace(currRune) || SQLOperatorRunes[currRune] {
				currState = Copy
			}

		case QuotedIdentifier:
			WriteRuneAndInc(currRune)
			if currRune == closingQuoteChar {
				currState = Copy
			} else if currRune == EscapeRune {
				currState = QuotedIdentifierEscape
			}

		case QuotedIdentifierEscape:
			WriteRuneAndInc(currRune)
			currState = QuotedIdentifier

		default:
			if lq, ok := s.literalQuotes[currRune]; ok {
				closingQuoteChar = lq
				currState = StringStart
			} else if iq, has := s.identifierQuotes[currRune]; has {
				WriteRuneAndInc(currRune)
				closingQuoteChar = iq
				currState = QuotedIdentifier
			} else if currRune == EscapeRune {
				WriteRuneAndInc(currRune)
				currState = CopyEscape
			} else if unicode.IsLetter(currRune) || currRune == '_' {
				WriteRuneAndInc(currRune)
				currState = Identifier
			} else if unicode.IsDigit(currRune) {
				WriteRuneAndInc(ReplacementRune)
				currState = Number
			} else {
				WriteRuneAndInc(currRune)
			}
		}
	}

	return buffer.String()
}

// SQLSanitize checks the sanitizer of the database type and does the sanitization
// accordingly. It uses the default sanitizer if the type is not found.
func SQLSanitize(dbType string, sql string) string {
	return sqlSanitize(sanitizers, dbType, sql)
}

func sqlSanitize(ss map[string]*SQLSanitizer, dbType string, sql string) string {
	if ss == nil {
		return sql
	}
	if s, ok := ss[dbType]; ok {
		return s.Sanitize(sql)
	}
	return ss[Default].Sanitize(sql)
}
