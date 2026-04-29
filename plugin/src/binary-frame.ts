// Binary WebSocket frame codec — mirror of internal/binary_frame.go.
//
// Wire format:
//   | 4 bytes magic "FMCP" | 1 byte version=0x02 | 1 byte msgType | 2 bytes flags |
//   | 4 bytes metaLen      | metaLen bytes JSON metadata                            |
//   | remaining bytes      | raw payload (image bytes, PDF bytes, ...)              |
//
// Used to transport image / PDF / large JSON payloads without the 33% base64
// inflation. Opt-in: gated on the `binary_frames_v1` capability advertised
// in the hello handshake.

export const MSG_REQ = 0x01;
export const MSG_RESP = 0x02;
export const MSG_PROGRESS = 0x03;
export const MSG_HEARTBEAT = 0x04;
export const MSG_CHUNK = 0x05;

export const FLAG_HAS_PAYLOAD = 0x0001;
export const FLAG_COMPRESSED = 0x0002;
export const FLAG_LAST_CHUNK = 0x0004;

const VERSION = 0x02;
const HEADER_SIZE = 12;
// "FMCP" big-endian
const MAGIC = [0x46, 0x4d, 0x43, 0x50];

// Figma's plugin sandbox (QuickJS) does not expose TextEncoder/TextDecoder on
// the main thread, so we ship our own UTF-8 codec. Bun + browsers have the
// natives — prefer them when present so tests don't pay the polyfill cost.
// Detection is per-call so tests can stub the globals to exercise the polyfill.

export const utf8Encode = (s: string): Uint8Array => {
  if (typeof TextEncoder !== "undefined") return new TextEncoder().encode(s);
  const out: number[] = [];
  for (let i = 0; i < s.length; i++) {
    let c = s.charCodeAt(i);
    if (c < 0x80) {
      out.push(c);
    } else if (c < 0x800) {
      out.push(0xc0 | (c >> 6), 0x80 | (c & 0x3f));
    } else if ((c & 0xfc00) === 0xd800 && i + 1 < s.length && (s.charCodeAt(i + 1) & 0xfc00) === 0xdc00) {
      const cp = 0x10000 + (((c & 0x3ff) << 10) | (s.charCodeAt(++i) & 0x3ff));
      out.push(
        0xf0 | (cp >> 18),
        0x80 | ((cp >> 12) & 0x3f),
        0x80 | ((cp >> 6) & 0x3f),
        0x80 | (cp & 0x3f),
      );
    } else {
      out.push(0xe0 | (c >> 12), 0x80 | ((c >> 6) & 0x3f), 0x80 | (c & 0x3f));
    }
  }
  return new Uint8Array(out);
};

export const utf8Decode = (b: Uint8Array): string => {
  if (typeof TextDecoder !== "undefined") return new TextDecoder().decode(b);
  let out = "";
  let i = 0;
  while (i < b.length) {
    const c = b[i++];
    if (c < 0x80) {
      out += String.fromCharCode(c);
    } else if (c < 0xe0) {
      out += String.fromCharCode(((c & 0x1f) << 6) | (b[i++] & 0x3f));
    } else if (c < 0xf0) {
      const c2 = b[i++], c3 = b[i++];
      out += String.fromCharCode(((c & 0xf) << 12) | ((c2 & 0x3f) << 6) | (c3 & 0x3f));
    } else {
      const c2 = b[i++], c3 = b[i++], c4 = b[i++];
      const cp = (((c & 0x7) << 18) | ((c2 & 0x3f) << 12) | ((c3 & 0x3f) << 6) | (c4 & 0x3f)) - 0x10000;
      out += String.fromCharCode(0xd800 | (cp >> 10), 0xdc00 | (cp & 0x3ff));
    }
  }
  return out;
};

export interface BinaryFrame<M = unknown> {
  msgType: number;
  flags: number;
  meta: M;
  payload: Uint8Array;
}

export const encodeBinaryFrame = (
  msgType: number,
  flags: number,
  meta: unknown,
  payload?: Uint8Array | null,
): Uint8Array => {
  const metaBytes = utf8Encode(JSON.stringify(meta));
  const payloadBytes = payload ?? new Uint8Array();
  if (payloadBytes.length > 0) flags |= FLAG_HAS_PAYLOAD;
  const total = HEADER_SIZE + metaBytes.length + payloadBytes.length;
  const buf = new Uint8Array(total);
  buf[0] = MAGIC[0];
  buf[1] = MAGIC[1];
  buf[2] = MAGIC[2];
  buf[3] = MAGIC[3];
  buf[4] = VERSION;
  buf[5] = msgType;
  const view = new DataView(buf.buffer);
  view.setUint16(6, flags, false); // big-endian
  view.setUint32(8, metaBytes.length, false);
  buf.set(metaBytes, HEADER_SIZE);
  if (payloadBytes.length) buf.set(payloadBytes, HEADER_SIZE + metaBytes.length);
  return buf;
};

export const decodeBinaryFrame = <M = unknown>(data: Uint8Array): BinaryFrame<M> | null => {
  if (data.length < HEADER_SIZE) return null;
  if (
    data[0] !== MAGIC[0] ||
    data[1] !== MAGIC[1] ||
    data[2] !== MAGIC[2] ||
    data[3] !== MAGIC[3]
  ) {
    return null;
  }
  if (data[4] !== VERSION) return null;
  const msgType = data[5];
  const view = new DataView(data.buffer, data.byteOffset, data.byteLength);
  const flags = view.getUint16(6, false);
  const metaLen = view.getUint32(8, false);
  const metaEnd = HEADER_SIZE + metaLen;
  if (metaEnd > data.length) return null;
  const metaText = utf8Decode(data.subarray(HEADER_SIZE, metaEnd));
  let meta: M;
  try {
    meta = JSON.parse(metaText) as M;
  } catch {
    return null;
  }
  const payload = data.subarray(metaEnd);
  return { msgType, flags, meta, payload };
};

// splicePayload assigns `value` at a dotted JSON path inside `root`.
// Used to put the binary payload back into the field where the handler
// expects it (e.g. "params.imageData" on the request side).
export const splicePayload = (
  root: Record<string, unknown>,
  path: string,
  value: unknown,
): void => {
  if (!path) return;
  const parts = path.split(".");
  let cur: Record<string, unknown> = root;
  for (let i = 0; i < parts.length; i++) {
    const p = parts[i];
    if (i === parts.length - 1) {
      cur[p] = value;
      return;
    }
    let next = cur[p] as Record<string, unknown> | undefined;
    if (!next || typeof next !== "object") {
      next = {};
      cur[p] = next;
    }
    cur = next;
  }
};
