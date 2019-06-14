package reporter

import (
	"strings"
	"unicode"
	"unicode/utf8"

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
	FSMUninitialized FSMState = iota
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
	Ellipsis = '…'
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

	// For double-dollar quoted literals: $tag$literal$tag$
	if dbType == PostgreSQL {
		sanitizer.literalQuotes['$'] = '$'
	}

	return &sanitizer
}

// Sanitize does the SQL sanitization by removing literals from the statement, it
// also truncates the statement after sanitization if it's longer than MaxSQLLen.
func (s *SQLSanitizer) Sanitize(sql string) string {

	currState := FSMCopy
	prevState := FSMUninitialized

	var closingQuote rune

	// A very simple stack implementation solely for the sanitizer. It doesn't
	// enforce enough boundary checks and is not concurrent-safe.
	buffer := make([]rune, utf8.RuneCountInString(sql))
	runeCnt := 0                // should not exceed MaxSQLLen-1
	maxRuneCnt := MaxSQLLen - 1 // len("…")

	StackPush := func(r rune) {
		if runeCnt < len(buffer) {
			buffer[runeCnt] = r
			runeCnt++
		}
	}

	StackPop := func() {
		if runeCnt > 0 {
			runeCnt--
		}
	}

	// depth = 1 means to peek the top element
	StackPeek := func(depth int) rune {
		if runeCnt < depth {
			// too deep
			return 0
		}
		return buffer[runeCnt-depth]
	}

	StackCopy := func() []rune {
		return buffer[:runeCnt]
	}

	StackSize := func() int {
		return runeCnt
	}

	for _, currRune := range sql {
		if StackSize() >= maxRuneCnt {
			StackPush(Ellipsis)
			break
		}

		if currState != prevState {
			prevState = currState
		}

		switch currState {
		case FSMStringStart:
			// Handle PostgreSQL's double-dollar quoted literal
			if s.dbType == PostgreSQL && closingQuote == '$' {
				if currRune == '$' {
					currState = FSMStringBody
					StackPush(ReplacementRune)
				}
				break // break out of switch
			}

			StackPush(ReplacementRune)

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
			// Handle PostgreSQL's double-dollar quoted literal
			if s.dbType == PostgreSQL && closingQuote == '$' {
				if currRune == '$' {
					// The real end of closing quote
					currState = FSMCopy
				}
				break // break out of switch
			}

			if currRune == closingQuote {
				currState = FSMStringBody
			} else {
				StackPush(currRune)
				currState = FSMCopy
			}

		case FSMCopyEscape:
			StackPush(currRune)
			currState = FSMCopy

		case FSMNumber:
			if !unicode.IsDigit(currRune) {
				if currRune == '.' || currRune == 'E' {
					currState = FSMNumericExtension
				} else {
					StackPush(currRune)
					currState = FSMCopy
				}
			}

		case FSMNumericExtension:
			currState = FSMNumber

		case FSMIdentifier:
			if c, ok := s.literalQuotes[currRune]; ok {
				// PostgreSQL has literals like X'FEFF' or U&'\0441'
				top1 := unicode.ToUpper(StackPeek(1))
				top2 := unicode.ToUpper(StackPeek(2))
				if top1 == 'X' || top1 == 'B' || top1 == 'U' || top1 == 'N' {
					StackPop()
				} else if top1 == '&' && top2 == 'U' {
					StackPop()
					StackPop()
				}
				closingQuote = c
				currState = FSMStringStart
			} else {
				StackPush(currRune)
				if unicode.IsSpace(currRune) || SQLOperatorRunes[currRune] {
					currState = FSMCopy
				}
			}

		case FSMQuotedIdentifier:
			StackPush(currRune)
			if currRune == closingQuote {
				currState = FSMCopy
			} else if currRune == EscapeRune {
				currState = FSMQuotedIdentifierEscape
			}

		case FSMQuotedIdentifierEscape:
			StackPush(currRune)
			currState = FSMQuotedIdentifier

		default:
			if lq, ok := s.literalQuotes[currRune]; ok {
				closingQuote = lq
				currState = FSMStringStart
			} else if iq, has := s.identifierQuotes[currRune]; has {
				StackPush(currRune)
				closingQuote = iq
				currState = FSMQuotedIdentifier
			} else if currRune == EscapeRune {
				StackPush(currRune)
				currState = FSMCopyEscape
			} else if unicode.IsLetter(currRune) || currRune == '_' {
				StackPush(currRune)
				currState = FSMIdentifier
			} else if unicode.IsDigit(currRune) {
				StackPush(ReplacementRune)
				currState = FSMNumber
			} else {
				StackPush(currRune)
			}
		}
	}

	return string(StackCopy())
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
