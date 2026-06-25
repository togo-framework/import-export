// Package importexport adds CSV & Excel (XLSX) data import/export to togo —
// the togo answer to Laravel Excel / django-import-export.
//
// Export any rows ([]map[string]any) or a registered Resource to CSV/XLSX, and
// import from CSV/XLSX with column mapping, header detection, and per-row
// validation that reports row/column errors instead of failing the whole file.
//
//	ie, _ := importexport.FromKernel(k)
//	csv, _ := ie.Export(ctx, rows, importexport.ExportOpts{Format: "csv", Columns: []string{"id","name"}})
//	out, errs, _ := ie.Import(ctx, data, importexport.ImportOpts{Format: "xlsx"})
package importexport

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"sync"

	"github.com/go-chi/chi/v5"
	"github.com/togo-framework/togo"
	"github.com/xuri/excelize/v2"
)

// Resource is a named, importable/exportable dataset.
type Resource struct {
	Name    string
	Columns []string
	Fetch   func(ctx context.Context) ([]map[string]any, error)        // for export
	Insert  func(ctx context.Context, row map[string]any) error        // for import
}

// ExportOpts controls an export.
type ExportOpts struct {
	Format  string   // "csv" (default) or "xlsx"
	Columns []string // column order; inferred (sorted) from the rows when empty
	Sheet   string   // XLSX sheet name (default "Sheet1")
}

// ImportOpts controls an import.
type ImportOpts struct {
	Format   string            // "csv" (default) or "xlsx"
	Sheet    string            // XLSX sheet to read (default first)
	Mapping  map[string]string // source header -> target field; identity when empty
	Validate func(row map[string]any) error
}

// RowError reports a problem with one imported row.
type RowError struct {
	Row     int    `json:"row"`              // 1-based data row (excludes header)
	Column  string `json:"column,omitempty"`
	Message string `json:"message"`
}

func (e RowError) Error() string { return fmt.Sprintf("row %d: %s", e.Row, e.Message) }

var (
	regMu     sync.RWMutex
	resources = map[string]Resource{}
)

// Register registers a named resource (call from a plugin init()/Boot).
func Register(r Resource) {
	regMu.Lock()
	resources[r.Name] = r
	regMu.Unlock()
}

// Service is the import/export runtime stored on the kernel.
type Service struct{ k *togo.Kernel }

func init() {
	togo.RegisterProviderFunc("import-export", togo.PriorityService, func(k *togo.Kernel) error {
		s := &Service{k: k}
		k.Set("import-export", s)
		if k.Router != nil {
			s.mountRoutes(k.Router)
		}
		return nil
	})
}

// FromKernel returns the import-export Service.
func FromKernel(k *togo.Kernel) (*Service, bool) {
	v, ok := k.Get("import-export")
	if !ok {
		return nil, false
	}
	s, ok := v.(*Service)
	return s, ok
}

// Export renders rows to CSV or XLSX bytes.
func (s *Service) Export(ctx context.Context, rows []map[string]any, opts ExportOpts) ([]byte, error) {
	cols := opts.Columns
	if len(cols) == 0 {
		cols = inferColumns(rows)
	}
	switch opts.Format {
	case "xlsx":
		return exportXLSX(rows, cols, opts.Sheet)
	default:
		return exportCSV(rows, cols)
	}
}

// ExportResource fetches a registered resource and exports it.
func (s *Service) ExportResource(ctx context.Context, name string, opts ExportOpts) ([]byte, error) {
	regMu.RLock()
	r, ok := resources[name]
	regMu.RUnlock()
	if !ok || r.Fetch == nil {
		return nil, fmt.Errorf("import-export: resource %q not registered (or has no Fetch)", name)
	}
	rows, err := r.Fetch(ctx)
	if err != nil {
		return nil, err
	}
	if len(opts.Columns) == 0 {
		opts.Columns = r.Columns
	}
	return s.Export(ctx, rows, opts)
}

// Import parses CSV/XLSX into rows, applying mapping + validation. It returns
// the valid rows, a list of per-row errors, and a fatal parse error (if any).
func (s *Service) Import(ctx context.Context, data []byte, opts ImportOpts) ([]map[string]any, []RowError, error) {
	var header []string
	var records [][]string
	var err error
	switch opts.Format {
	case "xlsx":
		header, records, err = readXLSX(data, opts.Sheet)
	default:
		header, records, err = readCSV(data)
	}
	if err != nil {
		return nil, nil, err
	}

	var out []map[string]any
	var errs []RowError
	for i, rec := range records {
		row := map[string]any{}
		for c, col := range header {
			field := col
			if opts.Mapping != nil {
				if m, ok := opts.Mapping[col]; ok {
					field = m
				}
			}
			val := ""
			if c < len(rec) {
				val = rec[c]
			}
			row[field] = val
		}
		if opts.Validate != nil {
			if vErr := opts.Validate(row); vErr != nil {
				errs = append(errs, RowError{Row: i + 1, Message: vErr.Error()})
				continue
			}
		}
		out = append(out, row)
	}
	return out, errs, nil
}

