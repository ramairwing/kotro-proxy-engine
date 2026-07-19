import * as vscode from 'vscode';
import * as https from 'https';
import * as http from 'http';
import * as fs from 'fs';
import * as path from 'path';
import * as crypto from 'crypto';
import { execFile } from 'child_process';
import { promisify } from 'util';
import { binaryBasename } from './binary-target';

const execFileAsync = promisify(execFile);
const REPO = 'kotro-labs/kotro-proxy-engine';
const USER_AGENT = 'Kotro-VSCode-Extension';

type ReleaseAsset = { name: string; browser_download_url: string };

/**
 * Ensure a verified Kotro proxy binary exists in extension globalStorage.
 * Downloads the platform tarball/exe from GitHub Releases, verifies SHA-256
 * against that release's checksums.txt, then extracts (unix) / installs (win).
 */
export async function ensureBinary(
  context: vscode.ExtensionContext,
  output: vscode.OutputChannel,
): Promise<{ path: string; freshlyDownloaded: boolean } | null> {
  const globalStorage = context.globalStorageUri.fsPath;
  fs.mkdirSync(globalStorage, { recursive: true });

  const binName = binaryBasename(process.platform, process.arch);
  const binPath = path.join(globalStorage, binName);

  if (fs.existsSync(binPath)) {
    output.appendLine(`Found existing binary at ${binPath}`);
    return { path: binPath, freshlyDownloaded: false };
  }

  const pick = await vscode.window.showInformationMessage(
    'Kotro needs to download the proxy binary (~15MB) from GitHub Releases, verify its SHA-256 against checksums.txt, then install it into this extension’s global storage. Proceed?',
    'Proceed',
    'Cancel',
  );
  if (pick !== 'Proceed') {
    output.appendLine('User cancelled binary download.');
    return null;
  }

  const archiveName =
    process.platform === 'win32' ? binName : `${binName}.tar.gz`;
  const tmpArchive = path.join(globalStorage, `.download-${archiveName}`);

  try {
    output.appendLine(`Fetching latest release metadata…`);
    const { assets, tag } = await getLatestRelease();
    output.appendLine(`Latest release: ${tag}`);

    const archiveAsset = assets.find((a) => a.name === archiveName);
    if (!archiveAsset) {
      throw new Error(`Asset ${archiveName} not found in ${tag}`);
    }

    const checksumsAsset = assets.find((a) => a.name === 'checksums.txt');
    if (!checksumsAsset) {
      throw new Error(
        `checksums.txt missing from ${tag}. Refusing to install an unverified binary.`,
      );
    }

    output.appendLine(`Downloading ${archiveName}…`);
    await downloadFile(archiveAsset.browser_download_url, tmpArchive, output);

    output.appendLine(`Fetching checksums.txt…`);
    const checksumsBody = await fetchText(checksumsAsset.browser_download_url);
    const expected = parseChecksum(checksumsBody, archiveName);
    if (!expected) {
      throw new Error(`No SHA-256 entry for ${archiveName} in checksums.txt`);
    }

    const actual = await sha256File(tmpArchive);
    output.appendLine(`SHA-256 expected: ${expected}`);
    output.appendLine(`SHA-256 actual:   ${actual}`);
    if (actual !== expected) {
      fs.unlinkSync(tmpArchive);
      throw new Error(
        `SHA-256 mismatch for ${archiveName}. Expected ${expected}, got ${actual}.`,
      );
    }
    output.appendLine('Checksum OK.');

    if (process.platform === 'win32') {
      fs.renameSync(tmpArchive, binPath);
    } else {
      await execFileAsync('tar', ['-xzf', tmpArchive, '-C', globalStorage]);
      fs.unlinkSync(tmpArchive);
      if (!fs.existsSync(binPath)) {
        throw new Error(`Extracted archive but ${binName} not found in ${globalStorage}`);
      }
      fs.chmodSync(binPath, 0o755);
    }

    output.appendLine(`Installed verified binary to ${binPath}`);
    return { path: binPath, freshlyDownloaded: true };
  } catch (e: unknown) {
    const message = e instanceof Error ? e.message : String(e);
    try {
      if (fs.existsSync(tmpArchive)) {
        fs.unlinkSync(tmpArchive);
      }
    } catch {
      /* ignore */
    }
    output.appendLine(`Failed to download/verify binary: ${message}`);
    void vscode.window.showErrorMessage(`Failed to download Kotro binary: ${message}`);
    return null;
  }
}

