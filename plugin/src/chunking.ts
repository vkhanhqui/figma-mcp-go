// Plugin-side mirror of internal/chunking.go. Splits oversized binary frames
// into ~64KB chunks so the WebSocket writer can interleave them with smaller
// messages, and reassembles incoming chunks transparently.

import {
  encodeBinaryFrame,
  decodeBinaryFrame,
  MSG_CHUNK,
  FLAG_LAST_CHUNK,
} from "./binary-frame";

export const CHUNK_SIZE = 64 * 1024;
export const CHUNK_THRESHOLD = 1 * 1024 * 1024;
const ASSEMBLE_TTL_MS = 30 * 1000;

export interface ChunkMeta {
  requestId: string;
  seq: number;
  total: number;
}

export const shouldChunk = (frame: Uint8Array): boolean =>
  frame.length > CHUNK_THRESHOLD;

// splitIntoChunks produces an array of pre-encoded chunk frames. Each chunk
// contains its slice of the original binary frame as the payload, with
// metadata describing its position in the stream.
export const splitIntoChunks = (
  requestId: string,
  frame: Uint8Array,
): Uint8Array[] => {
  if (frame.length === 0) return [];
  const total = Math.max(1, Math.ceil(frame.length / CHUNK_SIZE));
  const out: Uint8Array[] = [];
  for (let i = 0; i < total; i++) {
    const start = i * CHUNK_SIZE;
    const end = Math.min(start + CHUNK_SIZE, frame.length);
    const flags = i === total - 1 ? FLAG_LAST_CHUNK : 0;
    const meta: ChunkMeta = {
      requestId,
      seq: i,
      total,
    };
    out.push(
      encodeBinaryFrame(MSG_CHUNK, flags, meta, frame.subarray(start, end)),
    );
  }
  return out;
};

interface ChunkBuf {
  parts: (Uint8Array | undefined)[];
  total: number;
  received: number;
  expiry: number;
}

// ChunkAssembler buffers incoming chunks until a stream completes, then
// returns the assembled byte slice. Misbehaving senders can't pin memory
// forever — partial streams expire after ASSEMBLE_TTL_MS.
export class ChunkAssembler {
  private streams = new Map<string, ChunkBuf>();

  receive(meta: ChunkMeta, payload: Uint8Array): Uint8Array | null {
    if (!meta.requestId) {
      throw new Error("chunk meta missing requestId");
    }
    if (meta.total <= 0) {
      throw new Error(`chunk meta has non-positive total: ${meta.total}`);
    }
    if (meta.seq < 0 || meta.seq >= meta.total) {
      throw new Error(`chunk seq out of range: ${meta.seq}/${meta.total}`);
    }

    this.gcExpired();
    let st = this.streams.get(meta.requestId);
    if (!st) {
      st = {
        parts: new Array(meta.total),
        total: meta.total,
        received: 0,
        expiry: Date.now() + ASSEMBLE_TTL_MS,
      };
      this.streams.set(meta.requestId, st);
    } else if (st.total !== meta.total) {
      throw new Error(
        `chunk total mismatch: existing=${st.total} incoming=${meta.total}`,
      );
    }
    if (st.parts[meta.seq] !== undefined) {
      throw new Error(
        `duplicate chunk seq ${meta.seq} for ${meta.requestId}`,
      );
    }
    // Copy because the WebSocket buffer may be reused.
    st.parts[meta.seq] = payload.slice();
    st.received++;
    st.expiry = Date.now() + ASSEMBLE_TTL_MS;

    if (st.received < st.total) return null;
    this.streams.delete(meta.requestId);

    let total = 0;
    for (const p of st.parts) total += p?.length ?? 0;
    const out = new Uint8Array(total);
    let off = 0;
    for (const p of st.parts) {
      if (p) {
        out.set(p, off);
        off += p.length;
      }
    }
    return out;
  }

  reset(): void {
    this.streams.clear();
  }

  pending(): number {
    return this.streams.size;
  }

  private gcExpired(): void {
    const now = Date.now();
    for (const [id, st] of this.streams) {
      if (now > st.expiry) this.streams.delete(id);
    }
  }
}

// extractChunkMeta unwraps a decoded chunk frame's metadata. The decoder gives
// us `meta` as `unknown`; here we narrow it to ChunkMeta with shape checks so
// the caller doesn't have to.
export const extractChunkMeta = (frame: {
  meta: unknown;
}): ChunkMeta | null => {
  const m = frame.meta as Partial<ChunkMeta> | null | undefined;
  if (!m || typeof m !== "object") return null;
  if (typeof m.requestId !== "string") return null;
  if (typeof m.seq !== "number" || typeof m.total !== "number") return null;
  return { requestId: m.requestId, seq: m.seq, total: m.total };
};

// reassembleFrame is a small convenience: given a chunk frame, run it through
// the assembler and return the original frame bytes when complete. Returns
// null while more chunks are expected; throws on protocol errors.
export const reassembleFrame = (
  asm: ChunkAssembler,
  chunkBytes: Uint8Array,
): Uint8Array | null => {
  const frame = decodeBinaryFrame<ChunkMeta>(chunkBytes);
  if (!frame || frame.msgType !== MSG_CHUNK) return null;
  const meta = extractChunkMeta(frame);
  if (!meta) return null;
  return asm.receive(meta, frame.payload);
};
