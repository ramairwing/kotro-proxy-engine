/** Platform → release asset basename. Keep in sync with distributions/shared/binary-target.js */

export function binaryBasename(platform: NodeJS.Platform, arch: string): string {
  if (platform === 'darwin') {
    return arch === 'arm64'
      ? 'kotro-proxy-aarch64-apple-darwin'
      : 'kotro-proxy-x86_64-apple-darwin';
  }
  if (platform === 'linux') {
    return 'kotro-proxy-x86_64-unknown-linux-gnu';
  }
  if (platform === 'win32') {
    return 'kotro-proxy-x86_64-pc-windows-msvc.exe';
  }
  throw new Error(`Unsupported platform: ${platform}/${arch}`);
}
