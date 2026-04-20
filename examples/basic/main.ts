import { SQL } from "bun";

import { Runner } from "./gen/runner";

const db = new SQL({
  url: "postgres://user:password@localhost:5432/db?sslmode=disable",
});

const r = new Runner(db);

async function main() {
  const users = await r
    .select("User", {
      id: true,
      name: true,
      email: true,
      posts: r
        .select("Post", { title: true, content: true })
        .where("authorId", "=", "`User.id`"),
    })
    .where("email", "=", "user@mail.com")
    .or("email", "=", "user-2@mail.com")
    .all();

  for (const user of users) {
    console.log(JSON.stringify(user, null, 2));
  }
}

main();
