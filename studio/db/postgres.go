package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Postgres is a live, introspection-backed Store.
type Postgres struct {
	pool *pgxpool.Pool
	name string
}

// Connect opens a pool against url and verifies it is reachable.
func Connect(ctx context.Context, url string) (*Postgres, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	pingCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, err
	}
	return &Postgres{pool: pool, name: cfg.ConnConfig.Database}, nil
}

func (p *Postgres) Name() string        { return p.name }
func (p *Postgres) Live() bool          { return true }
func (p *Postgres) Close()              { p.pool.Close() }
func (p *Postgres) Pool() *pgxpool.Pool { return p.pool }

const introspectTablesSQL = `
SELECT n.nspname AS schema, c.relname AS name,
       COALESCE(c.reltuples::bigint, 0) AS rows
FROM pg_class c
JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE c.relkind IN ('r', 'p')
  AND n.nspname NOT IN ('pg_catalog', 'information_schema')
ORDER BY n.nspname, c.relname;`

func (p *Postgres) Tables(ctx context.Context) ([]Table, error) {
	rows, err := p.pool.Query(ctx, introspectTablesSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []Table
	for rows.Next() {
		var t Table
		if err := rows.Scan(&t.Schema, &t.Name, &t.Rows); err != nil {
			return nil, err
		}
		tables = append(tables, t)
	}
	return tables, rows.Err()
}

const introspectColumnsSQL = `
SELECT
  a.attname AS name,
  format_type(a.atttypid, a.atttypmod) AS data_type,
  NOT a.attnotnull AS nullable,
  COALESCE(pg_get_expr(ad.adbin, ad.adrelid), '') AS default_expr,
  COALESCE(pk.is_pk, false) AS is_pk,
  fk.ref AS fk_ref
FROM pg_attribute a
JOIN pg_class c ON c.oid = a.attrelid
JOIN pg_namespace n ON n.oid = c.relnamespace
LEFT JOIN pg_attrdef ad ON ad.adrelid = a.attrelid AND ad.adnum = a.attnum
LEFT JOIN (
  SELECT conrelid, unnest(conkey) AS attnum, true AS is_pk
  FROM pg_constraint WHERE contype = 'p'
) pk ON pk.conrelid = a.attrelid AND pk.attnum = a.attnum
LEFT JOIN (
  SELECT con.conrelid, con.conkey[1] AS attnum,
         fn.nspname || '.' || fc.relname || '.' || fa.attname AS ref
  FROM pg_constraint con
  JOIN pg_class fc ON fc.oid = con.confrelid
  JOIN pg_namespace fn ON fn.oid = fc.relnamespace
  JOIN pg_attribute fa ON fa.attrelid = con.confrelid AND fa.attnum = con.confkey[1]
  WHERE con.contype = 'f'
) fk ON fk.conrelid = a.attrelid AND fk.attnum = a.attnum
WHERE n.nspname = $1 AND c.relname = $2
  AND a.attnum > 0 AND NOT a.attisdropped
ORDER BY a.attnum;`

func (p *Postgres) Table(ctx context.Context, schema, name string) (Table, error) {
	t := Table{Schema: schema, Name: name}
	rows, err := p.pool.Query(ctx, introspectColumnsSQL, schema, name)
	if err != nil {
		return t, err
	}
	defer rows.Close()

	for rows.Next() {
		var c Column
		var fkRef *string
		if err := rows.Scan(&c.Name, &c.DataType, &c.Nullable, &c.Default, &c.IsPrimaryKey, &fkRef); err != nil {
			return t, err
		}
		if fkRef != nil {
			c.IsForeignKey = true
			c.ForeignRef = *fkRef
		}
		t.Columns = append(t.Columns, c)
	}
	if err := rows.Err(); err != nil {
		return t, err
	}
	if len(t.Columns) == 0 {
		return t, fmt.Errorf("table %s.%s not found", schema, name)
	}
	return t, nil
}

func (p *Postgres) Read(ctx context.Context, schema, name string, limit, offset int, order *Order) (Rows, error) {
	t, err := p.Table(ctx, schema, name)
	if err != nil {
		return Rows{}, err
	}
	ref := fmt.Sprintf("%s.%s", pgIdent(schema), pgIdent(name))

	var total int64
	if err := p.pool.QueryRow(ctx, "SELECT count(*) FROM "+ref).Scan(&total); err != nil {
		return Rows{}, err
	}

	orderClause := ""
	if order != nil && order.Column != "" {
		dir := "ASC"
		if order.Desc {
			dir = "DESC"
		}
		orderClause = " ORDER BY " + pgIdent(order.Column) + " " + dir
	} else if pks := t.PrimaryKeys(); len(pks) > 0 {
		orderClause = " ORDER BY " + pgIdent(pks[0]) + " ASC"
	}

	q := fmt.Sprintf("SELECT * FROM %s%s LIMIT $1 OFFSET $2", ref, orderClause)
	rows, err := p.pool.Query(ctx, q, limit, offset)
	if err != nil {
		return Rows{}, err
	}
	defer rows.Close()

	values, err := collect(rows)
	if err != nil {
		return Rows{}, err
	}
	return Rows{Columns: t.Columns, Values: values, Total: total, Offset: offset, Limit: limit}, nil
}

func (p *Postgres) Query(ctx context.Context, sql string) QueryResult {
	start := time.Now()
	rows, err := p.pool.Query(ctx, sql)
	if err != nil {
		return QueryResult{Err: err.Error(), Elapsed: time.Since(start).String()}
	}
	defer rows.Close()

	var cols []string
	for _, fd := range rows.FieldDescriptions() {
		cols = append(cols, fd.Name)
	}
	values, err := collect(rows)
	if err != nil {
		return QueryResult{Err: err.Error(), Elapsed: time.Since(start).String()}
	}
	tag := rows.CommandTag()
	return QueryResult{
		Columns: cols,
		Rows:    values,
		Command: tag.String(),
		Elapsed: time.Since(start).Round(time.Microsecond).String(),
	}
}

func collect(rows pgx.Rows) ([][]any, error) {
	var out [][]any
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		out = append(out, vals)
	}
	return out, rows.Err()
}

// pgIdent quotes an SQL identifier, escaping embedded quotes.
func pgIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}
