// Serializers — shared read/write helpers for converting Figma node data to JSON.

export const isMixed = (value: any) => typeof value === "symbol";

// Round floating-point pixel values to 2 decimal places.
// Figma sometimes returns values like 123.99999999999999 instead of 124.
const pixelRound = (v: number) => Math.round(v * 100) / 100;

export const toHex = (color: any) => {
  const clamp = (v: any) => Math.min(255, Math.max(0, Math.round(v * 255)));
  const [r, g, b] = [clamp(color.r), clamp(color.g), clamp(color.b)];
  return `#${[r, g, b].map((v) => v.toString(16).padStart(2, "0")).join("")}`;
};

export const serializePaints = (paints: any) => {
  if (isMixed(paints)) return "mixed";

  if (!paints || !Array.isArray(paints)) return undefined;

  const result = paints
    .filter((paint: any) => paint.type === "SOLID" && "color" in paint)
    .map((paint: any) => {
      const hex = toHex(paint.color);
      const opacity = paint.opacity != null ? paint.opacity : 1;
      if (opacity === 1) return hex;
      return (
        hex +
        Math.round(opacity * 255)
          .toString(16)
          .padStart(2, "0")
      );
    });

  return result.length > 0 ? result : undefined;
};

export const getBounds = (node: any) => {
  if ("x" in node && "y" in node && "width" in node && "height" in node) {
    return {
      x: pixelRound(node.x),
      y: pixelRound(node.y),
      width: pixelRound(node.width),
      height: pixelRound(node.height),
    };
  }

  return undefined;
};

// Resolve a single VariableAlias { type, id } to the variable's name, or null.
const resolveVariableName = async (alias: any): Promise<string | null> => {
  if (!alias || !alias.id) return null;
  try {
    const variable = await figma.variables.getVariableByIdAsync(alias.id);
    return variable?.name ?? null;
  } catch {
    return null;
  }
};

// Resolve an array of VariableAlias objects to an array of variable names.
const resolveVariableNames = async (aliases: any[]): Promise<(string | null)[]> => {
  return Promise.all(aliases.map(resolveVariableName));
};

export const serializeStyles = async (node: any) => {
  const styles: any = {};
  const bv = node.boundVariables;

  if ("fills" in node) {
    if (node.fillStyleId && typeof node.fillStyleId === "string") {
      const style = await figma.getStyleByIdAsync(node.fillStyleId);
      if (style) styles.fillStyle = style.name;
    }
    const fills = serializePaints(node.fills);
    if (fills !== undefined) styles.fills = fills;

    if (bv?.fills && Array.isArray(bv.fills)) {
      const names = await resolveVariableNames(bv.fills);
      if (names.some((n) => n !== null)) styles.fillVariables = names;
    } else if (!isMixed(node.fills) && Array.isArray(node.fills)) {
      const paintVarNames = await Promise.all(
        node.fills.map(async (p: any) => {
          const alias = p.boundVariables?.color;
          if (!alias) return null;
          return resolveVariableName(alias);
        })
      );
      if (paintVarNames.some((n: any) => n !== null)) styles.fillVariables = paintVarNames;
    }
  }

  if ("strokes" in node) {
    if (node.strokeStyleId && typeof node.strokeStyleId === "string") {
      const style = await figma.getStyleByIdAsync(node.strokeStyleId);
      if (style) styles.strokeStyle = style.name;
    }
    const strokes = serializePaints(node.strokes);
    if (strokes !== undefined) styles.strokes = strokes;

    if (bv?.strokes && Array.isArray(bv.strokes)) {
      const names = await resolveVariableNames(bv.strokes);
      if (names.some((n) => n !== null)) styles.strokeVariables = names;
    } else if (!isMixed(node.strokes) && Array.isArray(node.strokes)) {
      const strokeVarNames = await Promise.all(
        node.strokes.map(async (p: any) => {
          const alias = p.boundVariables?.color;
          if (!alias) return null;
          return resolveVariableName(alias);
        })
      );
      if (strokeVarNames.some((n: any) => n !== null)) styles.strokeVariables = strokeVarNames;
    }
  }

  if ("cornerRadius" in node) {
    const cr = isMixed(node.cornerRadius) ? "mixed" : node.cornerRadius;
    if (cr !== 0) styles.cornerRadius = cr;
  }

  if ("paddingLeft" in node) {
    styles.padding = {
      top: node.paddingTop,
      right: node.paddingRight,
      bottom: node.paddingBottom,
      left: node.paddingLeft,
    };
  }

  // Resolve scalar bound variables (opacity, cornerRadius, padding, itemSpacing, etc.)
  if (bv) {
    const scalarProps = [
      "opacity", "cornerRadius", "itemSpacing",
      "paddingTop", "paddingRight", "paddingBottom", "paddingLeft",
    ];
    for (const prop of scalarProps) {
      if (bv[prop]) {
        const name = await resolveVariableName(bv[prop]);
        if (name) styles[`${prop}Variable`] = name;
      }
    }
  }

  return styles;
};

