DO $$
BEGIN
    IF EXISTS (SELECT FROM sim_keepgo_sandbox_issuance) THEN
        RAISE EXCEPTION 'Keepgo sandbox issuance history exists; retain the schema';
    END IF;
END $$;

DROP TABLE sim_keepgo_sandbox_issuance;
