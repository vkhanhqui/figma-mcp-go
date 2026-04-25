import { describe, it, expect, beforeEach } from "bun:test";
import { handleWriteModifyRequest } from "./write-modify";

// ── Figma global mock ─────────────────────────────────────────────────────────

let mockNodes: Record<string, any>;
let commitUndoCalled: boolean;

const makeRequest = (type: string, nodeIds?: string[], params?: any) => ({
  type,
  requestId: "req-test-1",
  nodeIds: nodeIds ?? [],
  params: params ?? {},
});

beforeEach(() => {
  commitUndoCalled = false;
  mockNodes = {};
  (globalThis as any).figma = {
    getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
    commitUndo: () => { commitUndoCalled = true; },
  };
});

// ── set_opacity ───────────────────────────────────────────────────────────────

describe("set_opacity", () => {
  it("sets opacity on a node", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Frame", opacity: 1 };
    const res = await handleWriteModifyRequest(makeRequest("set_opacity", ["1:1"], { opacity: 0.5 }));
    expect(res?.data.results[0].opacity).toBe(0.5);
    expect(mockNodes["1:1"].opacity).toBe(0.5);
    expect(commitUndoCalled).toBe(true);
  });

  it("sets opacity to 0", async () => {
    mockNodes["1:1"] = { id: "1:1", opacity: 1 };
    const res = await handleWriteModifyRequest(makeRequest("set_opacity", ["1:1"], { opacity: 0 }));
    expect(res?.data.results[0].opacity).toBe(0);
  });

  it("reports error for missing node", async () => {
    const res = await handleWriteModifyRequest(makeRequest("set_opacity", ["9:9"], { opacity: 0.5 }));
    expect(res?.data.results[0].error).toBe("Node not found");
  });

  it("reports error for node without opacity support", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Page" }; // no opacity property
    const res = await handleWriteModifyRequest(makeRequest("set_opacity", ["1:1"], { opacity: 0.5 }));
    expect(res?.data.results[0].error).toContain("does not support opacity");
  });

  it("handles multiple nodeIds", async () => {
    mockNodes["1:1"] = { id: "1:1", opacity: 1 };
    mockNodes["2:2"] = { id: "2:2", opacity: 1 };
    const res = await handleWriteModifyRequest(makeRequest("set_opacity", ["1:1", "2:2"], { opacity: 0.25 }));
    expect(res?.data.results).toHaveLength(2);
    expect(mockNodes["1:1"].opacity).toBe(0.25);
    expect(mockNodes["2:2"].opacity).toBe(0.25);
  });

  it("throws for empty nodeIds", async () => {
    await expect(handleWriteModifyRequest(makeRequest("set_opacity", [], { opacity: 0.5 }))).rejects.toThrow();
  });
});

// ── set_corner_radius ─────────────────────────────────────────────────────────

describe("set_corner_radius", () => {
  it("sets uniform cornerRadius", async () => {
    mockNodes["1:1"] = { id: "1:1", cornerRadius: 0 };
    const res = await handleWriteModifyRequest(makeRequest("set_corner_radius", ["1:1"], { cornerRadius: 8 }));
    expect(mockNodes["1:1"].cornerRadius).toBe(8);
    expect(res?.data.results[0].cornerRadius).toBe(8);
    expect(commitUndoCalled).toBe(true);
  });

  it("sets per-corner radii independently", async () => {
    mockNodes["1:1"] = {
      id: "1:1", cornerRadius: 0,
      topLeftRadius: 0, topRightRadius: 0, bottomLeftRadius: 0, bottomRightRadius: 0,
    };
    await handleWriteModifyRequest(makeRequest("set_corner_radius", ["1:1"], {
      topLeftRadius: 8, topRightRadius: 0, bottomLeftRadius: 8, bottomRightRadius: 0,
    }));
    expect(mockNodes["1:1"].topLeftRadius).toBe(8);
    expect(mockNodes["1:1"].topRightRadius).toBe(0);
    expect(mockNodes["1:1"].bottomLeftRadius).toBe(8);
    expect(mockNodes["1:1"].bottomRightRadius).toBe(0);
  });

  it("reports error for missing node", async () => {
    const res = await handleWriteModifyRequest(makeRequest("set_corner_radius", ["9:9"], { cornerRadius: 4 }));
    expect(res?.data.results[0].error).toBe("Node not found");
  });

  it("reports error for node without cornerRadius support", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Text" }; // no cornerRadius property
    const res = await handleWriteModifyRequest(makeRequest("set_corner_radius", ["1:1"], { cornerRadius: 4 }));
    expect(res?.data.results[0].error).toContain("does not support corner radius");
  });

  it("handles multiple nodeIds", async () => {
    mockNodes["1:1"] = { id: "1:1", cornerRadius: 0 };
    mockNodes["2:2"] = { id: "2:2", cornerRadius: 0 };
    const res = await handleWriteModifyRequest(makeRequest("set_corner_radius", ["1:1", "2:2"], { cornerRadius: 12 }));
    expect(res?.data.results).toHaveLength(2);
    expect(mockNodes["1:1"].cornerRadius).toBe(12);
    expect(mockNodes["2:2"].cornerRadius).toBe(12);
  });

  it("returns null for unrecognised type", async () => {
    const res = await handleWriteModifyRequest(makeRequest("unknown_op"));
    expect(res).toBeNull();
  });
});

