import { describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import { forwardRef, type ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import type { TimelineEntry } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";

// An agent's failure notice in the activity timeline offers a plain Retry and
// a "Retry with" picker for another model or effort, targeting the failed run
// and the agent that ran it. The picker itself is covered by
// retry-run-button.test.tsx; this suite pins the wiring on the card.

vi.mock("@multica/core/api", () => ({
  api: { uploadFile: vi.fn() },
  dispatchReasonCode: () => undefined,
  errorCode: () => undefined,
}));

vi.mock("../../navigation", () => ({
  useNavigation: () => ({
    push: vi.fn(),
    pathname: "/acme/issues",
    getShareableUrl: (p: string) => `https://app.example${p}`,
  }),
}));

vi.mock("@multica/core/workspace/hooks", () => ({
  useActorName: () => ({ getActorName: () => "Ada" }),
}));

vi.mock("../../common/actor-avatar", () => ({
  ActorAvatar: () => null,
}));

const retryWithProps = vi.hoisted(() => vi.fn());
vi.mock("./retry-run-button", () => ({
  RetryWithButton: (props: Record<string, unknown>) => {
    retryWithProps(props);
    return <button type="button">Retry with</button>;
  },
}));

vi.mock("../hooks/use-comment-trigger-preview", () => ({
  useCommentTriggerPreview: () => ({ agents: [], blocked: [] }),
}));

vi.mock("../../editor", async () => ({
  ...(await vi.importActual<typeof import("../../editor/use-upload-gate")>("../../editor/use-upload-gate")),
  ...(await vi.importActual<typeof import("../../editor/use-lazy-editor")>("../../editor/use-lazy-editor")),
  ...(await vi.importActual<typeof import("../../editor/use-composer-submit")>("../../editor/use-composer-submit")),
  useEditorUpload: () => ({ uploadWithToast: vi.fn(), upload: vi.fn(), uploading: false }),
  useFileDropZone: () => ({ isDragOver: false, dropZoneProps: {} }),
  FileDropOverlay: () => null,
  ReadonlyContent: ({ content }: { content: string }) => <div>{content}</div>,
  Attachment: () => null,
  AttachmentDownloadProvider: ({ children }: { children: ReactNode }) => <>{children}</>,
  ContentEditor: forwardRef(function MockContentEditor() {
    return <textarea data-testid="editor" />;
  }),
}));

import { CommentCard } from "./comment-card";

function failureNotice(): TimelineEntry {
  return {
    type: "comment",
    id: "failure-1",
    actor_type: "agent",
    actor_id: "agent-1",
    content: "The run failed: model overloaded",
    parent_id: null,
    comment_type: "system",
    source_task_id: "task-1",
    reactions: [],
    attachments: [],
    created_at: "2026-09-11T07:00:00Z",
    updated_at: "2026-09-11T07:00:00Z",
    revision: 1,
  };
}

describe("CommentCard — failed run notice", () => {
  it("offers Retry and Retry with for the failed run's agent", () => {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
    renderWithI18n(
      <QueryClientProvider client={qc}>
        <CommentCard
          issueId="issue-1"
          entry={failureNotice()}
          replies={[]}
          currentUserId="user-1"
          onReply={vi.fn().mockResolvedValue(true)}
          onEdit={vi.fn().mockResolvedValue(undefined)}
          onDelete={vi.fn()}
          onToggleReaction={vi.fn()}
        />
      </QueryClientProvider>,
    );

    expect(screen.getByRole("button", { name: "Retry run" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Retry with" })).toBeInTheDocument();
    expect(retryWithProps).toHaveBeenCalledWith(
      expect.objectContaining({ taskId: "task-1", agentId: "agent-1", issueId: "issue-1" }),
    );
  });
});
