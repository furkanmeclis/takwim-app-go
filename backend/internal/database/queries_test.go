package database_test

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestTenantQueriesFilterByOrganization is the isolation guard rail of
// docs/design/tenant-core.md §8: every sqlc query that touches a tenant table
// (listed in queries/tenant_tables.txt) must reference organization_id.
func TestTenantQueriesFilterByOrganization(t *testing.T) {
	t.Parallel()
	tables := readTenantTables(t, filepath.Join("queries", "tenant_tables.txt"))
	if len(tables) == 0 {
		t.Fatal("tenant_tables.txt lists no tables")
	}
	files, err := filepath.Glob(filepath.Join("queries", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	nameRE := regexp.MustCompile(`(?m)^-- name: (\w+)`)
	for _, file := range files {
		raw, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		src := string(raw)
		idx := nameRE.FindAllStringSubmatchIndex(src, -1)
		for i, m := range idx {
			end := len(src)
			if i+1 < len(idx) {
				end = idx[i+1][0]
			}
			name := src[m[2]:m[3]]
			body := stripComments(src[m[1]:end])
			for _, table := range tables {
				re := regexp.MustCompile(`(?i)\b(from|join|update|into)\s+` + regexp.QuoteMeta(table) + `\b`)
				if re.MatchString(body) && !strings.Contains(body, "organization_id") {
					t.Errorf("%s: query %s touches tenant table %s without organization_id", filepath.Base(file), name, table)
				}
			}
		}
	}
}

func readTenantTables(t *testing.T, path string) []string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func stripComments(sql string) string {
	var b strings.Builder
	for _, line := range strings.Split(sql, "\n") {
		if i := strings.Index(line, "--"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