// ── set_visible ───────────────────────────────────────────────────────────────

describe("set_visible", () => {
  it("hides a node", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Frame", visible: true };
    const res = await handleWriteModifyRequest(makeRequest("set_visible", ["1:1"], { visible: false }));
    expect(mockNodes["1:1"].visible).toBe(false);
    expect(res?.data.results[0].visible).toBe(false);
    expect(commitUndoCalled).toBe(true);
  });

  it("shows a hidden node", async () => {
    mockNodes["1:1"] = { id: "1:1", visible: false };
    const res = await handleWriteModifyRequest(makeRequest("set_visible", ["1:1"], { visible: true }));
    expect(mockNodes["1:1"].visible).toBe(true);
    expect(res?.data.results[0].visible).toBe(true);
  });

  it("reports error for missing node", async () => {
    const res = await handleWriteModifyRequest(makeRequest("set_visible", ["9:9"], { visible: false }));
    expect(res?.data.results[0].error).toBe("Node not found");
  });

  it("reports error for node without visibility support", async () => {
    mockNodes["1:1"] = { id: "1:1" }; // no visible property
    const res = await handleWriteModifyRequest(makeRequest("set_visible", ["1:1"], { visible: false }));
    expect(res?.data.results[0].error).toContain("does not support visibility");
  });

  it("handles multiple nodes", async () => {
    mockNodes["1:1"] = { id: "1:1", visible: true };
    mockNodes["2:2"] = { id: "2:2", visible: true };
    const res = await handleWriteModifyRequest(makeRequest("set_visible", ["1:1", "2:2"], { visible: false }));
    expect(res?.data.results).toHaveLength(2);
    expect(mockNodes["1:1"].visible).toBe(false);
    expect(mockNodes["2:2"].visible).toBe(false);
  });

  it("throws for empty nodeIds", async () => {
    await expect(handleWriteModifyRequest(makeRequest("set_visible", [], { visible: false }))).rejects.toThrow();
  });
});

// ── lock_nodes / unlock_nodes ─────────────────────────────────────────────────

describe("lock_nodes", () => {
  it("locks a node", async () => {
    mockNodes["1:1"] = { id: "1:1", locked: false };
    const res = await handleWriteModifyRequest(makeRequest("lock_nodes", ["1:1"]));
    expect(mockNodes["1:1"].locked).toBe(true);
    expect(res?.data.results[0].locked).toBe(true);
    expect(commitUndoCalled).toBe(true);
  });

  it("reports error for missing node", async () => {
    const res = await handleWriteModifyRequest(makeRequest("lock_nodes", ["9:9"]));
    expect(res?.data.results[0].error).toBe("Node not found");
  });

  it("reports error for node without locked support", async () => {
    mockNodes["1:1"] = { id: "1:1" }; // no locked property
    const res = await handleWriteModifyRequest(makeRequest("lock_nodes", ["1:1"]));
    expect(res?.data.results[0].error).toContain("does not support locking");
  });
});

