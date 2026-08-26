CREATE EXTENSION IF NOT EXISTS pgcrypto;
CREATE TABLE IF NOT EXISTS organizations (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), name text NOT NULL, slug text NOT NULL UNIQUE,
  status text NOT NULL DEFAULT 'active' CHECK(status IN ('provisioning','active','suspended','archived')),
  subscription_status text NOT NULL DEFAULT 'trialing' CHECK(subscription_status IN ('trialing','active','past_due','cancelled')),
  trial_ends_at timestamptz NOT NULL DEFAULT now()+interval '14 days', timezone text NOT NULL DEFAULT 'Asia/Tashkent',
  active_student_limit integer NOT NULL DEFAULT 100 CHECK(active_student_limit>=0),
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS service_catalog (
  code text PRIMARY KEY, name text NOT NULL, unit text NOT NULL DEFAULT 'attempt', category text NOT NULL,
  default_monthly_limit integer NOT NULL CHECK(default_monthly_limit>=0), default_daily_limit integer,
  enabled boolean NOT NULL DEFAULT true, description text NOT NULL DEFAULT ''
);
INSERT INTO service_catalog(code,name,unit,category,default_monthly_limit,default_daily_limit,description) VALUES
 ('placement','Level Placement Test','attempt','english',300,NULL,'A1-C1 placement assessment'),
 ('vocabulary_test','Vocabulary Assessment','attempt','english',500,NULL,'Dedicated vocabulary level assessment'),
 ('level_upgrade','Level Upgrade Test','attempt','english',100,NULL,'A1→A2, A2→B1, B1→B2, B2→C1'),
 ('progress','Progress Test','attempt','english',300,NULL,'Periodic progress measurement'),
 ('grammar','Grammar Diagnostic','attempt','english',300,NULL,'Grammar-topic diagnostic'),
 ('ielts_readiness','IELTS Readiness','attempt','english',150,NULL,'Readiness for IELTS-focused study'),
 ('weakness','Weakness Diagnostic','attempt','english',300,NULL,'Personalized weak-topic assessment'),
 ('speaking','Speaking Assessment','submission','english',100,NULL,'Rubric-based speaking review'),
 ('writing','Writing Assessment','submission','english',100,NULL,'Rubric-based writing review'),
 ('mock','IELTS-style Mock','attempt','english',100,NULL,'Composite auto+manual mock assessment'),
 ('final_exit','Final / Exit Assessment','attempt','english',100,NULL,'Course exit assessment'),
 ('listening','Listening Assessment','attempt','listening',300,NULL,'Secure audio-based assessment'),
 ('daily_vocabulary','Daily Vocabulary','word','vocabulary',10000,50,'Daily level-matched vocabulary practice'),
 ('sat_math','SAT Math','attempt','sat',300,NULL,'Original SAT-style English-language math assessment')
