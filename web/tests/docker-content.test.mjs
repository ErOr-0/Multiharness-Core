import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";
import { execFileSync } from "node:child_process";
import { dockerCommands } from "../src/content.js";

test("published commands use the same reusable container as the guide", async () => {
  const guide = await readFile(
    new URL("../../docs/docker.md", import.meta.url),
    "utf8",
  );
  for (const command of Object.values(dockerCommands))
    assert.ok(guide.includes(command));
  const compose = await readFile(
    new URL("../../compose.yaml", import.meta.url),
    "utf8",
  );
  assert.match(compose, /container_name: multiharness/);
  assert.match(compose, /image: er0r2\/multiharness:latest/);
  assert.match(compose, /name: magent-state/);
  assert.match(compose, /create_host_path: false/);
  const component = await readFile(
    new URL("../src/components/GettingStarted.jsx", import.meta.url),
    "utf8",
  );
  assert.ok(
    !component.includes("magent-host") && !component.includes("./magent"),
  );
});

test("configuration bundle contains the exact maintained files and no executable launcher", () => {
  execFileSync("python3", ["scripts/package-docker.py"], {
    cwd: new URL("../..", import.meta.url),
  });
  execFileSync(
    "python3",
    [
      "-c",
      `from pathlib import Path
from zipfile import ZipFile
with ZipFile('dist/multiharness-docker.zip') as z:
 assert 'compose.yaml' in z.namelist()
 assert len(z.namelist()) == 11
 for name in z.namelist():
  assert z.read(name) == Path(name).read_bytes(), name
`,
    ],
    { cwd: new URL("../..", import.meta.url) },
  );
});
