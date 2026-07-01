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

// Convert an RGBA color to a hex string, appending an 8-bit alpha suffix
// when the color is translucent (alpha < 1). Defaults alpha to 1.
export const toHexA = (color: any) => {
  const hex = toHex(color);
  const a = color && color.a != null ? color.a : 1;
  if (a >= 1) return hex;
  return hex + Math.round(a * 255).toString(16).padStart(2, "0");
};

const GRADIENT_TYPES = new Set([
  "GRADIENT_LINEAR",
  "GRADIENT_RADIAL",
  "GRADIENT_ANGULAR",
  "GRADIENT_DIAMOND",
]);

// A blend mode worth emitting — Figma's default is NORMAL (PASS_THROUGH on
// containers); both mean "no special mixing" so they're omitted.
const meaningfulBlendMode = (mode: any) =>
  typeof mode === "string" && mode !== "NORMAL" && mode !== "PASS_THROUGH"
    ? mode
    : undefined;

// Resolve a single gradient stop to { position, color } with a hex/hex-alpha color.
const serializeGradientStop = (stop: any) => {
  const out: any = { position: pixelRound(stop.position) };
  if (stop.color) out.color = toHexA(stop.color);
  return out;
};

// Serialize one paint. Solid paints collapse to a hex (or hex+alpha) string —
// keeping the historical shape. Gradient paints resolve inline to an object
// carrying type, stops, and transform. Other paint types (IMAGE, VIDEO) drop.
export const serializePaint = (paint: any) => {
  if (paint.type === "SOLID" && "color" in paint) {
    const hex = toHex(paint.color);
    const opacity = paint.opacity != null ? paint.opacity : 1;
    const color =
      opacity === 1
        ? hex
        : hex + Math.round(opacity * 255).toString(16).padStart(2, "0");
    // A solid with a non-default mix mode can't be a bare hex string — promote
    // it to an object so the blendMode survives. Plain solids stay strings to
    // preserve the historical shape (and the dedup contract).
    const blend = meaningfulBlendMode(paint.blendMode);
    if (blend) return { type: "SOLID", color, blendMode: blend };
    return color;
  }
  if (GRADIENT_TYPES.has(paint.type)) {
    const g: any = { type: paint.type };
    if (Array.isArray(paint.gradientStops))
      g.gradientStops = paint.gradientStops.map(serializeGradientStop);
    if (paint.gradientTransform) g.gradientTransform = paint.gradientTransform;
    if (paint.opacity != null && paint.opacity !== 1) g.opacity = paint.opacity;
    const blend = meaningfulBlendMode(paint.blendMode);
    if (blend) g.blendMode = blend;
    return g;
  }
  return undefined;
};

export const serializePaints = (paints: any) => {
  if (isMixed(paints)) return "mixed";

  if (!paints || !Array.isArray(paints)) return undefined;

  const result = paints
    .map(serializePaint)
    .filter((paint: any) => paint !== undefined);

  return result.length > 0 ? result : undefined;
};

