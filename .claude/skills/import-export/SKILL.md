---
name: import-export
description: Export/import CSV & Excel data in a togo app — render rows/resources to CSV or XLSX, and import with column mapping, header detection and per-row validation that reports row/column errors.
---

# togo import-export

## Export
```go
ie, _ := importexport.FromKernel(k)
b, _ := ie.Export(ctx, rows, importexport.ExportOpts{Format:"xlsx", Columns:[]string{"id","name"}})
```

## Import (mapping + validation)
```go
rows, rowErrs, err := ie.Import(ctx, data, importexport.ImportOpts{
  Format:"csv", Mapping:map[string]string{"Full Name":"name"},
  Validate: func(r map[string]any) error { /* ... */ return nil },
})
```
Invalid rows → `[]RowError{Row,Column,Message}` (the rest still import).

## Registered resources + REST
`importexport.Register(Resource{Name, Columns, Fetch, Insert})` enables:
- `GET /api/export/{resource}?format=csv|xlsx`
- `POST /api/import/{resource}?format=csv|xlsx` → `{inserted, errors}`

## Notes
- CSV = stdlib; XLSX = excelize. First row is the header on import.
- Keep `Validate` pure + fast; report don't panic. Map source headers to your field names.
