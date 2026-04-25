import type React from 'react';

export interface ActivityDefinition {
  type: string;
  displayName: string;
  icon?: React.ReactNode;
  render: React.FC<{ vc: string; params: unknown }>;
  controls?: React.FC<{ vc: string }>;
}

const registry = new Map<string, ActivityDefinition>();

export function register(def: ActivityDefinition): void {
  registry.set(def.type, def);
}

export function lookup(type: string): ActivityDefinition | null {
  return registry.get(type) ?? null;
}

export function list(): ActivityDefinition[] {
  return Array.from(registry.values());
}

export function _resetForTests(): void {
  registry.clear();
}
