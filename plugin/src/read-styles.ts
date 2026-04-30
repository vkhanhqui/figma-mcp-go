import { serializeVariableValue } from "./serializers";

export const handleReadStyleRequest = async (request: any) => {
  switch (request.type) {
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
          text: textStyles.map((s) => {
            const entry: any = {
              id: s.id,
              name: s.name,
              fontSize: s.fontSize,
              fontFamily: s.fontName ? s.fontName.family : undefined,
              fontStyle: s.fontName ? s.fontName.style : undefined,
              textDecoration:
                s.textDecoration !== "NONE" ? s.textDecoration : undefined,
              lineHeight: (s as any).lineHeight,
              letterSpacing: (s as any).letterSpacing,
            };
            const trunc = (s as any).textTruncation;
            if (trunc && trunc !== "DISABLED") entry.textTruncation = trunc;
            const maxLines = (s as any).maxLines;
            if (maxLines != null) entry.maxLines = maxLines;
            return entry;
          }),
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

    case "export_tokens": {
      const format = (request.params && request.params.format) || "json";

      const collections = await figma.variables.getLocalVariableCollectionsAsync();
      const paintStyles = await figma.getLocalPaintStylesAsync();

      if (format === "css") {
        const lines: string[] = [":root {"];
        for (const coll of collections) {
          const firstMode = coll.modes[0];
          if (!firstMode) continue;
          for (const varId of coll.variableIds) {
            const variable = await figma.variables.getVariableByIdAsync(varId);
            if (!variable) continue;
            const val = variable.valuesByMode[firstMode.modeId];
            const cssName = "--" + variable.name.toLowerCase().replace(/[/\s]+/g, "-").replace(/[^a-z0-9-]/g, "");
            let cssValue: string | null = null;
            if (variable.resolvedType === "COLOR" && val && typeof val === "object" && "r" in val) {
              const c = val as RGBA;
              const r = Math.round(c.r * 255);
              const g = Math.round(c.g * 255);
              const b = Math.round(c.b * 255);
              cssValue = c.a < 1 ? `rgba(${r}, ${g}, ${b}, ${c.a.toFixed(2)})` : `rgb(${r}, ${g}, ${b})`;
            } else if (variable.resolvedType === "FLOAT" || variable.resolvedType === "STRING" || variable.resolvedType === "BOOLEAN") {
              cssValue = String(val);
            }
            if (cssValue !== null) lines.push(`  ${cssName}: ${cssValue};`);
          }
        }
        for (const style of paintStyles) {
          if (style.paints.length === 1 && style.paints[0].type === "SOLID") {
            const paint = style.paints[0] as SolidPaint;
            const cssName = "--" + style.name.toLowerCase().replace(/[/\s]+/g, "-").replace(/[^a-z0-9-]/g, "");
            const r = Math.round(paint.color.r * 255);
            const g = Math.round(paint.color.g * 255);
            const b = Math.round(paint.color.b * 255);
            const a = paint.opacity ?? 1;
            const cssValue = a < 1 ? `rgba(${r}, ${g}, ${b}, ${a.toFixed(2)})` : `rgb(${r}, ${g}, ${b})`;
            lines.push(`  ${cssName}: ${cssValue};`);
          }
        }
        lines.push("}");
        return { type: request.type, requestId: request.requestId, data: { css: lines.join("\n") } };
      }

      // JSON format: nested token tree per collection
      const tokens: any = {};
      for (const coll of collections) {
        const collTokens: any = {};
        for (const varId of coll.variableIds) {
          const variable = await figma.variables.getVariableByIdAsync(varId);
          if (!variable) continue;
          const modeValues: any = {};
          for (const mode of coll.modes) {
            modeValues[mode.name] = serializeVariableValue(variable.valuesByMode[mode.modeId]);
          }
          const parts = variable.name.split("/");
          let obj = collTokens;
          for (let i = 0; i < parts.length - 1; i++) {
            if (!obj[parts[i]]) obj[parts[i]] = {};
            obj = obj[parts[i]];
          }
          obj[parts[parts.length - 1]] = { type: variable.resolvedType, value: modeValues };
        }
        tokens[coll.name] = collTokens;
      }
      const styleTokens: any = {};
      for (const style of paintStyles) {
          if (style.paints.length === 1 && style.paints[0].type === "SOLID") {
            const paint = style.paints[0] as SolidPaint;
            const r = Math.round(paint.color.r * 255).toString(16).padStart(2, "0");
            const g = Math.round(paint.color.g * 255).toString(16).padStart(2, "0");
            const b = Math.round(paint.color.b * 255).toString(16).padStart(2, "0");
            const parts = style.name.split("/");
            let obj = styleTokens;
            for (let i = 0; i < parts.length - 1; i++) {
              if (!obj[parts[i]]) obj[parts[i]] = {};
              obj = obj[parts[i]];
            }
            obj[parts[parts.length - 1]] = { type: "COLOR", value: `#${r}${g}${b}` };
          }
      }
      if (Object.keys(styleTokens).length > 0) {
        tokens["_styles"] = { paint: styleTokens };
      }
      return { type: request.type, requestId: request.requestId, data: { tokens } };
    }

    default:
      return null;
  }
};
