package data

import (
	"context"
	"database/sql"
	"time"

	"golang.org/x/exp/slices"
)

// Permissions will contains user permissions
type Permissions []string

// Include checks if a perms slice contains a code
func (p Permissions) Include(code string) bool {
	return slices.Contains(p, code)
}

// PermissionModel contains queries for user permissions
type PermissionModel struct {
	DB *sql.DB
}

// GetAllForuser returns all permission code for a specific user
func (m PermissionModel) GetAllForuser(userID int64) (Permissions, error) {
	query := `
        SELECT permissions.code
        FROM permissions
        INNER JOIN users_permissions ON users_permissions.permission_id == permissions.id
        INNER JOIN users ON user_permissions.user_id == users.id
        WHERE users.id = $1`

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	rows, err := m.DB.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var permissions Permissions

	for rows.Next() {
		var permission string

		err := rows.Scan(&permission)
		if err != nil {
			return nil, err
		}

		permissions = append(permissions, permission)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return permissions, nil
}
