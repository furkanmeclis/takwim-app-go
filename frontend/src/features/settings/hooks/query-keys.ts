export const systemKeys = {
  all: ["platform", "system"] as const,
  info: () => [...systemKeys.all, "info"] as const,
};
