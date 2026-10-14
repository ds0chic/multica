import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { render, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { I18nProvider } from "@multica/core/i18n/react";
import { chatKeys } from "@multica/core/chat/queries";
import { notificationPreferenceKeys } from "@multica/core/notification-preferences/queries";
import type { ChatSession } from "@multica/core/types";
import enChat from "../../locales/en/chat.json";

const handlers = vi.hoisted(() => new Map<string, (payload: unknown) => void>());

vi.mock("@multica/core/realtime", () => ({
  useWSEvent: (event: string, handler: (payload: unknown) => void) => {
    handlers.set(event, handler);
  },
}));

vi.mock("@multica/core/paths", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/paths")>()),
  useCurrentWorkspace: () => ({ id: "ws-1", slug: "acme" }),
}));

import { ChatTurnNotifier } from "./chat-turn-notifier";

const session: ChatSession = {
  id: "session-1",
  workspace_id: "ws-1",
  agent_id: "agent-1",
  creator_id: "user-1",
  title: "Fix the login bug",
  status: "active",
  has_unread: true,
  created_at: new Date(0).toISOString(),
  updated_at: new Date(0).toISOString(),
};

let showNotification: ReturnType<typeof vi.fn>;

function renderNotifier() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  qc.setQueryData(chatKeys.sessions("ws-1"), [session]);
  qc.setQueryData(notificationPreferenceKeys.all("ws-1"), {
    preferences: { system_notifications: "all" },
  });
  render(
    <QueryClientProvider client={qc}>
      <I18nProvider locale="en" resources={{ en: { chat: enChat } }}>
        <ChatTurnNotifier />
      </I18nProvider>
    </QueryClientProvider>,
  );
}

describe("ChatTurnNotifier", () => {
  beforeEach(() => {
    handlers.clear();
    showNotification = vi.fn();
    (window as unknown as Record<string, unknown>).desktopAPI = { showNotification };
    vi.spyOn(document, "hasFocus").mockReturnValue(false);
  });

  afterEach(() => {
    delete (window as unknown as Record<string, unknown>).desktopAPI;
    vi.restoreAllMocks();
  });

  it("notifies when a chat reply finishes in the background", async () => {
    renderNotifier();

    handlers.get("chat:done")!({
      chat_session_id: "session-1",
      task_id: "task-1",
      content: "The login form now validates email.",
    });

    await waitFor(() =>
      expect(showNotification).toHaveBeenCalledWith({
        slug: "acme",
        itemId: "chat:task-1",
        issueKey: "session-1",
        title: "Fix the login bug",
        body: "The login form now validates email.",
        path: "/acme/chat?session=session-1",
      }),
    );
  });

  it("notifies on a final chat failure but not on one that will be retried", async () => {
    renderNotifier();

    handlers.get("task:failed")!({
      task_id: "task-1",
      chat_session_id: "session-1",
      retry_pending: true,
    });
    handlers.get("task:failed")!({ task_id: "task-2", issue_id: "issue-1" });
    handlers.get("task:failed")!({ task_id: "task-3", chat_session_id: "session-1" });

    await waitFor(() => expect(showNotification).toHaveBeenCalledTimes(1));
    expect(showNotification).toHaveBeenCalledWith(
      expect.objectContaining({
        itemId: "chat:task-3",
        body: enChat.system_notification.failed,
      }),
    );
  });

  it("stays quiet while the app is focused", async () => {
    vi.spyOn(document, "hasFocus").mockReturnValue(true);
    renderNotifier();

    handlers.get("chat:done")!({ chat_session_id: "session-1", task_id: "task-1" });

    await new Promise((r) => setTimeout(r, 20));
    expect(showNotification).not.toHaveBeenCalled();
  });
});