describe("unlock_nodes", () => {
  it("unlocks a node", async () => {
    mockNodes["1:1"] = { id: "1:1", locked: true };
    const res = await handleWriteModifyRequest(makeRequest("unlock_nodes", ["1:1"]));
    expect(mockNodes["1:1"].locked).toBe(false);
    expect(res?.data.results[0].locked).toBe(false);
  });

  it("handles multiple nodes", async () => {
    mockNodes["1:1"] = { id: "1:1", locked: true };
    mockNodes["2:2"] = { id: "2:2", locked: true };
    const res = await handleWriteModifyRequest(makeRequest("unlock_nodes", ["1:1", "2:2"]));
    expect(res?.data.results).toHaveLength(2);
    expect(mockNodes["1:1"].locked).toBe(false);
    expect(mockNodes["2:2"].locked).toBe(false);
  });
});

// ── rotate_nodes ──────────────────────────────────────────────────────────────

describe("rotate_nodes", () => {
  it("rotates a node", async () => {
    mockNodes["1:1"] = { id: "1:1", rotation: 0 };
    const res = await handleWriteModifyRequest(makeRequest("rotate_nodes", ["1:1"], { rotation: 45 }));
    expect(mockNodes["1:1"].rotation).toBe(45);
    expect(res?.data.results[0].rotation).toBe(45);
    expect(commitUndoCalled).toBe(true);
  });

  it("sets negative rotation", async () => {
    mockNodes["1:1"] = { id: "1:1", rotation: 0 };
    await handleWriteModifyRequest(makeRequest("rotate_nodes", ["1:1"], { rotation: -90 }));
    expect(mockNodes["1:1"].rotation).toBe(-90);
  });

  it("reports error for missing node", async () => {
    const res = await handleWriteModifyRequest(makeRequest("rotate_nodes", ["9:9"], { rotation: 45 }));
    expect(res?.data.results[0].error).toBe("Node not found");
  });

  it("reports error for node without rotation support", async () => {
    mockNodes["1:1"] = { id: "1:1" }; // no rotation property
    const res = await handleWriteModifyRequest(makeRequest("rotate_nodes", ["1:1"], { rotation: 45 }));
    expect(res?.data.results[0].error).toContain("does not support rotation");
  });

  it("handles multiple nodes", async () => {
    mockNodes["1:1"] = { id: "1:1", rotation: 0 };
    mockNodes["2:2"] = { id: "2:2", rotation: 0 };
    const res = await handleWriteModifyRequest(makeRequest("rotate_nodes", ["1:1", "2:2"], { rotation: 90 }));
    expect(res?.data.results).toHaveLength(2);
    expect(mockNodes["1:1"].rotation).toBe(90);
    expect(mockNodes["2:2"].rotation).toBe(90);
  });
});

// ── reorder_nodes ─────────────────────────────────────────────────────────────

describe("reorder_nodes", () => {
  const makeParent = (children: any[]) => ({
    children,
    insertChild(index: number, child: any) {
      const i = this.children.indexOf(child);
      if (i !== -1) this.children.splice(i, 1);
      this.children.splice(index, 0, child);
    },
  });

  it("brings node to front", async () => {
    const parent = makeParent([]);
    const nodeA = { id: "1:1", parent };
    const nodeB = { id: "2:2", parent };
    parent.children = [nodeA, nodeB];
    mockNodes["1:1"] = nodeA;
    const res = await handleWriteModifyRequest(makeRequest("reorder_nodes", ["1:1"], { order: "bringToFront" }));
    expect(res?.data.results[0].index).toBe(1);
    expect(parent.children[1]).toBe(nodeA);
    expect(commitUndoCalled).toBe(true);
  });

  it("sends node to back", async () => {
    const parent = makeParent([]);
    const nodeA = { id: "1:1", parent };
    const nodeB = { id: "2:2", parent };
    parent.children = [nodeA, nodeB];
    mockNodes["2:2"] = nodeB;
    const res = await handleWriteModifyRequest(makeRequest("reorder_nodes", ["2:2"], { order: "sendToBack" }));
    expect(res?.data.results[0].index).toBe(0);
    expect(parent.children[0]).toBe(nodeB);
  });

  it("brings forward one step", async () => {
    const parent = makeParent([]);
    const nodeA = { id: "1:1", parent };
    const nodeB = { id: "2:2", parent };
    const nodeC = { id: "3:3", parent };
    parent.children = [nodeA, nodeB, nodeC];
    mockNodes["1:1"] = nodeA;
    const res = await handleWriteModifyRequest(makeRequest("reorder_nodes", ["1:1"], { order: "bringForward" }));
    expect(res?.data.results[0].index).toBe(1);
  });

  it("sends backward one step", async () => {
    const parent = makeParent([]);
    const nodeA = { id: "1:1", parent };
    const nodeB = { id: "2:2", parent };
    parent.children = [nodeA, nodeB];
    mockNodes["2:2"] = nodeB;
    const res = await handleWriteModifyRequest(makeRequest("reorder_nodes", ["2:2"], { order: "sendBackward" }));
    expect(res?.data.results[0].index).toBe(0);
  });

  it("throws for invalid order", async () => {
    mockNodes["1:1"] = { id: "1:1" };
    await expect(handleWriteModifyRequest(makeRequest("reorder_nodes", ["1:1"], { order: "invalid" }))).rejects.toThrow();
  });

  it("reports error for missing node", async () => {
    const res = await handleWriteModifyRequest(makeRequest("reorder_nodes", ["9:9"], { order: "bringToFront" }));
    expect(res?.data.results[0].error).toBe("Node not found");
  });

  it("reports error for node without parent", async () => {
    mockNodes["1:1"] = { id: "1:1", parent: null };
    const res = await handleWriteModifyRequest(makeRequest("reorder_nodes", ["1:1"], { order: "bringToFront" }));
    expect(res?.data.results[0].error).toContain("no reorderable parent");
  });
});

