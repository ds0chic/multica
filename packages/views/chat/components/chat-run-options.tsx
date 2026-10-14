"use client";

import { useQuery } from "@tanstack/react-query";
import { useAuthStore } from "@multica/core/auth";
import { selectChatRunOptions, useChatRunOptionsStore } from "@multica/core/chat";
import { useWorkspaceId } from "@multica/core/hooks";
import {
  isRuntimeUsableForUser,
  runtimeListOptions,
  runtimeModelsOptions,
} from "@multica/core/runtimes";
import type { Agent } from "@multica/core/types";
import { ModelPicker } from "../../agents/components/inspector/model-picker";
import { ThinkingPicker } from "../../agents/components/inspector/thinking-picker";
import { pickModelEntry } from "../../agents/components/inspector/thinking-prop-row";

/**
 * Model and effort chips for the chat composer. The pick applies to the next
 * messages sent to this agent only (sent as a per-run override) and never
 * edits the agent. With no pick, the chips show the agent's own settings, and
 * choosing that same value again clears the pick.
 */
export function ChatRunOptions({ agent }: { agent: Agent | null | undefined }) {
  const wsId = useWorkspaceId();
  const userId = useAuthStore((s) => s.user?.id);
  const agentId = agent?.id ?? null;
  const pick = useChatRunOptionsStore(selectChatRunOptions(agentId));
  const setModel = useChatRunOptionsStore((s) => s.setModel);
  const setThinkingLevel = useChatRunOptionsStore((s) => s.setThinkingLevel);

  const { data: runtimes = [] } = useQuery(runtimeListOptions(wsId));
  const runtime = agent?.runtime_id
    ? runtimes.find((r) => r.id === agent.runtime_id)
    : undefined;
  const runtimeId = agent?.runtime_id || null;
  const canDiscover =
    runtime?.status === "online" && isRuntimeUsableForUser(runtime, userId ?? null);
  const modelsQuery = useQuery(runtimeModelsOptions(canDiscover ? runtimeId : null));

  if (!agent || !runtimeId) return null;

  const provider = runtime?.provider ?? "";
  const agentModel = agent.model ?? "";
  const agentThinking = agent.thinking_level ?? "";
  const model = pick.model || agentModel;
  const thinkingLevel = pick.thinkingLevel || agentThinking;
  const levels =
    pickModelEntry(modelsQuery.data?.models ?? [], model, provider)?.thinking
      ?.supported_levels ?? [];

  return (
    <>
      <ModelPicker
        leadingIcon
        runtimeId={runtimeId}
        runtimeOnline={canDiscover}
        value={model}
        onChange={(next) => setModel(agent.id, next === agentModel ? "" : next)}
      />
      {(levels.length > 0 || thinkingLevel) && (
        <ThinkingPicker
          leadingIcon
          value={thinkingLevel}
          levels={levels}
          onChange={(next) =>
            setThinkingLevel(agent.id, next === agentThinking ? "" : next)
          }
        />
      )}
    </>
  );
}
