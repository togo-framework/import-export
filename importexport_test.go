package importexport

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func rows() []map[string]any {
	return []map[string]any{
		{"id": "1", "name": "Alice"},
		{"id": "2", "name": "Bob"},
	}
}

func TestExportCSVParsesBack(t *testing.T) {
	s := &Service{}
	out, err := s.Export(context.Background(), rows(), ExportOpts{Format: "csv", Columns: []string{"id", "name"}})
	if err != nil {
		t.Fatal(err)
	}
	recs, err := csv.NewReader(bytes.NewReader(out)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 { // header + 2 rows
		t.Fatalf("got %d records", len(recs))
	}
	if recs[0][0] != "id" || recs[0][1] != "name" {
		t.Fatalf("header = %v", recs[0])
	}
	if recs[1][1] != "Alice" || recs[2][1] != "Bob" {
		t.Fatalf("data = %v %v", recs[1], recs[2])
	}
}

func TestExportXLSXReadsBack(t *testing.T) {
	s := &Service{}
	out, err := s.Export(context.Background(), rows(), ExportOpts{Format: "xlsx", Columns: []string{"id", "name"}})
	if err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenReader(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := f.GetRows(f.GetSheetName(0))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0][0] != "id" || got[1][1] != "Alice" || got[2][1] != "Bob" {
		t.Fatalf("xlsx rows = %v", got)
	}
}

func TestImportCSVWithMappingAndValidation(t *testing.T) {
	s := &Service{}
	data := []byte("Identifier,Full Name\n1,Alice\n,Missing\n3,Carol\n")
	mapping := map[string]string{"Identifier": "id", "Full Name": "name"}
	validate := func(row map[string]any) error {
		if row["id"] == "" {
			return errors.New("id is required")
		}
		return nil
	}
	got, errs, err := s.Import(context.Background(), data, ImportOpts{Format: "csv", Mapping: mapping, Validate: validate})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 valid rows, got %d", len(got))
	}
	if len(errs) != 1 || errs[0].Row != 2 {
		t.Fatalf("expected 1 error on row 2, got %+v", errs)
	}
	// Mapping applied: source headers became id/name.
	if got[0]["id"] != "1" || got[0]["name"] != "Alice" {
		t.Fatalf("mapping wrong: %+v", got[0])
	}
	if got[1]["id"] != "3" {
		t.Fatalf("row 3 = %+v", got[1])
	}
}

func TestRoundTripCSV(t *testing.T) {
	s := &Service{}
	out, err := s.Export(context.Background(), rows(), ExportOpts{Format: "csv", Columns: []string{"id", "name"}})
	if err != nil {
		t.Fatal(err)
	}
	got, errs, err := s.Import(context.Background(), out, ImportOpts{Format: "csv"})
	if err != nil || len(errs) != 0 {
		t.Fatalf("import err=%v errs=%v", err, errs)
	}
	if len(got) != 2 || got[0]["id"] != "1" || got[0]["name"] != "Alice" || got[1]["name"] != "Bob" {
		t.Fatalf("round-trip mismatch: %+v", got)
	}
}

func TestInferColumns(t *testing.T) {
	s := &Service{}
	out, err := s.Export(context.Background(), rows(), ExportOpts{Format: "csv"}) // no Columns → inferred sorted
	if err != nil {
		t.Fatal(err)
	}
	header := strings.SplitN(strings.TrimSpace(strings.Split(string(out), "\n")[0]), ",", 2)
	if header[0] != "id" || header[1] != "name" { // sorted
		t.Fatalf("inferred header = %v", header)
	}
}

func TestResourceRegistryExport(t *testing.T) {
	Register(Resource{
		Name:    "people",
		Columns: []string{"id", "name"},
		Fetch:   func(ctx context.Context) ([]map[string]any, error) { return rows(), nil },
	})
	s := &Service{}
	out, err := s.ExportResource(context.Background(), "people", ExportOpts{Format: "csv"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(out, []byte("Alice")) {
		t.Fatalf("resource export missing data: %s", out)
	}
	if _, err := s.ExportResource(context.Background(), "missing", ExportOpts{}); err == nil {
		t.Fatal("expected error for unregistered resource")
	}
}
