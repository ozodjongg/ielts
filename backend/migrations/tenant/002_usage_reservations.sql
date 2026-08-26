-- Idempotent quota reservations + concurrency leases.
CREATE TABLE IF NOT EXISTS usage_reservations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  organization_id uuid NOT NULL,
  service_code text NOT NULL REFERENCES service_catalog(code),
  reservation_key text NOT NULL,
  amount integer NOT NULL CHECK(amount>0),
  holds_concurrency boolean NOT NULL DEFAULT false,
  expires_at timestamptz NOT NULL DEFAULT now()+interval '3 hours',
  released_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(organization_id,service_code,reservation_key)
);
CREATE INDEX IF NOT EXISTS usage_reservations_active_idx
  ON usage_reservations(organization_id,service_code,expires_at)
  WHERE holds_concurrency AND released_at IS NULL;

CREATE OR REPLACE FUNCTION reserve_service_usage(
  p_org uuid,
  p_service text,
  p_amount integer DEFAULT 1,
  p_reservation_key text DEFAULT NULL,
  p_hold_concurrency boolean DEFAULT false,
  p_lease_minutes integer DEFAULT 180
)
RETURNS TABLE(allowed boolean, used bigint, monthly_limit integer, remaining bigint, reason text, reservation_id uuid)
LANGUAGE plpgsql AS $$
DECLARE
  lim organization_service_limits%ROWTYPE;
  cat service_catalog%ROWTYPE;
  org organizations%ROWTYPE;
  existing usage_reservations%ROWTYPE;
  pmonth date;
  pday date;
  mu bigint:=0;
  du bigint:=0;
  active_count integer:=0;
  rid uuid;
