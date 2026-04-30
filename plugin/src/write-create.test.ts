import { describe, it, expect, beforeEach } from "bun:test";
import { handleWriteCreateRequest } from "./write-create";

// ── Figma global mock ─────────────────────────────────────────────────────────

let mockNodes: Record<string, any>;
let commitUndoCalled: boolean;
let createdComponents: any[];

const makeRequest = (type: string, nodeIds?: string[], params?: any) => ({
  type,
  requestId: "req-test-1",
  nodeIds: nodeIds ?? [],
  params: params ?? {},
});

beforeEach(() => {
  commitUndoCalled = false;
  createdComponents = [];
  mockNodes = {};
  (globalThis as any).figma = {
    get currentPage() { return { id: "0:1", name: "Page 1", appendChild: () => {} }; },
    getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
    createComponent: () => {
      const comp: any = {
        id: "comp:new",
        name: "Component",
        type: "COMPONENT",
        x: 0, y: 0, width: 100, height: 100,
        fills: [], strokes: [], cornerRadius: 0, layoutMode: "NONE",
        children: [] as any[],
        resize(w: number, h: number) { this.width = w; this.height = h; },
        appendChild(child: any) { this.children.push(child); },
      };
      createdComponents.push(comp);
      return comp;
    },
    commitUndo: () => { commitUndoCalled = true; },
    mixed: Symbol("mixed"),
  };
});

// ── create_component ──────────────────────────────────────────────────────────

describe("create_component", () => {
  const makeParent = () => ({
    id: "0:1",
    children: [] as any[],
    insertChild(_: number, c: any) { this.children.push(c); },
  });

  it("converts a FRAME to a COMPONENT in place", async () => {
    const child = { id: "2:1", type: "RECTANGLE" };
    let frameRemoved = false;
    const parent = makeParent();
    const frame = {
      id: "1:1", name: "Card", type: "FRAME",
      x: 10, y: 20, width: 200, height: 100,
      fills: [{ type: "SOLID" }], strokes: [],
      cornerRadius: 8, layoutMode: "NONE",
      children: [child], parent,
      remove() { frameRemoved = true; },
    };
    parent.children = [frame];
    mockNodes["1:1"] = frame;

    const res = await handleWriteCreateRequest(makeRequest("create_component", ["1:1"]));
    expect(res?.data.type).toBe("COMPONENT");
    expect(createdComponents[0].name).toBe("Card");
    expect(createdComponents[0].cornerRadius).toBe(8);
    expect(createdComponents[0].children).toContain(child);
    expect(frameRemoved).toBe(true);
    expect(commitUndoCalled).toBe(true);
  });

  it("copies frame dimensions", async () => {
    const parent = makeParent();
    const frame = {
      id: "1:1", name: "Banner", type: "FRAME",
      x: 0, y: 0, width: 320, height: 64,
      fills: [], strokes: [], cornerRadius: 0, layoutMode: "NONE",
      children: [], parent,
      remove() {},
    };
    parent.children = [frame];
    mockNodes["1:1"] = frame;

    await handleWriteCreateRequest(makeRequest("create_component", ["1:1"]));
    expect(createdComponents[0].width).toBe(320);
    expect(createdComponents[0].height).toBe(64);
  });

  it("uses custom name when provided", async () => {
    const parent = makeParent();
    const frame = {
      id: "1:1", name: "Frame", type: "FRAME",
      x: 0, y: 0, width: 100, height: 100,
      fills: [], strokes: [], cornerRadius: 0, layoutMode: "NONE",
      children: [], parent,
      remove() {},
    };
    parent.children = [frame];
    mockNodes["1:1"] = frame;

    await handleWriteCreateRequest(makeRequest("create_component", ["1:1"], { name: "Button" }));
    expect(createdComponents[0].name).toBe("Button");
  });

  it("copies auto-layout properties when layoutMode is set", async () => {
    const parent = makeParent();
    const frame = {
      id: "1:1", name: "Row", type: "FRAME",
      x: 0, y: 0, width: 200, height: 48,
      fills: [], strokes: [], cornerRadius: 0,
      layoutMode: "HORIZONTAL",
      paddingTop: 8, paddingRight: 16, paddingBottom: 8, paddingLeft: 16,
      itemSpacing: 12,
      primaryAxisAlignItems: "CENTER",
      counterAxisAlignItems: "CENTER",
      children: [], parent,
      remove() {},
    };
    parent.children = [frame];
    mockNodes["1:1"] = frame;

    await handleWriteCreateRequest(makeRequest("create_component", ["1:1"]));
    const comp = createdComponents[0];
    expect(comp.layoutMode).toBe("HORIZONTAL");
    expect(comp.paddingTop).toBe(8);
    expect(comp.paddingRight).toBe(16);
    expect(comp.itemSpacing).toBe(12);
    expect(comp.primaryAxisAlignItems).toBe("CENTER");
  });

  it("throws when nodeId not found", async () => {
    await expect(
      handleWriteCreateRequest(makeRequest("create_component", ["9:9"]))
    ).rejects.toThrow("Node not found: 9:9");
  });

  it("throws when node is not a FRAME", async () => {
    mockNodes["1:1"] = { id: "1:1", type: "RECTANGLE" };
    await expect(
      handleWriteCreateRequest(makeRequest("create_component", ["1:1"]))
    ).rejects.toThrow("is not a FRAME");
  });

  it("throws when no nodeId provided", async () => {
    await expect(
      handleWriteCreateRequest(makeRequest("create_component", []))
    ).rejects.toThrow("nodeId is required");
  });
});

