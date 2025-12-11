package ax

import (
	uuid "github.com/google/uuid"
	"time"
)

type Post struct {
	Id        uuid.UUID `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
	Title     string    `db:"title" json:"title"`
	Content   string    `db:"content" json:"content"`
	Author    User      `db:"author" json:"author"`
	Likes     []*User   `db:"likes" json:"likes"`
}