// ImportResource imports + inserts rows into a registered resource. Rows that
// fail validation or insert are reported; the rest are inserted.
func (s *Service) ImportResource(ctx context.Context, name string, data []byte, opts ImportOpts) (int, []RowError, error) {
	regMu.RLock()
	r, ok := resources[name]
	regMu.RUnlock()
	if !ok || r.Insert == nil {
		return 0, nil, fmt.Errorf("import-export: resource %q not registered (or has no Insert)", name)
	}
	rows, errs, err := s.Import(ctx, data, opts)
	if err != nil {
		return 0, nil, err
	}
	inserted := 0
	for i, row := range rows {
		if iErr := r.Insert(ctx, row); iErr != nil {
			errs = append(errs, RowError{Row: i + 1, Message: iErr.Error()})
			continue
		}
		inserted++
	}
	return inserted, errs, nil
}

// --- encoders ---

func inferColumns(rows []map[string]any) []string {
	seen := map[string]bool{}
	var cols []string
	for _, r := range rows {
		for k := range r {
			if !seen[k] {
				seen[k] = true
				cols = append(cols, k)
			}
		}
	}
	sort.Strings(cols)
	return cols
}

func cell(v any) string {
	if v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return t
	default:
		return fmt.Sprintf("%v", t)
	}
}

func exportCSV(rows []map[string]any, cols []string) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(cols); err != nil {
		return nil, err
	}
	for _, r := range rows {
		rec := make([]string, len(cols))
		for i, c := range cols {
			rec[i] = cell(r[c])
		}
		if err := w.Write(rec); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

func exportXLSX(rows []map[string]any, cols []string, sheet string) ([]byte, error) {
	if sheet == "" {
		sheet = "Sheet1"
	}
	f := excelize.NewFile()
	defer f.Close()
	idx, err := f.NewSheet(sheet)
	if err != nil {
		return nil, err
	}
	f.SetActiveSheet(idx)
	if sheet != "Sheet1" {
		_ = f.DeleteSheet("Sheet1")
	}
	header := make([]any, len(cols))
	for i, c := range cols {
		header[i] = c
	}
	if err := f.SetSheetRow(sheet, "A1", &header); err != nil {
		return nil, err
	}
	for ri, r := range rows {
		rec := make([]any, len(cols))
		for i, c := range cols {
			rec[i] = cell(r[c])
		}
		if err := f.SetSheetRow(sheet, fmt.Sprintf("A%d", ri+2), &rec); err != nil {
			return nil, err
		}
	}
	buf, err := f.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// --- decoders ---

func readCSV(data []byte) (header []string, records [][]string, err error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	rows, err := r.ReadAll()
	if err != nil {
		return nil, nil, err
	}
	if len(rows) == 0 {
		return nil, nil, nil
	}
	return rows[0], rows[1:], nil
}

func readXLSX(data []byte, sheet string) (header []string, records [][]string, err error) {
	f, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	if sheet == "" {
		sheet = f.GetSheetName(0)
	}
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, nil, err
	}
	if len(rows) == 0 {
		return nil, nil, nil
	}
	return rows[0], rows[1:], nil
}

// --- REST ---

func (s *Service) mountRoutes(r chi.Router) {
	r.Get("/api/export/{resource}", func(w http.ResponseWriter, req *http.Request) {
		name := chi.URLParam(req, "resource")
		format := req.URL.Query().Get("format")
		if format == "" {
			format = "csv"
		}
		out, err := s.ExportResource(req.Context(), name, ExportOpts{Format: format})
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		ct, ext := "text/csv", "csv"
		if format == "xlsx" {
			ct, ext = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "xlsx"
		}
		w.Header().Set("Content-Type", ct)
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s.%s", name, ext))
		_, _ = w.Write(out)
	})
	r.Post("/api/import/{resource}", func(w http.ResponseWriter, req *http.Request) {
		name := chi.URLParam(req, "resource")
		format := req.URL.Query().Get("format")
		if format == "" {
			format = "csv"
		}
		body, _ := readAll(req.Body)
		inserted, errs, err := s.ImportResource(req.Context(), name, body, ImportOpts{Format: format})
		if err != nil {
			writeJSON(w, 400, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, 200, map[string]any{"inserted": inserted, "errors": errs})
	})
}

func readAll(r interface{ Read([]byte) (int, error) }) ([]byte, error) {
	var buf bytes.Buffer
	tmp := make([]byte, 32*1024)
	for {
		n, err := r.Read(tmp)
		if n > 0 {
			buf.Write(tmp[:n])
		}
		if err != nil {
			if err.Error() == "EOF" {
				return buf.Bytes(), nil
			}
			return buf.Bytes(), err
		}
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
