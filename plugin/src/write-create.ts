import { getBounds } from "./serializers";
import { makeSolidPaint, getParentNode, base64ToBytes, applyAutoLayout } from "./write-helpers";

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
      if (p.textTruncation !== undefined) {
        if (p.textTruncation !== "DISABLED" && p.textTruncation !== "ENDING") {
          throw new Error(`textTruncation must be 'DISABLED' or 'ENDING', got: ${p.textTruncation}`);
        }
        textNode.textTruncation = p.textTruncation;
      }
      if (p.maxLines !== undefined) {
        if (p.maxLines !== null) {
          const n = Number(p.maxLines);
          if (!Number.isFinite(n) || n < 1) {
            throw new Error("maxLines must be null or a positive integer");
          }
          textNode.maxLines = n;
        } else {
          textNode.maxLines = null;
        }
      }
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
      if (!p.imageData) throw new Error("imageData (base64) is required");
      const parent = await getParentNode(p.parentId);
      const bytes = base64ToBytes(p.imageData);
      const image = figma.createImage(bytes);
      const rect = figma.createRectangle();
      rect.resize(p.width || 200, p.height || 200);
      rect.x = p.x != null ? p.x : 0;
      rect.y = p.y != null ? p.y : 0;
      if (p.name) rect.name = p.name;
      rect.fills = [{ type: "IMAGE", imageHash: image.hash, scaleMode: p.scaleMode || "FILL" }];
      (parent as any).appendChild(rect);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: rect.id, name: rect.name, type: rect.type, bounds: getBounds(rect) },
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

    case "create_instance": {
      const p = request.params || {};
      const parent = await getParentNode(p.parentId);

      let component: ComponentNode | null = null;

      if (p.componentKey && typeof p.componentKey === "string") {
        try {
          component = await figma.importComponentByKeyAsync(p.componentKey);
        } catch (err) {
          throw new Error(`Failed to import component by key '${p.componentKey}': ${err instanceof Error ? err.message : String(err)}`);
        }
      } else if (p.componentId && typeof p.componentId === "string") {
        const node = await figma.getNodeByIdAsync(p.componentId);
        if (!node) throw new Error(`Component not found: ${p.componentId}`);
        if (node.type === "COMPONENT") {
          component = node as ComponentNode;
        } else if (node.type === "COMPONENT_SET") {
          // Pick a variant: explicit variantName/properties match, or the default (first child component).
          const set = node as ComponentSetNode;
          const variants = set.children.filter((c): c is ComponentNode => c.type === "COMPONENT");
          if (variants.length === 0) throw new Error(`Component set ${p.componentId} has no variants`);
          if (p.variantProperties && typeof p.variantProperties === "object") {
            const wanted = p.variantProperties as Record<string, string>;
            const match = variants.find((v) => {
              const vp = v.variantProperties;
              if (!vp) return false;
              for (const k of Object.keys(wanted)) if (vp[k] !== wanted[k]) return false;
              return true;
            });
            if (!match) throw new Error(`No variant in set ${p.componentId} matches properties: ${JSON.stringify(wanted)}`);
            component = match;
          } else {
            component = (set as any).defaultVariant ?? variants[0];
          }
        } else if (node.type === "INSTANCE") {
          throw new Error(`Node ${p.componentId} is an INSTANCE, not a COMPONENT — pass its mainComponentId, or use clone_node if you only need a visual copy`);
        } else {
          throw new Error(`Node ${p.componentId} is type ${node.type} — must be COMPONENT or COMPONENT_SET`);
        }
      } else {
        throw new Error("componentId or componentKey is required");
      }

      const instance = component!.createInstance();
      if (p.x != null) instance.x = Number(p.x);
      if (p.y != null) instance.y = Number(p.y);
      if (p.name) instance.name = p.name;
      (parent as any).appendChild(instance);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          id: instance.id,
          name: instance.name,
          type: instance.type,
          bounds: getBounds(instance),
          mainComponentId: component!.id,
          mainComponentKey: component!.key,
        },
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
