-- +goose Up

-- ============================================================
-- Cloud Lab Gateway — initial schema
-- ============================================================
-- All state enums live as TEXT + CHECK constraints to keep migrations
-- simple. Domain code constants are the source of truth for valid values.
--
-- goose splits this file on `;`. The only places where we wrap statements
-- in -- +goose StatementBegin/End are the PL/pgSQL function (because the
-- function body contains `;` characters that goose must not split on).

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- ------------------------------------------------------------
-- Identity & Access
-- ------------------------------------------------------------
CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    display_name  TEXT NOT NULL,
    email         TEXT,
    role          TEXT NOT NULL CHECK (role IN ('student', 'teacher', 'admin')),
    password_hash TEXT,  -- only for teacher/admin login
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_unique ON users (lower(email)) WHERE email IS NOT NULL;

CREATE TABLE lti_identities (
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    iss        TEXT NOT NULL,
    sub        TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (iss, sub)
);
CREATE INDEX lti_identities_user_id ON lti_identities (user_id);

CREATE TABLE courses (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    external_id  TEXT NOT NULL UNIQUE,  -- Moodle course_id
    name         TEXT NOT NULL,
    ki_domain_id TEXT NOT NULL UNIQUE,  -- 1 course = 1 КИ domain
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE enrollments (
    user_id        UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    course_id      UUID NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
    role_in_course TEXT NOT NULL CHECK (role_in_course IN ('learner', 'teacher')),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, course_id)
);

-- ------------------------------------------------------------
-- Pool (Projects)
-- ------------------------------------------------------------
CREATE TABLE projects (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    ki_project_id          TEXT NOT NULL UNIQUE,
    ki_domain_id           TEXT NOT NULL,
    name                   TEXT NOT NULL,
    state                  TEXT NOT NULL CHECK (state IN ('free', 'allocated', 'cleaning', 'quarantine', 'decommissioned')),
    allocated_to_lab_id    UUID,
    cleanup_failures       INT NOT NULL DEFAULT 0,
    last_state_change_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT projects_allocation_consistency
        CHECK (
            (state = 'allocated' AND allocated_to_lab_id IS NOT NULL)
            OR (state <> 'allocated' AND allocated_to_lab_id IS NULL)
            OR (state IN ('cleaning'))  -- cleaning may retain allocated_to_lab_id for audit
        )
);
CREATE INDEX projects_domain_state ON projects (ki_domain_id, state);
CREATE INDEX projects_state_change ON projects (last_state_change_at);

-- ------------------------------------------------------------
-- Lab Templates
-- ------------------------------------------------------------
CREATE TABLE lab_templates (
    id                       UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug                     TEXT NOT NULL UNIQUE,
    name                     TEXT NOT NULL,
    description              TEXT NOT NULL DEFAULT '',
    topology                 JSONB NOT NULL,
    default_cleanup_after_s  INT NOT NULL DEFAULT 7200,   -- 2h
    default_freeze_for_s     INT NOT NULL DEFAULT 86400,  -- 24h
    default_check_template_id UUID,
    created_at               TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at               TIMESTAMPTZ
);

-- ------------------------------------------------------------
-- Lab Instances
-- ------------------------------------------------------------
CREATE TABLE lab_instances (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    student_user_id  UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    course_id        UUID NOT NULL REFERENCES courses(id) ON DELETE RESTRICT,
    lab_template_id  UUID NOT NULL REFERENCES lab_templates(id) ON DELETE RESTRICT,
    project_id       UUID REFERENCES projects(id) ON DELETE RESTRICT,

    state            TEXT NOT NULL CHECK (state IN (
        'pending_quota','pending_project','deploying','ready','checking',
        'frozen','failed','cleaning','done','rejected'
    )),
    state_reason     TEXT NOT NULL DEFAULT '',

    ki_resources     JSONB NOT NULL DEFAULT '{}'::jsonb,

    cleanup_at       TIMESTAMPTZ,
    unfreeze_at      TIMESTAMPTZ,
    frozen_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    frozen_reason    TEXT,

    student_ssh_key_secret_id UUID,
    checker_ssh_key_secret_id UUID,

    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT lab_ready_has_cleanup_at
        CHECK (state <> 'ready' OR cleanup_at IS NOT NULL),
    CONSTRAINT lab_frozen_has_unfreeze
        CHECK (state <> 'frozen' OR (unfreeze_at IS NOT NULL AND frozen_by_user_id IS NOT NULL))
);

CREATE INDEX lab_instances_student_course ON lab_instances (student_user_id, course_id);
CREATE INDEX lab_instances_state ON lab_instances (state);
CREATE INDEX lab_instances_cleanup_at ON lab_instances (cleanup_at) WHERE state = 'ready';
CREATE INDEX lab_instances_unfreeze_at ON lab_instances (unfreeze_at) WHERE state = 'frozen';

-- One active lab per (student, course). State machine treats active = NOT terminal.
CREATE UNIQUE INDEX lab_instances_one_active_per_student_course
    ON lab_instances (student_user_id, course_id)
    WHERE state NOT IN ('done', 'rejected', 'failed');

-- ------------------------------------------------------------
-- Deploy Saga steps
-- ------------------------------------------------------------
CREATE TABLE lab_deploy_steps (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lab_instance_id UUID NOT NULL REFERENCES lab_instances(id) ON DELETE CASCADE,
    step_name       TEXT NOT NULL CHECK (step_name IN (
        'allocate_project','create_keypair','provision_network','boot_vm',
        'wait_ssh','initial_check'
    )),
    status          TEXT NOT NULL CHECK (status IN (
        'pending','in_progress','succeeded','failed','compensated'
    )),
    attempt         INT NOT NULL DEFAULT 1,
    last_error      TEXT,
    result          JSONB,
    started_at      TIMESTAMPTZ,
    finished_at     TIMESTAMPTZ,
    UNIQUE (lab_instance_id, step_name)
);