// ── create_section ────────────────────────────────────────────────────────────

describe("create_section", () => {
  let createdSection: any;

  beforeEach(() => {
    createdSection = null;
    (globalThis as any).figma = {
      ...(globalThis as any).figma,
      currentPage: { id: "0:1", name: "Page 1", appendChild: () => {} },
      createSection: () => {
        createdSection = {
          id: "section:new", name: "Section", type: "SECTION",
          x: 0, y: 0, width: 200, height: 200,
          resizeWithoutConstraints(w: number, h: number) { this.width = w; this.height = h; },
        };
        return createdSection;
      },
    };
  });

  it("creates a section with a name", async () => {
    const res = await handleWriteCreateRequest(makeRequest("create_section", [], { name: "Sprint 1" }));
    expect(createdSection.name).toBe("Sprint 1");
    expect(res?.data.type).toBe("SECTION");
    expect(res?.data.id).toBe("section:new");
    expect(commitUndoCalled).toBe(true);
  });

  it("creates a section at a specific position", async () => {
    const res = await handleWriteCreateRequest(makeRequest("create_section", [], { x: 100, y: 200 }));
    expect(createdSection.x).toBe(100);
    expect(createdSection.y).toBe(200);
  });

  it("creates a section with custom size", async () => {
    await handleWriteCreateRequest(makeRequest("create_section", [], { width: 800, height: 600 }));
    expect(createdSection.width).toBe(800);
    expect(createdSection.height).toBe(600);
  });

  it("creates a section with default values when no params given", async () => {
    const res = await handleWriteCreateRequest(makeRequest("create_section", [], {}));
    expect(res?.data.id).toBe("section:new");
  });
});

// ── create_instance ───────────────────────────────────────────────────────────

