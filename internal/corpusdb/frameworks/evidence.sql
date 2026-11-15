CREATE OR REPLACE FUNCTION check_evidence_dependencies() RETURNS trigger LANGUAGE plpgsql AS $$
DECLARE dependency bigint;
BEGIN
 IF NEW.licensed_version_ids IS NULL OR coalesce(array_ndims(NEW.licensed_version_ids),1) <> 1 THEN
  RAISE EXCEPTION 'evidence requires a one dimensional dependency set' USING ERRCODE='23514', CONSTRAINT='mapping_evidence_dependencies';
 END IF;
 IF array_position(NEW.licensed_version_ids,NULL) IS NOT NULL OR
    NEW.licensed_version_ids IS DISTINCT FROM ARRAY(SELECT DISTINCT v FROM unnest(NEW.licensed_version_ids) v ORDER BY v) THEN
  RAISE EXCEPTION 'evidence requires a canonical dependency set' USING ERRCODE='23514', CONSTRAINT='mapping_evidence_dependencies';
 END IF;
 FOREACH dependency IN ARRAY NEW.licensed_version_ids LOOP
  PERFORM id FROM framework_version WHERE id=dependency AND access='licensed' FOR SHARE;
  IF NOT FOUND THEN
   RAISE EXCEPTION 'evidence dependency must name a licensed version' USING ERRCODE='23503', CONSTRAINT='mapping_evidence_dependencies';
  END IF;
 END LOOP;
 RETURN NEW;
END;
$$;
CREATE OR REPLACE TRIGGER mapping_evidence_dependencies
 BEFORE INSERT OR UPDATE ON mapping_evidence
 FOR EACH ROW EXECUTE FUNCTION check_evidence_dependencies();
