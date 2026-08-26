-- Refund a quota reservation only when provisioning/startup failed before the
-- reservation was released as a successfully-created attempt/session.
CREATE OR REPLACE FUNCTION cancel_service_usage(
  p_org uuid,
  p_service text,
  p_reservation_key text
)
RETURNS boolean
LANGUAGE plpgsql AS $$
DECLARE
  r usage_reservations%ROWTYPE;
  org organizations%ROWTYPE;
  pmonth date;
  pday date;
BEGIN
  IF p_reservation_key IS NULL OR btrim(p_reservation_key)='' THEN
    RETURN false;
  END IF;

  SELECT * INTO org FROM organizations WHERE id=p_org;
  IF NOT FOUND THEN
    RETURN false;
  END IF;

  -- DELETE is intentional: a retry with the same idempotency key must be able
  -- to reserve again after a failed startup.
  DELETE FROM usage_reservations
   WHERE organization_id=p_org
     AND service_code=p_service
     AND reservation_key=p_reservation_key
     AND released_at IS NULL
  RETURNING * INTO r;

  IF NOT FOUND THEN
    RETURN false;
  END IF;

  pday := (r.created_at AT TIME ZONE org.timezone)::date;
  pmonth := date_trunc('month', r.created_at AT TIME ZONE org.timezone)::date;

  UPDATE usage_monthly
     SET used=GREATEST(used-r.amount,0), updated_at=now()
   WHERE organization_id=p_org AND service_code=p_service AND period=pmonth;

  UPDATE usage_daily
     SET used=GREATEST(used-r.amount,0), updated_at=now()
   WHERE organization_id=p_org AND service_code=p_service AND day=pday;

  RETURN true;
END $$;
