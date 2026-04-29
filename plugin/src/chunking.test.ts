import { describe, expect, test } from "bun:test";
import {
  CHUNK_SIZE,
  CHUNK_THRESHOLD,
  ChunkAssembler,
  shouldChunk,
  splitIntoChunks,
  reassembleFrame,
  extractChunkMeta,
} from "./chunking";
import { decodeBinaryFrame, MSG_CHUNK } from "./binary-frame";

const randomBytes = (n: number): Uint8Array => {
  const out = new Uint8Array(n);
  for (let i = 0; i < n; i++) out[i] = Math.floor(Math.random() * 256);
  return out;
};

describe("shouldChunk", () => {
  test("at threshold does not chunk", () => {
    expect(shouldChunk(new Uint8Array(CHUNK_THRESHOLD))).toBe(false);
  });
  test("above threshold chunks", () => {
    expect(shouldChunk(new Uint8Array(CHUNK_THRESHOLD + 1))).toBe(true);
  });
});

describe("splitIntoChunks + ChunkAssembler", () => {
  test("round-trips a 5MB frame in order", () => {
    const src = randomBytes(5 * 1024 * 1024 + 257);
    const chunks = splitIntoChunks("req-rt", src);
    const expected = Math.ceil(src.length / CHUNK_SIZE);
    expect(chunks.length).toBe(expected);

    const asm = new ChunkAssembler();
    let assembled: Uint8Array | null = null;
    for (let i = 0; i < chunks.length; i++) {
      const out = reassembleFrame(asm, chunks[i]);
      if (i < chunks.length - 1) {
        expect(out).toBeNull();
      } else {
        expect(out).not.toBeNull();
        assembled = out;
      }
    }
    expect(assembled).not.toBeNull();
    expect(assembled!.length).toBe(src.length);
    for (let i = 0; i < src.length; i++) {
      if (assembled![i] !== src[i]) {
        throw new Error(`mismatch at byte ${i}`);
      }
    }
    expect(asm.pending()).toBe(0);
  });

  test("handles out-of-order delivery", () => {
    const src = randomBytes(CHUNK_SIZE * 4);
    const chunks = splitIntoChunks("req-ooo", src);
    const asm = new ChunkAssembler();
    // Reverse delivery — only the last index drained yields the assembled
    // bytes, regardless of arrival order.
    let assembled: Uint8Array | null = null;
    for (let i = chunks.length - 1; i >= 0; i--) {
      const out = reassembleFrame(asm, chunks[i]);
      if (i > 0) expect(out).toBeNull();
      else assembled = out;
    }
    expect(assembled).not.toBeNull();
    expect(assembled!.length).toBe(src.length);
  });

  test("rejects duplicate seq on the same stream", () => {
    const src = randomBytes(CHUNK_SIZE * 2);
    const chunks = splitIntoChunks("req-dup", src);
    const frame = decodeBinaryFrame(chunks[0]);
    expect(frame).not.toBeNull();
    expect(frame!.msgType).toBe(MSG_CHUNK);
    const meta = extractChunkMeta(frame!);
    expect(meta).not.toBeNull();
    const asm = new ChunkAssembler();
    asm.receive(meta!, frame!.payload);
    expect(() => asm.receive(meta!, frame!.payload)).toThrow();
  });

  test("emits at least one chunk for tiny inputs", () => {
    const src = new Uint8Array([1, 2, 3]);
    const chunks = splitIntoChunks("req-small", src);
    expect(chunks.length).toBe(1);
    const out = reassembleFrame(new ChunkAssembler(), chunks[0]);
    expect(out).not.toBeNull();
    expect(out!.length).toBe(src.length);
  });

  test("rejects total mismatch across chunks of one stream", () => {
    const asm = new ChunkAssembler();
    asm.receive({ requestId: "x", seq: 0, total: 2 }, new Uint8Array([1]));
    expect(() =>
      asm.receive({ requestId: "x", seq: 1, total: 3 }, new Uint8Array([2])),
    ).toThrow();
  });
});