// Serialize a single effect (shadow or blur) to a compact object, emitting
// only the fields that apply. Blur effects carry just a radius; shadows add
// color/offset/spread.
export const serializeEffect = (effect: any) => {
  const out: any = { type: effect.type };
  if (effect.color) out.color = toHexA(effect.color);
  if (effect.offset)
    out.offset = { x: pixelRound(effect.offset.x), y: pixelRound(effect.offset.y) };
  if (typeof effect.radius === "number") out.radius = effect.radius;
  if (typeof effect.spread === "number" && effect.spread !== 0)
    out.spread = effect.spread;
  const blend = meaningfulBlendMode(effect.blendMode);
  if (blend) out.blendMode = blend;
  return out;
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

export const serializeStyles = async (node: any) => {
  const styles: any = {};

  if ("fills" in node) {
    // Prefer named style over raw fill values when a style is applied.
    if (node.fillStyleId && typeof node.fillStyleId === "string") {
      const style = await figma.getStyleByIdAsync(node.fillStyleId);
      if (style) styles.fillStyle = style.name;
    }
    const fills = serializePaints(node.fills);
    if (fills !== undefined) styles.fills = fills;
  }

  if ("strokes" in node) {
    if (node.strokeStyleId && typeof node.strokeStyleId === "string") {
      const style = await figma.getStyleByIdAsync(node.strokeStyleId);
      if (style) styles.strokeStyle = style.name;
    }
    const strokes = serializePaints(node.strokes);
    if (strokes !== undefined) styles.strokes = strokes;

    // Stroke geometry — border thickness, side, and dash. These affect layout
    // box size (strokeAlign) and edge coverage, so consumers can't infer them
    // from color alone.
    const hasStroke =
      strokes !== undefined ||
      (Array.isArray(node.strokes) && node.strokes.length > 0);
    if (hasStroke) {
      if ("strokeWeight" in node && typeof node.strokeWeight === "number") {
        styles.strokeWeight = node.strokeWeight;
      }
      // Per-side weights: emit only when they actually differ (uniform weight
      // is already captured by strokeWeight, so emitting all four is noise).
      const sides = ["Top", "Right", "Bottom", "Left"] as const;
      const sideWeights = sides.map((s) => node[`stroke${s}Weight`]);
      const hasPerSide = sideWeights.every((w) => typeof w === "number");
      const perSideDiffer =
        hasPerSide && sideWeights.some((w) => w !== sideWeights[0]);
      if (perSideDiffer) {
        sides.forEach((s) => {
          styles[`stroke${s}Weight`] = node[`stroke${s}Weight`];
        });
      }
      if ("strokeAlign" in node && typeof node.strokeAlign === "string") {
        styles.strokeAlign = node.strokeAlign;
      }
      if (Array.isArray(node.dashPattern) && node.dashPattern.length > 0) {
        styles.dashPattern = node.dashPattern;
      }
    }
  }

  if ("cornerRadius" in node) {
    if (isMixed(node.cornerRadius)) {
      // Corners differ — resolve each named corner when Figma exposes it.
      const corners: any = {};
      const map = [
        ["topLeft", "topLeftRadius"],
        ["topRight", "topRightRadius"],
        ["bottomRight", "bottomRightRadius"],
        ["bottomLeft", "bottomLeftRadius"],
      ] as const;
      for (const [key, prop] of map) {
        if (typeof node[prop] === "number") corners[key] = node[prop];
      }
      styles.cornerRadius =
        Object.keys(corners).length > 0 ? corners : "mixed";
    } else if (node.cornerRadius !== 0) {
      styles.cornerRadius = node.cornerRadius;
    }
  }

  // Effects — shadows and blurs are otherwise summarized away entirely.
  if (Array.isArray(node.effects) && node.effects.length > 0) {
    const effects = node.effects
      .filter((e: any) => e.visible !== false)
      .map(serializeEffect);
    if (effects.length > 0) styles.effects = effects;
  }

  // Node-level translucency and mix mode.
  if (typeof node.opacity === "number" && node.opacity !== 1) {
    styles.opacity = pixelRound(node.opacity);
  }
  const nodeBlend = meaningfulBlendMode(node.blendMode);
  if (nodeBlend) styles.blendMode = nodeBlend;

  // Auto-layout gap — the icon↔label / item spacing consumers can't measure.
  if (
    "layoutMode" in node &&
    typeof node.layoutMode === "string" &&
    node.layoutMode !== "NONE"
  ) {
    styles.layoutMode = node.layoutMode;
    if (typeof node.itemSpacing === "number") {
      styles.itemSpacing = node.itemSpacing;
    }
  }

  if ("paddingLeft" in node) {
    styles.padding = {
      top: node.paddingTop,
      right: node.paddingRight,
      bottom: node.paddingBottom,
      left: node.paddingLeft,
    };
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
      textCase: isMixed(node.textCase)
        ? "mixed"
        : node.textCase && node.textCase !== "ORIGINAL"
          ? node.textCase
          : undefined,
      lineHeight: serializeLineHeight(node.lineHeight),
      letterSpacing: serializeLetterSpacing(node.letterSpacing),
      textAlignHorizontal: isMixed(node.textAlignHorizontal)
        ? "mixed"
        : node.textAlignHorizontal,
      textAlignVertical: isMixed(node.textAlignVertical)
        ? "mixed"
        : node.textAlignVertical,
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