function getLatestRelease(): Promise<{ tag: string; assets: ReleaseAsset[] }> {
  return new Promise((resolve, reject) => {
    https
      .get(
        {
          hostname: 'api.github.com',
          path: `/repos/${REPO}/releases/latest`,
          headers: { 'User-Agent': USER_AGENT, Accept: 'application/vnd.github+json' },
        },
        (res) => {
          followRedirects(res, (err, body) => {
            if (err) {
              reject(err);
              return;
            }
            try {
              const json = JSON.parse(body);
              resolve({
                tag: json.tag_name ?? 'unknown',
                assets: (json.assets ?? []).map((a: ReleaseAsset) => ({
                  name: a.name,
                  browser_download_url: a.browser_download_url,
                })),
              });
            } catch (parseErr) {
              reject(parseErr);
            }
          });
        },
      )
      .on('error', reject);
  });
}

function followRedirects(
  res: http.IncomingMessage,
  cb: (err: Error | null, body: string) => void,
  redirects = 0,
): void {
  if (res.statusCode && res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
    if (redirects > 5) {
      cb(new Error('Too many redirects'), '');
      return;
    }
    const loc = res.headers.location;
    const url = new URL(loc, 'https://api.github.com');
    const lib = url.protocol === 'http:' ? http : https;
    lib
      .get(
        url,
        { headers: { 'User-Agent': USER_AGENT, Accept: 'application/vnd.github+json' } },
        (next) => followRedirects(next, cb, redirects + 1),
      )
      .on('error', (e) => cb(e, ''));
    return;
  }
  if (res.statusCode !== 200) {
    cb(new Error(`GitHub API returned ${res.statusCode}`), '');
    return;
  }
  let data = '';
  res.on('data', (chunk) => (data += chunk));
  res.on('end', () => cb(null, data));
}

function fetchText(url: string): Promise<string> {
  return new Promise((resolve, reject) => {
    const get = (u: string, redirects = 0) => {
      https
        .get(u, { headers: { 'User-Agent': USER_AGENT } }, (res) => {
          if (res.statusCode && res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
            if (redirects > 5) {
              reject(new Error('Too many redirects'));
              return;
            }
            get(res.headers.location, redirects + 1);
            return;
          }
          if (res.statusCode !== 200) {
            reject(new Error(`Download failed with status ${res.statusCode}`));
            return;
          }
          let data = '';
          res.on('data', (c) => (data += c));
          res.on('end', () => resolve(data));
        })
        .on('error', reject);
    };
    get(url);
  });
}

function parseChecksum(checksumsBody: string, assetName: string): string | null {
  for (const line of checksumsBody.split(/\r?\n/)) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith('#')) {
      continue;
    }
    // sha256sum format: "<hash>  <filename>" or "<hash> <filename>"
    const m = trimmed.match(/^([a-fA-F0-9]{64})\s+\*?(.+)$/);
    if (!m) {
      continue;
    }
    const file = path.basename(m[2].trim());
    if (file === assetName) {
      return m[1].toLowerCase();
    }
  }
  return null;
}

function sha256File(filePath: string): Promise<string> {
  return new Promise((resolve, reject) => {
    const hash = crypto.createHash('sha256');
    const stream = fs.createReadStream(filePath);
    stream.on('data', (chunk) => hash.update(chunk));
    stream.on('error', reject);
    stream.on('end', () => resolve(hash.digest('hex')));
  });
}

function downloadFile(url: string, dest: string, output: vscode.OutputChannel): Promise<void> {
  return new Promise((resolve, reject) => {
    const file = fs.createWriteStream(dest);

    const doDownload = (downloadUrl: string, redirects = 0) => {
      https
        .get(downloadUrl, { headers: { 'User-Agent': USER_AGENT } }, (res) => {
          if (res.statusCode && res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
            if (redirects > 5) {
              fs.unlink(dest, () => reject(new Error('Too many redirects')));
              return;
            }
            doDownload(res.headers.location, redirects + 1);
            return;
          }
          if (res.statusCode !== 200) {
            fs.unlink(dest, () =>
              reject(new Error(`Download failed with status ${res.statusCode}`)),
            );
            return;
          }
          res.pipe(file);
          file.on('finish', () => {
            file.close();
            output.appendLine(`Downloaded to ${dest}`);
            resolve();
          });
        })
        .on('error', (err) => {
          fs.unlink(dest, () => reject(err));
        });
    };

    doDownload(url);
  });
}
