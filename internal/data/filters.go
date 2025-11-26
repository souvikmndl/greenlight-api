package data

import "github.com/souvikmndl/greenlight-api/internal/validator"

// Filters struct contains params for filtering and sorting results
type Filters struct {
	Page         int
	PageSize     int
	Sort         string
	SortSafelist []string
}

// ValidateFilters checks whether filter values are set correctly
func ValidateFilters(v *validator.Validator, f Filters) {
	v.Check(f.Page > 0, "page", "must be greater than zero")
	v.Check(f.Page <= 10_000_000, "page", "must be a maximum of 10 million")
	v.Check(f.PageSize > 0, "page_size", "must be greater than zero")
	v.Check(f.PageSize <= 100, "page_size", "must be a maximum of 100")

	v.Check(validator.PermittedValue(f.Sort, f.SortSafelist...), "sort", "invalid sort value")
}
