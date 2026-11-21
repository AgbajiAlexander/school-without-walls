param(
    [string]$PostgresBin = 'C:/Program Files/PostgreSQL/18/bin',
    [string]$OpenSSL = 'C:/Program Files/Git/usr/bin/openssl.exe'
)

# Run in a separate PowerShell process. Fixtures use ephemeral process variables.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$b=$PostgresBin
$p=Join-Path ([IO.Path]::GetTempPath()) ('sww-academic-e2e-'+[guid]::NewGuid())
if(Get-NetTCPConnection -LocalPort 15435,13003,13443,18083 -State Listen -ErrorAction SilentlyContinue){throw 'One of the isolated browser ports is occupied'}
& "$b/initdb.exe" -D $p -U sww_browser -A trust --no-locale --encoding=UTF8 | Out-Null
if($LASTEXITCODE){throw 'initdb failed'}
& "$b/pg_ctl.exe" -D $p -l "$p/server.log" -o '-p 15435 -h 127.0.0.1' -w start
if($LASTEXITCODE){throw 'start failed'}
try {
 & "$b/createdb.exe" -h 127.0.0.1 -p 15435 -U sww_browser sww_e2e_test
 if($LASTEXITCODE){throw 'create failed'}
 $env:DATABASE_URL='postgres://sww_browser@127.0.0.1:15435/sww_e2e_test?sslmode=disable'
 $env:SWW_TEST_DATABASE_URL=$env:DATABASE_URL
 $env:E2E_ADMIN_PASSWORD='A!a1'+[guid]::NewGuid().ToString('N')
 $env:BOOTSTRAP_ADMIN_PASSWORD=$env:E2E_ADMIN_PASSWORD
 $env:BOOTSTRAP_ADMIN_EMAIL='academic-owner@example.test'
 $env:BOOTSTRAP_ADMIN_NAME='Fictional Owner'
 $env:E2E_TLS_CERT=Join-Path $p 'browser.crt'
 $env:E2E_TLS_KEY=Join-Path $p 'browser.key'
 $ErrorActionPreference='Continue'
 & $OpenSSL req -x509 -newkey rsa:2048 -sha256 -nodes -days 1 -keyout $env:E2E_TLS_KEY -out $env:E2E_TLS_CERT -subj '/CN=127.0.0.1' -addext 'subjectAltName=IP:127.0.0.1' 2>$null
 $ErrorActionPreference='Stop'
 if($LASTEXITCODE){throw 'TLS fixture failed'}
 Set-Location "$root/services/api"
 go run ./cmd/migrate up
 if($LASTEXITCODE){throw 'disposable migration failed'}
 $env:SERVER_ENV='development'
 $env:SWW_ENABLE_DEV_BOOTSTRAP='true'
 go run ./cmd/bootstrap
 if($LASTEXITCODE){throw 'fictional identity setup failed'}
 Remove-Item Env:SWW_ENABLE_DEV_BOOTSTRAP,Env:BOOTSTRAP_ADMIN_PASSWORD -ErrorAction SilentlyContinue
 $env:SERVER_ENV='staging'
 $env:E2E_API_PORT='18083'
 $env:E2E_WEB_PORT='13003'
 $env:E2E_HTTPS_PORT='13443'
 Remove-Item Env:SMTP_HOST,Env:SMTP_PORT,Env:SMTP_USERNAME,Env:SMTP_PASSWORD,Env:SMTP_FROM -ErrorAction SilentlyContinue
 @'
INSERT INTO users (email, password_hash, full_name, status, verified_at)
SELECT fixture.email, password_hash, fixture.name, 'ACTIVE', CURRENT_TIMESTAMP
FROM users CROSS JOIN (VALUES
    ('academic-teacher@example.test', 'Fictional Teacher'),
    ('academic-curriculum@example.test', 'Fictional Curriculum Member'),
    ('academic-reviewer@example.test', 'Fictional Curriculum Reviewer')
) AS fixture(email, name)
WHERE users.email = 'academic-owner@example.test';
INSERT INTO platform_admins (user_id)
SELECT id FROM users WHERE email = 'academic-reviewer@example.test';
INSERT INTO tenants (id, slug, name, status) VALUES
(gen_random_uuid(), 'academic-fixture-a', 'Fictional Academic School A', 'ACTIVE'),
(gen_random_uuid(), 'academic-fixture-b', 'Fictional Academic School B', 'ACTIVE');
INSERT INTO memberships (id, tenant_id, user_id, role)
SELECT gen_random_uuid(), t.id, u.id, CASE WHEN u.email = 'academic-owner@example.test' THEN 'SCHOOL_OWNER' ELSE 'TEACHER' END
FROM tenants t CROSS JOIN users u
WHERE t.slug = 'academic-fixture-a' AND u.email IN ('academic-owner@example.test','academic-teacher@example.test','academic-curriculum@example.test');

DO $$
DECLARE
    school UUID := (SELECT id FROM tenants WHERE slug = 'academic-fixture-a');
    session UUID := gen_random_uuid();
    grade_a UUID := gen_random_uuid();
    grade_b UUID := gen_random_uuid();
    class_a UUID := gen_random_uuid();
BEGIN
    INSERT INTO academic_sessions (id, tenant_id, name, starts_on, ends_on)
    VALUES (session, school, 'Enrollment fixture session', '2034-09-01', '2035-07-31');
    INSERT INTO academic_terms (id, tenant_id, session_id, sequence, name, starts_on, ends_on) VALUES
    (gen_random_uuid(), school, session, 1, 'Fixture First Term', '2034-09-01', '2034-12-15'),
    (gen_random_uuid(), school, session, 2, 'Fixture Second Term', '2035-01-06', '2035-03-31'),
    (gen_random_uuid(), school, session, 3, 'Fixture Third Term', '2035-04-01', '2035-07-31');
    UPDATE academic_sessions SET status = 'ACTIVE' WHERE id = session;
    INSERT INTO grade_levels (id, tenant_id, stage, code, name, sort_order) VALUES
    (grade_a, school, 'PRIMARY', 'FIXA', 'Fictional grade A', 1),
    (grade_b, school, 'PRIMARY', 'FIXB', 'Fictional grade B', 2);
    INSERT INTO classes (id, tenant_id, session_id, grade_level_id, code, name) VALUES
    (class_a, school, session, grade_a, 'FIXA', 'Fixture Class A'),
    (gen_random_uuid(), school, session, grade_b, 'FIXB', 'Fixture Class B');
    INSERT INTO class_arms (id, tenant_id, class_id, code, name)
    VALUES (gen_random_uuid(), school, class_a, 'A', 'Fixture Arm A');
END
$$;

INSERT INTO curriculum_framework_versions
    (id,jurisdiction,publisher,title,edition,framework_kind,effective_from,source_url,license_note)
VALUES
    ('82c5c04e-9009-443e-829a-ac8443248780','NG','Fictional Publisher','Fictional Nigerian Framework','Example 1','NIGERIAN_PRIMARY','2030-01-01','https://example.test/curriculum','Fictional source note'),
    (gen_random_uuid(),'GB-ENG','Fictional Publisher','Fictional British Framework','Example 1','ENRICHMENT','2030-01-01','https://example.test/enrichment','Fictional source note'),
    (gen_random_uuid(),'NG','Fictional Publisher','Fictional Unpublished Framework','Draft','NIGERIAN_PRIMARY','2030-01-01','https://example.test/draft','Fictional source note');
INSERT INTO curriculum_objectives (id,framework_version_id,stage,grade_band,subject,strand,code,objective_text)
VALUES
    (gen_random_uuid(),'82c5c04e-9009-443e-829a-ac8443248780','PRIMARY','Primary 1','Mathematics','Number','FIX.COUNT','Count the objects in a fictional group.'),
    (gen_random_uuid(),'82c5c04e-9009-443e-829a-ac8443248780','EARLY_YEARS','Nursery','Mathematics','Shape','FIX.SHAPE','Compare two fictional shapes.');
INSERT INTO curriculum_objectives (id,framework_version_id,stage,grade_band,subject,strand,code,objective_text)
SELECT gen_random_uuid(), f.id, 'PRIMARY', 'Year 1', 'Mathematics', 'Number', x.code, x.objective_text
FROM curriculum_framework_versions f CROSS JOIN (VALUES
    ('FIX.EXTEND', 'Group fictional objects in pairs.'),
    ('FIX.DRAFT', 'Fictional draft alignment text.')
) AS x(code, objective_text)
WHERE f.title = 'Fictional British Framework';
UPDATE curriculum_framework_versions SET status = 'PUBLISHED'
WHERE title IN ('Fictional Nigerian Framework', 'Fictional British Framework');
INSERT INTO curriculum_objective_alignments (id, primary_objective_id, related_objective_id, relation)
SELECT gen_random_uuid(), p.id, r.id, 'COMPLEMENTARY'
FROM curriculum_objectives p CROSS JOIN curriculum_objectives r
WHERE p.code = 'FIX.COUNT' AND r.code IN ('FIX.EXTEND', 'FIX.DRAFT');
UPDATE curriculum_objective_alignments a SET review_status = 'HUMAN_REVIEWED', reviewer_id = u.id
FROM curriculum_objectives r CROSS JOIN users u
WHERE a.related_objective_id = r.id AND r.code = 'FIX.EXTEND' AND u.email = 'academic-owner@example.test';

DO $$
DECLARE
    school_id UUID := gen_random_uuid();
    session_id_value UUID := gen_random_uuid();
    term_id_value UUID := gen_random_uuid();
    grade_id UUID := gen_random_uuid();
    class_id_value UUID := gen_random_uuid();
    subject_id_value UUID := gen_random_uuid();
    offering_id_value UUID := gen_random_uuid();
    teacher_id UUID := (SELECT id FROM users WHERE email='academic-teacher@example.test');
    teacher_membership UUID := gen_random_uuid();
    learner_id UUID := gen_random_uuid();
    student_id_value UUID := gen_random_uuid();
    selection_id UUID := gen_random_uuid();
    framework_id UUID := '82c5c04e-9009-443e-829a-ac8443248780';
    objective_id UUID := (SELECT id FROM curriculum_objectives WHERE framework_version_id=framework_id AND code='FIX.COUNT');
BEGIN
    INSERT INTO tenants (id,slug,name,status) VALUES(school_id,'classroom-fixture','Fictional Classroom School','ACTIVE');
    INSERT INTO users (id,email,password_hash,full_name,status,verified_at)
    SELECT learner_id,'academic-learner@example.test',password_hash,'Fictional Learner','ACTIVE',clock_timestamp()
    FROM users WHERE id=teacher_id;
    INSERT INTO memberships (id,tenant_id,user_id,role) VALUES
    (teacher_membership,school_id,teacher_id,'TEACHER'),
    (gen_random_uuid(),school_id,learner_id,'STUDENT');
    INSERT INTO academic_sessions (id,tenant_id,name,starts_on,ends_on)
    VALUES(session_id_value,school_id,'Classroom fixture',CURRENT_DATE-20,CURRENT_DATE+160);
    INSERT INTO academic_terms (id,tenant_id,session_id,sequence,name,starts_on,ends_on) VALUES
    (term_id_value,school_id,session_id_value,1,'Classroom First Term',CURRENT_DATE-20,CURRENT_DATE+20),
    (gen_random_uuid(),school_id,session_id_value,2,'Classroom Second Term',CURRENT_DATE+21,CURRENT_DATE+80),
    (gen_random_uuid(),school_id,session_id_value,3,'Classroom Third Term',CURRENT_DATE+81,CURRENT_DATE+160);
    UPDATE academic_sessions SET status='ACTIVE' WHERE id=session_id_value;
    INSERT INTO grade_levels (id,tenant_id,stage,code,name,sort_order) VALUES (grade_id,school_id,'PRIMARY','ROOM','Fictional grade',1);
    INSERT INTO classes (id,tenant_id,session_id,grade_level_id,code,name) VALUES
    (class_id_value,school_id,session_id_value,grade_id,'ROOM','Fictional classroom');
    INSERT INTO subjects (id,tenant_id,code,name) VALUES(subject_id_value,school_id,'COUNT','Fictional number work');
    INSERT INTO subject_offerings (id,tenant_id,subject_id,term_id,class_id) VALUES
    (offering_id_value,school_id,subject_id_value,term_id_value,class_id_value);
    INSERT INTO teacher_assignments (id,tenant_id,offering_id,teacher_membership_id) VALUES
    (gen_random_uuid(),school_id,offering_id_value,teacher_membership);
    INSERT INTO school_curriculum_selections (id,tenant_id,session_id,stage,curriculum_role,revision,framework_version_id,selected_by)
    VALUES(selection_id,school_id,session_id_value,'PRIMARY','PRIMARY',1,framework_id,teacher_id);
    INSERT INTO students (id,tenant_id,user_id,admission_no,display_name,stage) VALUES
    (student_id_value,school_id,learner_id,'ROOM-1','Fictional Learner','PRIMARY');
    INSERT INTO enrollments (id,tenant_id,student_id,session_id,term_id,class_id) VALUES
    (gen_random_uuid(),school_id,student_id_value,session_id_value,term_id_value,class_id_value);
    INSERT INTO lessons (id,tenant_id,offering_id,author_membership_id,curriculum_selection_id,primary_objective_id,title,content) VALUES
    ('13d0ffdf-aae1-4b72-b64c-6828436e94f1',school_id,offering_id_value,teacher_membership,selection_id,objective_id,'Count the classroom objects','Read the example, then count <strong>three</strong> objects.'),
    ('13d0ffdf-aae1-4b72-b64c-6828436e94f2',school_id,offering_id_value,teacher_membership,selection_id,objective_id,'Unpublished classroom draft','Draft text.');
    UPDATE lessons SET status='PUBLISHED' WHERE id='13d0ffdf-aae1-4b72-b64c-6828436e94f1';
END
$$;
'@ | & "$b/psql.exe" -h 127.0.0.1 -p 15435 -U sww_browser -d sww_e2e_test -v ON_ERROR_STOP=1
 if($LASTEXITCODE){throw 'fixture setup failed'}
 Set-Location "$root/apps/web"
 $env:API_INTERNAL_URL='http://127.0.0.1:18083'
 npm.cmd run build
 if($LASTEXITCODE){throw 'build failed'}
 npm.cmd exec -- playwright test --config playwright.academics.config.ts --output "$p/playwright"
 if($LASTEXITCODE){throw 'browser acceptance failed'}
 $counts = & "$b/psql.exe" -h 127.0.0.1 -p 15435 -U sww_browser -d sww_e2e_test -v ON_ERROR_STOP=1 -At -c "SELECT count(*) FILTER (WHERE t.slug = 'academic-fixture-a' AND s.name = '2030/2031 fixture'), count(*) FILTER (WHERE t.slug = 'academic-fixture-b' OR s.name = 'Denied fixture') FROM academic_sessions s JOIN tenants t ON t.id = s.tenant_id;"
 if($LASTEXITCODE -or $counts -ne '1|0'){throw 'Unexpected persisted academic rows'}
 'Verified one authorized session and zero denied or cross-school rows'
} finally {
 & "$b/pg_ctl.exe" -D $p -m fast -w stop
 if($LASTEXITCODE){throw 'Could not stop the disposable PostgreSQL server'}
}
