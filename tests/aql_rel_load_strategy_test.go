package tests

import (
	"strings"
	"testing"

	"github.com/struckchure/axel/core/aql"
	"github.com/struckchure/axel/core/compiler"
)

const relLoadSchema = `
type Profile {
  required id: uuid;
  required bio: str;
}

type User {
  required id: uuid;
  required email: str;
  profile: Profile;
  multi link posts: Post;
}

type Post {
  required id: uuid;
  required title: str;
  author: User;
}
`

func TestRelLoadStrategyQuery(t *testing.T) {
	ir := parseSchema(t, relLoadSchema)

	// Single and multi links using default query strategy
	q := `select User {
  id,
  profile: { id, bio },
  posts: { id, title }
} filter .id = $id<uuid>;`

	stmt, err := aql.ParseString(q)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	res, err := compiler.Compile(stmt, ir)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	// Should contain correlated subqueries in SELECT list
	if !strings.Contains(res.SQL, "row_to_json(p_profile_sub)") {
		t.Errorf("expected row_to_json in subquery:\n%s", res.SQL)
	}
	if !strings.Contains(res.SQL, "json_agg(row_to_json(p_posts_sub))") {
		t.Errorf("expected json_agg in subquery:\n%s", res.SQL)
	}
	if strings.Contains(res.SQL, "LEFT JOIN LATERAL") {
		t.Errorf("did not expect LEFT JOIN LATERAL in query strategy:\n%s", res.SQL)
	}
}

func TestRelLoadStrategyJoinDirective(t *testing.T) {
	ir := parseSchema(t, relLoadSchema)

	// Single and multi links using @rel_load_strategy join
	q := `@rel_load_strategy join

select User {
  id,
  profile: { id, bio },
  posts: { id, title }
} filter .id = $id<uuid>;`

	stmt, err := aql.ParseString(q)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	res, err := compiler.Compile(stmt, ir)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	// Should contain LEFT JOIN LATERAL clauses
	if !strings.Contains(res.SQL, "LEFT JOIN LATERAL") {
		t.Errorf("expected LEFT JOIN LATERAL in join strategy:\n%s", res.SQL)
	}
	if !strings.Contains(res.SQL, "p_profile_lat.profile AS profile") {
		t.Errorf("expected p_profile_lat.profile AS profile in SELECT list:\n%s", res.SQL)
	}
	if !strings.Contains(res.SQL, "COALESCE(p_posts_lat.posts, '[]') AS posts") {
		t.Errorf("expected COALESCE(p_posts_lat.posts, '[]') AS posts in SELECT list:\n%s", res.SQL)
	}
}

func TestRelLoadStrategyComputedSubquery(t *testing.T) {
	ir := parseSchema(t, relLoadSchema)

	q := `@rel_load_strategy join

select User {
  id,
  user_posts := (multi select Post filter .author = User.id)
} filter .id = $id<uuid>;`

	stmt, err := aql.ParseString(q)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	res, err := compiler.Compile(stmt, ir)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}

	if !strings.Contains(res.SQL, "LEFT JOIN LATERAL (SELECT COALESCE(json_agg") {
		t.Errorf("expected LEFT JOIN LATERAL for computed subquery:\n%s", res.SQL)
	}
}

func TestRelLoadStrategyInvalidDirective(t *testing.T) {
	ir := parseSchema(t, relLoadSchema)

	q := `@rel_load_strategy invalid_strategy
select User { id };`

	stmt, err := aql.ParseString(q)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	_, err = compiler.Compile(stmt, ir)
	if err == nil {
		t.Fatalf("expected error for invalid rel_load_strategy")
	}
	if !strings.Contains(err.Error(), "invalid rel_load_strategy") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRelLoadStrategyConfigAndOverride(t *testing.T) {
	ir := parseSchema(t, relLoadSchema)

	// Base query without directive compiled with RelLoadStrategy: "join"
	q1 := `select User { id, posts: { id, title } };`
	stmt1, err := aql.ParseString(q1)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	res1, err := compiler.CompileWithOptions(stmt1, ir, compiler.CompileOptions{RelLoadStrategy: "join"})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.Contains(res1.SQL, "LEFT JOIN LATERAL") {
		t.Errorf("expected LEFT JOIN LATERAL when configured globally as join:\n%s", res1.SQL)
	}

	// Query with explicit @rel_load_strategy query overrides global join config
	q2 := `@rel_load_strategy query
select User { id, posts: { id, title } };`
	stmt2, err := aql.ParseString(q2)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	res2, err := compiler.CompileWithOptions(stmt2, ir, compiler.CompileOptions{RelLoadStrategy: "join"})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if strings.Contains(res2.SQL, "LEFT JOIN LATERAL") {
		t.Errorf("directive @rel_load_strategy query should override global join:\n%s", res2.SQL)
	}
}
