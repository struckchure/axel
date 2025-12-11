package clients

import (
	"context"
	"fmt"
	"strings"

	"github.com/jmoiron/sqlx"
	"github.com/samber/lo"
)

func Q[T any](db *sqlx.DB) *Query[T] {
	return &Query[T]{db: db, sql: []string{}}
}

type Query[T any] struct {
	db  *sqlx.DB
	sql []string
}

func (q *Query[T]) Where(ops ...*UserOp) *WhereQuery[T] {
	ops = lo.Filter(ops, func(f *UserOp, idx int) bool { return f != nil })

	wq := &WhereQuery[T]{
		db: q.db,
		filters: lo.Map(ops, func(f *UserOp, idx int) string {
			if f.required {
				return fmt.Sprintf(`%s %s $%d`, f.column, f.operator, idx+1)
			}

			return fmt.Sprintf(`%s %s`, f.column, f.operator)
		}),
		args: lo.Map(
			lo.Filter(ops, func(f *UserOp, idx int) bool { return f.required }), // exclude filters that doesn't require args
			func(f *UserOp, idx int) any { return f.value },
		),
	}

	return wq
}

type WhereQuery[T any] struct {
	db      *sqlx.DB
	filters []string
	args    []any
}

func (q *WhereQuery[T]) All(ctx context.Context) ([]*T, error) {
	sql := strings.Builder{}

	sql.WriteString(fmt.Sprintf("SELECT * FROM %s", UserTableName))
	if len(q.filters) > 0 {
		sql.WriteString(fmt.Sprintf(" WHERE %s", strings.Join(q.filters, " AND ")))
	}

	var users []*T
	err := q.db.Select(&users, sql.String(), q.args...)
	if err != nil {
		return nil, err
	}

	return users, nil
}

func (q *WhereQuery[T]) First(ctx context.Context) (*T, error) {
	sql := strings.Builder{}

	sql.WriteString(fmt.Sprintf("SELECT * FROM %s", UserTableName))
	if len(q.filters) > 0 {
		sql.WriteString(fmt.Sprintf(" WHERE %s", strings.Join(q.filters, " AND ")))
	}

	fmt.Println(sql.String())

	var user T
	err := q.db.Get(&user, sql.String(), q.args...)
	if err != nil {
		return nil, err
	}

	return &user, nil
}
