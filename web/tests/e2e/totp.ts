import { createHmac } from 'node:crypto';

const alphabet = 'ABCDEFGHIJKLMNOPQRSTUVWXYZ234567';

/** Decodes an RFC 4648 base32 string, ignoring case, spaces and padding. */
export function base32Decode(input: string): Buffer {
  let bits = 0;
  let value = 0;
  const bytes: number[] = [];
  for (const char of input.toUpperCase().replace(/[\s=]/g, '')) {
    const index = alphabet.indexOf(char);
    if (index < 0) throw new Error(`not a base32 character: ${char}`);
    value = (value << 5) | index;
    bits += 5;
    if (bits >= 8) {
      bits -= 8;
      bytes.push((value >>> bits) & 0xff);
    }
  }
  return Buffer.from(bytes);
}

/** The 6-digit RFC 6238 code for a base32 secret, where stepOffset moves to a neighbouring 30-second step. */
export function totpCode(secret: string, stepOffset = 0, nowMs = Date.now()): string {
  const step = Math.floor(nowMs / 1000 / 30) + stepOffset;
  const counter = Buffer.alloc(8);
  counter.writeBigUInt64BE(BigInt(step));
  const hmac = createHmac('sha1', base32Decode(secret)).update(counter).digest();
  const offset = hmac[hmac.length - 1] & 0x0f;
  const binary = hmac.readUInt32BE(offset) & 0x7fffffff;
  return String(binary % 1_000_000).padStart(6, '0');
}
