CREATE OR REPLACE FUNCTION reject_version_mutation() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'framework versions are immutable' USING ERRCODE='23514', CONSTRAINT='framework_version_immutable';
END;
$$;
CREATE OR REPLACE TRIGGER framework_version_immutable
 BEFORE UPDATE OR DELETE OR TRUNCATE ON framework_version
 FOR EACH STATEMENT EXECUTE FUNCTION reject_version_mutation();

CREATE OR REPLACE FUNCTION reject_lifecycle_reset() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'release lifecycle cannot be reset' USING ERRCODE='23514', CONSTRAINT=TG_NAME;
END;
$$;
CREATE OR REPLACE TRIGGER release_delete
 BEFORE DELETE ON release
 FOR EACH ROW EXECUTE FUNCTION reject_lifecycle_reset();
CREATE OR REPLACE TRIGGER release_truncate
 BEFORE TRUNCATE ON release
 FOR EACH STATEMENT EXECUTE FUNCTION reject_lifecycle_reset();
CREATE OR REPLACE TRIGGER mapping_evidence_truncate
 BEFORE TRUNCATE ON mapping_evidence
 FOR EACH STATEMENT EXECUTE FUNCTION reject_lifecycle_reset();

CREATE OR REPLACE FUNCTION check_version_insert() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE sealed boolean;
BEGIN
 SELECT versions_sealed INTO sealed FROM release WHERE singleton FOR SHARE;
 IF NOT FOUND OR sealed IS DISTINCT FROM false THEN
  RAISE EXCEPTION 'version insert requires an unsealed release' USING ERRCODE='23514', CONSTRAINT='framework_version_insert';
 END IF;
 RETURN NEW;
END;
$$;
CREATE OR REPLACE TRIGGER framework_version_insert
 BEFORE INSERT ON framework_version
 FOR EACH ROW EXECUTE FUNCTION check_version_insert();

CREATE OR REPLACE FUNCTION check_evidence_insert() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE sealed boolean;
BEGIN
 SELECT versions_sealed INTO sealed FROM release WHERE singleton FOR SHARE;
 IF NOT FOUND OR sealed IS DISTINCT FROM true THEN
  RAISE EXCEPTION 'evidence insert requires a sealed release' USING ERRCODE='23514', CONSTRAINT='mapping_evidence_insert';
 END IF;
 RETURN NEW;
END;
$$;
CREATE OR REPLACE TRIGGER mapping_evidence_insert
 BEFORE INSERT ON mapping_evidence
 FOR EACH ROW EXECUTE FUNCTION check_evidence_insert();

CREATE OR REPLACE FUNCTION check_versions_seal_transition() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF OLD.versions_sealed AND NOT NEW.versions_sealed THEN
  RAISE EXCEPTION 'versions cannot be unsealed' USING ERRCODE='23514', CONSTRAINT='release_versions_sealed';
 END IF;
 RETURN NEW;
END;
$$;
CREATE OR REPLACE TRIGGER release_versions_sealed
 BEFORE UPDATE OF versions_sealed ON release
 FOR EACH ROW EXECUTE FUNCTION check_versions_seal_transition();