-- ------------------------------------------------------------
-- Verification
-- ------------------------------------------------------------
CREATE TABLE check_templates (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug            TEXT NOT NULL,
    name            TEXT NOT NULL,
    lab_template_id UUID REFERENCES lab_templates(id) ON DELETE CASCADE,
    playbook_path   TEXT NOT NULL,
    timeout_seconds INT NOT NULL DEFAULT 300,
    expected_outcome JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (slug, lab_template_id)
);

CREATE TABLE check_runs (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    lab_instance_id    UUID NOT NULL REFERENCES lab_instances(id) ON DELETE CASCADE,
    check_template_id  UUID NOT NULL REFERENCES check_templates(id) ON DELETE RESTRICT,
    triggered_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    state              TEXT NOT NULL CHECK (state IN (
        'queued','running','passed','failed','timeout','errored'
    )),
    summary            TEXT NOT NULL DEFAULT '',
    ansible_stdout     TEXT NOT NULL DEFAULT '',
    ansible_stats      JSONB NOT NULL DEFAULT '{}'::jsonb,
    started_at         TIMESTAMPTZ,
    finished_at        TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX check_runs_lab ON check_runs (lab_instance_id, created_at DESC);

CREATE TABLE check_run_steps (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    check_run_id UUID NOT NULL REFERENCES check_runs(id) ON DELETE CASCADE,
    seq          INT NOT NULL,
    task_name    TEXT NOT NULL,
    status       TEXT NOT NULL CHECK (status IN ('ok','changed','failed','unreachable','skipped')),
    expected     JSONB,
    actual       JSONB,
    message      TEXT,
    UNIQUE (check_run_id, seq)
);

-- ------------------------------------------------------------
-- Encrypted secrets (envelope encryption)
-- ------------------------------------------------------------
CREATE TABLE encrypted_secrets (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind               TEXT NOT NULL,
    ref_id             TEXT NOT NULL,
    dek_ciphertext     BYTEA NOT NULL,
    dek_nonce          BYTEA NOT NULL,
    payload_ciphertext BYTEA NOT NULL,
    payload_nonce      BYTEA NOT NULL,
    aad                TEXT NOT NULL,
    kek_version        INT NOT NULL,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX encrypted_secrets_kind_ref ON encrypted_secrets (kind, ref_id);

-- ------------------------------------------------------------
-- Settings
-- ------------------------------------------------------------
CREATE TABLE settings (
    key                TEXT NOT NULL,
    scope              TEXT NOT NULL CHECK (scope IN ('global','per_course','per_lab_template')),
    scope_id           TEXT,  -- nullable for global
    value              JSONB NOT NULL,
    updated_by_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (key, scope, COALESCE(scope_id, ''))
);

-- ------------------------------------------------------------
-- Audit log (append-only)
-- ------------------------------------------------------------
CREATE TABLE audit_events (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind          TEXT NOT NULL,
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    subject_type  TEXT NOT NULL,
    subject_id    TEXT,
    payload       JSONB NOT NULL DEFAULT '{}'::jsonb,
    request_id    TEXT,
    occurred_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX audit_events_kind ON audit_events (kind, occurred_at DESC);
CREATE INDEX audit_events_subject ON audit_events (subject_type, subject_id);
CREATE INDEX audit_events_actor ON audit_events (actor_user_id, occurred_at DESC);

-- ------------------------------------------------------------
-- Outbox (for transactional event publishing)
-- ------------------------------------------------------------
CREATE TABLE outbox (
    id          BIGSERIAL PRIMARY KEY,
    topic       TEXT NOT NULL,
    payload     JSONB NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    published_at TIMESTAMPTZ,
    attempts    INT NOT NULL DEFAULT 0
);
CREATE INDEX outbox_unpublished ON outbox (id) WHERE published_at IS NULL;

-- ------------------------------------------------------------
-- Quota snapshot cache (single row, fast read)
-- ------------------------------------------------------------
CREATE TABLE quota_cache (
    id           SMALLINT PRIMARY KEY CHECK (id = 1),
    snapshot     JSONB NOT NULL,
    fetched_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ------------------------------------------------------------
-- Triggers: prevent UPDATE/DELETE on audit_events
-- ------------------------------------------------------------

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION reject_audit_mutations() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_events is append-only';
END
$$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER audit_events_no_update BEFORE UPDATE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION reject_audit_mutations();
CREATE TRIGGER audit_events_no_delete BEFORE DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION reject_audit_mutations();

-- +goose Down
DROP TABLE IF EXISTS outbox CASCADE;
DROP TABLE IF EXISTS audit_events CASCADE;
DROP TABLE IF EXISTS settings CASCADE;
DROP TABLE IF EXISTS quota_cache CASCADE;
DROP TABLE IF EXISTS encrypted_secrets CASCADE;
DROP TABLE IF EXISTS check_run_steps CASCADE;
DROP TABLE IF EXISTS check_runs CASCADE;
DROP TABLE IF EXISTS check_templates CASCADE;
DROP TABLE IF EXISTS lab_deploy_steps CASCADE;
DROP TABLE IF EXISTS lab_instances CASCADE;
DROP TABLE IF EXISTS lab_templates CASCADE;
DROP TABLE IF EXISTS projects CASCADE;
DROP TABLE IF EXISTS enrollments CASCADE;
DROP TABLE IF EXISTS courses CASCADE;
DROP TABLE IF EXISTS lti_identities CASCADE;
DROP TABLE IF EXISTS users CASCADE;
DROP FUNCTION IF EXISTS reject_audit_mutations() CASCADE;