// ── set_blend_mode ────────────────────────────────────────────────────────────

describe("set_blend_mode", () => {
  it("sets blend mode on a node", async () => {
    mockNodes["1:1"] = { id: "1:1", blendMode: "NORMAL" };
    const res = await handleWriteModifyRequest(makeRequest("set_blend_mode", ["1:1"], { blendMode: "MULTIPLY" }));
    expect(mockNodes["1:1"].blendMode).toBe("MULTIPLY");
    expect(res?.data.results[0].blendMode).toBe("MULTIPLY");
    expect(commitUndoCalled).toBe(true);
  });

  it("reports error for missing node", async () => {
    const res = await handleWriteModifyRequest(makeRequest("set_blend_mode", ["9:9"], { blendMode: "MULTIPLY" }));
    expect(res?.data.results[0].error).toBe("Node not found");
  });

  it("reports error for node without blend mode support", async () => {
    mockNodes["1:1"] = { id: "1:1" }; // no blendMode property
    const res = await handleWriteModifyRequest(makeRequest("set_blend_mode", ["1:1"], { blendMode: "MULTIPLY" }));
    expect(res?.data.results[0].error).toContain("does not support blend mode");
  });

  it("handles multiple nodes", async () => {
    mockNodes["1:1"] = { id: "1:1", blendMode: "NORMAL" };
    mockNodes["2:2"] = { id: "2:2", blendMode: "NORMAL" };
    const res = await handleWriteModifyRequest(makeRequest("set_blend_mode", ["1:1", "2:2"], { blendMode: "SCREEN" }));
    expect(res?.data.results).toHaveLength(2);
    expect(mockNodes["1:1"].blendMode).toBe("SCREEN");
    expect(mockNodes["2:2"].blendMode).toBe("SCREEN");
  });
});

// ── set_constraints ───────────────────────────────────────────────────────────

describe("set_constraints", () => {
  it("sets horizontal constraint", async () => {
    mockNodes["1:1"] = { id: "1:1", constraints: { horizontal: "MIN", vertical: "MIN" } };
    const res = await handleWriteModifyRequest(makeRequest("set_constraints", ["1:1"], { horizontal: "CENTER" }));
    expect(mockNodes["1:1"].constraints.horizontal).toBe("CENTER");
    expect(mockNodes["1:1"].constraints.vertical).toBe("MIN"); // unchanged
    expect(commitUndoCalled).toBe(true);
  });

  it("sets vertical constraint", async () => {
    mockNodes["1:1"] = { id: "1:1", constraints: { horizontal: "MIN", vertical: "MIN" } };
    await handleWriteModifyRequest(makeRequest("set_constraints", ["1:1"], { vertical: "MAX" }));
    expect(mockNodes["1:1"].constraints.vertical).toBe("MAX");
    expect(mockNodes["1:1"].constraints.horizontal).toBe("MIN"); // unchanged
  });

  it("sets both constraints simultaneously", async () => {
    mockNodes["1:1"] = { id: "1:1", constraints: { horizontal: "MIN", vertical: "MIN" } };
    await handleWriteModifyRequest(makeRequest("set_constraints", ["1:1"], { horizontal: "STRETCH", vertical: "STRETCH" }));
    expect(mockNodes["1:1"].constraints.horizontal).toBe("STRETCH");
    expect(mockNodes["1:1"].constraints.vertical).toBe("STRETCH");
  });

  it("reports error for missing node", async () => {
    const res = await handleWriteModifyRequest(makeRequest("set_constraints", ["9:9"], { horizontal: "CENTER" }));
    expect(res?.data.results[0].error).toBe("Node not found");
  });

  it("reports error for node without constraints support", async () => {
    mockNodes["1:1"] = { id: "1:1" }; // no constraints property
    const res = await handleWriteModifyRequest(makeRequest("set_constraints", ["1:1"], { horizontal: "CENTER" }));
    expect(res?.data.results[0].error).toContain("does not support constraints");
  });
});

