package database_test

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// The isolation guard rails of docs/design/tenant-core.md §8.
//
//   - queries/tenant_tables.txt lists organization-owned tables. Every sqlc
//     query that touches one must reference organization_id, and when the
//     table has an alias, alias.organization_id (so a join of a second
//     tenant table is constrained too).
//   - queries/mixed_scope_tables.txt lists tables that hold platform and
//     organization rows (jobs, activity, notifications, roles). A query on
//     one must mention one of the table's scope markers, or be listed in
//     queries/isolation_exempt.txt with the reason the caller checks the
//     scope itself (e.g. job lookups by uuid followed by
//     orgctx.SameEngineScope).
//   - Hand-written SQL in Go (pgx Exec/Query outside sqlc) on a tenant table
//     must reference organization_id too.

var sqlTableRef = `(?i)\b(?:from|join|update|into)\s+`

type mixedTable struct {
	name    string
	markers []string
}

type sqlQuery struct {
	file, name, body string
}

func TestTenantQueriesFilterByOrganization(t *testing.T) {
	t.Parallel()
	tables := readTenantTables(t, filepath.Join("queries", "tenant_tables.txt"))
	if len(tables) == 0 {
		t.Fatal("tenant_tables.txt lists no tables")
	}
	for _, q := range readQueries(t) {
		for _, table := range tables {
			for _, problem := range tenantTableProblems(q.body, table) {
				t.Errorf("%s: query %s %s", q.file, q.name, problem)
			}
		}
	}
}

func TestMixedScopeQueriesNameTheirScope(t *testing.T) {
	t.Parallel()
	mixed := readMixedTables(t, filepath.Join("queries", "mixed_scope_tables.txt"))
	if len(mixed) == 0 {
		t.Fatal("mixed_scope_tables.txt lists no tables")
	}
	exempt := readExempt(t, filepath.Join("queries", "isolation_exempt.txt"))
	used := map[string]bool{}
	for _, q := range readQueries(t) {
		for _, table := range mixed {
			re := cachedRE(sqlTableRef + regexp.QuoteMeta(table.name) + `\b`)
			if !re.MatchString(q.body) || mentionsAny(q.body, table.markers) {
				continue
			}
			if _, ok := exempt[q.name]; ok {
				used[q.name] = true
				continue
			}
			t.Errorf("%s: query %s touches %s without %s; filter by the scope or add it to isolation_exempt.txt with the reason",
				q.file, q.name, table.name, strings.Join(table.markers, " / "))
		}
	}
	stale := make([]string, 0)
	for name := range exempt {
		if !used[name] {
			stale = append(stale, name)
		}
	}
	sort.Strings(stale)
	for _, name := range stale {
		t.Errorf("isolation_exempt.txt: %s is not needed (unknown query, or it now names its scope); remove it", name)
	}
}

