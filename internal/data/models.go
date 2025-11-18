package data

import (
	"database/sql"
	"errors"
)

var (
	// ErrRecordNotFound represent db err when entry is not found
	ErrRecordNotFound = errors.New("record not found")
)

// Models wraps all individual models
type Models struct {
	Movies MovieModel
}

// NewModels creates a new instances of models inside Models
func NewModels(db *sql.DB) Models {
	return Models{
		Movies: MovieModel{DB: db},
	}
}
