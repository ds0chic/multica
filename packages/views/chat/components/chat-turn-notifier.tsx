"use client";

import { useCallback, useRef } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { notifyChatTurnFinished } from "@multica/core/chat/turn-notification";
import { useCurrentWorkspace } from "@multica/core/paths";
import { useWSEvent } from "@multica/core/realtime";
import type { ChatDonePayload, TaskFailedPayload } from "@multica/core/types";
import { useT } from "../../i18n";

/**
 * Shows a system notification when one of your chat replies finishes or
 * fails while Multica is in the background. Mounted once per app shell (web
 * dashboard layout, desktop shell); renders nothing.
 */
export function ChatTurnNotifier() {
  const qc = useQueryClient();
  const { t } = useT("chat");
  const workspace = useCurrentWorkspace();
  // Handlers stay stable so the WS subscription isn't torn down on every
  // render; they always read the latest workspace and copy.
  const latest = useRef({ workspace, t });
  latest.current = { workspace, t };

  const notify = useCallback(
    (
      sessionId: string | undefined,
      taskId: string,
      outcome: "done" | "failed",
      content?: string,
    ) => {
      const { workspace: ws, t: tr } = latest.current;
      if (!ws || !sessionId) return;
      void notifyChatTurnFinished(qc, {
        wsId: ws.id,
        slug: ws.slug,
        sessionId,
        taskId,
        outcome,
        content,
        copy: {
          untitled: tr(($) => $.window.untitled),
          noResponse: tr(($) => $.system_notification.no_response),
          failed: tr(($) => $.system_notification.failed),
        },
      });
    },
    [qc],
  );

  const onDone = useCallback(
    (raw: unknown) => {
      const p = raw as ChatDonePayload;
      notify(p.chat_session_id, p.task_id, "done", p.content);
    },
    [notify],
  );
  const onFailed = useCallback(
    (raw: unknown) => {
      const p = raw as TaskFailedPayload;
      // A failure that will be retried automatically isn't the end of the turn.
      if (p.retry_pending) return;
      notify(p.chat_session_id, p.task_id, "failed");
    },
    [notify],
  );

  useWSEvent("chat:done", onDone);
  useWSEvent("task:failed", onFailed);
  return null;
}
