-- PostgreSQL / Supabase migration for Goals and Goal Activities
-- Safe and additive: creates new tables and indexes without modifying existing tables or data.

CREATE TABLE IF NOT EXISTS goals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    goal_name VARCHAR(100) NOT NULL,
    deadline_at TIMESTAMPTZ NOT NULL,
    timezone VARCHAR(64) NOT NULL DEFAULT 'UTC',
    motivation_line VARCHAR(200),
    deleted_at TIMESTAMPTZ,
    purge_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS ix_goals_user_id ON goals(user_id);
CREATE INDEX IF NOT EXISTS ix_goals_user_deleted ON goals(user_id, deleted_at);
CREATE INDEX IF NOT EXISTS ix_goals_purge_at ON goals(purge_at);

CREATE TABLE IF NOT EXISTS goal_activities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    goal_id UUID NOT NULL REFERENCES goals(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    activity_text VARCHAR(50) NOT NULL,
    activity_date DATE NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS ix_goal_activities_goal_id ON goal_activities(goal_id);
CREATE INDEX IF NOT EXISTS ix_goal_activities_user_id ON goal_activities(user_id);
CREATE INDEX IF NOT EXISTS ix_goal_activities_goal_date ON goal_activities(goal_id, activity_date);
CREATE INDEX IF NOT EXISTS ix_goal_activities_user_date ON goal_activities(user_id, activity_date);
