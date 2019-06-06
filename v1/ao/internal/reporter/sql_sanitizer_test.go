package reporter

import (
	"fmt"
	"os"
	"strconv"
	"testing"

	"github.com/appoptics/appoptics-apm-go/v1/ao/internal/config"
	"github.com/stretchr/testify/assert"
)

// SanitizeTestCase defines the sanitizer input and output
type SanitizeTestCase struct {
	mode         int
	dbType       string
	sql          string
	sanitizedSQL string
}

func TestSQLSanitize(t *testing.T) {
	cases := []SanitizeTestCase{
		// Disabled
		{
			Disabled,
			MySQL,
			"SELECT name FROM employees WHERE age = 37 AND firstName = 'Eric'",
			"SELECT name FROM employees WHERE age = 37 AND firstName = 'Eric'",
		},
		{
			Disabled,
			Oracle,
			"SELECT name FROM employees WHERE age = 37 AND firstName = 'Eric'",
			"SELECT name FROM employees WHERE age = 37 AND firstName = 'Eric'",
		},
		{
			Disabled,
			Sybase,
			"SELECT name FROM employees WHERE age = 37 AND firstName = 'Eric'",
			"SELECT name FROM employees WHERE age = 37 AND firstName = 'Eric'",
		},
		{
			Disabled,
			SQLServer,
			"SELECT name FROM employees WHERE age = 37 AND firstName = 'Eric'",
			"SELECT name FROM employees WHERE age = 37 AND firstName = 'Eric'",
		},
		{
			Disabled,
			PostgreSQL,
			"SELECT name FROM employees WHERE age = 37 AND firstName = 'Eric'",
			"SELECT name FROM employees WHERE age = 37 AND firstName = 'Eric'",
		},
		{
			Disabled,
			DefaultDB,
			"SELECT name FROM employees WHERE age = 37 AND firstName = 'Eric'",
			"SELECT name FROM employees WHERE age = 37 AND firstName = 'Eric'",
		},
		// EnabledAuto
		{
			EnabledAuto,
			DefaultDB,
			"select * from schema.tbl where name = '';",
			"select * from schema.tbl where name = ?;",
		},
		{
			EnabledDropDoubleQuoted,
			DefaultDB,
			"select * from tbl where name = 'private';",
			"select * from tbl where name = ?;",
		},
		{
			EnabledKeepDoubleQuoted,
			DefaultDB,
			"select * from tbl where name = 'private' order by age;",
			"select * from tbl where name = ? order by age;",
		},
		{
			EnabledAuto,
			DefaultDB,
			"select ssn from accounts where password = \"mypass\" group by dept order by age;",
			"select ssn from accounts where password = ? group by dept order by age;",
		},
		{
			EnabledDropDoubleQuoted,
			DefaultDB,
			"select ssn from accounts where password = \"mypass\";",
			"select ssn from accounts where password = ?;",
		},
		{
			EnabledKeepDoubleQuoted,
			DefaultDB,
			"select ssn from accounts where password = \"mypass\";",
			"select ssn from accounts where password = \"mypass\";",
		},
		{
			EnabledAuto,
			DefaultDB,
			"select ssn from accounts where name = 'Chris O''Corner'",
			"select ssn from accounts where name = ?",
		},
		{
			EnabledAuto,
			DefaultDB,
			"SELECT name FROM employees WHERE age = 37 AND firstName = 'Eric'",
			"SELECT name FROM employees WHERE age = ? AND firstName = ?",
		},
		{
			EnabledAuto,
			DefaultDB,
			"SELECT name FROM employees WHERE name IN ('Eric', 'Tom')",
			"SELECT name FROM employees WHERE name IN (?, ?)",
		},
		{
			EnabledAuto,
			DefaultDB,
			"SELECT TOP 10 FROM employees WHERE age > 28",
			"SELECT TOP ? FROM employees WHERE age > ?",
		},
		{
			EnabledAuto,
			DefaultDB,
			"UPDATE Customers SET zip='V3B 6Z6', phone='000-000-0000'",
			"UPDATE Customers SET zip=?, phone=?",
		},
		{
			EnabledAuto,
			DefaultDB,
			"SELECT id FROM employees WHERE date BETWEEN '01/01/2019' AND '05/30/2019'",
			"SELECT id FROM employees WHERE date BETWEEN ? AND ?",
		},
		{
			EnabledAuto,
			DefaultDB,
			`SELECT name FROM employees 
			 WHERE EXISTS (
				SELECT eid FROM orders WHERE employees.id = orders.eid AND price > 1000
			 )`,
			`SELECT name FROM employees 
			 WHERE EXISTS (
				SELECT eid FROM orders WHERE employees.id = orders.eid AND price > ?
			 )`,
		},
		{
			EnabledAuto,
			DefaultDB,
			"SELECT COUNT(id), team FROM employees GROUP BY team HAVING MIN(age) > 30",
			"SELECT COUNT(id), team FROM employees GROUP BY team HAVING MIN(age) > ?",
		},
		{
			EnabledAuto,
			DefaultDB,
			`WITH tmp AS (SELECT * FROM employees WHERE team = 'IT') 
			 SELECT * FROM tmp WHERE name = 'Tom'`,
			`WITH tmp AS (SELECT * FROM employees WHERE team = ?) 
			 SELECT * FROM tmp WHERE name = ?`,
		},
		{
			EnabledKeepDoubleQuoted,
			Oracle,
			`WITH tmp AS (SELECT * FROM \"Employees\" WHERE team = 'IT') 
			 SELECT * FROM tmp WHERE name = 'Tom'`,
			`WITH tmp AS (SELECT * FROM \"Employees\" WHERE team = ?) 
			 SELECT * FROM tmp WHERE name = ?`,
		},
		{
			EnabledKeepDoubleQuoted,
			PostgreSQL,
			`WITH tmp AS (SELECT * FROM \"Employees\" WHERE team = 'IT') 
			 SELECT * FROM tmp WHERE name = 'Tom'`,
			`WITH tmp AS (SELECT * FROM \"Employees\" WHERE team = ?) 
			 SELECT * FROM tmp WHERE name = ?`,
		},
		{
			EnabledKeepDoubleQuoted,
			MySQL,
			"WITH tmp AS (SELECT * FROM `Employees` WHERE team = 'IT') SELECT * FROM tmp WHERE name = 'Tom'",
			"WITH tmp AS (SELECT * FROM `Employees` WHERE team = ?) SELECT * FROM tmp WHERE name = ?",
		},
		{
			EnabledDropDoubleQuoted,
			DefaultDB,
			`WITH tmp AS (SELECT * FROM employees WHERE team = "IT") 
			 SELECT * FROM tmp WHERE name = 'Tom' 
			 LEFT JOIN tickets 
			 WHERE eid = id AND last_update BETWEEN '01/01/2019' AND '05/30/2019'`,
			`WITH tmp AS (SELECT * FROM employees WHERE team = ?) 
			 SELECT * FROM tmp WHERE name = ? 
			 LEFT JOIN tickets 
			 WHERE eid = id AND last_update BETWEEN ? AND ?`,
		},
		{ // unicode support
			EnabledDropDoubleQuoted,
			DefaultDB,
			`select ssn from accounts where name = '中文'`,
			`select ssn from accounts where name = ?`,
		},
		{ // Truncate long SQL statement
			EnabledDropDoubleQuoted,
			DefaultDB,
			`WITH tmp AS (SELECT * FROM employees WHERE team = "IT") 
			 SELECT
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_1,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_2,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_3,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_4,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_5,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_6,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_7,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_8,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_9,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_10,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_11,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_12,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_13,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_14,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_15,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_16,
			 FROM tmp WHERE name = 'Tom' 
			 LEFT JOIN tickets 
			 WHERE eid = id AND last_update BETWEEN '01/01/2019' AND '05/30/2019'`,
			`WITH tmp AS (SELECT * FROM employees WHERE team = ?) 
			 SELECT
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_1,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_2,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_3,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_4,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_5,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_6,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_7,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_8,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_9,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_10,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_11,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_12,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_13,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_14,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_15,
			 very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_very_long_column_16,
			 FROM tmp WHERE name = ? 
			 LEFT JOIN tickets 
			 WHERE eid = id …`,
		},
		{ // prepared statements: placeholders + literals
			EnabledAuto,
			DefaultDB,
			"SELECT name FROM employees WHERE age = ? AND firstName = 'Eric'",
			"SELECT name FROM employees WHERE age = ? AND firstName = ?",
		},
		{ // back quotes for table/column names
			EnabledAuto,
			MySQL,
			"select `a` from `b`.`c` where `d` = `e` limit 10,100",
			"select `a` from `b`.`c` where `d` = `e` limit ?,?",
		},
		{ // double quotes for both column names and single quote for values
			EnabledAuto,
			PostgreSQL,
			`select "a" from "b.c" where "d" = 'e' limit 10,100`,
			`select "a" from "b.c" where "d" = ? limit ?,?`,
		},
		{ // back/single/double quotes mixed
			EnabledDropDoubleQuoted,
			DefaultDB,
			"select `a` from `b`.`c` where `d` = 'e' and `f` = \"g\" limit 10,100",
			"select `a` from `b`.`c` where `d` = ? and `f` = ? limit ?,?",
		},
		{ // single/double quotes mixed
			EnabledAuto,
			PostgreSQL,
			`select "a" from "b.c" where "d" = 'e' and "f" = 5 limit 10,100`,
			`select "a" from "b.c" where "d" = ? and "f" = ? limit ?,?`,
		},
		{ // back quotes for column names and values
			EnabledAuto,
			DefaultDB,
			"select `a2a` from `b2b`.`c2c` where `d2d` = `e2e` limit 10,100",
			"select `a2a` from `b2b`.`c2c` where `d2d` = `e2e` limit ?,?",
		},
		{ // double quotes for column names and values
			EnabledAuto,
			SQLServer,
			`select [col2] from [tbl] where [tbl].[col1] = 'val'`,
			`select [col2] from [tbl] where [tbl].[col1] = ?`,
		},
		{ // table names with numbers should not be replaced with ?
			EnabledAuto,
			DefaultDB,
			`select table_123.col from table_123 where id = 123;`,
			`select table_123.col from table_123 where id = ?;`,
		},
		{ // Double-quoted strings with embedded single quotes
			EnabledAuto,
			MySQL,
			`select ssn from accounts where password = "\\'";`,
			`select ssn from accounts where password = ?;`,
		},
		{ // Double-quoted strings with escaped embedded double quotes
			EnabledAuto,
			PostgreSQL,
			`select ssn from accounts where password = \"\\\"abc\\\"\";`,
			`select ssn from accounts where password = \"\\\"abc\\\"\";`,
		},
		{ // Double-quoted strings with twinned embedded double quotes
			EnabledAuto,
			MySQL,
			`select ssn from accounts where password = "\"\"abc\"\"";`,
			`select ssn from accounts where password = ?;`,
		},
		{ // Single-back-quoted identifiers (ie. MySQL style)
			EnabledAuto,
			MySQL,
			"select `ssn -1` from `account's` where password = 'mypass';",
			"select `ssn -1` from `account's` where password = ?;",
		},
		{ // Unicode
			EnabledAuto,
			MySQL,
			`select col from tbl where name = '\xE2\x98\x83 unicode'`,
			`select col from tbl where name = ?`,
		},
		{ // Binary string
			EnabledAuto,
			MySQL,
			`select col from tbl where name = X'FEFF' and age = 37`,
			`select col from tbl where name = ? and age = ?`,
		},
		{ // Russian word "slon" (elephant) in Cyrillic Unicode letters:
			// From: http://www.postgresql.org/docs/9.0/static/sql-syntax-lexical.html
			EnabledAuto,
			PostgreSQL,
			`select col from tbl where name = U&'\0441\043B\043E\043D'`,
			`select col from tbl where name = ?`,
		},
		{ // Time and date
			EnabledAuto,
			MySQL,
			`select col from tbl where birth = 2013-01-26 11:03:30`,
			`select col from tbl where birth = ?-?-? ?:?:?`,
		},
		{ // Type declaration parameters
			EnabledAuto,
			MySQL,
			`CREATE TABLE tbl (col1 VARCHAR(1000, 50), col2 CHAR(123, col3 DEC(16,2), col4 MONEY(10, 2))`,
			`CREATE TABLE tbl (col1 VARCHAR(?, ?), col2 CHAR(?, col3 DEC(?,?), col4 MONEY(?, ?))`,
		},
		{ // No spaces around operator
			EnabledAuto,
			MySQL,
			`UPDATE extent_descriptor SET sample_rate=0.02, sample_source=NULL 
			 WHERE layer='worker' AND host='etl1.tlys.us' AND app_id=39`,
			`UPDATE extent_descriptor SET sample_rate=?, sample_source=NULL 
			 WHERE layer=? AND host=? AND app_id=?`,
		},
		{ // No spaces around parentheses
			EnabledAuto,
			MySQL,
			`INSERT INTO d_trace_stats (trace_id, trace_size, event_max_size, 
			 benchmark_queue, benchmark_since_added)VALUES(130580, 130536, 1314, NULL, NULL)`,
			`INSERT INTO d_trace_stats (trace_id, trace_size, event_max_size, 
			 benchmark_queue, benchmark_since_added)VALUES(?, ?, ?, NULL, NULL)`,
		},
		{ // invalid SQL statement #1
			EnabledAuto,
			DefaultDB,
			"",
			"",
		},
		{ // invalid SQL statement #2
			EnabledAuto,
			DefaultDB,
			"SELECT",
			"SELECT",
		},
	}

	for _, c := range cases {
		_ = os.Setenv("APPOPTICS_SQL_SANITIZE", strconv.Itoa(c.mode))
		assert.Nil(t, config.Load())
		ss := initSanitizersMap()

		assert.Equal(t, c.sanitizedSQL, sqlSanitize(ss, c.dbType, c.sql),
			fmt.Sprintf("Test case: %+v", c))
	}
}

