export const handleReadExportRequest = async (request: any) => {
  switch (request.type) {
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
          // Hand raw Uint8Array up to the dispatcher; main.ts encodes a
          // binary frame so we skip the figma.base64Encode hot path on
          // multi-MB screenshots. Server-side decodeImageBytes accepts
          // either binary `bytes` or legacy `base64` so older clients keep
          // working.
          return {
            nodeId: node.id,
            nodeName: node.name,
            format,
            bytes,
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

    case "export_nodes_batch": {
      // Per-item export settings. Used by save_screenshots so the Go side
      // gets one round-trip for the whole batch instead of N. Errors are
      // returned per-item rather than thrown so partial success is preserved.
      const items: any[] = Array.isArray(request.params?.items) ? request.params.items : [];
      const defaultFormat: string = request.params?.format || "PNG";
      const defaultScale: number = request.params?.scale != null ? Number(request.params.scale) : 2;

      const results = await Promise.all(items.map(async (item: any) => {
        const idx = item?.index ?? -1;
        const nodeId: string = item?.nodeId;
        const format: string = (item?.format || defaultFormat).toUpperCase();
        const scale: number = item?.scale != null ? Number(item.scale) : defaultScale;
        try {
          if (!nodeId) throw new Error("nodeId is required");
          const node: any = await figma.getNodeByIdAsync(nodeId);
          if (!node) throw new Error(`Node ${nodeId} not found`);
          if (node.type === "DOCUMENT" || node.type === "PAGE") throw new Error(`Node ${nodeId} (${node.type}) is not exportable`);
          const settings: any = format === "SVG"
            ? { format: "SVG" }
            : format === "PDF"
              ? { format: "PDF" }
              : { format, constraint: { type: "SCALE", value: scale } };
          const bytes = await node.exportAsync(settings);
          return {
            index: idx,
            success: true,
            nodeId: node.id,
            nodeName: node.name,
            format,
            bytes,
            width: node.width,
            height: node.height,
          };
        } catch (err) {
          return {
            index: idx,
            success: false,
            nodeId,
            error: (err as Error).message || String(err),
          };
        }
      }));

      return {
        type: request.type,
        requestId: request.requestId,
        data: { results },
      };
    }

    case "export_frames_to_pdf": {
      const nodeIds: string[] = request.nodeIds ?? [];
      if (nodeIds.length === 0) {
        throw new Error("nodeIds is required and must not be empty");
      }
      const frames: any[] = [];
      for (const id of nodeIds) {
        const node = await figma.getNodeByIdAsync(id);
        if (!node || node.type === "DOCUMENT" || node.type === "PAGE") {
          throw new Error(`Node ${id} not found or is not exportable`);
        }
        const bytes = await (node as any).exportAsync({ format: "PDF" });
        frames.push({
          nodeId: node.id,
          nodeName: node.name,
          bytes,
        });
      }
      return {
        type: request.type,
        requestId: request.requestId,
        data: { frames },
      };
    }

    default:
      return null;
  }
};
