/** The link that opens an authenticator app, or undefined unless the server sent a plain otpauth://totp URL. */
export function authenticatorLink(url: string): string | undefined {
  return /^otpauth:\/\/totp\/\S+$/.test(url) ? url : undefined;
}
