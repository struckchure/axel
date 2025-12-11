package ax

import (
	uuid "github.com/google/uuid"
	"time"
)

type User struct {
	Id        uuid.UUID `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
	Email     string    `db:"email" json:"email"`
	Name      *string   `db:"name" json:"name"`
	Age       int32     `db:"age" json:"age"`
	Health    int32     `db:"health" json:"health"`
	Active    *bool     `db:"active" json:"active"`
}