export const serializeLineHeight = (lineHeight: any) => {
  if (isMixed(lineHeight)) return "mixed";

  if (!lineHeight || lineHeight.unit === "AUTO") return undefined;

  return { value: lineHeight.value, unit: lineHeight.unit };
};

export const serializeLetterSpacing = (letterSpacing: any) => {
  if (isMixed(letterSpacing)) return "mixed";

  if (!letterSpacing || letterSpacing.value === 0) return undefined;

  return { value: letterSpacing.value, unit: letterSpacing.unit };
};

export const serializeText = async (node: any, base: any) => {
  let fontFamily: any;
  let fontStyle: any;

  if (typeof node.fontName === "symbol") {
    fontFamily = "mixed";
    fontStyle = "mixed";
  } else if (node.fontName) {
    fontFamily = node.fontName.family;
    fontStyle = node.fontName.style;
  }

  const textStyleName =
    node.textStyleId && typeof node.textStyleId === "string"
      ? ((await figma.getStyleByIdAsync(node.textStyleId))?.name ?? undefined)
      : undefined;

  return Object.assign({}, base, {
    characters: node.characters,
    styles: Object.assign({}, base.styles, {
      ...(textStyleName ? { textStyle: textStyleName } : {}),
      fontSize: isMixed(node.fontSize) ? "mixed" : node.fontSize,
      fontFamily,
      fontStyle,
      fontWeight: isMixed(node.fontWeight) ? "mixed" : node.fontWeight,
      textDecoration: isMixed(node.textDecoration)
        ? "mixed"
        : node.textDecoration !== "NONE"
          ? node.textDecoration
          : undefined,
      lineHeight: serializeLineHeight(node.lineHeight),
      letterSpacing: serializeLetterSpacing(node.letterSpacing),
      textAlignHorizontal: isMixed(node.textAlignHorizontal)
        ? "mixed"
        : node.textAlignHorizontal,
    }),
  });
};

export const serializeNode = async (node: any): Promise<any> => {
  const styles = await serializeStyles(node);
  const base = {
    id: node.id,
    name: node.name,
    type: node.type,
    bounds: getBounds(node),
    styles,
  };
  if (node.type === "TEXT") return serializeText(node, base);
  if ("children" in node) {
    return Object.assign({}, base, {
      children: await Promise.all(node.children.map((child: any) => serializeNode(child))),
    });
  }
  return base;
};

// deduplicateStyles does a two-pass walk over a serialized node tree.
// First pass: count how many times each fills/strokes array value appears.
// Second pass: replace values that appear more than once with a short ref key.
// Returns the rewritten tree and a globalVars.styles map (or undefined if nothing was deduped).
export const deduplicateStyles = (tree: any): { tree: any; globalVars: Record<string, any> | undefined } => {
  // Pass 1: count occurrences of each serialized fill/stroke value
  const counts = new Map<string, number>();
  const countWalk = (node: any) => {
    if (!node || typeof node !== "object") return;
    const s = node.styles;
    if (s) {
      if (Array.isArray(s.fills)) counts.set(JSON.stringify(s.fills), (counts.get(JSON.stringify(s.fills)) ?? 0) + 1);
      if (Array.isArray(s.strokes)) counts.set(JSON.stringify(s.strokes), (counts.get(JSON.stringify(s.strokes)) ?? 0) + 1);
    }
    if (Array.isArray(node.children)) node.children.forEach(countWalk);
  };
  countWalk(tree);

  // Build ref map for values that appear more than once
  let counter = 0;
  const keyToRef = new Map<string, string>();
  const refs: Record<string, any> = {};
  for (const [key, count] of counts) {
    if (count > 1) {
      const ref = `s${++counter}`;
      keyToRef.set(key, ref);
      refs[ref] = JSON.parse(key);
    }
  }
  if (keyToRef.size === 0) return { tree, globalVars: undefined };

  // Pass 2: replace repeated values with ref keys
  const replaceWalk = (node: any): any => {
    if (!node || typeof node !== "object") return node;
    let result = node;
    const s = node.styles;
    if (s) {
      let newStyles = s;
      if (Array.isArray(s.fills)) {
        const ref = keyToRef.get(JSON.stringify(s.fills));
        if (ref) newStyles = { ...newStyles, fills: ref };
      }
      if (Array.isArray(s.strokes)) {
        const ref = keyToRef.get(JSON.stringify(s.strokes));
        if (ref) newStyles = { ...newStyles, strokes: ref };
      }
      if (newStyles !== s) result = { ...node, styles: newStyles };
    }
    if (Array.isArray(node.children)) {
      const newChildren = node.children.map(replaceWalk);
      result = { ...result, children: newChildren };
    }
    return result;
  };

  return { tree: replaceWalk(tree), globalVars: { styles: refs } };
};

export const serializeVariableValue = (value: any) => {
  if (typeof value !== "object" || value === null) return value;

  if ("type" in value && value.type === "VARIABLE_ALIAS") {
    return { type: "VARIABLE_ALIAS", id: value.id };
  }

  if ("r" in value && "g" in value && "b" in value) {
    return {
      type: "COLOR",
      r: value.r,
      g: value.g,
      b: value.b,
      a: "a" in value ? value.a : 1,
    };
  }

  return value;
};