BEGIN
  IF p_amount<=0 THEN
    RETURN QUERY SELECT false,0::bigint,0,0::bigint,'invalid_amount',NULL::uuid; RETURN;
  END IF;
  IF p_lease_minutes<1 OR p_lease_minutes>1440 THEN
    RETURN QUERY SELECT false,0::bigint,0,0::bigint,'invalid_lease',NULL::uuid; RETURN;
  END IF;

  SELECT * INTO org FROM organizations WHERE id=p_org;
  IF NOT FOUND OR org.status<>'active' THEN
    RETURN QUERY SELECT false,0::bigint,0,0::bigint,'organization_inactive',NULL::uuid; RETURN;
  END IF;
  IF org.subscription_status NOT IN ('active','trialing') OR (org.subscription_status='trialing' AND org.trial_ends_at<now()) THEN
    RETURN QUERY SELECT false,0::bigint,0,0::bigint,'subscription_inactive',NULL::uuid; RETURN;
  END IF;
  BEGIN
    pday := (now() AT TIME ZONE org.timezone)::date;
    pmonth := date_trunc('month', now() AT TIME ZONE org.timezone)::date;
  EXCEPTION WHEN invalid_parameter_value THEN
    RETURN QUERY SELECT false,0::bigint,0,0::bigint,'organization_timezone_invalid',NULL::uuid; RETURN;
  END;

  SELECT * INTO cat FROM service_catalog WHERE code=p_service AND enabled=true;
  IF NOT FOUND THEN
    RETURN QUERY SELECT false,0::bigint,0,0::bigint,'service_unavailable',NULL::uuid; RETURN;
  END IF;

  INSERT INTO organization_service_limits(organization_id,service_code,monthly_limit,daily_limit)
    VALUES(p_org,p_service,cat.default_monthly_limit,cat.default_daily_limit)
    ON CONFLICT DO NOTHING;
  SELECT * INTO lim FROM organization_service_limits
    WHERE organization_id=p_org AND service_code=p_service FOR UPDATE;
  IF NOT lim.enabled THEN
    RETURN QUERY SELECT false,0::bigint,lim.monthly_limit,0::bigint,'service_disabled',NULL::uuid; RETURN;
  END IF;

  -- A retry with the same key must never consume quota twice.
  IF p_reservation_key IS NOT NULL AND btrim(p_reservation_key)<>'' THEN
    SELECT * INTO existing FROM usage_reservations
      WHERE organization_id=p_org AND service_code=p_service AND reservation_key=p_reservation_key;
    IF FOUND THEN
      SELECT coalesce(u.used,0) INTO mu FROM usage_monthly u
        WHERE u.organization_id=p_org AND u.service_code=p_service AND u.period=pmonth;
      RETURN QUERY SELECT true,coalesce(mu,0),lim.monthly_limit,
        GREATEST(lim.monthly_limit-coalesce(mu,0),0)::bigint,'idempotent',existing.id;
      RETURN;
    END IF;
  END IF;

  IF p_hold_concurrency THEN
    SELECT count(*) INTO active_count FROM usage_reservations ur
      WHERE ur.organization_id=p_org AND ur.service_code=p_service
        AND ur.holds_concurrency AND ur.released_at IS NULL AND ur.expires_at>now();
    IF active_count>=lim.concurrency_limit THEN
      SELECT coalesce(u.used,0) INTO mu FROM usage_monthly u
        WHERE u.organization_id=p_org AND u.service_code=p_service AND u.period=pmonth;
      RETURN QUERY SELECT false,coalesce(mu,0),lim.monthly_limit,
        GREATEST(lim.monthly_limit-coalesce(mu,0),0)::bigint,'concurrency_limit',NULL::uuid;
      RETURN;
    END IF;
  END IF;

  INSERT INTO usage_monthly(organization_id,service_code,period,used)
    VALUES(p_org,p_service,pmonth,0) ON CONFLICT DO NOTHING;
  SELECT u.used INTO mu FROM usage_monthly u
    WHERE u.organization_id=p_org AND u.service_code=p_service AND u.period=pmonth FOR UPDATE;
  IF mu+p_amount>lim.monthly_limit THEN
    RETURN QUERY SELECT false,mu,lim.monthly_limit,GREATEST(lim.monthly_limit-mu,0)::bigint,'monthly_limit',NULL::uuid; RETURN;
  END IF;

  IF lim.daily_limit IS NOT NULL THEN
    INSERT INTO usage_daily(organization_id,service_code,day,used)
      VALUES(p_org,p_service,pday,0) ON CONFLICT DO NOTHING;
    SELECT u.used INTO du FROM usage_daily u
      WHERE u.organization_id=p_org AND u.service_code=p_service AND u.day=pday FOR UPDATE;
    IF du+p_amount>lim.daily_limit THEN
      RETURN QUERY SELECT false,mu,lim.monthly_limit,GREATEST(lim.monthly_limit-mu,0)::bigint,'daily_limit',NULL::uuid; RETURN;
    END IF;
  END IF;

  IF p_reservation_key IS NOT NULL AND btrim(p_reservation_key)<>'' THEN
    INSERT INTO usage_reservations(organization_id,service_code,reservation_key,amount,holds_concurrency,expires_at)
      VALUES(p_org,p_service,p_reservation_key,p_amount,p_hold_concurrency,now()+make_interval(mins=>p_lease_minutes))
      RETURNING id INTO rid;
  END IF;

  UPDATE usage_monthly SET used=usage_monthly.used+p_amount,updated_at=now()
    WHERE organization_id=p_org AND service_code=p_service AND period=pmonth
    RETURNING usage_monthly.used INTO mu;
  IF lim.daily_limit IS NOT NULL THEN
    UPDATE usage_daily SET used=usage_daily.used+p_amount,updated_at=now()
      WHERE organization_id=p_org AND service_code=p_service AND day=pday;
  END IF;

  RETURN QUERY SELECT true,mu,lim.monthly_limit,GREATEST(lim.monthly_limit-mu,0)::bigint,''::text,rid;
END $$;
