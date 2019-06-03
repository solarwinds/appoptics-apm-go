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
			 WHERE eid = i...`,
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
