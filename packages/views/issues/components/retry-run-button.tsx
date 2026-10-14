"use client";

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Loader2, RotateCcw, SlidersHorizontal } from "lucide-react";
import { toast } from "sonner";
import { api, dispatchReasonCode } from "@multica/core/api";
import { useAuthStore } from "@multica/core/auth";
import { useWorkspaceId } from "@multica/core/hooks";
import {
  isRuntimeUsableForUser,
  runtimeListOptions,
  runtimeModelsOptions,
} from "@multica/core/runtimes";
import type { AgentTask, ChatRunOptions } from "@multica/core/types";
import { agentListOptions } from "@multica/core/workspace/queries";
import { Button } from "@multica/ui/components/ui/button";
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from "@multica/ui/components/ui/popover";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@multica/ui/components/ui/tooltip";
import { ModelPicker } from "../../agents/components/inspector/model-picker";
import { ThinkingPicker } from "../../agents/components/inspector/thinking-picker";
import { pickModelEntry } from "../../agents/components/inspector/thinking-prop-row";
import { useT } from "../../i18n";

/** Retry only makes sense for terminal-but-not-success runs. */
export function canRetryRun(task: AgentTask): boolean {
  return task.status === "failed" || task.status === "cancelled";
}

function useRetryRun(task: AgentTask, issueId: string) {
  const { t } = useT("issues");
  const [retrying, setRetrying] = useState(false);

  const retry = async (runOptions?: ChatRunOptions): Promise<boolean> => {
    if (retrying) return false;
    setRetrying(true);
    try {
      // Passing task.id targets this row's agent — without it, the rerun
      // endpoint would fall back to the issue's current assignee and the wrong
      // agent would fire on rows whose agent has since been displaced
      // (reassignment, squad worker, or a one-off @-mention agent).
      await api.rerunIssue(issueId, task.id, runOptions);
      return true;
    } catch (e) {
      // A rerun is re-gated on the operator's invoke permission (MUL-4525): a
      // structured 403 means the agent can't be triggered, not a transient
      // failure — localize it instead of echoing the server's generic message.
      toast.error(
        dispatchReasonCode(e) === "invocation_not_allowed"
          ? t(($) => $.execution_log.retry_blocked)
          : e instanceof Error
            ? e.message
            : t(($) => $.execution_log.retry_failed),
      );
      return false;
    } finally {
      // Reset on both success and failure: the row stays mounted (its task.id
      // is unchanged), so leaving `retrying` true on success would pin the
      // button as a permanent spinner.
      setRetrying(false);
    }
  };

  return { retrying, retry };
}

// Re-runs one specific past run. Shared by the execution log's rows and the
// Runs dialog so a failed run retries the same way wherever it is listed.
export function RetryRunButton({ task, issueId }: { task: AgentTask; issueId: string }) {
  const { t } = useT("issues");
  const { retrying, retry } = useRetryRun(task, issueId);
  const handleRetry = () => void retry();

  return (
    <>
      <Tooltip>
        <TooltipTrigger
          render={
            <button
              type="button"
              onClick={handleRetry}
              disabled={retrying}
              aria-label={t(($) => $.execution_log.retry_task_aria)}
            />
          }
          className="flex items-center justify-center rounded-xs p-1 text-muted-foreground transition-colors hover:bg-accent/50 hover:text-foreground disabled:cursor-not-allowed disabled:opacity-50"
        >
          {retrying ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : (
            <RotateCcw className="h-3.5 w-3.5" />
          )}
        </TooltipTrigger>
        <TooltipContent>{t(($) => $.execution_log.retry_task_tooltip)}</TooltipContent>
      </Tooltip>
      <RetryWithButton task={task} issueId={issueId} />
    </>
  );
}

/**
 * "Retry with" picks a model and effort for the new run only. The agent's
 * saved settings stay as they are. The pickers start on the agent's own
 * values, and whatever is shown is what the retry runs with.
 */
