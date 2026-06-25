# import-export — usage

## Export
```go
ie, _ := importexport.FromKernel(k)
csv,  _ := ie.Export(ctx, rows, importexport.ExportOpts{Format:"csv", Columns:[]string{"id","name"}})
xlsx, _ := ie.Export(ctx, rows, importexport.ExportOpts{Format:"xlsx", Sheet:"People"})
```
`Columns` controls order; when empty they are inferred (sorted) from the rows.

## Import
```go
rows, rowErrors, err := ie.Import(ctx, data, importexport.ImportOpts{
    Format:   "csv",                                   // or "xlsx"
    Mapping:  map[string]string{"Full Name": "name"},  // source header -> field
    Validate: func(row map[string]any) error { /* per-row */ return nil },
})
```
The first line is the header. Invalid rows are collected into `[]RowError{Row, Column, Message}` (1-based, excludes header) and excluded from `rows` — the file still imports.

## Registered resources
`Register(Resource{Name, Columns, Fetch, Insert})` so a named dataset can be
exported (`ExportResource`) and imported+inserted (`ImportResource`), including
over REST:

| Method | Path |
|---|---|
| GET | `/api/export/{resource}?format=csv\|xlsx` |
| POST | `/api/import/{resource}?format=csv\|xlsx` |
