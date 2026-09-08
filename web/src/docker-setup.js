// Small, uncompressed ZIP writer for the four public configuration files.
// No executable launcher or provider credentials are included in this download.
export function configurationZip(files) {
  const encoder = new TextEncoder();
  const chunks = [],
    directory = [];
  let offset = 0,
    directorySize = 0;
  for (const [name, text] of Object.entries(files)) {
    const filename = encoder.encode(name),
      bytes = encoder.encode(text);
    let crc = 0xffffffff;
    for (const byte of bytes) {
      crc ^= byte;
      for (let bit = 0; bit < 8; bit++)
        crc = (crc >>> 1) ^ (crc & 1 ? 0xedb88320 : 0);
    }
    crc = (crc ^ 0xffffffff) >>> 0;
    const local = new Uint8Array(30 + filename.length),
      l = new DataView(local.buffer);
    l.setUint32(0, 0x04034b50, true);
    l.setUint16(4, 20, true);
    l.setUint32(14, crc, true);
    l.setUint32(18, bytes.length, true);
    l.setUint32(22, bytes.length, true);
    l.setUint16(26, filename.length, true);
    local.set(filename, 30);
    chunks.push(local, bytes);
    const central = new Uint8Array(46 + filename.length),
      c = new DataView(central.buffer);
    c.setUint32(0, 0x02014b50, true);
    c.setUint16(4, 20, true);
    c.setUint16(6, 20, true);
    c.setUint32(16, crc, true);
    c.setUint32(20, bytes.length, true);
    c.setUint32(24, bytes.length, true);
    c.setUint16(28, filename.length, true);
    c.setUint32(42, offset, true);
    central.set(filename, 46);
    directory.push(central);
    directorySize += central.length;
    offset += local.length + bytes.length;
  }
  const end = new Uint8Array(22),
    e = new DataView(end.buffer);
  e.setUint32(0, 0x06054b50, true);
  e.setUint16(8, directory.length, true);
  e.setUint16(10, directory.length, true);
  e.setUint32(12, directorySize, true);
  e.setUint32(16, offset, true);
  return new Blob([...chunks, ...directory, end], { type: "application/zip" });
}

export function configureCompose(template, hostPath, platform) {
  const path = hostPath.trim().replaceAll("\\", "/");
  if (
    !(platform === "Windows" ? /^[a-z]:\/.+/i : /^\/.+/).test(path) ||
    /[\x00-\x1f]/.test(path)
  ) {
    throw new Error("Enter the full path of an existing project folder.");
  }
  const document = JSON.parse(template.replace(/^#[^\n]*(?:\n|$)/gm, ""));
  document.services.magent.volumes[0].source = path.replaceAll("$", () => "$$");
  if (platform === "Linux")
    document.services.magent.security_opt.push("apparmor=magent-container-v1");
  return JSON.stringify(document, null, 2) + "\n";
}

export async function downloadConfiguration(hostPath, platform) {
  const names = ["compose.yaml", "seccomp.json", "NOTICE.md", "LICENSE.moby"];
  const files = Object.fromEntries(
    await Promise.all(
      names.map(async (name) => {
        const response = await fetch(`/${name}`);
        if (!response.ok)
          throw new Error("Configuration download failed. Please try again.");
        return [name, await response.text()];
      }),
    ),
  );
  files["compose.yaml"] = configureCompose(
    files["compose.yaml"],
    hostPath,
    platform,
  );
  files["README.txt"] =
    "Start Docker. From this extracted folder run:\ndocker compose run --rm magent setup\ndocker compose run --rm magent\n\nSelect a workspace before chatting; /config changes your team and workspace.\nYour host files are edited directly. Keep the magent-state volume to retain logins.\nUpdates: docker compose pull, then restart your session.\nLinux: follow the AppArmor and UID setup in https://github.com/ErOr-0/Multiharness-Core/blob/main/docs/docker.md#linux-apparmor-setup\n";
  const url = URL.createObjectURL(configurationZip(files));
  const link = document.createElement("a");
  link.href = url;
  link.download = "multiharness-docker.zip";
  link.click();
  setTimeout(() => URL.revokeObjectURL(url), 1000);
}
