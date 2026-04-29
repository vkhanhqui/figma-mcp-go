import { getBounds } from "./serializers";
import { makeSolidPaint, getParentNode, base64ToBytes, applyAutoLayout } from "./write-helpers";

// Module-level cache: content SHA-256 (hex) → Figma imageHash.
// Cleared when plugin reloads. Reuse skips both decode + figma.createImage
// when the same bytes are imported again in this session.
const imageHashCache = new Map<string, string>();

const sha256Hex = async (bytes: Uint8Array): Promise<string> => {
  const subtle = (globalThis as any).crypto?.subtle;
  if (subtle && typeof subtle.digest === "function") {
    const buf = await subtle.digest("SHA-256", bytes.buffer.slice(bytes.byteOffset, bytes.byteOffset + bytes.byteLength));
    const view = new Uint8Array(buf);
    let hex = "";
    for (let i = 0; i < view.length; i++) hex += view[i].toString(16).padStart(2, "0");
    return hex;
  }
  // FNV-1a 64-bit fallback — collision-safe enough for in-session dedupe.
  let h1 = 0x811c9dc5, h2 = 0xcbf29ce4;
  for (let i = 0; i < bytes.length; i++) {
    h1 ^= bytes[i]; h1 = (h1 * 0x01000193) >>> 0;
    h2 ^= bytes[i]; h2 = (h2 * 0x100000001b3) >>> 0;
  }
  return h1.toString(16).padStart(8, "0") + h2.toString(16).padStart(8, "0") + bytes.length.toString(16);
};

const getOrCreateImageHash = async (bytes: Uint8Array): Promise<{ imageHash: string; cached: boolean; contentHash: string }> => {
  const contentHash = await sha256Hex(bytes);
  const cached = imageHashCache.get(contentHash);
  if (cached) return { imageHash: cached, cached: true, contentHash };
  const image = figma.createImage(bytes);
  imageHashCache.set(contentHash, image.hash);
  return { imageHash: image.hash, cached: false, contentHash };
};

interface ImageRectResult {
  id: string;
  name: string;
  type: string;
  bounds: any;
  imageHash: string;
  contentHash?: string;
  cached: boolean;
}

// createImageRect resolves an image (server-side cache hit ⇒ bytes are absent
// and imageHash is provided directly; otherwise decode + createImage with our
// own per-session content cache) and attaches it as an IMAGE fill to a new
// rectangle. Shared by single-import and batch-import handlers so the fast
// path stays consistent.
const createImageRect = async (p: any, parent: any): Promise<ImageRectResult> => {
  let imageHash: string | undefined = p.imageHash;
  let cached = !!imageHash;
  let contentHash: string | undefined = p.contentHash;

  if (!imageHash) {
    if (!p.imageData) throw new Error("imageData (base64) or imageHash is required");
    // Binary frames place a Uint8Array here directly so we skip the base64
    // decode hot path on large icons. Text frames still arrive as base64
    // strings.
    const bytes =
      p.imageData instanceof Uint8Array ? p.imageData : base64ToBytes(p.imageData);
    const result = await getOrCreateImageHash(bytes);
    imageHash = result.imageHash;
    cached = result.cached;
    contentHash = result.contentHash;
  } else if (contentHash) {
    // Server cache hit — mirror it locally so subsequent rounds via this same
    // plugin instance also short-circuit decoding.
    if (!imageHashCache.has(contentHash)) imageHashCache.set(contentHash, imageHash);
  }

  const rect = figma.createRectangle();
  rect.resize(p.width || 200, p.height || 200);
  rect.x = p.x != null ? p.x : 0;
  rect.y = p.y != null ? p.y : 0;
  if (p.name) rect.name = p.name;
  rect.fills = [{ type: "IMAGE", imageHash: imageHash!, scaleMode: p.scaleMode || "FILL" }];
  (parent as any).appendChild(rect);
  return { id: rect.id, name: rect.name, type: rect.type, bounds: getBounds(rect), imageHash: imageHash!, contentHash, cached };
};

