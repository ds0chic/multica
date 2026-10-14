import { useImperativeHandle, useRef, useState } from "react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderWithI18n } from "../../test/i18n";

// The autopilot's own model / thinking pick for its runs: shown on the agent's
// values, sent only when changed, and cleared back to the agent's setting by
// picking the agent's value again.

const mockCreateAutopilot = vi.hoisted(() => vi.fn());
const mockUpdateAutopilot = vi.hoisted(() => vi.fn());

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-test" }));
vi.mock("@multica/core/paths", () => ({ useCurrentWorkspace: () => ({ name: "Acme" }) }));

vi.mock("@multica/core/workspace/queries", () => ({
  agentListOptions: (wsId: string) => ({
    queryKey: ["agents", wsId],
    queryFn: async () => [
      {
        id: "agent-1",
        name: "Scout",
        description: "Researches things",
        archived_at: null,
        runtime_id: "runtime-1",
        model: "claude-sonnet",
        thinking_level: "high",
      },
    ],
  }),
  squadListOptions: (wsId: string) => ({
    queryKey: ["squads", wsId],
    queryFn: async () => [],
  }),
}));

vi.mock("@multica/core/projects/queries", () => ({
  projectListOptions: (wsId: string) => ({
    queryKey: ["projects", wsId],
    queryFn: async () => [{ id: "proj-1", title: "Fleet", icon: null }],
  }),
}));

vi.mock("@multica/core/autopilots/queries", () => ({
  cronPreviewOptions: (wsId: string, expr: string, tz: string) => ({
    queryKey: ["cron-preview", wsId, expr, tz],
    queryFn: async () => ({ next_runs: ["2126-07-14T01:00:00Z"] }),
    retry: false,
  }),
}));

vi.mock("@multica/core/autopilots/mutations", () => ({
  useCreateAutopilot: () => ({ mutateAsync: mockCreateAutopilot }),
  useCreateAutopilotTrigger: () => ({ mutateAsync: vi.fn().mockResolvedValue({ id: "trg-new" }) }),
  useUpdateAutopilot: () => ({ mutateAsync: mockUpdateAutopilot }),
  useUpdateAutopilotTrigger: () => ({ mutateAsync: vi.fn() }),
}));

vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));
// The assignee agent's model / thinking section reads the signed-in user and
// the runtime catalog; an offline runtime keeps it to the agent's own values.
vi.mock("@multica/core/auth", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/auth")>()),
  useAuthStore: (selector: (state: { user: { id: string } }) => unknown) =>
    selector({ user: { id: "user-1" } }),
}));
vi.mock("@multica/core/runtimes", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/runtimes")>()),
  isRuntimeUsableForUser: () => true,
  runtimeListOptions: (wsId: string) => ({
    queryKey: ["runtimes", wsId],
    queryFn: async () => [{ id: "runtime-1", provider: "claude", status: "online" }],
  }),
  runtimeModelsOptions: (runtimeId: string | null) => ({
    enabled: Boolean(runtimeId),
    queryKey: ["runtime-models", runtimeId],
    queryFn: async () => ({
      models: [
        {
          id: "claude-sonnet",
          label: "Sonnet",
          thinking: { supported_levels: [{ value: "low", label: "Low" }, { value: "high", label: "High" }] },
        },
        {
          id: "claude-opus",
          label: "Opus",
          thinking: { supported_levels: [{ value: "low", label: "Low" }, { value: "high", label: "High" }] },
        },
      ],
      supported: true,
    }),
  }),
}));

vi.mock("../../editor", () => ({
  TitleEditor: ({ ref, defaultValue, placeholder, onChange, onSubmit }: any) => {
    const [value, setValue] = useState(defaultValue ?? "");
    const inputRef = useRef<HTMLInputElement>(null);
    useImperativeHandle(ref, () => ({
      getText: () => value,
      focus: () => inputRef.current?.focus(),
      focusAtCoords: () => inputRef.current?.focus(),
    }));
    return (
      <input
        ref={inputRef}
        aria-label="title"
        value={value}
        placeholder={placeholder}
        onChange={(e) => {
          setValue(e.target.value);
          onChange?.(e.target.value);
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter") onSubmit?.();
        }}
      />
    );
  },
  ContentEditor: ({ placeholder }: any) => <textarea aria-label="runbook" placeholder={placeholder} />,
}));

