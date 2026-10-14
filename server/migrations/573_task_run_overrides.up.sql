-- Per-run model / thinking-level overrides. A chat message can pick a model
-- and effort for its own run without touching the agent's saved defaults.
-- NULL means "no override": the claim falls back to agent.model /
-- agent.thinking_level exactly as before. Retries inherit the parent's values
-- so a retried turn runs with the same selection.
ALTER TABLE agent_task_queue ADD COLUMN IF NOT EXISTS model_override TEXT;
ALTER TABLE agent_task_queue ADD COLUMN IF NOT EXISTS thinking_level_override TEXT;
