package data

import (
	"database/sql"
	"errors"
)

var (
	// ErrRecordNotFound represent db err when entry is not found
	ErrRecordNotFound = errors.New("record not found")
	// ErrEditConflict represents data race error, look this up in notes
	ErrEditConflict = errors.New("edit conflict")
)

// Models wraps all individual models
type Models struct {
	Movies MovieModel
	Users  UserModel
	Tokens TokenModel
}

// NewModels creates a new instances of models inside Models
func NewModels(db *sql.DB) Models {
	return Models{
		Movies: MovieModel{DB: db},
		Users:  UserModel{DB: db},
		Tokens: TokenModel{DB: db},
	}
}