describe("create_instance", () => {
  let createdInstances: any[];
  let parentNode: any;
  let importedKey: string | null;

  const makeComponent = (overrides?: any) => ({
    id: "comp:src",
    name: "SourceComponent",
    type: "COMPONENT",
    key: "abc123def456",
    createInstance() {
      const inst = {
        id: `inst:${createdInstances.length + 1}`,
        name: this.name,
        type: "INSTANCE",
        x: 0, y: 0, width: 100, height: 100,
      };
      createdInstances.push(inst);
      return inst;
    },
    ...overrides,
  });

  beforeEach(() => {
    createdInstances = [];
    importedKey = null;
    parentNode = { id: "page:1", appendChild(c: any) { (this.children ||= []).push(c); }, children: [] };
    (globalThis as any).figma = {
      ...(globalThis as any).figma,
      currentPage: parentNode,
      getNodeByIdAsync: async (id: string) => mockNodes[id] ?? null,
      importComponentByKeyAsync: async (key: string) => {
        importedKey = key;
        return makeComponent({ id: "lib:1", name: "LibComponent", key });
      },
      commitUndo: () => { commitUndoCalled = true; },
    };
  });

  it("creates an instance from componentId pointing to a COMPONENT", async () => {
    mockNodes["comp:src"] = makeComponent();
    const res = await handleWriteCreateRequest(makeRequest("create_instance", [], { componentId: "comp:src" }));
    expect(res?.data.type).toBe("INSTANCE");
    expect(res?.data.mainComponentId).toBe("comp:src");
    expect(res?.data.mainComponentKey).toBe("abc123def456");
    expect(commitUndoCalled).toBe(true);
    expect(parentNode.children).toHaveLength(1);
  });

  it("imports a library component by key", async () => {
    const res = await handleWriteCreateRequest(makeRequest("create_instance", [], { componentKey: "abc123def456" }));
    expect(importedKey).toBe("abc123def456");
    expect(res?.data.mainComponentId).toBe("lib:1");
    expect(res?.data.type).toBe("INSTANCE");
  });

  it("picks the default variant when given a COMPONENT_SET without variantProperties", async () => {
    const variantA = makeComponent({ id: "v:a", name: "A", variantProperties: { Size: "Small" } });
    const variantB = makeComponent({ id: "v:b", name: "B", variantProperties: { Size: "Large" } });
    mockNodes["set:1"] = {
      id: "set:1", name: "Button", type: "COMPONENT_SET",
      children: [variantA, variantB],
      defaultVariant: variantA,
    };
    const res = await handleWriteCreateRequest(makeRequest("create_instance", [], { componentId: "set:1" }));
    expect(res?.data.mainComponentId).toBe("v:a");
  });

  it("picks the matching variant when variantProperties are provided", async () => {
    const variantA = makeComponent({ id: "v:a", name: "A", variantProperties: { Size: "Small", State: "Default" } });
    const variantB = makeComponent({ id: "v:b", name: "B", variantProperties: { Size: "Large", State: "Hover" } });
    mockNodes["set:1"] = { id: "set:1", type: "COMPONENT_SET", children: [variantA, variantB] };
    const res = await handleWriteCreateRequest(makeRequest("create_instance", [], {
      componentId: "set:1",
      variantProperties: { Size: "Large", State: "Hover" },
    }));
    expect(res?.data.mainComponentId).toBe("v:b");
  });

  it("rejects when no variant matches the requested properties", async () => {
    const variantA = makeComponent({ id: "v:a", variantProperties: { Size: "Small" } });
    mockNodes["set:1"] = { id: "set:1", type: "COMPONENT_SET", children: [variantA] };
    await expect(
      handleWriteCreateRequest(makeRequest("create_instance", [], {
        componentId: "set:1",
        variantProperties: { Size: "XL" },
      })),
    ).rejects.toThrow("No variant in set");
  });

  it("rejects an INSTANCE id with a clear message", async () => {
    mockNodes["inst:1"] = { id: "inst:1", type: "INSTANCE" };
    await expect(
      handleWriteCreateRequest(makeRequest("create_instance", [], { componentId: "inst:1" })),
    ).rejects.toThrow("is an INSTANCE, not a COMPONENT");
  });

  it("rejects unrelated node types", async () => {
    mockNodes["frame:1"] = { id: "frame:1", type: "FRAME" };
    await expect(
      handleWriteCreateRequest(makeRequest("create_instance", [], { componentId: "frame:1" })),
    ).rejects.toThrow("must be COMPONENT or COMPONENT_SET");
  });

  it("rejects when neither componentId nor componentKey is provided", async () => {
    await expect(
      handleWriteCreateRequest(makeRequest("create_instance", [], {})),
    ).rejects.toThrow("componentId or componentKey is required");
  });

  it("applies x, y and name overrides", async () => {
    mockNodes["comp:src"] = makeComponent();
    const res = await handleWriteCreateRequest(makeRequest("create_instance", [], {
      componentId: "comp:src", x: 50, y: 75, name: "Renamed",
    }));
    const inst = createdInstances[0];
    expect(inst.x).toBe(50);
    expect(inst.y).toBe(75);
    expect(inst.name).toBe("Renamed");
    expect(res?.data.name).toBe("Renamed");
  });
});
