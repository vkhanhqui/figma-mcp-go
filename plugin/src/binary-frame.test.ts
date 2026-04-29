import { describe, expect, test } from "bun:test";
import {
  encodeBinaryFrame,
  decodeBinaryFrame,
  splicePayload,
  MSG_REQ,
  MSG_RESP,
  FLAG_HAS_PAYLOAD,
} from "./binary-frame";

describe("binary frame codec", () => {
  test("round-trip without payload", () => {
    const meta = { requestId: "01", type: "ping" };
    const frame = encodeBinaryFrame(MSG_REQ, 0, meta);
    const got = decodeBinaryFrame<typeof meta>(frame);
    expect(got).not.toBeNull();
    expect(got!.msgType).toBe(MSG_REQ);
    expect(got!.flags & FLAG_HAS_PAYLOAD).toBe(0);
    expect(got!.payload.length).toBe(0);
    expect(got!.meta.requestId).toBe("01");
  });

  test("round-trip with payload", () => {
    const meta = { requestId: "02", payloadField: "params.imageData" };
    const payload = new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]);
    const frame = encodeBinaryFrame(MSG_REQ, 0, meta, payload);
    const got = decodeBinaryFrame<typeof meta>(frame);
    expect(got).not.toBeNull();
    expect(got!.flags & FLAG_HAS_PAYLOAD).toBe(FLAG_HAS_PAYLOAD);
    expect(Array.from(got!.payload)).toEqual(Array.from(payload));
  });

  test("response frame round-trip", () => {
    const meta = { requestId: "03", type: "get_screenshot", payloadField: "data.image_data" };
    const payload = new Uint8Array(1024).fill(0xAB);
    const frame = encodeBinaryFrame(MSG_RESP, 0, meta, payload);
    const got = decodeBinaryFrame<typeof meta>(frame);
    expect(got!.msgType).toBe(MSG_RESP);
    expect(got!.payload.length).toBe(1024);
    expect(got!.payload[0]).toBe(0xAB);
  });

  test("rejects bad magic", () => {
    const bad = new Uint8Array([0x58, 0x58, 0x58, 0x58, 0x02, 0x01, 0, 0, 0, 0, 0, 0]);
    expect(decodeBinaryFrame(bad)).toBeNull();
  });

  test("rejects bad version", () => {
    const bad = new Uint8Array([0x46, 0x4d, 0x43, 0x50, 0xff, 0x01, 0, 0, 0, 0, 0, 0]);
    expect(decodeBinaryFrame(bad)).toBeNull();
  });

  test("rejects truncated header", () => {
    const bad = new Uint8Array([0x46, 0x4d, 0x43, 0x50]);
    expect(decodeBinaryFrame(bad)).toBeNull();
  });

  test("round-trips when TextEncoder/TextDecoder are absent (Figma sandbox)", () => {
    const origEnc = (globalThis as any).TextEncoder;
    const origDec = (globalThis as any).TextDecoder;
    try {
      (globalThis as any).TextEncoder = undefined;
      (globalThis as any).TextDecoder = undefined;
      const meta = { requestId: "utf-1", type: "ping", note: "việt — 漢字 — 🎨" };
      const payload = new Uint8Array([1, 2, 3, 0xff]);
      const frame = encodeBinaryFrame(MSG_REQ, 0, meta, payload);
      const got = decodeBinaryFrame<typeof meta>(frame);
      expect(got).not.toBeNull();
      expect(got!.meta.note).toBe("việt — 漢字 — 🎨");
      expect(Array.from(got!.payload)).toEqual([1, 2, 3, 0xff]);
    } finally {
      (globalThis as any).TextEncoder = origEnc;
      (globalThis as any).TextDecoder = origDec;
    }
  });
});

describe("splicePayload", () => {
  test("assigns at top level", () => {
    const root: Record<string, unknown> = {};
    splicePayload(root, "imageData", new Uint8Array([1, 2, 3]));
    expect(root.imageData).toBeInstanceOf(Uint8Array);
  });

  test("walks nested path and preserves siblings", () => {
    const root: Record<string, unknown> = { params: { x: 1 } };
    splicePayload(root, "params.imageData", new Uint8Array([0xAB]));
    const params = root.params as Record<string, unknown>;
    expect(params.x).toBe(1);
    expect((params.imageData as Uint8Array)[0]).toBe(0xAB);
  });

  test("creates intermediate maps", () => {
    const root: Record<string, unknown> = {};
    splicePayload(root, "data.image_data", new Uint8Array([5]));
    const data = root.data as Record<string, unknown>;
    expect((data.image_data as Uint8Array)[0]).toBe(5);
  });
});
