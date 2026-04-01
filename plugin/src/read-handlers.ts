// Read handlers — all read-only Figma operations.
// Returns null for unknown request types so the caller can try write handlers next.

import { serializeNode, getBounds, serializeStyles, serializeVariableValue } from "./serializers";

export const handleReadRequest = async (request: any) => {
  switch (request.type) {
    case "get_document":
      return {
        type: request.type,
        requestId: request.requestId,
        data: await serializeNode(figma.currentPage),
      };

    case "get_selection":
      return {
        type: request.type,
        requestId: request.requestId,
        data: await Promise.all(figma.currentPage.selection.map((node) => serializeNode(node))),
      };

    case "get_node": {
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeIds is required for get_node");
      const node = await figma.getNodeByIdAsync(nodeId);
      if (!node || node.type === "DOCUMENT")
        throw new Error(`Node not found: ${nodeId}`);
      return {
        type: request.type,
        requestId: request.requestId,
        data: await serializeNode(node),
      };
    }

    case "get_styles": {
      const [paintStyles, textStyles, effectStyles, gridStyles] =
        await Promise.all([
          figma.getLocalPaintStylesAsync(),
          figma.getLocalTextStylesAsync(),
          figma.getLocalEffectStylesAsync(),
          figma.getLocalGridStylesAsync(),
        ]);
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          paints: paintStyles.map((s) => ({
            id: s.id,
            name: s.name,
            paints: s.paints,
          })),
          text: textStyles.map((s) => ({
            id: s.id,
            name: s.name,
            fontSize: s.fontSize,
            fontFamily: s.fontName ? s.fontName.family : undefined,
            fontStyle: s.fontName ? s.fontName.style : undefined,
            textDecoration:
              s.textDecoration !== "NONE" ? s.textDecoration : undefined,
            lineHeight: (s as any).lineHeight,
            letterSpacing: (s as any).letterSpacing,
          })),
          effects: effectStyles.map((s) => ({
            id: s.id,
            name: s.name,
            effects: s.effects,
          })),
          grids: gridStyles.map((s) => ({
            id: s.id,
            name: s.name,
            layoutGrids: s.layoutGrids,
          })),
        },
      };
    }

    case "get_metadata":
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          fileName: figma.root.name,
          currentPageId: figma.currentPage.id,
          currentPageName: figma.currentPage.name,
          pageCount: figma.root.children.length,
          pages: figma.root.children.map((page) => ({
            id: page.id,
            name: page.name,
          })),
        },
      };

    case "get_design_context": {
      const depth =
        request.params && request.params.depth != null
          ? request.params.depth
          : 2;
      const detail = (request.params && request.params.detail) || "full";

      const serializeForDetail = async (n: any) => {
        const base = { id: n.id, name: n.name, type: n.type, bounds: getBounds(n) };
        if (detail === "minimal") return base;
        const styles = await serializeStyles(n);
        const result: any = Object.assign({}, base);
        if (Object.keys(styles).length > 0) result.styles = styles;
        if ("opacity" in n && n.opacity !== 1) result.opacity = n.opacity;
        if ("visible" in n && !n.visible) result.visible = false;
        if (detail === "compact") return result;
        return await serializeNode(n);
      };

      const serializeWithDepth = async (node: any, currentDepth: number): Promise<any> => {
        if (detail === "full") {
          const serialized = await serializeNode(node);
          if (currentDepth >= depth && serialized.children) {
            return Object.assign({}, serialized, {
              children: undefined,
              childCount: node.children ? node.children.length : 0,
            });
          }
          if (serialized.children) {
            const childNodes = await Promise.all(
              serialized.children.map((child: any) =>
                figma.getNodeByIdAsync(child.id),
              ),
            );
            const serializedChildren = await Promise.all(
              childNodes
                .filter((n) => n !== null && n.type !== "DOCUMENT")
                .map((n) => serializeWithDepth(n, currentDepth + 1)),
            );
            return Object.assign({}, serialized, { children: serializedChildren });
          }
          return serialized;
        }

        const serialized = await serializeForDetail(node);
        const hasChildren = "children" in node && node.children.length > 0;
        if (!hasChildren) return serialized;
        if (currentDepth >= depth) {
          return Object.assign({}, serialized, { childCount: node.children.length });
        }
        const serializedChildren = await Promise.all(
          node.children
            .filter((n: any) => n.type !== "DOCUMENT")
            .map((n: any) => serializeWithDepth(n, currentDepth + 1)),
        );
        return Object.assign({}, serialized, { children: serializedChildren });
      };

      const selection = figma.currentPage.selection;
      const contextNodes =
        selection.length > 0
          ? await Promise.all(
              selection.map((node) => serializeWithDepth(node, 0)),
            )
          : [await serializeWithDepth(figma.currentPage, 0)];
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          fileName: figma.root.name,
          currentPage: {
            id: figma.currentPage.id,
            name: figma.currentPage.name,
          },
          selectionCount: selection.length,
          context: contextNodes,
        },
      };
    }

    case "get_variable_defs": {
      const collections =
        await figma.variables.getLocalVariableCollectionsAsync();
      const variableData = await Promise.all(
        collections.map(async (collection) => {
          const variables = await Promise.all(
            collection.variableIds.map((id) =>
              figma.variables.getVariableByIdAsync(id),
            ),
          );
          return {
            id: collection.id,
            name: collection.name,
            modes: collection.modes.map((mode) => ({
              modeId: mode.modeId,
              name: mode.name,
            })),
            variables: variables
              .filter((v) => v !== null)
              .map((variable) => ({
                id: variable!.id,
                name: variable!.name,
                resolvedType: variable!.resolvedType,
                valuesByMode: Object.fromEntries(
                  Object.entries(variable!.valuesByMode).map(
                    ([modeId, value]) => [
                      modeId,
                      serializeVariableValue(value),
                    ],
                  ),
                ),
              })),
          };
        }),
      );
      return {
        type: request.type,
        requestId: request.requestId,
        data: { collections: variableData },
      };
    }

    case "get_screenshot": {
      const format =
        request.params && request.params.format
          ? request.params.format
          : "PNG";
      const scale =
        request.params && request.params.scale != null
          ? request.params.scale
          : 2;
      let targetNodes: any[];
      if (request.nodeIds && request.nodeIds.length > 0) {
        const nodes = await Promise.all(
          request.nodeIds.map((id: string) => figma.getNodeByIdAsync(id)),
        );
        targetNodes = nodes.filter(
          (n) => n !== null && n.type !== "DOCUMENT" && n.type !== "PAGE",
        );
      } else {
        targetNodes = figma.currentPage.selection.slice();
      }
      if (targetNodes.length === 0)
        throw new Error(
          "No nodes to export. Select nodes or provide nodeIds.",
        );
      const exports = await Promise.all(
        targetNodes.map(async (node: any) => {
          const settings: any =
            format === "SVG"
              ? { format: "SVG" }
              : format === "PDF"
                ? { format: "PDF" }
                : format === "JPG"
                  ? {
                      format: "JPG",
                      constraint: { type: "SCALE", value: scale },
                    }
                  : {
                      format: "PNG",
                      constraint: { type: "SCALE", value: scale },
                    };
          const bytes = await node.exportAsync(settings);
          const base64 = figma.base64Encode(bytes);
          return {
            nodeId: node.id,
            nodeName: node.name,
            format,
            base64,
            width: node.width,
            height: node.height,
          };
        }),
      );
      return {
        type: request.type,
        requestId: request.requestId,
        data: { exports },
      };
    }

    case "get_nodes_info": {
      if (!request.nodeIds || request.nodeIds.length === 0)
        throw new Error("nodeIds is required for get_nodes_info");
      const nodes = await Promise.all(
        request.nodeIds.map((id: string) => figma.getNodeByIdAsync(id)),
      );
      return {
        type: request.type,
        requestId: request.requestId,
        data: await Promise.all(
          nodes
            .filter((n) => n !== null && n.type !== "DOCUMENT")
            .map((n) => serializeNode(n)),
        ),
      };
    }

    case "get_local_components": {
      const pages = figma.root.children;
      const allComponents: any[] = [];
      const componentSetsMap = new Map<string, any>();
      for (let i = 0; i < pages.length; i++) {
        const page = pages[i];
        await page.loadAsync();
        const pageNodes = page.findAllWithCriteria({
          types: ["COMPONENT", "COMPONENT_SET"],
        });
        for (const n of pageNodes) {
          if (n.type === "COMPONENT_SET") {
            componentSetsMap.set(n.id, {
              id: n.id,
              name: n.name,
              key: "key" in n ? n.key : null,
            });
          } else {
            const parentIsSet =
              n.parent && n.parent.type === "COMPONENT_SET";
            allComponents.push({
              id: n.id,
              name: n.name,
              key: "key" in n ? n.key : null,
              componentSetId: parentIsSet ? n.parent!.id : null,
              variantProperties:
                "variantProperties" in n ? n.variantProperties : null,
            });
          }
        }
        figma.ui.postMessage({
          type: "progress_update",
          requestId: request.requestId,
          progress: Math.round(((i + 1) / pages.length) * 90) + 1,
          message: `Scanned ${page.name}: ${allComponents.length} components so far`,
        });
        await new Promise((r) => setTimeout(r, 0));
      }
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          count: allComponents.length,
          components: allComponents,
          componentSets: Array.from(componentSetsMap.values()),
        },
      };
    }

    case "get_pages":
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          currentPageId: figma.currentPage.id,
          pages: figma.root.children.map((page) => ({
            id: page.id,
            name: page.name,
          })),
        },
      };

    case "get_viewport":
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          center: { x: figma.viewport.center.x, y: figma.viewport.center.y },
          zoom: figma.viewport.zoom,
          bounds: {
            x: figma.viewport.bounds.x,
            y: figma.viewport.bounds.y,
            width: figma.viewport.bounds.width,
            height: figma.viewport.bounds.height,
          },
        },
      };

    case "get_fonts": {
      const fontMap = new Map<string, any>();
      const collectFonts = (n: any) => {
        if (n.type === "TEXT") {
          const fontName = n.fontName;
          if (typeof fontName !== "symbol" && fontName) {
            const key = `${fontName.family}::${fontName.style}`;
            if (!fontMap.has(key)) {
              fontMap.set(key, { family: fontName.family, style: fontName.style, nodeCount: 0 });
            }
            fontMap.get(key).nodeCount++;
          }
        }
        if ("children" in n) n.children.forEach(collectFonts);
      };
      collectFonts(figma.currentPage);
      const fonts = Array.from(fontMap.values()).sort((a, b) => b.nodeCount - a.nodeCount);
      return {
        type: request.type,
        requestId: request.requestId,
        data: { count: fonts.length, fonts },
      };
    }

    case "search_nodes": {
      const query = request.params && request.params.query
        ? request.params.query.toLowerCase()
        : "";
      const scopeNodeId = request.params && request.params.nodeId;
      const types = request.params && request.params.types ? request.params.types : [];
      const limit = request.params && request.params.limit ? request.params.limit : 50;
      const root = scopeNodeId
        ? await figma.getNodeByIdAsync(scopeNodeId)
        : figma.currentPage;
      if (!root) throw new Error(`Node not found: ${scopeNodeId}`);
      const results: any[] = [];
      const search = async (n: any) => {
        if (results.length >= limit) return;
        if (n !== root) {
          const nameMatch = !query || n.name.toLowerCase().includes(query);
          const typeMatch = types.length === 0 || types.includes(n.type);
          if (nameMatch && typeMatch) {
            results.push({
              id: n.id,
              name: n.name,
              type: n.type,
              bounds: getBounds(n),
            });
          }
        }
        if (results.length < limit && "children" in n) {
          for (const child of n.children) await search(child);
        }
      };
      await search(root);
      return {
        type: request.type,
        requestId: request.requestId,
        data: { count: results.length, nodes: results },
      };
    }

    case "get_reactions": {
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeId is required for get_reactions");
      const node = await figma.getNodeByIdAsync(nodeId);
      if (!node || node.type === "DOCUMENT") throw new Error(`Node not found: ${nodeId}`);
      const reactions = "reactions" in node ? node.reactions : [];
      return {
        type: request.type,
        requestId: request.requestId,
        data: { nodeId: node.id, name: node.name, reactions },
      };
    }

    case "get_annotations": {
      const nodeId = request.params && request.params.nodeId;
      const nodeAnnotations = (n: any) => {
        const anns = n.annotations;
        return Array.isArray(anns) ? anns : null;
      };
      if (nodeId) {
        const node = await figma.getNodeByIdAsync(nodeId);
        if (!node) throw new Error(`Node not found: ${nodeId}`);
        const mergedAnnotations: any[] = [];
        const collect = async (n: any) => {
          const anns = nodeAnnotations(n);
          if (anns)
            for (const a of anns)
              mergedAnnotations.push({ nodeId: n.id, annotation: a });
          if ("children" in n)
            for (const child of n.children) await collect(child);
        };
        await collect(node);
        return {
          type: request.type,
          requestId: request.requestId,
          data: {
            nodeId: node.id,
            name: node.name,
            annotations: mergedAnnotations,
          },
        };
      }
      const annotated: any[] = [];
      const processNode = async (n: any) => {
        const anns = nodeAnnotations(n);
        if (anns && anns.length > 0)
          annotated.push({ nodeId: n.id, name: n.name, annotations: anns });
        if ("children" in n)
          for (const child of n.children) await processNode(child);
      };
      await processNode(figma.currentPage);
      return {
        type: request.type,
        requestId: request.requestId,
        data: { annotatedNodes: annotated },
      };
    }

    case "scan_text_nodes": {
      const nodeId = request.params && request.params.nodeId;
      if (!nodeId) throw new Error("nodeId is required for scan_text_nodes");
      const root = await figma.getNodeByIdAsync(nodeId);
      if (!root) throw new Error(`Node not found: ${nodeId}`);
      const textNodes: any[] = [];
      const findText = async (n: any) => {
        if (n.type === "TEXT") {
          textNodes.push({
            id: n.id,
            name: n.name,
            characters: n.characters,
            fontSize: n.fontSize === figma.mixed ? "mixed" : n.fontSize,
            fontName: n.fontName === figma.mixed ? "mixed" : n.fontName,
          });
        }
        if ("children" in n)
          for (const child of n.children) await findText(child);
      };
      figma.ui.postMessage({
        type: "progress_update",
        requestId: request.requestId,
        progress: 10,
        message: "Scanning text nodes...",
      });
      await new Promise((r) => setTimeout(r, 0));
      await findText(root);
      return {
        type: request.type,
        requestId: request.requestId,
        data: { count: textNodes.length, textNodes },
      };
    }

    case "scan_nodes_by_types": {
      const nodeId = request.params && request.params.nodeId;
      const types =
        request.params && request.params.types ? request.params.types : [];
      if (!nodeId)
        throw new Error("nodeId is required for scan_nodes_by_types");
      if (types.length === 0)
        throw new Error("types must be a non-empty array");
      const root = await figma.getNodeByIdAsync(nodeId);
      if (!root) throw new Error(`Node not found: ${nodeId}`);
      const matchingNodes: any[] = [];
      const findByTypes = async (n: any) => {
        if ("visible" in n && !n.visible) return;
        if (types.includes(n.type)) {
          matchingNodes.push({
            id: n.id,
            name: n.name,
            type: n.type,
            bbox: {
              x: "x" in n ? n.x : 0,
              y: "y" in n ? n.y : 0,
              width: "width" in n ? n.width : 0,
              height: "height" in n ? n.height : 0,
            },
          });
        }
        if ("children" in n)
          for (const child of n.children) await findByTypes(child);
      };
      figma.ui.postMessage({
        type: "progress_update",
        requestId: request.requestId,
        progress: 10,
        message: `Scanning for types: ${types.join(", ")}...`,
      });
      await new Promise((r) => setTimeout(r, 0));
      await findByTypes(root);
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          count: matchingNodes.length,
          matchingNodes,
          searchedTypes: types,
        },
      };
    }

    default:
      return null;
  }
};