// ── reparent_nodes ────────────────────────────────────────────────────────────

describe("reparent_nodes", () => {
  it("moves a node to a new parent", async () => {
    const children: any[] = [];
    const newParent = { id: "2:2", appendChild: (n: any) => children.push(n) };
    mockNodes["1:1"] = { id: "1:1", name: "Node" };
    mockNodes["2:2"] = newParent;
    const res = await handleWriteModifyRequest(makeRequest("reparent_nodes", ["1:1"], { parentId: "2:2" }));
    expect(children).toHaveLength(1);
    expect(res?.data.results[0].newParentId).toBe("2:2");
    expect(commitUndoCalled).toBe(true);
  });

  it("throws if parentId is missing", async () => {
    await expect(handleWriteModifyRequest(makeRequest("reparent_nodes", ["1:1"], {}))).rejects.toThrow("parentId is required");
  });

  it("throws if parent node not found", async () => {
    mockNodes["1:1"] = { id: "1:1" };
    await expect(handleWriteModifyRequest(makeRequest("reparent_nodes", ["1:1"], { parentId: "9:9" }))).rejects.toThrow("Parent not found");
  });

  it("throws if parent cannot contain children", async () => {
    mockNodes["1:1"] = { id: "1:1" };
    mockNodes["2:2"] = { id: "2:2" }; // no appendChild
    await expect(handleWriteModifyRequest(makeRequest("reparent_nodes", ["1:1"], { parentId: "2:2" }))).rejects.toThrow("cannot contain children");
  });

  it("reports error for missing child node", async () => {
    const newParent = { id: "2:2", appendChild: () => {} };
    mockNodes["2:2"] = newParent;
    const res = await handleWriteModifyRequest(makeRequest("reparent_nodes", ["9:9"], { parentId: "2:2" }));
    expect(res?.data.results[0].error).toBe("Node not found");
  });
});

// ── batch_rename_nodes ────────────────────────────────────────────────────────

describe("batch_rename_nodes", () => {
  it("renames with find/replace", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Button/Primary" };
    mockNodes["2:2"] = { id: "2:2", name: "Button/Secondary" };
    const res = await handleWriteModifyRequest(makeRequest("batch_rename_nodes", ["1:1", "2:2"], {
      find: "Button", replace: "Btn",
    }));
    expect(mockNodes["1:1"].name).toBe("Btn/Primary");
    expect(mockNodes["2:2"].name).toBe("Btn/Secondary");
    expect(res?.data.results[0].oldName).toBe("Button/Primary");
    expect(res?.data.results[0].name).toBe("Btn/Primary");
    expect(commitUndoCalled).toBe(true);
  });

  it("adds prefix and suffix", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Card" };
    const res = await handleWriteModifyRequest(makeRequest("batch_rename_nodes", ["1:1"], {
      prefix: "UI/", suffix: "_v2",
    }));
    expect(mockNodes["1:1"].name).toBe("UI/Card_v2");
    expect(res?.data.results[0].name).toBe("UI/Card_v2");
  });

  it("renames using regex", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Frame 123" };
    const res = await handleWriteModifyRequest(makeRequest("batch_rename_nodes", ["1:1"], {
      find: "\\d+", replace: "X", useRegex: true,
    }));
    expect(mockNodes["1:1"].name).toBe("Frame X");
  });

  it("captures regex error per-node", async () => {
    mockNodes["1:1"] = { id: "1:1", name: "Card" };
    const res = await handleWriteModifyRequest(makeRequest("batch_rename_nodes", ["1:1"], {
      find: "[invalid", replace: "X", useRegex: true,
    }));
    expect(res?.data.results[0].error).toContain("Invalid regex");
  });

  it("reports error for missing node", async () => {
    const res = await handleWriteModifyRequest(makeRequest("batch_rename_nodes", ["9:9"], { prefix: "x" }));
    expect(res?.data.results[0].error).toBe("Node not found");
  });

  it("throws for empty nodeIds", async () => {
    await expect(handleWriteModifyRequest(makeRequest("batch_rename_nodes", [], { prefix: "x" }))).rejects.toThrow();
  });
});

