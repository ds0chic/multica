import type { QueryClient } from "@tanstack/react-query";
import { notificationPreferenceOptions } from "../notification-preferences/queries";
import { paths } from "../paths";
import { dispatchSystemNotification } from "../platform/system-notification";
import { chatSessionsOptions } from "./queries";

/** Localized strings the host supplies; core has no translation catalog. */
export interface ChatTurnNotificationCopy {
  /** Title when the session has no title yet. */
  untitled: string;
  /** Body for a turn that finished without a text reply. */
  noResponse: string;
  /** Body for a failed turn. */
  failed: string;
}

export interface ChatTurnNotificationInput {
  wsId: string;
  slug: string;
  sessionId: string;
  taskId: string;
  outcome: "done" | "failed";
  /** Reply text from chat:done; absent on failure or from older servers. */
  content?: string;
  copy: ChatTurnNotificationCopy;
}

const BODY_LIMIT = 200;

function summarize(content: string): string {
  const flat = content.replace(/\s+/g, " ").trim();
  return flat.length > BODY_LIMIT ? `${flat.slice(0, BODY_LIMIT - 1)}…` : flat;
}

/**
 * Shows a system notification when a chat turn finishes while Multica is in
 * the background. Chat replies never create inbox items, so without this the
 * only signal is the unread badge inside the app.
 *
 * chat:done / task:failed fan out to every member of the workspace, so the
 * banner is limited to sessions in the current user's own chat list (the list
 * endpoint is creator-scoped). Honors the workspace's system-notification
 * mute preference, the same switch inbox banners use. Returns whether a
 * banner was dispatched.
 */
export async function notifyChatTurnFinished(
  qc: QueryClient,
  input: ChatTurnNotificationInput,
): Promise<boolean> {
  if (typeof document !== "undefined" && document.hasFocus()) return false;
  if (!input.wsId || !input.slug || !input.sessionId) return false;

  let title: string;
  try {
    const sessions = await qc.ensureQueryData(chatSessionsOptions(input.wsId));
    const session = sessions.find((s) => s.id === input.sessionId);
    if (!session) return false;
    title = session.title.trim() || input.copy.untitled;
  } catch {
    return false;
  }

  try {
    const prefs = await qc.ensureQueryData(
      notificationPreferenceOptions(input.wsId, input.slug),
    );
    if (prefs?.preferences?.system_notifications === "muted") return false;
  } catch {
    // Fall through with the default ("all"), matching inbox banners.
  }

  const body =
    input.outcome === "failed"
      ? input.copy.failed
      : summarize(input.content ?? "") || input.copy.noResponse;

  dispatchSystemNotification({
    slug: input.slug,
    // One banner per turn; the desktop main process and the browser both
    // collapse repeats on this id across windows and reconnect replays.
    itemId: `chat:${input.taskId || input.sessionId}`,
    issueKey: input.sessionId,
    title,
    body,
    path: paths.workspace(input.slug).chatSession(input.sessionId),
  });
  return true;
}
