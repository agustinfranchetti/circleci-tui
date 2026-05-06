#!/usr/bin/env node
// Postinstall script: download the right circleci-tui binary for the host's
// OS and architecture from the matching GitHub release, verify the SHA256
// against checksums.txt, and drop it at bin/circleci-tui.
//
// Pattern borrowed from esbuild / biome / swc: keeps the npm tarball tiny
// (the binary lives on GitHub Releases, not inside the npm package) while
// still letting users install via the familiar `npm i -g circleci-tui`.

"use strict";

const https = require("https");
const fs = require("fs");
const path = require("path");
const os = require("os");
const crypto = require("crypto");
const { spawnSync } = require("child_process");
const zlib = require("zlib");

const REPO = "agustinfranchetti/circleci-tui";

function platformTuple() {
  const platformMap = { darwin: "darwin", linux: "linux", win32: "windows" };
  const archMap = { x64: "amd64", arm64: "arm64" };
  const goos = platformMap[process.platform];
  const goarch = archMap[process.arch];
  if (!goos || !goarch) {
    throw new Error(
      `unsupported platform ${process.platform}/${process.arch}. ` +
        `If you'd like circleci-tui on this combo, please open an issue at ` +
        `https://github.com/${REPO}/issues`
    );
  }
  return { goos, goarch };
}

function archiveName(version, goos, goarch) {
  const ext = goos === "windows" ? "zip" : "tar.gz";
  return `circleci-tui-${version}-${goos}-${goarch}.${ext}`;
}

// fetch follows redirects (GitHub Release downloads bounce through a
// short-lived S3 URL) and resolves to a Buffer.
function fetch(url) {
  return new Promise((resolve, reject) => {
    https
      .get(url, { headers: { "User-Agent": "circleci-tui-installer" } }, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          fetch(res.headers.location).then(resolve, reject);
          return;
        }
        if (res.statusCode !== 200) {
          reject(new Error(`download failed ${res.statusCode}: ${url}`));
          return;
        }
        const chunks = [];
        res.on("data", (c) => chunks.push(c));
        res.on("end", () => resolve(Buffer.concat(chunks)));
      })
      .on("error", reject);
  });
}

function sha256(buf) {
  return crypto.createHash("sha256").update(buf).digest("hex");
}

function findChecksum(checksumsText, archive) {
  // checksums.txt format: "<sha256>  <filename>" per line.
  for (const line of checksumsText.split(/\r?\n/)) {
    const m = line.trim().match(/^([0-9a-f]{64})\s+(\S+)$/);
    if (m && m[2] === archive) return m[1];
  }
  return null;
}

function extract(archiveBuffer, archiveName, dest) {
  if (archiveName.endsWith(".zip")) {
    // Avoid pulling a JS unzip dep; shell out to the system unzip — it's on
    // every Windows install since Win10 build 17063 and pre-installed on macOS.
    const tmp = path.join(os.tmpdir(), archiveName);
    fs.writeFileSync(tmp, archiveBuffer);
    const r = spawnSync("unzip", ["-o", tmp, "-d", dest], { stdio: "inherit" });
    if (r.status !== 0) throw new Error("unzip failed");
    return;
  }
  // .tar.gz — gunzip then tar -xf via the system tar binary.
  const tmp = path.join(os.tmpdir(), archiveName.replace(/\.gz$/, ""));
  fs.writeFileSync(tmp, zlib.gunzipSync(archiveBuffer));
  const r = spawnSync("tar", ["-xf", tmp, "-C", dest], { stdio: "inherit" });
  if (r.status !== 0) throw new Error("tar -xf failed");
  fs.unlinkSync(tmp);
}

async function main() {
  // CIRCLECI_TUI_SKIP_INSTALL is for monorepo / CI scenarios where you
  // don't want every checkout to hit GitHub.
  if (process.env.CIRCLECI_TUI_SKIP_INSTALL === "1") {
    console.log("CIRCLECI_TUI_SKIP_INSTALL=1 — skipping binary download");
    return;
  }

  const pkg = require("../package.json");
  const version = pkg.version;
  const { goos, goarch } = platformTuple();
  const archive = archiveName(version, goos, goarch);
  const tag = `v${version}`;
  const baseURL = `https://github.com/${REPO}/releases/download/${tag}`;

  console.log(`circleci-tui: fetching ${archive}…`);
  const [buf, checksums] = await Promise.all([
    fetch(`${baseURL}/${archive}`),
    fetch(`${baseURL}/checksums.txt`),
  ]);

  const expected = findChecksum(checksums.toString("utf8"), archive);
  if (!expected) {
    throw new Error(`no checksum entry for ${archive} — refusing to install`);
  }
  const actual = sha256(buf);
  if (actual !== expected) {
    throw new Error(
      `checksum mismatch for ${archive}: expected ${expected}, got ${actual}`
    );
  }

  const binDir = path.resolve(__dirname);
  fs.mkdirSync(binDir, { recursive: true });
  extract(buf, archive, binDir);

  const binName = goos === "windows" ? "circleci-tui.exe" : "circleci-tui";
  const binPath = path.join(binDir, binName);
  if (!fs.existsSync(binPath)) {
    throw new Error(`expected ${binPath} after extraction; got: ${fs.readdirSync(binDir).join(", ")}`);
  }
  if (goos !== "windows") {
    fs.chmodSync(binPath, 0o755);
    // package.json's "bin" entry points at "bin/circleci-tui" (no .exe).
    // On Unix that already matches; on Windows npm rewrites the symlink.
  }
  console.log(`circleci-tui: installed ${binPath}`);
}

main().catch((err) => {
  console.error("circleci-tui install failed:", err.message || err);
  console.error(
    "you can still build from source: " +
      "https://github.com/" +
      REPO +
      "#building-from-source"
  );
  process.exit(1);
});
