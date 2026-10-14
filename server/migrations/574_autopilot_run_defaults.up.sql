-- Per-autopilot model / thinking-level defaults. Runs started by an autopilot
-- (its run_only task, and every task on the issue it created) use these
-- instead of the assignee agent's saved values, so a scheduled job can run on
-- a cheaper or stronger model without editing the agent. NULL means "use the
-- agent's own setting". Only agent-assigned autopilots carry them.
ALTER TABLE autopilot ADD COLUMN IF NOT EXISTS model TEXT;
ALTER TABLE autopilot ADD COLUMN IF NOT EXISTS thinking_level TEXT;
