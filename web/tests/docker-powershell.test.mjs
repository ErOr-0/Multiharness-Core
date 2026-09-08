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
import { launchCommand, composeSource } from "../src/docker-install.js";
const hasPwsh =
  spawnSync("pwsh", ["-NoProfile", "-Command", "exit 0"]).status === 0;
test(
  "PowerShell preserves literal paths and stops after failed creation",
  { skip: !hasPwsh },
  () => {
    const scratch = mkdtempSync(join(tmpdir(), "multiharness-powershell-"));
    try {
      const log = join(scratch, "calls.json");
      const marker = join(scratch, "must-not-exist");
      const folder = `C:\\Users\\O'Brien $HOME $(New-Item '${marker}') \\Projects`;
      const script = join(scratch, "fixture.ps1");
      for (const failed of [false, true]) {
        writeFileSync(
          script,
          `$script:calls = @()\nfunction docker { $script:calls += @{ args = @($args); folder = $env:MULTIHARNESS_WORKSPACE }; $global:LASTEXITCODE = ${failed ? 17 : 0} }\n${launchCommand(folder, "Windows")}\nConvertTo-Json -Depth 5 -InputObject @($script:calls) | Set-Content -LiteralPath $env:CALLS\n`,
        );
        const p = spawnSync("pwsh", ["-NoProfile", "-File", script], {
          env: { ...process.env, CALLS: log },
          encoding: "utf8",
        });
        assert.equal(p.status, 0, p.stderr);
        const calls = JSON.parse(
          readFileSync(log, "utf8").replace(/^\uFEFF/, ""),
        );
        assert.equal(calls.length, failed ? 1 : 2);
        assert.equal(calls[0].folder, folder);
        assert.deepEqual(calls[0].args, [
          "compose",
          "-f",
          composeSource,
          "create",
        ]);
        if (!failed)
          assert.deepEqual(calls[1].args, ["start", "-ai", "multiharness"]);
        assert.equal(existsSync(marker), false);
      }
    } finally {
      rmSync(scratch, { recursive: true, force: true });
    }
  },
);
