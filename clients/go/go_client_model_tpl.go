package clients

import (
	"time"
)

type User struct {
	Id        string    `db:"id"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
	Email     string    `db:"email"`
	Name      string    `db:"name"`
	Age       int32     `db:"age"`
	Health    int       `db:"health"`
	Active    bool      `db:"active"`
}