vi.mock("../../common/actor-avatar", () => ({
  ActorAvatar: ({ actorId }: { actorId: string }) => <span data-testid="actor-avatar">{actorId}</span>,
}));

vi.mock("./subscriber-multi-select", () => ({
  SubscriberMultiSelect: () => <div data-testid="subscriber-multi-select" />,
}));

// Stand-in picker: renders the dialog's own trigger (so the selected project
// title is asserted against the real markup) plus a button that reports a
// selection, which is how the create-mode test binds a project.
vi.mock("../../projects/components/project-picker", () => ({
  ProjectPicker: ({
    triggerRender,
    onUpdate,
  }: {
    triggerRender: React.ReactElement;
    onUpdate: (updates: { project_id: string | null }) => void;
  }) => (
    <div>
      {triggerRender}
      <button type="button" onClick={() => onUpdate({ project_id: "proj-1" })}>
        pick Fleet
      </button>
    </div>
  ),
}));

vi.mock("./pickers/timezone-picker", () => ({
  TimezonePicker: ({ value }: { value: string }) => <div data-testid="timezone-picker">{value}</div>,
}));

import { AutopilotDialog } from "./autopilot-dialog";

function renderDialog(mode: "create" | "edit", model: string | null = null) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return renderWithI18n(
    <QueryClientProvider client={qc}>
      {mode === "create" ? (
        <AutopilotDialog
          mode="create"
          open
          onOpenChange={vi.fn()}
          initial={{ assignee_type: "agent", assignee_id: "agent-1", execution_mode: "run_only" }}
        />
      ) : (
        <AutopilotDialog
          mode="edit"
          open
          onOpenChange={vi.fn()}
          autopilotId="ap-1"
          initial={{
            title: "Nightly triage",
            description: "",
            project_id: null,
            assignee_type: "agent",
            assignee_id: "agent-1",
            execution_mode: "run_only",
            model,
            thinking_level: null,
            subscriber_user_ids: [],
          }}
          triggers={[]}
          collaborators={[]}
          canManageAccess={false}
        />
      )}
    </QueryClientProvider>,
  );
}

describe("AutopilotDialog run model", () => {
  beforeEach(() => {
    mockCreateAutopilot.mockReset().mockResolvedValue({ id: "ap-1" });
    mockUpdateAutopilot.mockReset().mockResolvedValue({ id: "ap-1" });
  });

  it("sends a model picked while creating", async () => {
    const user = userEvent.setup();
    renderDialog("create");

    await user.type(screen.getByLabelText("title"), "Nightly triage");
    await user.click(await screen.findByRole("button", { name: /^Model · / }));
    await user.click(await screen.findByText("Opus"));
    await user.click(screen.getByRole("button", { name: "Create autopilot" }));

    await waitFor(() => expect(mockCreateAutopilot).toHaveBeenCalledTimes(1));
    expect(mockCreateAutopilot.mock.calls[0]?.[0]).toMatchObject({
      model: "claude-opus",
      thinking_level: undefined,
    });
  });

  it("clears the pick when the agent's own model is chosen again", async () => {
    const user = userEvent.setup();
    renderDialog("edit", "claude-opus");

    await user.click(await screen.findByRole("button", { name: /^Model · / }));
    await user.click(await screen.findByText("Sonnet"));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(mockUpdateAutopilot).toHaveBeenCalledTimes(1));
    expect(mockUpdateAutopilot.mock.calls[0]?.[0]).toMatchObject({
      model: null,
      thinking_level: null,
    });
  });
});