// ── find_replace_text ─────────────────────────────────────────────────────────

describe("find_replace_text", () => {
  beforeEach(() => {
    (globalThis as any).figma = {
      ...(globalThis as any).figma,
      currentPage: {
        type: "PAGE",
        children: [],
      },
      loadFontAsync: async () => {},
    };
  });

  it("replaces text in matching TEXT nodes", async () => {
    const textNode = {
      id: "1:1", name: "Label", type: "TEXT", characters: "Hello World",
      fontName: { family: "Inter", style: "Regular" },
    };
    (globalThis as any).figma.currentPage = { type: "PAGE", children: [textNode] };
    const res = await handleWriteModifyRequest(makeRequest("find_replace_text", [], { find: "World", replace: "Figma" }));
    expect(textNode.characters).toBe("Hello Figma");
    expect(res?.data.replaced).toBe(1);
    expect(res?.data.results[0].newText).toBe("Hello Figma");
    expect(commitUndoCalled).toBe(true);
  });

  it("skips nodes where text does not match", async () => {
    const textNode = { id: "1:1", name: "Label", type: "TEXT", characters: "Goodbye", fontName: { family: "Inter", style: "Regular" } };
    (globalThis as any).figma.currentPage = { type: "PAGE", children: [textNode] };
    const res = await handleWriteModifyRequest(makeRequest("find_replace_text", [], { find: "Hello", replace: "Hi" }));
    expect(res?.data.replaced).toBe(0);
    expect(textNode.characters).toBe("Goodbye");
  });

  it("searches recursively through nested children", async () => {
    const textNode = { id: "2:2", name: "Nested", type: "TEXT", characters: "foo bar", fontName: { family: "Inter", style: "Regular" } };
    const frame = { id: "1:1", type: "FRAME", children: [textNode] };
    (globalThis as any).figma.currentPage = { type: "PAGE", children: [frame] };
    const res = await handleWriteModifyRequest(makeRequest("find_replace_text", [], { find: "foo", replace: "baz" }));
    expect(textNode.characters).toBe("baz bar");
    expect(res?.data.replaced).toBe(1);
  });

  it("supports scoped search within a subtree when nodeId provided", async () => {
    const textNode = { id: "2:2", name: "Inner", type: "TEXT", characters: "target", fontName: { family: "Inter", style: "Regular" } };
    const frame = { id: "1:1", type: "FRAME", children: [textNode] };
    mockNodes["1:1"] = frame;
    const res = await handleWriteModifyRequest(makeRequest("find_replace_text", ["1:1"], { find: "target", replace: "done" }));
    expect(textNode.characters).toBe("done");
    expect(res?.data.replaced).toBe(1);
  });

  it("uses regex when useRegex is true", async () => {
    const textNode = { id: "1:1", name: "Label", type: "TEXT", characters: "Price: $99", fontName: { family: "Inter", style: "Regular" } };
    (globalThis as any).figma.currentPage = { type: "PAGE", children: [textNode] };
    const res = await handleWriteModifyRequest(makeRequest("find_replace_text", [], { find: "\\$\\d+", replace: "$199", useRegex: true }));
    expect(textNode.characters).toBe("Price: $199");
    expect(res?.data.replaced).toBe(1);
  });

  it("captures regex error per-node", async () => {
    const textNode = { id: "1:1", type: "TEXT", characters: "hello", fontName: { family: "Inter", style: "Regular" } };
    (globalThis as any).figma.currentPage = { type: "PAGE", children: [textNode] };
    const res = await handleWriteModifyRequest(makeRequest("find_replace_text", [], { find: "[bad", replace: "x", useRegex: true }));
    expect(res?.data.replaced).toBe(0);
    expect(res?.data.results[0].error).toContain("Invalid regex");
  });

  it("throws if find is missing", async () => {
    await expect(handleWriteModifyRequest(makeRequest("find_replace_text", [], { replace: "x" }))).rejects.toThrow("find is required");
  });

  it("throws if replace is missing", async () => {
    await expect(handleWriteModifyRequest(makeRequest("find_replace_text", [], { find: "x" }))).rejects.toThrow("replace is required");
  });
});