ON CONFLICT(code) DO UPDATE SET name=EXCLUDED.name, unit=EXCLUDED.unit, category=EXCLUDED.category, description=EXCLUDED.description;
CREATE TABLE IF NOT EXISTS organization_service_limits (
  organization_id uuid NOT NULL, service_code text NOT NULL REFERENCES service_catalog(code), enabled boolean NOT NULL DEFAULT true,
  monthly_limit integer NOT NULL CHECK(monthly_limit>=0), daily_limit integer, concurrency_limit integer NOT NULL DEFAULT 10 CHECK(concurrency_limit>0),
  updated_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(organization_id,service_code)
);
CREATE TABLE IF NOT EXISTS usage_monthly (
  organization_id uuid NOT NULL, service_code text NOT NULL, period date NOT NULL,
  used bigint NOT NULL DEFAULT 0 CHECK(used>=0), updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(organization_id,service_code,period)
);
CREATE TABLE IF NOT EXISTS usage_daily (
  organization_id uuid NOT NULL, service_code text NOT NULL, day date NOT NULL,
  used bigint NOT NULL DEFAULT 0 CHECK(used>=0), updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY(organization_id,service_code,day)
);
CREATE TABLE IF NOT EXISTS groups (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(), organization_id uuid NOT NULL, name text NOT NULL,
  level text, teacher_name text, status text NOT NULL DEFAULT 'active' CHECK(status IN ('active','archived')),
  created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(organization_id,name)
);
CREATE TABLE IF NOT EXISTS group_members (
  group_id uuid NOT NULL REFERENCES groups(id) ON DELETE CASCADE, organization_id uuid NOT NULL, student_user_id uuid NOT NULL,
  joined_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(group_id,student_user_id)
);
CREATE INDEX IF NOT EXISTS group_members_student_idx ON group_members(organization_id,student_user_id);
CREATE OR REPLACE FUNCTION reserve_service_usage(p_org uuid,p_service text,p_amount integer DEFAULT 1)
RETURNS TABLE(allowed boolean, used bigint, monthly_limit integer, remaining bigint, reason text)
LANGUAGE plpgsql AS $$
DECLARE lim organization_service_limits%ROWTYPE; cat service_catalog%ROWTYPE; pmonth date:=date_trunc('month',now())::date; pday date:=current_date; mu bigint; du bigint;
BEGIN
  IF p_amount<=0 THEN RETURN QUERY SELECT false,0::bigint,0,0::bigint,'invalid_amount'; RETURN; END IF;
  SELECT * INTO cat FROM service_catalog WHERE code=p_service AND enabled=true; IF NOT FOUND THEN RETURN QUERY SELECT false,0::bigint,0,0::bigint,'service_unavailable'; RETURN; END IF;
  SELECT * INTO lim FROM organization_service_limits WHERE organization_id=p_org AND service_code=p_service FOR UPDATE;
  IF NOT FOUND THEN INSERT INTO organization_service_limits(organization_id,service_code,monthly_limit,daily_limit) VALUES(p_org,p_service,cat.default_monthly_limit,cat.default_daily_limit) RETURNING * INTO lim; END IF;
  IF NOT lim.enabled THEN RETURN QUERY SELECT false,0::bigint,lim.monthly_limit,0::bigint,'service_disabled'; RETURN; END IF;
  INSERT INTO usage_monthly(organization_id,service_code,period,used) VALUES(p_org,p_service,pmonth,0) ON CONFLICT DO NOTHING;
  SELECT used INTO mu FROM usage_monthly WHERE organization_id=p_org AND service_code=p_service AND period=pmonth FOR UPDATE;
  IF mu+p_amount>lim.monthly_limit THEN RETURN QUERY SELECT false,mu,lim.monthly_limit,GREATEST(lim.monthly_limit-mu,0)::bigint,'monthly_limit'; RETURN; END IF;
  IF lim.daily_limit IS NOT NULL THEN
    INSERT INTO usage_daily(organization_id,service_code,day,used) VALUES(p_org,p_service,pday,0) ON CONFLICT DO NOTHING;
    SELECT used INTO du FROM usage_daily WHERE organization_id=p_org AND service_code=p_service AND day=pday FOR UPDATE;
    IF du+p_amount>lim.daily_limit THEN RETURN QUERY SELECT false,mu,lim.monthly_limit,GREATEST(lim.monthly_limit-mu,0)::bigint,'daily_limit'; RETURN; END IF;
    UPDATE usage_daily SET used=used+p_amount,updated_at=now() WHERE organization_id=p_org AND service_code=p_service AND day=pday;
  END IF;
  UPDATE usage_monthly SET used=used+p_amount,updated_at=now() WHERE organization_id=p_org AND service_code=p_service AND period=pmonth RETURNING usage_monthly.used INTO mu;
  RETURN QUERY SELECT true,mu,lim.monthly_limit,GREATEST(lim.monthly_limit-mu,0)::bigint,''::text;
END $$;
