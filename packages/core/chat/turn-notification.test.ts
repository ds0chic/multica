// @vitest-environment node
import { QueryClient } from "@tanstack/react-query";
import { afterEach, describe, expect, it, vi } from "vitest";
import { notificationPreferenceKeys } from "../notification-preferences/queries";
import type { ChatSession } from "../types";
import { chatKeys } from "./queries";
import {
  notifyChatTurnFinished,
  type ChatTurnNotificationInput,
} from "./turn-notification";

const copy = { untitled: "New chat", noResponse: "No reply", failed: "Reply failed" };

function session(overrides: Partial<ChatSession> = {}): ChatSession {
  return {
    id: "session-1",
    workspace_id: "ws-1",
    agent_id: "agent-1",
    creator_id: "member-1",
    title: "Fix the login bug",
    status: "active",
    has_unread: true,
    created_at: "2026-10-14T00:00:00Z",
    updated_at: "2026-10-14T00:00:00Z",
    ...overrides,
  };
}

function setup(sessions: ChatSession[], muted = false) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(chatKeys.sessions("ws-1"), sessions);
  qc.setQueryData(notificationPreferenceKeys.all("ws-1"), {
    preferences: { system_notifications: muted ? "muted" : "all" },
  });
  const showNotification = vi.fn();
  (globalThis as Record<string, unknown>).desktopAPI = { showNotification };
  return { qc, showNotification };
}

function input(overrides: Partial<ChatTurnNotificationInput> = {}): ChatTurnNotificationInput {
  return {
    wsId: "ws-1",
    slug: "acme",
    sessionId: "session-1",
    taskId: "task-1",
    outcome: "done",
    content: "Done.\n\nThe login form now   validates email.",
    copy,
    ...overrides,
  };
}

afterEach(() => {
  delete (globalThis as Record<string, unknown>).desktopAPI;
});

describe("notifyChatTurnFinished", () => {
  it("shows a banner for a finished reply that opens the chat session", async () => {
    const { qc, showNotification } = setup([session()]);

    await expect(notifyChatTurnFinished(qc, input())).resolves.toBe(true);

    expect(showNotification).toHaveBeenCalledWith({
      slug: "acme",
      itemId: "chat:task-1",
      issueKey: "session-1",
      title: "Fix the login bug",
      body: "Done. The login form now validates email.",
      path: "/acme/chat?session=session-1",
    });
  });

  it("uses the failure copy for a failed turn", async () => {
    const { qc, showNotification } = setup([session({ title: "" })]);

    await notifyChatTurnFinished(qc, input({ outcome: "failed", content: undefined }));

    expect(showNotification).toHaveBeenCalledWith(
      expect.objectContaining({ title: "New chat", body: "Reply failed" }),
    );
  });

  it("falls back to the no-reply copy and truncates long replies", async () => {
    const { qc, showNotification } = setup([session()]);

    await notifyChatTurnFinished(qc, input({ content: "" }));
    await notifyChatTurnFinished(qc, input({ taskId: "task-2", content: "x".repeat(500) }));

    expect(showNotification.mock.calls[0]![0].body).toBe("No reply");
    expect(showNotification.mock.calls[1]![0].body).toHaveLength(200);
  });

  it("stays silent for another member's chat session", async () => {
    // chat:done fans out to the whole workspace; the creator-scoped list
    // does not contain other members' sessions.
    const { qc, showNotification } = setup([session({ id: "someone-elses" })]);

    await expect(notifyChatTurnFinished(qc, input())).resolves.toBe(false);
    expect(showNotification).not.toHaveBeenCalled();
  });

  it("respects the muted system-notification preference", async () => {
    const { qc, showNotification } = setup([session()], true);

    await expect(notifyChatTurnFinished(qc, input())).resolves.toBe(false);
    expect(showNotification).not.toHaveBeenCalled();
  });
});
