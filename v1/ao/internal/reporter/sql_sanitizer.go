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
	DefaultDB  = "default"
)

// FSMState defines the states of a FSM
type FSMState int

// The states for the SQL sanitization FSM
const (
	FSMUninitialized = iota
	FSMCopy
	FSMCopyEscape
	FSMStringStart
	FSMStringBody
	FSMStringEscape
	FSMStringEnd
	FSMNumber
	FSMNumericExtension
	FSMIdentifier
	FSMQuotedIdentifier
	FSMQuotedIdentifierEscape
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
	for _, t := range []string{PostgreSQL, Oracle, MySQL, Sybase, SQLServer, DefaultDB} {
		ss[t] = NewSQLSanitizer(t, sanitizeFlag)
	}
	return ss
}

// NewSQLSanitizer returns the pointer of a new SQLSanitizer.
func NewSQLSanitizer(dbType string, sanitizeFlag int) *SQLSanitizer {
	sanitizer := SQLSanitizer{
		dbType:           strings.ToLower(dbType),
		literalQuotes:    make(map[rune]rune),
		identifierQuotes: make(map[rune]rune),
	}

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

	currState := FSMCopy
	prevState := FSMUninitialized

	var closingQuote rune

	runeCnt := 0                // should not exceed MaxSQLLen
	maxRuneCnt := MaxSQLLen - 3 // len("...")

	WriteRuneAndInc := func(r rune) {
		buffer.WriteRune(r)
		runeCnt++
	}

	for _, currRune := range sql {
		if runeCnt >= maxRuneCnt {
			buffer.WriteString(Ellipsis)
			break
		}

		if currState != prevState {
			prevState = currState
		}

		switch currState {
		case FSMStringStart:
			WriteRuneAndInc(ReplacementRune)

			if currRune == closingQuote {
				currState = FSMStringEnd
			} else if currRune == EscapeRune {
				currState = FSMStringEscape
			} else {
				currState = FSMStringBody
			}

		case FSMStringBody:
			if currRune == closingQuote {
				currState = FSMStringEnd
			} else if currRune == EscapeRune {
				currState = FSMStringEscape
			}

		case FSMStringEscape:
			currState = FSMStringBody

		case FSMStringEnd:
			if currRune == closingQuote {
				currState = FSMStringBody
			} else {
				WriteRuneAndInc(currRune)
				currState = FSMCopy
			}

		case FSMCopyEscape:
			WriteRuneAndInc(currRune)
			currState = FSMCopy

		case FSMNumber:
			if !unicode.IsDigit(currRune) {
				if currRune == '.' || currRune == 'E' {
					currState = FSMNumericExtension
				} else {
					WriteRuneAndInc(currRune)
					currState = FSMCopy
				}
			}

		case FSMNumericExtension:
			currState = FSMNumber

		case FSMIdentifier:
			WriteRuneAndInc(currRune)
			if c, ok := s.literalQuotes[currRune]; ok {
				closingQuote = c
				currState = FSMStringStart
			} else if unicode.IsSpace(currRune) || SQLOperatorRunes[currRune] {
				currState = FSMCopy
			}

		case FSMQuotedIdentifier:
			WriteRuneAndInc(currRune)
			if currRune == closingQuote {
				currState = FSMCopy
			} else if currRune == EscapeRune {
				currState = FSMQuotedIdentifierEscape
			}

		case FSMQuotedIdentifierEscape:
			WriteRuneAndInc(currRune)
			currState = FSMQuotedIdentifier

		default:
			if lq, ok := s.literalQuotes[currRune]; ok {
				closingQuote = lq
				currState = FSMStringStart
			} else if iq, has := s.identifierQuotes[currRune]; has {
				WriteRuneAndInc(currRune)
				closingQuote = iq
				currState = FSMQuotedIdentifier
			} else if currRune == EscapeRune {
				WriteRuneAndInc(currRune)
				currState = FSMCopyEscape
			} else if unicode.IsLetter(currRune) || currRune == '_' {
				WriteRuneAndInc(currRune)
				currState = FSMIdentifier
			} else if unicode.IsDigit(currRune) {
				WriteRuneAndInc(ReplacementRune)
				currState = FSMNumber
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
	return ss[DefaultDB].Sanitize(sql)
}
