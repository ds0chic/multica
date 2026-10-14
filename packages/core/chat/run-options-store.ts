"use client";

import { create } from "zustand";
import { createJSONStorage, persist } from "zustand/middleware";
import { defaultStorage } from "../platform/storage";
import type { ChatRunOptions } from "../types";

const EMPTY: ChatRunOptions = {};

/**
 * The composer's model / thinking-level pick, keyed by agent id. It is sent
 * with each chat message as a per-run override and never changes the agent's
 * saved settings. A pick stays until the user changes or clears it, so the
 * next message reuses it, like a CLI's `/model`.
 */
interface ChatRunOptionsState {
  byAgent: Record<string, ChatRunOptions>;
  /** Picking a different model clears the thinking level, since levels are per-model. */
  setModel: (agentId: string, model: string) => void;
  setThinkingLevel: (agentId: string, thinkingLevel: string) => void;
  reset: (agentId: string) => void;
}

function compact(options: ChatRunOptions): ChatRunOptions | null {
  const next: ChatRunOptions = {};
  if (options.model) next.model = options.model;
  if (options.thinkingLevel) next.thinkingLevel = options.thinkingLevel;
  return next.model || next.thinkingLevel ? next : null;
}

function withEntry(
  byAgent: Record<string, ChatRunOptions>,
  agentId: string,
  options: ChatRunOptions,
): Record<string, ChatRunOptions> {
  const { [agentId]: _previous, ...rest } = byAgent;
  const next = compact(options);
  return next ? { ...rest, [agentId]: next } : rest;
}

export const useChatRunOptionsStore = create<ChatRunOptionsState>()(
  persist(
    (set) => ({
      byAgent: {},
      setModel: (agentId, model) =>
        set((state) => {
          const current = state.byAgent[agentId] ?? EMPTY;
          if ((current.model ?? "") === model) return state;
          return { byAgent: withEntry(state.byAgent, agentId, { model }) };
        }),
      setThinkingLevel: (agentId, thinkingLevel) =>
        set((state) => {
          const current = state.byAgent[agentId] ?? EMPTY;
          if ((current.thinkingLevel ?? "") === thinkingLevel) return state;
          return {
            byAgent: withEntry(state.byAgent, agentId, { ...current, thinkingLevel }),
          };
        }),
      reset: (agentId) =>
        set((state) =>
          agentId in state.byAgent
            ? { byAgent: withEntry(state.byAgent, agentId, {}) }
            : state,
        ),
    }),
    {
      name: "multica:chat:run-options",
      storage: createJSONStorage(() => defaultStorage),
      partialize: (state) => ({ byAgent: state.byAgent }),
    },
  ),
);

/** Stable selector: the stored pick for an agent, or a shared empty object. */
export function selectChatRunOptions(agentId: string | null | undefined) {
  return (state: ChatRunOptionsState): ChatRunOptions =>
    (agentId ? state.byAgent[agentId] : undefined) ?? EMPTY;
}