function RetryWithButton({ task, issueId }: { task: AgentTask; issueId: string }) {
  const { t } = useT("issues");
  const [open, setOpen] = useState(false);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <Tooltip>
        <TooltipTrigger
          render={
            <PopoverTrigger
              render={
                <button
                  type="button"
                  aria-label={t(($) => $.execution_log.retry_with_aria)}
                />
              }
            />
          }
          className="flex items-center justify-center rounded-xs p-1 text-muted-foreground transition-colors hover:bg-accent/50 hover:text-foreground"
        >
          <SlidersHorizontal className="h-3.5 w-3.5" />
        </TooltipTrigger>
        <TooltipContent>{t(($) => $.execution_log.retry_with_tooltip)}</TooltipContent>
      </Tooltip>
      <PopoverContent align="end" className="w-auto min-w-64 p-2">
        {/* Mounted only while open, so a long run list does not subscribe
            every row to the agent, runtime and model catalogs. */}
        <RetryWithForm task={task} issueId={issueId} onDone={() => setOpen(false)} />
      </PopoverContent>
    </Popover>
  );
}

function RetryWithForm({
  task,
  issueId,
  onDone,
}: {
  task: AgentTask;
  issueId: string;
  onDone: () => void;
}) {
  const { t } = useT("issues");
  const wsId = useWorkspaceId();
  const userId = useAuthStore((s) => s.user?.id);
  const { retrying, retry } = useRetryRun(task, issueId);

  const { data: agents = [] } = useQuery(agentListOptions(wsId));
  const { data: runtimes = [] } = useQuery(runtimeListOptions(wsId));
  const agent = agents.find((a) => a.id === task.agent_id);
  const runtimeId = agent?.runtime_id || null;
  const runtime = runtimeId ? runtimes.find((r) => r.id === runtimeId) : undefined;
  const canDiscover =
    runtime?.status === "online" && isRuntimeUsableForUser(runtime, userId ?? null);
  const modelsQuery = useQuery(runtimeModelsOptions(canDiscover ? runtimeId : null));

  // null = untouched, showing the agent's own value.
  const [pickedModel, setPickedModel] = useState<string | null>(null);
  const [pickedThinking, setPickedThinking] = useState<string | null>(null);
  const model = pickedModel ?? agent?.model ?? "";
  const thinkingLevel = pickedThinking ?? agent?.thinking_level ?? "";

  const provider = runtime?.provider ?? "";
  const levelsFor = (m: string) =>
    pickModelEntry(modelsQuery.data?.models ?? [], m, provider)?.thinking
      ?.supported_levels ?? [];
  const levels = levelsFor(model);

  const handleModelChange = (next: string) => {
    setPickedModel(next);
    // An effort the new model does not offer would be dropped by the daemon,
    // so clear it here where the user can see it instead.
    const nextLevels = levelsFor(next);
    if (thinkingLevel && nextLevels.length > 0 && !nextLevels.some((l) => l.value === thinkingLevel)) {
      setPickedThinking("");
    }
  };

  const handleRetry = async () => {
    if (await retry({ model, thinkingLevel })) onDone();
  };

  return (
    <>
      <div className="px-1 pb-2 text-caption font-medium text-muted-foreground">
        {t(($) => $.execution_log.retry_with_title)}
      </div>
      {runtimeId && (
        <div className="flex flex-wrap items-center gap-1 px-1 pb-2">
          <ModelPicker
            runtimeId={runtimeId}
            runtimeOnline={canDiscover}
            value={model}
            onChange={handleModelChange}
          />
          {(levels.length > 0 || thinkingLevel) && (
            <ThinkingPicker value={thinkingLevel} levels={levels} onChange={setPickedThinking} />
          )}
        </div>
      )}
      <div className="flex justify-end px-1">
        <Button size="sm" onClick={handleRetry} disabled={retrying}>
          {retrying && <Loader2 className="animate-spin" />}
          {t(($) => $.execution_log.retry_with_submit)}
        </Button>
      </div>
    </>
  );
}
