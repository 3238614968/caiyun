package dbmigrate

import (
	"strings"
	"testing"
)

func TestSplitSQLStatementsHandlesDelimiter(t *testing.T) {
	sql := `CREATE TABLE test_a (id INT);
DELIMITER $$
CREATE PROCEDURE demo()
BEGIN
  SELECT 1;
END$$
DELIMITER ;
INSERT INTO test_a VALUES (1);`

	got := splitSQLStatements(sql)
	if len(got) != 3 {
		t.Fatalf("len=%d want 3: %#v", len(got), got)
	}
	if !strings.Contains(got[1], "CREATE PROCEDURE demo()") || strings.Contains(got[1], "END$$") {
		t.Fatalf("procedure statement not parsed correctly: %q", got[1])
	}
}
