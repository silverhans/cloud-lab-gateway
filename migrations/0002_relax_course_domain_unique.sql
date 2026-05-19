-- +goose Up

-- Реальность учебного кластера КИ: команде хакатона выдаётся один домен на всех
-- (например, "Hackhaton"). Несколько логических курсов в нашей БД могут
-- мапиться на тот же ki_domain_id. Пул проектов остаётся партиционированным
-- по ki_domain_id, но courses.ki_domain_id больше не уникален.
--
-- Логическая изоляция между курсами сохраняется на application-уровне
-- (RBAC + audit log + per-course view scoping).

ALTER TABLE courses DROP CONSTRAINT courses_ki_domain_id_key;
CREATE INDEX courses_ki_domain_id_idx ON courses (ki_domain_id);

-- +goose Down
DROP INDEX IF EXISTS courses_ki_domain_id_idx;
ALTER TABLE courses ADD CONSTRAINT courses_ki_domain_id_key UNIQUE (ki_domain_id);
