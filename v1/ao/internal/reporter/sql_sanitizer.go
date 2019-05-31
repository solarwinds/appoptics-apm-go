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
	// EnabledDropDoubleQuoted - enable SQL sanitizing and force dropping double
	// quoted characters.
	EnabledDropDoubleQuoted
	// EnabledKeepDoubleQuoted enable SQL sanitizing and force retaining double
	// quoted character.
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
	// ReplacementChar is the char to replace the removed literals
	ReplacementChar = '?'
	// EscapeChar is the SQL escape character
	EscapeChar = '\\'
)

// SQLOperatorChars defines the list of SQL operators
var SQLOperatorChars = map[rune]bool{
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

// Sanitize does the SQL sanitization by removing literals from the statement
func (s *SQLSanitizer) Sanitize(sql string) string {
	var buffer bytes.Buffer

	currState := Copy
	prevState := Uninitialized

	var closingQuoteChar rune

	for _, currChar := range sql {
		if currState != prevState {
			prevState = currState
		}

		switch currState {
		case StringStart:
			buffer.WriteRune(ReplacementChar)

			if currChar == closingQuoteChar {
				currState = StringEnd
			} else if currChar == EscapeChar {
				currState = StringEscape
			} else {
				currState = StringBody
			}

		case StringBody:
			if currChar == closingQuoteChar {
				currState = StringEnd
			} else if currChar == EscapeChar {
				currState = StringEscape
			}

		case StringEscape:
			currState = StringBody

		case StringEnd:
			if currChar == closingQuoteChar {
				currState = StringBody
			} else {
				buffer.WriteRune(currChar)
				currState = Copy
			}

		case CopyEscape:
			buffer.WriteRune(currChar)
			currState = Copy

		case Number:
			if !unicode.IsDigit(currChar) {
				if currChar == '.' || currChar == 'E' {
					currState = NumericExtension
				} else {
					buffer.WriteRune(currChar)
					currState = Copy
				}
			}

		case NumericExtension:
			currState = Number

		case Identifier:
			buffer.WriteRune(currChar)
			if c, ok := s.literalQuotes[currChar]; ok {
				closingQuoteChar = c
				currState = StringStart
			} else if unicode.IsSpace(currChar) || SQLOperatorChars[currChar] {
				currState = Copy
			}

		case QuotedIdentifier:
			buffer.WriteRune(currChar)
			if currChar == closingQuoteChar {
				currState = Copy
			} else if currChar == EscapeChar {
				currState = QuotedIdentifierEscape
			}

		case QuotedIdentifierEscape:
			buffer.WriteRune(currChar)
			currState = QuotedIdentifier

		default:
			if lq, ok := s.literalQuotes[currChar]; ok {
				closingQuoteChar = lq
				currState = StringStart
			} else if iq, has := s.identifierQuotes[currChar]; has {
				buffer.WriteRune(currChar)
				closingQuoteChar = iq
				currState = QuotedIdentifier
			} else if currChar == EscapeChar {
				buffer.WriteRune(currChar)
				currState = CopyEscape
			} else if unicode.IsLetter(currChar) || currChar == '_' {
				buffer.WriteRune(currChar)
				currState = Identifier
			} else if unicode.IsDigit(currChar) {
				buffer.WriteRune(ReplacementChar)
				currState = Number
			} else {
				buffer.WriteRune(currChar)
			}
		}
	}

	return buffer.String()
}

// SQLSanitize checks the sanitizer of the database type and does the sanitization
// accordingly. It uses the default sanitizer if the type is not found.
func SQLSanitize(dbType string, sql string) string {
	return sqlSanitizeInternal(sanitizers, dbType, sql)
}

func sqlSanitizeInternal(ss map[string]*SQLSanitizer, dbType string, sql string) string {
	if ss == nil {
		return sql
	}
	if s, ok := ss[dbType]; ok {
		return s.Sanitize(sql)
	}
	return ss[Default].Sanitize(sql)
}
