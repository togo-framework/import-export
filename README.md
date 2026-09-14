<div align="center">
  <picture><source media="(prefers-color-scheme: dark)" srcset=".github/assets/togo-mark-dark.svg" /><img src=".github/assets/togo-mark.svg" alt="ToGO" height="64" /></picture>
  <h1>togo-framework/import-export</h1>
  <p>
    <a href="https://to-go.dev/marketplace"><img src="https://img.shields.io/badge/marketplace-to--go.dev-1F8A99" alt="marketplace" /></a>
    <a href="https://pkg.go.dev/github.com/togo-framework/import-export"><img src="https://pkg.go.dev/badge/github.com/togo-framework/import-export.svg" alt="pkg.go.dev" /></a>
    <img src="https://img.shields.io/badge/license-MIT-blue" alt="MIT" />
  </p>
  <p><strong>CSV & Excel data import/export for <a href="https://to-go.dev">togo</a> — mapping, validation, resources.</strong></p>
</div>

## Install

```bash
togo install togo-framework/import-export
```

The togo answer to **Laravel Excel** / **django-import-export**: export any rows or a registered resource to **CSV/XLSX**, and import from CSV/XLSX with **column mapping**, **header detection**, and **per-row validation** that reports row/column errors instead of failing the whole file.

## Usage

### Export
```go
ie, _ := importexport.FromKernel(k)

rows := []map[string]any{{"id": 1, "name": "Alice"}, {"id": 2, "name": "Bob"}}
csvBytes,  _ := ie.Export(ctx, rows, importexport.ExportOpts{Format: "csv",  Columns: []string{"id", "name"}})
xlsxBytes, _ := ie.Export(ctx, rows, importexport.ExportOpts{Format: "xlsx", Sheet: "People"})
```

### Import (mapping + validation)
```go
data := []byte("Identifier,Full Name\n1,Alice\n,Missing\n")
rows, rowErrors, err := ie.Import(ctx, data, importexport.ImportOpts{
    Format:  "csv",
    Mapping: map[string]string{"Identifier": "id", "Full Name": "name"},
    Validate: func(row map[string]any) error {
        if row["id"] == "" { return errors.New("id is required") }
        return nil
    },
})
// rows = valid rows (mapped to id/name); rowErrors = []RowError{ {Row:2, Message:"id is required"} }
```

### Registered resources
```go
importexport.Register(importexport.Resource{
    Name:    "people",
    Columns: []string{"id", "name"},
    Fetch:   func(ctx context.Context) ([]map[string]any, error) { return loadPeople(ctx) },
    Insert:  func(ctx context.Context, row map[string]any) error { return savePerson(ctx, row) },
})
// then export/import by name via the Go API or REST.
```

## REST API

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/export/{resource}?format=csv\|xlsx` | Download a registered resource |
| `POST` | `/api/import/{resource}?format=csv\|xlsx` | Upload + insert; returns `{inserted, errors}` |

## Configuration

No required env. XLSX uses [`excelize`](https://github.com/qax-os/excelize); CSV uses the stdlib.

---

<div align="center">
  <h3>Premium sponsors</h3>
  <p>
    <a href="https://id8media.com"><strong>ID8 Media</strong></a> &nbsp;·&nbsp;
    <a href="https://one-studio.co"><strong>One Studio</strong></a>
  </p>
  <p><sub>Support togo — <a href="https://github.com/sponsors/fadymondy">become a sponsor</a>.</sub></p>
</div>