// TestHandWrittenSQLFiltersByOrganization scans the Go sources (generated
// sqlc code and tests excluded) for SQL string literals on tenant tables.
func TestHandWrittenSQLFiltersByOrganization(t *testing.T) {
	t.Parallel()
	tables := readTenantTables(t, filepath.Join("queries", "tenant_tables.txt"))
	root := filepath.Join("..", "..")
	skip := map[string]bool{
		filepath.Join(root, "internal", "database", "db"): true,
	}
	scanned := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (skip[path] || strings.HasPrefix(d.Name(), ".") || d.Name() == "testdata") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		scanned++
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			s, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			for _, table := range tables {
				for _, problem := range tenantTableProblems(stripComments(s), table) {
					t.Errorf("%s: SQL literal %s", fset.Position(lit.Pos()), problem)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned < 100 {
		t.Fatalf("scanned only %d Go files; is the backend root right?", scanned)
	}
}

func TestTenantTableProblems(t *testing.T) {
	t.Parallel()
	cases := []struct {
		sql  string
		want int
	}{
		{"SELECT * FROM vehicles WHERE organization_id = $1", 0},
		{"SELECT * FROM vehicles WHERE uuid = $1", 1},
		{"SELECT v.* FROM vehicles v WHERE v.organization_id = $1 AND v.uuid = $2", 0},
		// The joined tenant table is unconstrained.
		{"SELECT v.* FROM vehicles v JOIN branches b ON b.id = v.home_branch_id WHERE v.organization_id = $1", 1},
		{"SELECT v.* FROM vehicles AS v JOIN branches b ON b.organization_id = v.organization_id AND b.id = v.home_branch_id WHERE v.organization_id = $1", 0},
		{"INSERT INTO vehicles (organization_id, plate) VALUES ($1, $2)", 0},
		{"UPDATE vehicles SET plate = $2 WHERE id = $1", 1},
		{"SELECT 1 FROM vehicles_archive", 0},
		{"plain text mentioning vehicles from a branch", 0},
	}
	for _, c := range cases {
		got := len(tenantTableProblems(c.sql, "vehicles")) + len(tenantTableProblems(c.sql, "branches"))
		if got != c.want {
			t.Errorf("%q: %d problems, want %d", c.sql, got, c.want)
		}
	}
}

// aliasKeywords are words that can follow a table name but are not aliases.
var aliasKeywords = map[string]bool{
	"where": true, "on": true, "join": true, "left": true, "right": true, "inner": true, "outer": true,
	"cross": true, "full": true, "natural": true, "set": true, "values": true, "using": true,
	"group": true, "order": true, "limit": true, "offset": true, "returning": true, "select": true,
	"lateral": true, "for": true, "union": true, "except": true, "intersect": true, "having": true,
	"window": true, "default": true, "overriding": true, "with": true, "as": true, "only": true,
}

var bareOrgID = regexp.MustCompile(`(?:^|[^.\w])organization_id\b`)

// tenantTableProblems reports every reference to table in sql that is not
// constrained by organization_id (aliased references need
// alias.organization_id; unaliased ones a bare or table-qualified
// organization_id).
func tenantTableProblems(sql, table string) []string {
	if !strings.Contains(strings.ToLower(sql), table) {
		return nil
	}
	re := cachedRE(sqlTableRef + regexp.QuoteMeta(table) + `\b(?:\s*\(|\s+(?:as\s+)?(\w+))?`)
	var out []string
	for _, m := range re.FindAllStringSubmatch(sql, -1) {
		alias := m[1]
		if aliasKeywords[strings.ToLower(alias)] {
			alias = ""
		}
		var ok bool
		if alias != "" && alias != table {
			ok = cachedRE(`\b` + regexp.QuoteMeta(alias) + `\.organization_id\b`).MatchString(sql)
			if !ok && !cachedRE(`\b`+regexp.QuoteMeta(alias)+`\.`).MatchString(sql) {
				// The alias is never used qualified: the bare column belongs to it.
				ok = bareOrgID.MatchString(sql)
			}
		} else {
			ok = bareOrgID.MatchString(sql) ||
				cachedRE(`\b`+regexp.QuoteMeta(table)+`\.organization_id\b`).MatchString(sql)
		}
		if !ok {
			ref := table
			if alias != "" {
				ref += " " + alias
			}
			out = append(out, "touches tenant table "+ref+" without organization_id")
		}
	}
	return out
}

var reCache sync.Map

func cachedRE(expr string) *regexp.Regexp {
	if re, ok := reCache.Load(expr); ok {
		return re.(*regexp.Regexp)
	}
	re := regexp.MustCompile(expr)
	reCache.Store(expr, re)
	return re
}

func mentionsAny(body string, markers []string) bool {
	for _, m := range markers {
		if cachedRE(`\b` + regexp.QuoteMeta(m) + `\b`).MatchString(body) {
			return true
		}
	}
	return false
}

func readQueries(t *testing.T) []sqlQuery {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("queries", "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	nameRE := regexp.MustCompile(`(?m)^-- name: (\w+)`)
	var out []sqlQuery
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
			out = append(out, sqlQuery{
				file: filepath.Base(file), name: src[m[2]:m[3]], body: stripComments(src[m[1]:end]),
			})
		}
	}
	if len(out) == 0 {
		t.Fatal("no sqlc queries found")
	}
	return out
}

func readLines(t *testing.T, path string) []string {
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

func readTenantTables(t *testing.T, path string) []string {
	t.Helper()
	return readLines(t, path)
}

// readMixedTables parses "table marker [marker...]" lines.
func readMixedTables(t *testing.T, path string) []mixedTable {
	t.Helper()
	var out []mixedTable
	for _, line := range readLines(t, path) {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			t.Fatalf("%s: %q needs a table and at least one scope marker", path, line)
		}
		out = append(out, mixedTable{name: fields[0], markers: fields[1:]})
	}
	return out
}

// readExempt parses "QueryName reason..." lines; the reason is required.
func readExempt(t *testing.T, path string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, line := range readLines(t, path) {
		name, reason, _ := strings.Cut(line, " ")
		if strings.TrimSpace(reason) == "" {
			t.Fatalf("%s: %s needs a reason", path, name)
		}
		out[name] = strings.TrimSpace(reason)
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
