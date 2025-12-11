package clients

import "github.com/jmoiron/sqlx"

type Mutation struct {
	db  *sqlx.DB
	sql []string
}
