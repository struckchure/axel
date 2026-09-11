package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/struckchure/axel/core/codegen"
	"github.com/struckchure/axel/generators/typescript"
)

// Execute generated clients: a join option that is only exposed in types, or
// lost on recursive sub-selects, would otherwise silently benchmark the default.
func TestTsRuntimeRelationStrategies(t *testing.T) {
	bun, err := exec.LookPath("bun")
	if err != nil {
		t.Skip("bun is required to execute generated TypeScript")
	}
	ir := parseSchema(t, `
type Account { required id: int32; multi link posts: Post; }
type Post { required id: int32; required link author: Account; }
type Comment { required id: int32; required link post: Post; }
`)
	for _, client := range []string{"bun", "pg"} {
		t.Run(client, func(t *testing.T) {
			dir := t.TempDir()
			ctx := &codegen.Context{OutDir: dir, Options: map[string]string{"client": client}}
			if err := codegen.Walk(codegen.FromSchemaIR(ir), nil, &typescript.TsGenerator{}, ctx); err != nil {
				t.Fatal(err)
			}
			script := `
import assert from "node:assert/strict";
import {Runner} from "./runner";
let captured;
const db = {
  query: async (sql, params) => { captured = {sql, params}; return {rows: []}; },
  unsafe: async (sql, params) => { captured = {sql, params}; return []; },
};
assert.throws(() => new Runner(db, {relLoadStrategy: "invalid"}), /invalid relLoadStrategy/);
for (const strategy of [undefined, "query", "join"]) {
  const r = new Runner(db, {relLoadStrategy: strategy});
  const check = (joins, params) => {
    assert.equal((captured.sql.match(/LEFT JOIN LATERAL/g) ?? []).length, strategy === "join" ? joins : 0);
    assert.match(captured.sql, /json_agg/);
    assert.deepEqual(captured.params, params);
  };
  await r.run("select Account { id, posts := (select Post { id, comments := (select Comment { id } filter .post = Post.id) } filter .author = Account.id and .id > $min) } filter .id = $id", {min: 3, id: 1});
  check(2, [3, 1]);
  assert.match(captured.sql, /p\."id" > \$1/);
  assert.match(captured.sql, /a\."id" = \$2/);
  await r.select("Account", {id: true, children: r.select("Post", {id: true, comments: r.select("Comment", {id: true}).where("post", "=", "\x60Post.id\x60")}).where("author", "=", "\x60Account.id\x60")}).where("id", "=", 1).all();
  check(2, [1]);
  await r.select("Account", {id: true, posts: true}).where("id", "=", 1).all();
  check(1, [1]);
  assert.match(captured.sql, /COALESCE\(json_agg/);
  await r.select("Account", {id: true}).where("id", "=", 1).all();
  assert.doesNotMatch(captured.sql, /LATERAL/);
}
`
			path := filepath.Join(dir, "check.ts")
			if err := os.WriteFile(path, []byte(script), 0600); err != nil {
				t.Fatal(err)
			}
			if out, err := exec.Command(bun, path).CombinedOutput(); err != nil {
				t.Fatalf("generated runtime: %v\n%s", err, out)
			}
		})
	}
}
