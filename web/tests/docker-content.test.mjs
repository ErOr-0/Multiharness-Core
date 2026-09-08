import assert from "node:assert/strict";
import { readFile, writeFile } from "node:fs/promises";
import test from "node:test";
import { configureCompose, configurationZip } from "../src/docker-setup.js";
import { dockerCommands } from "../src/content.js";
const template = await readFile(
  new URL("../public/compose.yaml", import.meta.url),
  "utf8",
);
test("download maps original paths without shell interpolation or extra privileges", async () => {
  const doc = JSON.parse(
    configureCompose(template, "D:/Projects/My $App", "Windows"),
  );
  const service = doc.services.magent;
  assert.equal(service.volumes[0].source, "D:/Projects/My $$App");
  assert.equal(service.volumes[0].target, "/workspace");
  assert.equal(service.volumes[0].bind.create_host_path, false);
  assert.deepEqual(service.cap_drop, ["ALL"]);
  assert.deepEqual(service.security_opt, [
    "no-new-privileges=true",
    "seccomp=./seccomp.json",
  ]);
  assert.throws(() => configureCompose(template, "relative", "Windows"));
  assert.throws(() => configureCompose(template, "D:/test", "Linux"));
  const linux = JSON.parse(
    configureCompose(template, "/home/me/projects", "Linux"),
  );
  assert.ok(
    linux.services.magent.security_opt.includes("apparmor=magent-container-v1"),
  );
  const source = await readFile(
    new URL("../../docker/seccomp.json", import.meta.url),
    "utf8",
  );
  assert.deepEqual(
    JSON.parse(
      await readFile(
        new URL("../public/seccomp.json", import.meta.url),
        "utf8",
      ),
    ),
    JSON.parse(source),
  );
  const guide = await readFile(
    new URL("../../docs/docker.md", import.meta.url),
    "utf8",
  );
  for (const commands of Object.values(dockerCommands))
    for (const command of Object.values(commands))
      assert.ok(guide.includes(command));
});
test("configuration archive can be validated with a standard ZIP reader", async () => {
  const blob = configurationZip({
    "compose.yaml": configureCompose(template, "D:/Projects", "Windows"),
    "seccomp.json": await readFile(
      new URL("../public/seccomp.json", import.meta.url),
      "utf8",
    ),
  });
  await writeFile(
    new URL("../../.coverage/configuration-test.zip", import.meta.url),
    new Uint8Array(await blob.arrayBuffer()),
  );
  assert.ok(blob.size > 1000);
});
