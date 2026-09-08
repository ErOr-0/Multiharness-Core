import assert from "node:assert/strict";
import {
  mkdtempSync,
  writeFileSync,
  readFileSync,
  rmSync,
  existsSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { spawnSync } from "node:child_process";
import test from "node:test";
import {
  folderError,
  launchCommand,
  composeSource,
} from "../src/docker-install.js";

test("literal folder paths reach Docker; failed creation never starts another container", () => {
  const scratch = mkdtempSync(join(tmpdir(), "multiharness-command-"));
  try {
    const log = join(scratch, "calls.jsonl");
    writeFileSync(
      join(scratch, "docker"),
      `#!${process.execPath}\nconst fs=require('fs');fs.appendFileSync(process.env.CALLS,JSON.stringify({args:process.argv.slice(2),folder:process.env.MULTIHARNESS_WORKSPACE})+'\\n');if(process.argv[2]==='compose')process.exit(Number(process.env.CREATE_EXIT||0));\n`,
      { mode: 0o755 },
    );
    const marker = join(scratch, "must-not-exist");
    const folder = `/projects/O'Brien $HOME $(touch ${marker}) \`touch ${marker}\` spaces`;
    for (const platform of ["macOS", "Linux"]) {
      for (const exit of ["0", "17"]) {
        writeFileSync(log, "");
        const p = spawnSync(
          "bash",
          ["-c", launchCommand(folder, platform, true)],
          {
            env: {
              ...process.env,
              PATH: `${scratch}:${process.env.PATH}`,
              CALLS: log,
              CREATE_EXIT: exit,
            },
            encoding: "utf8",
          },
        );
        assert.equal(p.status, Number(exit), p.stderr);
        const calls = readFileSync(log, "utf8")
          .trim()
          .split("\n")
          .map(JSON.parse);
        assert.equal(calls.length, exit === "0" ? 2 : 1);
        assert.equal(calls[0].folder, folder);
        assert.deepEqual(calls[0].args, [
          "compose",
          "-f",
          composeSource,
          ...(platform === "Linux"
            ? ["-f", `${composeSource}:docker/compose.linux.yaml`]
            : []),
          "create",
        ]);
        if (exit === "0")
          assert.deepEqual(calls[1].args, ["start", "-ai", "multiharness"]);
        assert.equal(existsSync(marker), false);
      }
    }
  } finally {
    rmSync(scratch, { recursive: true, force: true });
  }
});

test("invalid paths cannot produce a launch command", () => {
  for (const platform of ["macOS", "Linux", "Windows"])
    for (const path of [
      "",
      "relative/project",
      "/tmp\nmalicious",
      "/tmp\u0000bad",
    ]) {
      assert.ok(folderError(path, platform));
      assert.equal(launchCommand(path, platform), "");
    }
  assert.equal(folderError("C:\\Users\\Sam\\Projects", "Windows"), "");
  assert.equal(folderError("/Users/sam/Projects", "macOS"), "");
});
