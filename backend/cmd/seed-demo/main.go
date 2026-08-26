package main

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/example/assessment-platform-v5/internal/dbx"
	"github.com/example/assessment-platform-v5/internal/passwordhash"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

var demoNS = uuid.MustParse("f891055f-b6ca-4dd6-9cab-b733463b0e73")

func stable(name string) uuid.UUID { return uuid.NewSHA1(demoNS, []byte(name)) }

func env(name, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(name)); v != "" {
		return v
	}
	return fallback
}

func mustExec(ctx context.Context, db *pgxpool.Pool, sql string, args ...any) {
	if _, err := db.Exec(ctx, sql, args...); err != nil {
		log.Fatal(err)
	}
}

func makeTwoToneWAV(path string) (int64, string, int, error) {
	const sampleRate = 8000
	const seconds = 3
	const channels = 1
	const bits = 16
	samples := make([]int16, sampleRate*seconds)
	for i := range samples {
		t := float64(i) / sampleRate
		active := (t >= 0.35 && t < 0.85) || (t >= 1.55 && t < 2.05)
		if active {
			samples[i] = int16(9000 * math.Sin(2*math.Pi*440*t))
		}
	}
	dataBytes := len(samples) * 2
	buf := make([]byte, 44+dataBytes)
	copy(buf[0:4], "RIFF")
	binary.LittleEndian.PutUint32(buf[4:8], uint32(36+dataBytes))
	copy(buf[8:12], "WAVE")
	copy(buf[12:16], "fmt ")
	binary.LittleEndian.PutUint32(buf[16:20], 16)
	binary.LittleEndian.PutUint16(buf[20:22], 1)
	binary.LittleEndian.PutUint16(buf[22:24], channels)
	binary.LittleEndian.PutUint32(buf[24:28], sampleRate)
	binary.LittleEndian.PutUint32(buf[28:32], sampleRate*channels*(bits/8))
	binary.LittleEndian.PutUint16(buf[32:34], channels*(bits/8))
	binary.LittleEndian.PutUint16(buf[34:36], bits)
	copy(buf[36:40], "data")
	binary.LittleEndian.PutUint32(buf[40:44], uint32(dataBytes))
	for i, sample := range samples {
		binary.LittleEndian.PutUint16(buf[44+i*2:46+i*2], uint16(sample))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return 0, "", 0, err
	}
	if err := os.WriteFile(path, buf, 0o640); err != nil {
		return 0, "", 0, err
	}
	sum := sha256.Sum256(buf)
	return int64(len(buf)), hex.EncodeToString(sum[:]), seconds * 1000, nil
}