// ── set_text ──────────────────────────────────────────────────────────────────

describe("set_text", () => {
  beforeEach(() => {
    (globalThis as any).figma = {
      ...(globalThis as any).figma,
      loadFontAsync: async () => {},
    };
  });

  const makeText = (overrides?: any) => ({
    id: "1:1",
    name: "Label",
    type: "TEXT",
    characters: "old",
    fontName: { family: "Inter", style: "Regular" },
    textTruncation: "DISABLED",
    maxLines: null,
    ...overrides,
  });

  it("updates characters when text is provided", async () => {
    mockNodes["1:1"] = makeText();
    const res = await handleWriteModifyRequest(makeRequest("set_text", ["1:1"], { text: "new" }));
    expect(mockNodes["1:1"].characters).toBe("new");
    expect(res?.data.characters).toBe("new");
    expect(commitUndoCalled).toBe(true);
  });

  it("sets textTruncation to ENDING without changing characters", async () => {
    mockNodes["1:1"] = makeText();
    const res = await handleWriteModifyRequest(makeRequest("set_text", ["1:1"], { textTruncation: "ENDING" }));
    expect(mockNodes["1:1"].characters).toBe("old"); // unchanged
    expect(mockNodes["1:1"].textTruncation).toBe("ENDING");
    expect(res?.data.textTruncation).toBe("ENDING");
  });

  it("sets maxLines to a positive integer", async () => {
    mockNodes["1:1"] = makeText({ textTruncation: "ENDING" });
    await handleWriteModifyRequest(makeRequest("set_text", ["1:1"], { maxLines: 2 }));
    expect(mockNodes["1:1"].maxLines).toBe(2);
  });

  it("clears maxLines when null is passed", async () => {
    mockNodes["1:1"] = makeText({ maxLines: 5 });
    await handleWriteModifyRequest(makeRequest("set_text", ["1:1"], { maxLines: null }));
    expect(mockNodes["1:1"].maxLines).toBeNull();
  });

  it("rejects invalid textTruncation", async () => {
    mockNodes["1:1"] = makeText();
    await expect(
      handleWriteModifyRequest(makeRequest("set_text", ["1:1"], { textTruncation: "TRUNCATE" })),
    ).rejects.toThrow("textTruncation must be 'DISABLED' or 'ENDING'");
  });

  it("rejects non-positive maxLines", async () => {
    mockNodes["1:1"] = makeText();
    await expect(
      handleWriteModifyRequest(makeRequest("set_text", ["1:1"], { maxLines: 0 })),
    ).rejects.toThrow("maxLines must be null or a positive integer");
  });

  it("throws if no text/truncation/maxLines provided", async () => {
    mockNodes["1:1"] = makeText();
    await expect(
      handleWriteModifyRequest(makeRequest("set_text", ["1:1"], {})),
    ).rejects.toThrow("at least one of text, textTruncation, or maxLines is required");
  });

  it("rejects non-TEXT nodes", async () => {
    mockNodes["1:1"] = { id: "1:1", type: "FRAME" };
    await expect(
      handleWriteModifyRequest(makeRequest("set_text", ["1:1"], { text: "x" })),
    ).rejects.toThrow("not a TEXT node");
  });

  it("can update text and truncation in one call", async () => {
    mockNodes["1:1"] = makeText();
    await handleWriteModifyRequest(makeRequest("set_text", ["1:1"], {
      text: "new", textTruncation: "ENDING", maxLines: 3,
    }));
    expect(mockNodes["1:1"].characters).toBe("new");
    expect(mockNodes["1:1"].textTruncation).toBe("ENDING");
    expect(mockNodes["1:1"].maxLines).toBe(3);
  });
});
