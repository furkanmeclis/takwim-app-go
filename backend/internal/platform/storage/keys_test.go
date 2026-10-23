package storage_test

import (
	"testing"

	"github.com/furkanmeclis/nextjs-go-boilerplate/backend/internal/platform/storage"
)

func TestOrganizationKeys(t *testing.T) {
	t.Parallel()
	const org = "11111111-1111-1111-1111-111111111111"
	const other = "22222222-2222-2222-2222-222222222222"
	if got := storage.OrganizationExportObjectKey(org, "job", ".csv"); got != "organizations/"+org+"/exports/job/export.csv" {
		t.Fatalf("export key = %s", got)
	}
	if got := storage.OrganizationImportSourceObjectKey(org, "job", "xlsx"); got != "organizations/"+org+"/imports/job/source.xlsx" {
		t.Fatalf("import key = %s", got)
	}
	cases := []struct {
		key, org string
		want     bool
	}{
		{storage.OrganizationExportObjectKey(org, "job", "csv"), org, true},
		{storage.OrganizationImportSourceObjectKey(org, "job", "xlsx"), org, true},
		{storage.OrganizationExportObjectKey(other, "job", "csv"), org, false},
		{"organizations/" + org + "/../" + other + "/x.pdf", org, false},
		{"organizations/" + org + "x/doc.pdf", org, false},
		{storage.ExportObjectKey("job", "csv"), org, false},
		{"organizations//x", "", false},
	}
	for _, c := range cases {
		if got := storage.OrganizationOwnsKey(c.key, c.org); got != c.want {
			t.Errorf("OrganizationOwnsKey(%q, %q) = %v, want %v", c.key, c.org, got, c.want)
		}
	}
}
