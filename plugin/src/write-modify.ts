import { getBounds } from "./serializers";
import { makeSolidPaint, getParentNode, applyAutoLayout } from "./write-helpers";

export const handleWriteModifyRequest = async (request: any) => {
  switch (request.type) {
    case "set_text": {
      const p = request.params || {};
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeId is required");
      const hasText = typeof p.text === "string";
      const hasTruncation = p.textTruncation !== undefined;
      const hasMaxLines = p.maxLines !== undefined;
      if (!hasText && !hasTruncation && !hasMaxLines) {
        throw new Error("at least one of text, textTruncation, or maxLines is required");
      }
      const node = await figma.getNodeByIdAsync(nodeId) as any;
      if (!node) throw new Error(`Node not found: ${nodeId}`);
      if (node.type !== "TEXT") throw new Error(`Node ${nodeId} is not a TEXT node`);
      if (hasText) {
        const fontName = typeof node.fontName === "symbol"
          ? { family: "Inter", style: "Regular" }
          : node.fontName;
        await figma.loadFontAsync(fontName);
        node.characters = p.text;
      }
      if (hasTruncation) {
        if (p.textTruncation !== "DISABLED" && p.textTruncation !== "ENDING") {
          throw new Error(`textTruncation must be 'DISABLED' or 'ENDING', got: ${p.textTruncation}`);
        }
        node.textTruncation = p.textTruncation;
      }
      if (hasMaxLines) {
        if (p.maxLines !== null) {
          const n = Number(p.maxLines);
          if (!Number.isFinite(n) || n < 1) {
            throw new Error("maxLines must be null or a positive integer");
          }
          node.maxLines = n;
        } else {
          node.maxLines = null;
        }
      }
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          id: node.id,
          name: node.name,
          characters: node.characters,
          textTruncation: node.textTruncation,
          maxLines: node.maxLines,
        },
      };
    }

    case "set_fills": {
      const p = request.params || {};
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeId is required");
      const node = await figma.getNodeByIdAsync(nodeId);
      if (!node) throw new Error(`Node not found: ${nodeId}`);
      if (!("fills" in node)) throw new Error(`Node ${nodeId} does not support fills`);
      const newFill = makeSolidPaint(p.color, p.opacity != null ? p.opacity : undefined);
      (node as any).fills = p.mode === "append"
        ? [...((node as any).fills as Paint[]), newFill]
        : [newFill];
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: node.id, name: node.name },
      };
    }

    case "set_strokes": {
      const p = request.params || {};
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeId is required");
      const node = await figma.getNodeByIdAsync(nodeId);
      if (!node) throw new Error(`Node not found: ${nodeId}`);
      if (!("strokes" in node)) throw new Error(`Node ${nodeId} does not support strokes`);
      const newStroke = makeSolidPaint(p.color);
      (node as any).strokes = p.mode === "append"
        ? [...((node as any).strokes as Paint[]), newStroke]
        : [newStroke];
      if (p.strokeWeight != null) (node as any).strokeWeight = p.strokeWeight;
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: node.id, name: node.name },
      };
    }

    case "move_nodes": {
      const p = request.params || {};
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        if (!("x" in n)) { results.push({ nodeId: nid, error: "Node does not support position" }); continue; }
        if (p.x != null) n.x = p.x;
        if (p.y != null) n.y = p.y;
        results.push({ nodeId: nid, x: n.x, y: n.y });
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "resize_nodes": {
      const p = request.params || {};
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        if (!("resize" in n)) { results.push({ nodeId: nid, error: "Node does not support resize" }); continue; }
        const w = p.width != null ? p.width : n.width;
        const h = p.height != null ? p.height : n.height;
        n.resize(w, h);
        results.push({ nodeId: nid, width: n.width, height: n.height });
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "rename_node": {
      const p = request.params || {};
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeId is required");
      const node = await figma.getNodeByIdAsync(nodeId);
      if (!node) throw new Error(`Node not found: ${nodeId}`);
      node.name = p.name;
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: node.id, name: node.name },
      };
    }

    case "clone_node": {
      const p = request.params || {};
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeId is required");
      const node = await figma.getNodeByIdAsync(nodeId) as any;
      if (!node) throw new Error(`Node not found: ${nodeId}`);
      const clone = node.clone();
      if (p.x != null) clone.x = p.x;
      if (p.y != null) clone.y = p.y;
      if (p.parentId) {
        const parent = await getParentNode(p.parentId);
        (parent as any).appendChild(clone);
      }
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: clone.id, name: clone.name, type: clone.type, bounds: getBounds(clone) },
      };
    }

    case "set_opacity": {
      const p = request.params || {};
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        if (!("opacity" in n)) { results.push({ nodeId: nid, error: "Node does not support opacity" }); continue; }
        n.opacity = p.opacity;
        results.push({ nodeId: nid, opacity: n.opacity });
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "set_corner_radius": {
      const p = request.params || {};
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        if (!("cornerRadius" in n)) { results.push({ nodeId: nid, error: "Node does not support corner radius" }); continue; }
        if (p.cornerRadius != null) n.cornerRadius = p.cornerRadius;
        if (p.topLeftRadius != null) n.topLeftRadius = p.topLeftRadius;
        if (p.topRightRadius != null) n.topRightRadius = p.topRightRadius;
        if (p.bottomLeftRadius != null) n.bottomLeftRadius = p.bottomLeftRadius;
        if (p.bottomRightRadius != null) n.bottomRightRadius = p.bottomRightRadius;
        results.push({ nodeId: nid, cornerRadius: n.cornerRadius });
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "set_auto_layout": {
      const p = request.params || {};
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeId is required");
      const node = await figma.getNodeByIdAsync(nodeId);
      if (!node) throw new Error(`Node not found: ${nodeId}`);
      if (node.type !== "FRAME") throw new Error(`Node ${nodeId} is not a FRAME`);
      applyAutoLayout(node, p);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: node.id, name: node.name },
      };
    }

    case "set_visible": {
      const p = request.params || {};
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        if (!("visible" in n)) { results.push({ nodeId: nid, error: "Node does not support visibility" }); continue; }
        n.visible = p.visible;
        results.push({ nodeId: nid, visible: n.visible });
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "lock_nodes":
    case "unlock_nodes": {
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const locked = request.type === "lock_nodes";
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        if (!("locked" in n)) { results.push({ nodeId: nid, error: "Node does not support locking" }); continue; }
        n.locked = locked;
        results.push({ nodeId: nid, locked: n.locked });
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "rotate_nodes": {
      const p = request.params || {};
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        if (!("rotation" in n)) { results.push({ nodeId: nid, error: "Node does not support rotation" }); continue; }
        n.rotation = p.rotation;
        results.push({ nodeId: nid, rotation: n.rotation });
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "reorder_nodes": {
      const p = request.params || {};
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const validOrders = ["bringToFront", "sendToBack", "bringForward", "sendBackward"];
      if (!validOrders.includes(p.order)) {
        throw new Error(`order must be bringToFront, sendToBack, bringForward, or sendBackward`);
      }
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        const parent = n.parent as any;
        if (!parent || !("children" in parent)) { results.push({ nodeId: nid, error: "Node has no reorderable parent" }); continue; }
        const siblings = parent.children as any[];
        const currentIndex = siblings.indexOf(n);
        let newIndex: number;
        switch (p.order) {
          case "bringToFront":   newIndex = siblings.length - 1; break;
          case "sendToBack":     newIndex = 0; break;
          case "bringForward":   newIndex = Math.min(currentIndex + 1, siblings.length - 1); break;
          case "sendBackward":   newIndex = Math.max(currentIndex - 1, 0); break;
          default:               newIndex = currentIndex;
        }
        parent.insertChild(newIndex, n);
        results.push({ nodeId: nid, index: newIndex });
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "set_blend_mode": {
      const p = request.params || {};
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        if (!("blendMode" in n)) { results.push({ nodeId: nid, error: "Node does not support blend mode" }); continue; }
        n.blendMode = p.blendMode;
        results.push({ nodeId: nid, blendMode: n.blendMode });
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "set_constraints": {
      const p = request.params || {};
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        if (!("constraints" in n)) { results.push({ nodeId: nid, error: "Node does not support constraints" }); continue; }
        const updated: any = { ...n.constraints };
        if (p.horizontal) updated.horizontal = p.horizontal;
        if (p.vertical)   updated.vertical   = p.vertical;
        n.constraints = updated;
        results.push({ nodeId: nid, constraints: n.constraints });
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "reparent_nodes": {
      const p = request.params || {};
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      if (!p.parentId) throw new Error("parentId is required");
      const newParent = await figma.getNodeByIdAsync(p.parentId) as any;
      if (!newParent) throw new Error(`Parent not found: ${p.parentId}`);
      if (!("appendChild" in newParent)) throw new Error(`Node ${p.parentId} cannot contain children`);
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        try {
          newParent.appendChild(n);
          results.push({ nodeId: nid, newParentId: p.parentId });
        } catch (e: any) {
          results.push({ nodeId: nid, error: e.message });
        }
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "batch_rename_nodes": {
      const p = request.params || {};
      const nodeIds = request.nodeIds || [];
      if (nodeIds.length === 0) throw new Error("nodeIds is required");
      const results: any[] = [];
      for (const nid of nodeIds) {
        const n = await figma.getNodeByIdAsync(nid) as any;
        if (!n) { results.push({ nodeId: nid, error: "Node not found" }); continue; }
        const oldName: string = n.name;
        let newName = oldName;
        if (p.find !== undefined && p.replace !== undefined) {
          if (p.useRegex) {
            try {
              const regex = new RegExp(p.find, p.regexFlags || "g");
              newName = newName.replace(regex, p.replace);
            } catch (e: any) {
              results.push({ nodeId: nid, error: `Invalid regex: ${e.message}` }); continue;
            }
          } else {
            newName = newName.split(p.find).join(p.replace);
          }
        }
        if (p.prefix) newName = p.prefix + newName;
        if (p.suffix) newName = newName + p.suffix;
        n.name = newName;
        results.push({ nodeId: nid, oldName, name: newName });
      }
      figma.commitUndo();
      return { type: request.type, requestId: request.requestId, data: { results } };
    }

    case "find_replace_text": {
      const p = request.params || {};
      if (!p.find) throw new Error("find is required");
      if (p.replace === undefined) throw new Error("replace is required");
      const rootNodeId = request.nodeIds && request.nodeIds[0];
      const root: any = rootNodeId
        ? await figma.getNodeByIdAsync(rootNodeId)
        : figma.currentPage;
      if (!root) throw new Error(`Root node not found: ${rootNodeId}`);
      const textNodes: any[] = [];
      const collect = (node: any) => {
        if (node.type === "TEXT") textNodes.push(node);
        if ("children" in node) (node.children as any[]).forEach(collect);
      };
      collect(root);
      const results: any[] = [];
      for (const tn of textNodes) {
        const originalText: string = tn.characters;
        let newText: string;
        if (p.useRegex) {
          try {
            const regex = new RegExp(p.find, p.regexFlags || "g");
            newText = originalText.replace(regex, p.replace);
          } catch (e: any) {
            results.push({ nodeId: tn.id, nodeName: tn.name, error: `Invalid regex: ${e.message}` });
            continue;
          }
        } else {
          newText = originalText.split(p.find).join(p.replace);
        }
        if (newText !== originalText) {
          const fontName = typeof tn.fontName === "symbol"
            ? { family: "Inter", style: "Regular" }
            : tn.fontName;
          await figma.loadFontAsync(fontName);
          tn.characters = newText;
          results.push({ nodeId: tn.id, nodeName: tn.name, oldText: originalText, newText });
        }
      }
      figma.commitUndo();
      const successCount = results.filter((r: any) => !r.error).length;
      return { type: request.type, requestId: request.requestId, data: { replaced: successCount, results } };
    }

    default:
      return null;
  }
};
