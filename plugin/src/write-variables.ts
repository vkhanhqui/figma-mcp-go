import { hexToRgb } from "./write-helpers";

const parseVariableValue = (type: string, value: any): VariableValue => {
  // ✅ Handle VARIABLE_ALIAS first
  if (typeof value === "string") {
    try {
      const parsed = JSON.parse(value);
      if (parsed?.type === "VARIABLE_ALIAS" && parsed?.id) {
        return { type: "VARIABLE_ALIAS", id: parsed.id };
      }
    } catch {}
  } else if (value?.type === "VARIABLE_ALIAS" && value?.id) {
    return { type: "VARIABLE_ALIAS", id: value.id };
  }

  if (type === "COLOR") {
    if (typeof value === "string") {
      const { r, g, b, a } = hexToRgb(value);
      return { r, g, b, a };
    }
    return value as RGBA;
  }

  if (type === "FLOAT") return Number(value);
  if (type === "BOOLEAN") return Boolean(value);

  return String(value);
};

export const handleWriteVariableRequest = async (request: any) => {
  switch (request.type) {
    case "create_variable_collection": {
      const p = request.params || {};
      if (!p.name) throw new Error("name is required");
      const collection = figma.variables.createVariableCollection(p.name);
      if (p.initialModeName && collection.modes.length > 0) {
        collection.renameMode(collection.modes[0].modeId, p.initialModeName);
      }
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          id: collection.id,
          name: collection.name,
          modes: collection.modes.map((m) => ({ modeId: m.modeId, name: m.name })),
        },
      };
    }

    case "add_variable_mode": {
      const p = request.params || {};
      if (!p.collectionId) throw new Error("collectionId is required");
      if (!p.modeName) throw new Error("modeName is required");
      const collection = await figma.variables.getVariableCollectionByIdAsync(p.collectionId);
      if (!collection) throw new Error(`Collection not found: ${p.collectionId}`);
      const modeId = collection.addMode(p.modeName);
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { collectionId: p.collectionId, modeId, modeName: p.modeName },
      };
    }

    case "create_variable": {
      const p = request.params || {};
      if (!p.name) throw new Error("name is required");
      if (!p.collectionId) throw new Error("collectionId is required");
      const validTypes = ["COLOR", "FLOAT", "STRING", "BOOLEAN"];
      if (!p.type || !validTypes.includes(p.type)) {
        throw new Error("type is required: COLOR, FLOAT, STRING, or BOOLEAN");
      }
      const collection = await figma.variables.getVariableCollectionByIdAsync(p.collectionId);
      if (!collection) throw new Error(`Collection not found: ${p.collectionId}`);
      const variable = figma.variables.createVariable(p.name, collection, p.type as VariableResolvedDataType);
      if (p.value != null && collection.modes.length > 0) {
        const modeId = collection.modes[0].modeId;
        variable.setValueForMode(modeId, parseVariableValue(p.type, p.value));
      }
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: {
          id: variable.id,
          name: variable.name,
          resolvedType: variable.resolvedType,
          collectionId: p.collectionId,
        },
      };
    }

    case "set_variable_value": {
      const p = request.params || {};
      if (!p.variableId) throw new Error("variableId is required");
      if (!p.modeId) throw new Error("modeId is required");
      if (p.value == null) throw new Error("value is required");
      const variable = await figma.variables.getVariableByIdAsync(p.variableId);
      if (!variable) throw new Error(`Variable not found: ${p.variableId}`);
      variable.setValueForMode(p.modeId, parseVariableValue(variable.resolvedType, p.value));
      figma.commitUndo();
      return {
        type: request.type,
        requestId: request.requestId,
        data: { variableId: variable.id, name: variable.name, modeId: p.modeId },
      };
    }

    case "delete_variable": {
      const p = request.params || {};
      if (p.variableId) {
        const variable = await figma.variables.getVariableByIdAsync(p.variableId);
        if (!variable) throw new Error(`Variable not found: ${p.variableId}`);
        variable.remove();
        figma.commitUndo();
        return {
          type: request.type,
          requestId: request.requestId,
          data: { variableId: p.variableId, deleted: true },
        };
      } else if (p.collectionId) {
        const collection = await figma.variables.getVariableCollectionByIdAsync(p.collectionId);
        if (!collection) throw new Error(`Collection not found: ${p.collectionId}`);
        collection.remove();
        figma.commitUndo();
        return {
          type: request.type,
          requestId: request.requestId,
          data: { collectionId: p.collectionId, deleted: true },
        };
      } else {
        throw new Error("variableId or collectionId is required");
      }
    }

    default:
      return null;
  }
};
