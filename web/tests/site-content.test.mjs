import assert from "node:assert/strict";
import { readdir, readFile } from "node:fs/promises";
import test from "node:test";

const web = new URL("../", import.meta.url);

async function sources(dir) {
  const files = [];
  for (const entry of await readdir(new URL(dir, web), {
    withFileTypes: true,
  })) {
    const path = `${dir}${entry.name}`;
    if (entry.isDirectory()) files.push(...(await sources(`${path}/`)));
    else if (/\.(jsx?|css|html)$/.test(entry.name)) files.push(path);
  }
  return files;
}

test("the site no longer describes the Docker app", async () => {
  const files = ["index.html", ...(await sources("src/"))];
  for (const file of files) {
    const text = await readFile(new URL(file, web), "utf8");
    assert.doesNotMatch(text, /docker|compose\.ya?ml/i, file);
    assert.doesNotMatch(text, /README\.md#docker/i, file);
  }
});
