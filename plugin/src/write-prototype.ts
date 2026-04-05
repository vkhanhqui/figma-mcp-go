function buildReaction(r: any): Reaction {
  // `actions` (plural array) is the current API; `action` (singular) is deprecated.
  // Accept either form so callers don't need to worry about the distinction.
  const actions: Action[] = r.actions ?? (r.action != null ? [r.action] : []);
  return { trigger: r.trigger ?? null, actions } as Reaction;
}

// The MCP framework may pass array params as a JSON string. Parse defensively.
function parseArray(v: any): any[] {
  if (Array.isArray(v)) return v;
  if (typeof v === "string") {
    try { return JSON.parse(v); } catch { return []; }
  }
  return [];
}

// setReactionsAsync is required when documentAccess is "dynamic-page".
// Fall back to direct assignment only when setReactionsAsync is unavailable (older Figma).
async function setReactions(node: any, reactions: Reaction[]): Promise<void> {
  if (typeof node.setReactionsAsync === "function") {
    await node.setReactionsAsync(reactions);
    return;
  }
  try {
    node.reactions = reactions;
  } catch (e) {
    throw new Error(`Failed to set reactions: ${e instanceof Error ? e.message : String(e)}`);
  }
}

export const handleWritePrototypeRequest = async (request: any) => {
  switch (request.type) {
    case "set_reactions": {
      const p = request.params || {};
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeId is required");
      const node = await figma.getNodeByIdAsync(nodeId);
      if (!node) throw new Error(`Node not found: ${nodeId}`);
      if (!("reactions" in node)) throw new Error(`Node ${nodeId} does not support reactions`);

      const incoming: Reaction[] = parseArray(p.reactions).map(buildReaction);
      const current: Reaction[] = (node as any).reactions;
      const final = p.mode === "append" ? [...current, ...incoming] : incoming;

      await setReactions(node, final);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { id: node.id, name: (node as any).name, reactionCount: final.length },
      };
    }

    case "remove_reactions": {
      const p = request.params || {};
      const nodeId = request.nodeIds && request.nodeIds[0];
      if (!nodeId) throw new Error("nodeId is required");
      const node = await figma.getNodeByIdAsync(nodeId);
      if (!node) throw new Error(`Node not found: ${nodeId}`);
      if (!("reactions" in node)) throw new Error(`Node ${nodeId} does not support reactions`);

      const current: Reaction[] = (node as any).reactions;
      let updated: Reaction[];
      if (p.indices == null) {
        // indices not provided → remove all
        updated = [];
      } else {
        const indices = parseArray(p.indices);
        if (indices.length === 0) {
          // indices provided but empty → remove all
          updated = [];
        } else {
          const toRemove = new Set<number>(indices);
          updated = current.filter((_: any, i: number) => !toRemove.has(i));
        }
      }

      await setReactions(node, updated);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          id: node.id,
          name: (node as any).name,
          removed: current.length - updated.length,
          reactionCount: updated.length,
        },
      };
    }

    default:
      return null;
  }
};
