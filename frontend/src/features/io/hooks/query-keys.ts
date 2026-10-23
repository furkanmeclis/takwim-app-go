export const ioKeys = {
  exports: {
    all: ["platform", "exports"] as const,
    lists: () => [...ioKeys.exports.all, "list"] as const,
    list: (params: object) => [...ioKeys.exports.lists(), params] as const,
    detail: (uuid: string) => [...ioKeys.exports.all, uuid] as const,
  },
  imports: {
    all: ["platform", "imports"] as const,
    lists: () => [...ioKeys.imports.all, "list"] as const,
    list: (params: object) => [...ioKeys.imports.lists(), params] as const,
    detail: (uuid: string) => [...ioKeys.imports.all, uuid] as const,
  },
  settings: {
    all: ["platform", "settings"] as const,
  },
  activity: {
    all: ["platform", "activity"] as const,
    meta: () => [...ioKeys.activity.all, "meta"] as const,
    lists: () => [...ioKeys.activity.all, "list"] as const,
    list: (params: object) => [...ioKeys.activity.lists(), params] as const,
  },
};
