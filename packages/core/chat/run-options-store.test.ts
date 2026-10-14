// @vitest-environment node
import { beforeEach, describe, expect, it } from "vitest";
import { selectChatRunOptions, useChatRunOptionsStore } from "./run-options-store";

describe("useChatRunOptionsStore", () => {
  beforeEach(() => {
    useChatRunOptionsStore.setState({ byAgent: {} });
  });

  it("keeps a pick per agent", () => {
    const store = useChatRunOptionsStore.getState();
    store.setModel("agent-a", "model-x");
    store.setThinkingLevel("agent-a", "high");
    store.setModel("agent-b", "model-y");

    const state = useChatRunOptionsStore.getState();
    expect(selectChatRunOptions("agent-a")(state)).toEqual({
      model: "model-x",
      thinkingLevel: "high",
    });
    expect(selectChatRunOptions("agent-b")(state)).toEqual({ model: "model-y" });
  });

  it("clears the thinking level when the model changes", () => {
    const store = useChatRunOptionsStore.getState();
    store.setModel("agent-a", "model-x");
    store.setThinkingLevel("agent-a", "high");
    store.setModel("agent-a", "model-z");

    expect(useChatRunOptionsStore.getState().byAgent["agent-a"]).toEqual({
      model: "model-z",
    });
  });

  it("drops the entry once nothing is picked", () => {
    const store = useChatRunOptionsStore.getState();
    store.setThinkingLevel("agent-a", "low");
    store.setThinkingLevel("agent-a", "");

    expect(useChatRunOptionsStore.getState().byAgent).toEqual({});
  });

  it("returns the same empty object for agents with no pick", () => {
    const state = useChatRunOptionsStore.getState();
    expect(selectChatRunOptions("agent-a")(state)).toBe(
      selectChatRunOptions(null)(state),
    );
  });
});
