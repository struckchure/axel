package clients

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

const (
	UserTableName      string = `"user"`
	UserFieldId        string = `"id"`
	UserFieldCreatedAt string = `"created_at"`
	UserFieldUpdatedAt string = `"updated_at"`
	UserFieldEmail     string = `"email"`
	UserFieldName      string = `"name"`
	UserFieldAge       string = `"age"`
	UserFieldHealth    string = `"health"`
	UserFieldActive    string = `"active"`
)

type Query struct {
	db  *sqlx.DB
	sql []string
}

type Filter struct {
	Column string
	Value  string
}

func (q *Query) Where(filters ...*Filter) *WhereQuery {
	filters = lo.Filter(filters, func(f *Filter, idx int) bool { return f != nil })

	wq := &WhereQuery{
		db: q.db,
		filters: lo.Map(filters, func(f *Filter, idx int) string {
			return fmt.Sprintf(`%s = $%d`, f.Column, idx+1)
		}),
		args: lo.Map(filters, func(f *Filter, idx int) any {
			return f.Value
		}),
	}

	return wq
}

type WhereQuery struct {
	db      *sqlx.DB
	filters []string
	args    []any
}

func (q *WhereQuery) All(ctx context.Context) ([]*User, error) {
	sql := strings.Builder{}

	sql.WriteString(fmt.Sprintf("SELECT * FROM %s", UserTableName))
	if len(q.filters) > 0 {
		sql.WriteString(fmt.Sprintf(" WHERE %s", strings.Join(q.filters, " AND ")))
	}

	users := []*User{}
	err := q.db.Select(&users, sql.String(), q.args...)
	if err != nil {
		return nil, err
	}

	return users, nil
}

func (q *WhereQuery) First(ctx context.Context) (*User, error) {
	sql := strings.Builder{}

	sql.WriteString(fmt.Sprintf("SELECT * FROM %s", UserTableName))
	if len(q.filters) > 0 {
		sql.WriteString(fmt.Sprintf(" WHERE %s", strings.Join(q.filters, " AND ")))
	}

	user := User{}
	err := q.db.Get(&user, sql.String(), q.args...)
	if err != nil {
		return nil, err
	}

	return &user, nil
}

type Mutation struct {
	db  *sqlx.DB
	sql []string
}

type User struct {
	Id        string    `db:"id"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
	Email     string    `db:"email"`
	Name      string    `db:"name"`
	Age       int32     `db:"age"`
	Health    int       `db:"health"`
	Active    bool      `db:"active"`

	query    *Query
	mutation *Mutation
}

func (u *User) Query(db *sqlx.DB) *Query {
	u.query = &Query{
		db:  db,
		sql: []string{},
	}

	return u.query
}

func (u *User) Mutation(db *sqlx.DB) *Mutation {
	u.mutation = &Mutation{
		db:  db,
		sql: []string{},
	}

	return u.mutation
}

func (u *User) IdEq(value string) *Filter {
	return &Filter{Column: UserFieldId, Value: value}
}

func (u *User) EmailEq(value string) *Filter {
	return &Filter{Column: UserFieldEmail, Value: value}
}

func (u *User) EmailEqNillable(value *string) *Filter {
	if value == nil {
		return nil
	}

	return &Filter{Column: UserFieldEmail, Value: *value}
}

func (u *User) NameEq(value string) *Filter {
	return &Filter{Column: UserFieldName, Value: value}
}

func (u *User) NameEqNillable(value *string) *Filter {
	if value == nil {
		return nil
	}

	return &Filter{Column: UserFieldName, Value: *value}
}