export const handleWriteCreateRequest = async (request: any) => {
  switch (request.type) {
    case "create_frame": {
      const p = request.params || {};
      const parent = await getParentNode(p.parentId);
      const frame = figma.createFrame();
      frame.resize(p.width || 100, p.height || 100);
      frame.x = p.x != null ? p.x : 0;
      frame.y = p.y != null ? p.y : 0;
      if (p.name) frame.name = p.name;
      if (p.fillColor) frame.fills = [makeSolidPaint(p.fillColor)];
      applyAutoLayout(frame, p);
      (parent as any).appendChild(frame);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: frame.id, name: frame.name, type: frame.type, bounds: getBounds(frame) },
      };
    }

    case "create_rectangle": {
      const p = request.params || {};
      const parent = await getParentNode(p.parentId);
      const rect = figma.createRectangle();
      rect.resize(p.width || 100, p.height || 100);
      rect.x = p.x != null ? p.x : 0;
      rect.y = p.y != null ? p.y : 0;
      if (p.name) rect.name = p.name;
      if (p.fillColor) rect.fills = [makeSolidPaint(p.fillColor)];
      if (p.cornerRadius != null) rect.cornerRadius = p.cornerRadius;
      (parent as any).appendChild(rect);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: rect.id, name: rect.name, type: rect.type, bounds: getBounds(rect) },
      };
    }

    case "create_ellipse": {
      const p = request.params || {};
      const parent = await getParentNode(p.parentId);
      const ellipse = figma.createEllipse();
      ellipse.resize(p.width || 100, p.height || 100);
      ellipse.x = p.x != null ? p.x : 0;
      ellipse.y = p.y != null ? p.y : 0;
      if (p.name) ellipse.name = p.name;
      if (p.fillColor) ellipse.fills = [makeSolidPaint(p.fillColor)];
      (parent as any).appendChild(ellipse);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: ellipse.id, name: ellipse.name, type: ellipse.type, bounds: getBounds(ellipse) },
      };
    }

    case "create_text": {
      const p = request.params || {};
      const parent = await getParentNode(p.parentId);
      const fontFamily = p.fontFamily || "Inter";
      const fontStyle = p.fontStyle || "Regular";
      await figma.loadFontAsync({ family: fontFamily, style: fontStyle });
      const textNode = figma.createText();
      textNode.fontName = { family: fontFamily, style: fontStyle };
      if (p.fontSize != null) textNode.fontSize = Number(p.fontSize);
      textNode.characters = p.text || "";
      textNode.x = p.x != null ? p.x : 0;
      textNode.y = p.y != null ? p.y : 0;
      if (p.name) textNode.name = p.name;
      if (p.fillColor) textNode.fills = [makeSolidPaint(p.fillColor)];
      (parent as any).appendChild(textNode);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: textNode.id, name: textNode.name, type: textNode.type, bounds: getBounds(textNode) },
      };
    }

    case "import_image": {
      const p = request.params || {};
      const parent = await getParentNode(p.parentId);
      const result = await createImageRect(p, parent);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: result.id, name: result.name, type: result.type, bounds: result.bounds, imageHash: result.imageHash, contentHash: result.contentHash, cached: result.cached },
      };
    }

    case "import_images": {
      const p = request.params || {};
      const parent = await getParentNode(p.parentId);
      const items: any[] = Array.isArray(p.items) ? p.items : [];
      const results: any[] = new Array(items.length);

      // Sequential creation keeps the undo history coherent and the layer
      // ordering deterministic. Image hashing is the actual heavy step and
      // it's already pipelined per-item by the cache.
      for (let i = 0; i < items.length; i++) {
        const item = items[i] || {};
        try {
          const r = await createImageRect(item, parent);
          results[i] = { index: i, success: true, id: r.id, name: r.name, type: r.type, bounds: r.bounds, imageHash: r.imageHash, contentHash: r.contentHash, cached: r.cached };
        } catch (err) {
          results[i] = { index: i, success: false, error: (err as Error).message || String(err) };
        }
        if ((i + 1) % 5 === 0 && i + 1 < items.length) {
          // Progress notification — bridge resets the inactivity timer so big
          // batches don't trip the per-tool timeout.
          figma.ui.postMessage({
            type: "progress_update",
            requestId: request.requestId,
            progress: Math.max(1, Math.floor(((i + 1) / items.length) * 100)),
            message: `${i + 1}/${items.length} imported`,
          });
        }
      }

      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { items: results },
      };
    }

    case "create_component": {
      const p = request.params || {};
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeId is required");
      const node = await figma.getNodeByIdAsync(nodeId) as any;
      if (!node) throw new Error(`Node not found: ${nodeId}`);
      if (node.type !== "FRAME") throw new Error(`Node ${nodeId} is not a FRAME — only frames can be converted to components`);

      const parent = node.parent as any;
      const index = parent.children.indexOf(node);

      const component = figma.createComponent();
      component.name = p.name || node.name;
      component.resize(node.width, node.height);
      component.x = node.x;
      component.y = node.y;
      component.fills = node.fills as Paint[];
      component.strokes = node.strokes as Paint[];
      if (node.cornerRadius != null && node.cornerRadius !== figma.mixed) {
        component.cornerRadius = node.cornerRadius as number;
      }
      if (node.layoutMode && node.layoutMode !== "NONE") {
        component.layoutMode = node.layoutMode;
        component.paddingTop = node.paddingTop;
        component.paddingRight = node.paddingRight;
        component.paddingBottom = node.paddingBottom;
        component.paddingLeft = node.paddingLeft;
        component.itemSpacing = node.itemSpacing;
        component.primaryAxisAlignItems = node.primaryAxisAlignItems;
        component.counterAxisAlignItems = node.counterAxisAlignItems;
      }
      // Move children from frame into component
      for (const child of [...node.children]) {
        component.appendChild(child);
      }
      parent.insertChild(index, component);
      node.remove();

      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: component.id, name: component.name, type: component.type, bounds: getBounds(component) },
      };
    }

    case "create_section": {
      const p = request.params || {};
      const section = figma.createSection();
      if (p.name) section.name = p.name;
      if (p.x != null) section.x = p.x;
      if (p.y != null) section.y = p.y;
      if (p.width != null || p.height != null) {
        section.resizeWithoutConstraints(p.width || section.width, p.height || section.height);
      }
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: section.id, name: section.name, type: section.type, bounds: getBounds(section) },
      };
    }

    default:
      return null;
  }
};