func main() {
	password := flag.String("password", "DemoPassword123!", "password for all demo accounts (10-128 characters)")
	storageDir := flag.String("listening-storage", env("LISTENING_STORAGE_DIR", filepath.Join("..", ".runtime", "data", "listening")), "listening storage directory")
	flag.Parse()
	if len(*password) < 10 || len(*password) > 128 {
		log.Fatal("--password must be 10-128 characters")
	}
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		log.Fatal("DATABASE_URL must be set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	db, err := dbx.Open(ctx, dsn)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	hash, err := passwordhash.Hash(*password)
	if err != nil {
		log.Fatal(err)
	}

	// Learning center.
	var orgID uuid.UUID
	err = db.QueryRow(ctx, `INSERT INTO tenant.organizations(name,slug,status,subscription_status,trial_ends_at,timezone,active_student_limit)
		VALUES('V5 Demo Learning Center','v5-demo-center','active','active',now()+interval '365 days','Asia/Tashkent',200)
		ON CONFLICT(slug) DO UPDATE SET name=excluded.name,status='active',subscription_status='active',trial_ends_at=excluded.trial_ends_at,active_student_limit=excluded.active_student_limit,updated_at=now()
		RETURNING id`).Scan(&orgID)
	if err != nil {
		log.Fatal(err)
	}
	mustExec(ctx, db, `INSERT INTO tenant.organization_service_limits(organization_id,service_code,enabled,monthly_limit,daily_limit,concurrency_limit)
		SELECT $1,code,true,GREATEST(default_monthly_limit,1000),CASE WHEN code='daily_vocabulary' THEN 100 ELSE default_daily_limit END,50
		FROM tenant.service_catalog WHERE enabled=true
		ON CONFLICT(organization_id,service_code) DO UPDATE SET enabled=true,monthly_limit=excluded.monthly_limit,daily_limit=excluded.daily_limit,concurrency_limit=excluded.concurrency_limit,updated_at=now()`, orgID)

	type user struct {
		ID    uuid.UUID
		Role  string
		Email string
		Name  string
		Level *string
	}
	level := func(v string) *string { return &v }
	users := []user{
		{stable("platform-admin"), "platform_admin", "demo.admin@v5.local", "Demo Platform Admin", nil},
		{stable("center-admin"), "center_admin", "demo.center@v5.local", "Demo Center Admin", nil},
		{stable("student-a1"), "student", "student.a1@v5.local", "Aziza A1", level("A1")},
		{stable("student-a2"), "student", "student.a2@v5.local", "Bekzod A2", level("A2")},
		{stable("student-b1"), "student", "student.b1@v5.local", "Dilnoza B1", level("B1")},
		{stable("student-b2"), "student", "student.b2@v5.local", "Jasur B2", level("B2")},
		{stable("student-c1"), "student", "student.c1@v5.local", "Madina C1", level("C1")},
		{stable("student-c2"), "student", "student.c2@v5.local", "Sardor C2", level("C2")},
	}
	for _, u := range users {
		var org any = orgID
		if u.Role == "platform_admin" {
			org = nil
		}
		mustExec(ctx, db, `INSERT INTO identity.profiles(user_id,organization_id,role,email,full_name,status,current_level,locale)
			VALUES($1,$2,$3,$4,$5,'active',$6,'uz')
			ON CONFLICT(user_id) DO UPDATE SET organization_id=excluded.organization_id,role=excluded.role,email=excluded.email,full_name=excluded.full_name,status='active',current_level=excluded.current_level,updated_at=now()`, u.ID, org, u.Role, u.Email, u.Name, u.Level)
		mustExec(ctx, db, `INSERT INTO identity.auth_credentials(user_id,password_hash,failed_attempts,locked_until,password_changed_at,updated_at)
			VALUES($1,$2,0,NULL,now(),now()) ON CONFLICT(user_id) DO UPDATE SET password_hash=excluded.password_hash,failed_attempts=0,locked_until=NULL,password_changed_at=now(),updated_at=now()`, u.ID, hash)
		mustExec(ctx, db, `UPDATE identity.auth_sessions SET revoked_at=coalesce(revoked_at,now()) WHERE user_id=$1 AND revoked_at IS NULL`, u.ID)
	}
	centerAdmin := users[1].ID
	students := users[2:]

	// Groups and memberships.
	groups := []struct {
		ID      uuid.UUID
		Name    string
		Level   string
		Teacher string
		Members []int
	}{
		{stable("group-foundation"), "Foundation", "A1", "Teacher Kamola", []int{0, 1}},
		{stable("group-intermediate"), "Intermediate", "B1", "Teacher Anvar", []int{2, 3}},
		{stable("group-advanced"), "Advanced", "C1", "Teacher Nilufar", []int{4, 5}},
	}
	for _, g := range groups {
		mustExec(ctx, db, `INSERT INTO tenant.groups(id,organization_id,name,level,teacher_name,status) VALUES($1,$2,$3,$4,$5,'active')
			ON CONFLICT(id) DO UPDATE SET organization_id=excluded.organization_id,name=excluded.name,level=excluded.level,teacher_name=excluded.teacher_name,status='active'`, g.ID, orgID, g.Name, g.Level, g.Teacher)
		for _, idx := range g.Members {
			mustExec(ctx, db, `INSERT INTO tenant.group_members(group_id,organization_id,student_user_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, g.ID, orgID, students[idx].ID)
		}
	}

	// English assignments covering automatic, manual and hybrid modes.
	type engAssignment struct {
		Name, Code, Title, TargetType string
		Target                        any
		From, To                      any
		Count                         int
		Status                        string
	}
	eng := []engAssignment{
		{"eng-placement", "placement", "Demo Placement Test", "all", nil, nil, nil, 80, "open"},
		{"eng-progress", "progress", "Weekly Progress Test", "all", nil, nil, nil, 30, "open"},
		{"eng-grammar", "grammar", "Intermediate Grammar Diagnostic", "group", groups[1].ID, nil, nil, 40, "open"},
		{"eng-upgrade", "level_upgrade", "A1 to A2 Upgrade", "student", students[0].ID, "A1", "A2", 40, "open"},
		{"eng-speaking", "speaking", "Speaking Review", "all", nil, nil, nil, 3, "open"},
		{"eng-writing", "writing", "Writing Review", "all", nil, nil, nil, 2, "open"},
		{"eng-mock", "mock", "IELTS-style Hybrid Mock", "all", nil, nil, nil, 60, "open"},
		{"eng-history", "progress", "Completed Progress Sample", "student", students[0].ID, nil, nil, 30, "closed"},
	}
	for _, a := range eng {
		mustExec(ctx, db, `INSERT INTO assessment.assignments(id,organization_id,service_code,title,target_type,target_id,from_level,to_level,question_count,opens_at,due_at,status,created_by)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,now()-interval '1 day',now()+interval '30 days',$10,$11)
			ON CONFLICT(id) DO UPDATE SET organization_id=excluded.organization_id,service_code=excluded.service_code,title=excluded.title,target_type=excluded.target_type,target_id=excluded.target_id,from_level=excluded.from_level,to_level=excluded.to_level,question_count=excluded.question_count,status=excluded.status,due_at=excluded.due_at`, stable(a.Name), orgID, a.Code, a.Title, a.TargetType, a.Target, a.From, a.To, a.Count, a.Status, centerAdmin)
	}
	// Completed assessment history and mastery for the A1 demo student.
	histAttempt := stable("assessment-history-attempt")
	mustExec(ctx, db, `INSERT INTO assessment.attempts(id,organization_id,assignment_id,student_user_id,service_code,bank_version,status,question_plan,auto_score,final_score,level_result,started_at,finished_at)
		VALUES($1,$2,$3,$4,'progress','demo-seed','completed','[]'::jsonb,76,76,'A1',now()-interval '8 days',now()-interval '8 days'+interval '25 minutes')
		ON CONFLICT(id) DO UPDATE SET status='completed',auto_score=76,final_score=76,level_result='A1',finished_at=excluded.finished_at`, histAttempt, orgID, stable("eng-history"), students[0].ID)
	mastery := []struct {
		Code              string
		Attempts, Correct int
		Score             float64
	}{{"Present Simple", 12, 9, .75}, {"Basic Vocabulary", 10, 8, .80}, {"Articles", 8, 5, .625}, {"Prepositions", 9, 5, .5556}}
	for _, m := range mastery {
		mustExec(ctx, db, `INSERT INTO assessment.topic_mastery(organization_id,student_user_id,subject_code,attempts,correct,mastery) VALUES($1,$2,$3,$4,$5,$6)
			ON CONFLICT(organization_id,student_user_id,subject_code) DO UPDATE SET attempts=excluded.attempts,correct=excluded.correct,mastery=excluded.mastery,updated_at=now()`, orgID, students[0].ID, m.Code, m.Attempts, m.Correct, m.Score)
	}

	// SAT assignments. The all-student assignment remains startable.
	mustExec(ctx, db, `INSERT INTO sat.sat_assignments(id,organization_id,title,target_type,target_id,question_count,due_at,created_by)
		VALUES($1,$2,'SAT Math Diagnostic','all',NULL,44,now()+interval '30 days',$3)
		ON CONFLICT(id) DO UPDATE SET title=excluded.title,target_type='all',target_id=NULL,question_count=44,due_at=excluded.due_at`, stable("sat-current"), orgID, centerAdmin)

	// Local listening system-check audio: two audible 440Hz tones in a generated WAV.
	audioID := stable("listening-audio")
	setID := stable("listening-set")
	listenAssignmentID := stable("listening-assignment")
	storageKey := "demo-two-tone.wav"
	audioPath := filepath.Join(*storageDir, storageKey)
	size, sum, duration, err := makeTwoToneWAV(audioPath)
	if err != nil {
		log.Fatal(err)
	}
	mustExec(ctx, db, `INSERT INTO listening.audio_assets(id,organization_id,title,storage_key,sha256,mime_type,size_bytes,duration_ms,level,max_plays,allow_seek,status,created_by)
		VALUES($1,$2,'Two-tone listening system check',$3,$4,'audio/wav',$5,$6,'A1',3,true,'active',$7)
		ON CONFLICT(id) DO UPDATE SET organization_id=excluded.organization_id,title=excluded.title,storage_key=excluded.storage_key,sha256=excluded.sha256,size_bytes=excluded.size_bytes,duration_ms=excluded.duration_ms,status='active'`, audioID, orgID, storageKey, sum, size, duration, centerAdmin)
	questions := []map[string]any{
		{"id": "tone-count", "prompt": "How many separate tones do you hear?", "options": []string{"One", "Two", "Three", "Four"}, "base_points": 1},
		{"id": "tone-pitch", "prompt": "Are the two tones the same pitch?", "options": []string{"Yes", "No"}, "base_points": 1},
	}
	answers := map[string]string{"tone-count": "Two", "tone-pitch": "Yes"}
	qJSON, _ := json.Marshal(questions)
	aJSON, _ := json.Marshal(answers)
	mustExec(ctx, db, `INSERT INTO listening.listening_sets(id,organization_id,audio_id,title,level,questions,answer_key,created_by)
		VALUES($1,$2,$3,'Two-tone QA listening set','A1',$4::jsonb,$5::jsonb,$6)
		ON CONFLICT(id) DO UPDATE SET audio_id=excluded.audio_id,title=excluded.title,level=excluded.level,questions=excluded.questions,answer_key=excluded.answer_key`, setID, orgID, audioID, string(qJSON), string(aJSON), centerAdmin)
	mustExec(ctx, db, `INSERT INTO listening.listening_assignments(id,organization_id,set_id,target_type,target_id,due_at,created_by)
		VALUES($1,$2,$3,'all',NULL,now()+interval '30 days',$4)
		ON CONFLICT(id) DO UPDATE SET set_id=excluded.set_id,target_type='all',target_id=NULL,due_at=excluded.due_at`, listenAssignmentID, orgID, setID, centerAdmin)

	// Review queue samples for center and student submission pages.
	mustExec(ctx, db, `INSERT INTO review.submissions(id,organization_id,student_user_id,service_code,prompt_id,text_submission,status,submitted_at)
		VALUES($1,$2,$3,'writing','demo-writing','Technology can support learning when it is used deliberately. This demo submission exists to exercise the review workflow, filtering, rubric form and status transitions in the center portal.','pending',now()-interval '2 hours')
		ON CONFLICT(id) DO UPDATE SET text_submission=excluded.text_submission,status='pending',reviewer_user_id=NULL,review_notes=NULL,score=NULL,reviewed_at=NULL`, stable("review-writing"), orgID, students[0].ID)
	mustExec(ctx, db, `INSERT INTO review.submissions(id,organization_id,student_user_id,service_code,prompt_id,text_submission,status,rubric,reviewer_user_id,review_notes,score,submitted_at,reviewed_at)
		VALUES($1,$2,$3,'writing','demo-reviewed','This second sample demonstrates a completed writing review.','reviewed','{"task":8,"language":7,"organization":8}'::jsonb,$4,'Clear structure and appropriate vocabulary.',78,now()-interval '5 days',now()-interval '4 days')
		ON CONFLICT(id) DO UPDATE SET status='reviewed',reviewer_user_id=excluded.reviewer_user_id,review_notes=excluded.review_notes,score=excluded.score,reviewed_at=excluded.reviewed_at`, stable("review-reviewed"), orgID, students[0].ID, centerAdmin)

	// Points and leaderboard data across all levels.
	for i, st := range students {
		points := float64(1200 - i*110)
		mustExec(ctx, db, `INSERT INTO points.point_ledger(organization_id,student_user_id,service_code,event_key,base_points,multiplier,awarded_points,reason,created_at)
			VALUES($1,$2,'progress',$3,$4,1,$4,'demo_seed',now()-($5::text||' days')::interval)
			ON CONFLICT(event_key) DO UPDATE SET awarded_points=excluded.awarded_points,base_points=excluded.base_points,created_at=excluded.created_at`, orgID, st.ID, "demo-points-"+st.ID.String(), points, i)
	}

	// Analytics events make admin/center/student dashboards non-empty.
	for i, st := range students {
		for n, eventType := range []string{"assessment.started", "assessment.completed", "vocabulary.graded"} {
			eventID := stable(fmt.Sprintf("analytics-%d-%d", i, n))
			payload := fmt.Sprintf(`{"source":"demo_seed","level":%q}`, *st.Level)
			mustExec(ctx, db, `INSERT INTO analytics.events(event_id,organization_id,student_user_id,event_type,service_code,occurred_at,payload)
				VALUES($1,$2,$3,$4,$5,now()-($6::text||' hours')::interval,$7::jsonb)
				ON CONFLICT(event_id) DO UPDATE SET event_type=excluded.event_type,service_code=excluded.service_code,occurred_at=excluded.occurred_at,payload=excluded.payload`, eventID, orgID, st.ID, eventType, []string{"progress", "progress", "daily_vocabulary"}[n], i*3+n+1, payload)
		}
	}

	fmt.Println("Demo data ready.")
	fmt.Printf("Center: %s (%s)\n", "V5 Demo Learning Center", orgID)
	fmt.Printf("Platform admin: %s\n", users[0].Email)
	fmt.Printf("Center admin:   %s\n", users[1].Email)
	fmt.Printf("Students:       student.a1@v5.local ... student.c2@v5.local\n")
	fmt.Printf("Password:       %s\n", *password)
	fmt.Printf("Listening WAV:  %s\n", audioPath)
}
