// @vitest-environment jsdom

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { I18nProvider } from "@multica/core/i18n/react";
import type { AgentTask, RuntimeModelsResult } from "@multica/core/types";
import { afterEach, describe, expect, it, vi } from "vitest";
import enAgents from "../../locales/en/agents.json";
import enCommon from "../../locales/en/common.json";
import enIssues from "../../locales/en/issues.json";
import { RetryRunButton } from "./retry-run-button";

const TEST_RESOURCES = {
  en: { common: enCommon, agents: enAgents, issues: enIssues },
};

const mockRerunIssue = vi.hoisted(() => vi.fn());

vi.mock("@multica/core/api", () => ({
  api: { rerunIssue: (...args: unknown[]) => mockRerunIssue(...args) },
  dispatchReasonCode: () => null,
}));

vi.mock("@multica/core/auth", () => ({
  useAuthStore: (selector: (state: { user: { id: string } }) => unknown) =>
    selector({ user: { id: "user-1" } }),
}));

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));

const CATALOG: RuntimeModelsResult = {
  models: [
    {
      id: "claude-sonnet",
      label: "Sonnet",
      thinking: { supported_levels: [{ value: "low", label: "Low" }, { value: "high", label: "High" }] },
    },
    {
      id: "claude-opus",
      label: "Opus",
      thinking: { supported_levels: [{ value: "low", label: "Low" }, { value: "max", label: "Max" }] },
    },
  ],
  supported: true,
};

vi.mock("@multica/core/runtimes", () => ({
  isRuntimeUsableForUser: () => true,
  runtimeListOptions: () => ({
    queryKey: ["runtimes"],
    queryFn: async () => [{ id: "rt-1", provider: "claude", status: "online" }],
  }),
  runtimeModelsOptions: (runtimeId: string | null) => ({
    enabled: Boolean(runtimeId),
    queryKey: ["runtime-models", runtimeId],
    queryFn: async () => CATALOG,
  }),
  refreshRuntimeModels: vi.fn(),
}));

vi.mock("@multica/core/workspace/queries", () => ({
  agentListOptions: () => ({
    queryKey: ["agents"],
    queryFn: async () => [
      { id: "agent-1", runtime_id: "rt-1", model: "claude-sonnet", thinking_level: "high" },
    ],
  }),
}));

const TASK = { id: "task-1", agent_id: "agent-1", status: "failed" } as AgentTask;

function renderButton() {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <I18nProvider locale="en" resources={TEST_RESOURCES}>
      <QueryClientProvider client={queryClient}>
        <RetryRunButton task={TASK} issueId="issue-1" />
      </QueryClientProvider>
    </I18nProvider>,
  );
}

describe("RetryRunButton", () => {
  afterEach(() => {
    cleanup();
    mockRerunIssue.mockReset();
  });

  it("retries with the agent's own settings by default", async () => {
    mockRerunIssue.mockResolvedValue({});
    renderButton();
    fireEvent.click(screen.getByRole("button", { name: "Retry run" }));
    await waitFor(() => expect(mockRerunIssue).toHaveBeenCalledWith("issue-1", "task-1", undefined));
  });

  it("retries with a model and effort picked for this run only", async () => {
    mockRerunIssue.mockResolvedValue({});
    renderButton();

    fireEvent.click(
      await screen.findByRole("button", { name: "Retry with another model or effort" }),
    );
    fireEvent.click(await screen.findByRole("button", { name: /^Model · / }));
    fireEvent.click(await screen.findByText("Opus"));
    // Opus does not offer the agent's "high", so the pick resets to the
    // runtime default instead of sending a level the daemon would drop.
    fireEvent.click(screen.getByRole("button", { name: "Retry" }));

    await waitFor(() =>
      expect(mockRerunIssue).toHaveBeenCalledWith("issue-1", "task-1", {
        model: "claude-opus",
        thinkingLevel: "",
      }),
    );
  });
});