func BenchmarkSQLSanitizeShort(b *testing.B) {
	sql := `WITH tmp AS (SELECT * FROM employees WHERE team = "IT") 
			 SELECT * FROM tmp WHERE name = 'Tom' 
			 LEFT JOIN tickets 
			 WHERE eid = id AND last_update BETWEEN '01/01/2019' AND '05/30/2019'`

	_ = os.Setenv("APPOPTICS_SQL_SANITIZE", "1")
	if err := config.Load(); err != nil {
		b.FailNow()
	}
	ss := initSanitizersMap()

	for n := 0; n < b.N; n++ {
		sqlSanitize(ss, DefaultDB, sql)
	}
	_ = os.Unsetenv("APPOPTICS_SQL_SANITIZE")
}

func BenchmarkSQLSanitizeLong(b *testing.B) {
	sql := "SELECT name FROM employees WHERE age = 37 AND firstName = 'Eric'"

	_ = os.Setenv("APPOPTICS_SQL_SANITIZE", "1")
	if err := config.Load(); err != nil {
		b.FailNow()
	}
	ss := initSanitizersMap()

	for n := 0; n < b.N; n++ {
		sqlSanitize(ss, DefaultDB, sql)
	}
	_ = os.Unsetenv("APPOPTICS_SQL_SANITIZE")
}
