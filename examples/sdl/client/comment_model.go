package ax

import (
	uuid "github.com/google/uuid"
	"time"
)

type Comment struct {
	Id        uuid.UUID `db:"id" json:"id"`
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	UpdatedAt time.Time `db:"updated_at" json:"updated_at"`
	Post      Post      `db:"post" json:"post"`
	Content   string    `db:"content" json:"content"`
	Author    User      `db:"author" json:"author"`
}
