// Plugin core — entry point, UI bootstrap, and request dispatch.

import { handleReadRequest } from "./read-handlers";
import { handleWriteRequest } from "./write-handlers";
import {
  decodeBinaryFrame,
  encodeBinaryFrame,
  splicePayload,
  MSG_CHUNK,
  MSG_REQ,
  MSG_RESP,
} from "./binary-frame";
import {
  ChunkAssembler,
  ChunkMeta,
  extractChunkMeta,
  shouldChunk,
  splitIntoChunks,
} from "./chunking";

// One assembler per process. Reset implicitly when the iframe reloads.
const chunker = new ChunkAssembler();

const sendStatus = () => {
  figma.ui.postMessage({
    type: "plugin-status",
    payload: {
      fileName: figma.root.name,
      pageName: figma.currentPage.name,
      selectionCount: figma.currentPage.selection.length,
    },
  });
};

const handleRequest = async (request: any) => {
  try {
    const result =
      (await handleReadRequest(request)) ?? (await handleWriteRequest(request));
    if (result === null)
      throw new Error(`Unknown request type: ${request.type}`);
    return result;
  } catch (error) {
    return {
      type: request.type,
      requestId: request.requestId,
      error: error instanceof Error ? error.message : String(error),
    };
  }
};

// Tool name → mapping of (data path → field carrying raw bytes) for binary
// response encoding. Each entry produces one payload slice in the binary
// frame. Keys are visited in insertion order so the wire layout is stable.
// Entries refer to a path inside `response.data`; we walk arrays automatically
// when an entry's `arrayPath` is set.
type BinaryEncoding = {
  arrayPath?: string; // e.g. "exports", "frames", "results" — iterate this array
  byteField: string;  // field name on each element (or on data) holding Uint8Array
  removeField?: string; // optional sibling field to drop (e.g. legacy "base64")
};

const binaryEncodings: Record<string, BinaryEncoding> = {
  get_screenshot: { arrayPath: "exports", byteField: "bytes" },
  export_nodes_batch: { arrayPath: "results", byteField: "bytes" },
  export_frames_to_pdf: { arrayPath: "frames", byteField: "bytes" },
};

// collectPayloadSlices walks the response's binary fields, builds a flat
// payload buffer, and produces wire-relative slice descriptors. Returns null
// if the response carries no binary fields (text JSON path stays correct).
const collectPayloadSlices = (
  response: any,
): { payload: Uint8Array; slices: { path: string; length: number }[] } | null => {
  const enc = binaryEncodings[response?.type];
  if (!enc) return null;
  const data = response?.data;
  if (!data) return null;
  const buffers: Uint8Array[] = [];
  const slices: { path: string; length: number }[] = [];
  if (enc.arrayPath) {
    const arr = data[enc.arrayPath];
    if (!Array.isArray(arr) || arr.length === 0) return null;
    for (let i = 0; i < arr.length; i++) {
      const item = arr[i];
      const bytes = item?.[enc.byteField];
      if (bytes instanceof Uint8Array) {
        buffers.push(bytes);
        slices.push({ path: `data.${enc.arrayPath}.${i}.${enc.byteField}`, length: bytes.length });
        if (enc.removeField) delete item[enc.removeField];
        delete item[enc.byteField];
      }
    }
  } else {
    const bytes = data[enc.byteField];
    if (bytes instanceof Uint8Array) {
      buffers.push(bytes);
      slices.push({ path: `data.${enc.byteField}`, length: bytes.length });
      if (enc.removeField) delete data[enc.removeField];
      delete data[enc.byteField];
    }
  }
  if (buffers.length === 0) return null;
  const total = buffers.reduce((a, b) => a + b.length, 0);
  const payload = new Uint8Array(total);
  let off = 0;
  for (const b of buffers) {
    payload.set(b, off);
    off += b.length;
  }
  return { payload, slices };
};

const sendResponse = (response: any) => {
  const slices = collectPayloadSlices(response);
  if (slices) {
    const meta = { ...response, payloadSlices: slices.slices };
    const frame = encodeBinaryFrame(MSG_RESP, 0, meta, slices.payload);
    // Split frames > 1MB so the server's writer can interleave them with
    // small messages. Single-frame path stays for the common case (small
    // icons, status responses, ...).
    if (shouldChunk(frame)) {
      const chunks = splitIntoChunks(response.requestId, frame);
      figma.ui.postMessage({
        type: "send-binary",
        requestId: response.requestId,
        payloads: chunks,
      });
      return;
    }
    figma.ui.postMessage({
      type: "send-binary",
      requestId: response.requestId,
      payload: frame,
    });
    return;
  }
  figma.ui.postMessage(response);
};

figma.showUI(__html__, { width: 320, height: 230 });
sendStatus();

figma.on("selectionchange", () => {
  sendStatus();
});

figma.on("currentpagechange", () => {
  sendStatus();
});

figma.ui.onmessage = async (message) => {
  if (message.type === "ui-ready") {
    sendStatus();
    return;
  }
  if (message.type === "get_ws_config") {
    const config = await figma.clientStorage.getAsync("ws_config");
    figma.ui.postMessage({
      type: "ws_config",
      host: config?.host ?? "127.0.0.1",
      port: config?.port ?? "1994",
    });
    return;
  }
  if (message.type === "save_ws_config") {
    await figma.clientStorage.setAsync("ws_config", {
      host: message.host,
      port: message.port,
    });
    return;
  }
  if (message.type === "server-request") {
    const response = await handleRequest(message.payload);
    try {
      sendResponse(response);
    } catch (err) {
      figma.ui.postMessage({
        type: response.type,
        requestId: response.requestId,
        error: err instanceof Error ? err.message : String(err),
      });
    }
    return;
  }

  if (message.type === "server-binary") {
    let frameBytes = message.payload as Uint8Array;
    // Reassemble chunked frames before dispatch. Mid-stream chunks return
    // null; we just wait for the rest to arrive.
    let initial = decodeBinaryFrame<unknown>(frameBytes);
    if (!initial) return;
    if (initial.msgType === MSG_CHUNK) {
      const meta = extractChunkMeta(initial as { meta: unknown });
      if (!meta) return;
      let assembled: Uint8Array | null;
      try {
        assembled = chunker.receive(meta as ChunkMeta, initial.payload);
      } catch (err) {
        figma.ui.postMessage({
          type: "chunk_error",
          requestId: meta.requestId,
          error: (err as Error).message ?? String(err),
        });
        return;
      }
      if (!assembled) return;
      frameBytes = assembled;
    }
    const frame = decodeBinaryFrame<{
      type?: string;
      requestId?: string;
      nodeIds?: string[];
      params?: Record<string, unknown>;
      payloadField?: string;
    }>(frameBytes);
    if (!frame) return;
    if (frame.msgType !== MSG_REQ) return;
    const meta = frame.meta;
    const request: any = {
      type: meta.type,
      requestId: meta.requestId,
      nodeIds: meta.nodeIds,
      params: meta.params || {},
    };
    if (meta.payloadField && frame.payload.length > 0) {
      // Splice the raw bytes into the request shape — handlers see exactly
      // the same structure they'd receive from a text request, except the
      // payload field carries a Uint8Array instead of a base64 string.
      splicePayload(request, meta.payloadField, frame.payload);
    }
    const response = await handleRequest(request);
    try {
      sendResponse(response);
    } catch (err) {
      figma.ui.postMessage({
        type: response.type,
        requestId: response.requestId,
        error: err instanceof Error ? err.message : String(err),
      });
    }
  }
};
