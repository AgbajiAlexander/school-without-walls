param(
    [string]$PostgresBin = 'C:/Program Files/PostgreSQL/18/bin',
    [string]$OpenSSL = 'C:/Program Files/Git/usr/bin/openssl.exe'
)

# Run in a separate PowerShell process. Fixtures use ephemeral process variables.
$ErrorActionPreference='Stop'
$root=Split-Path -Parent $PSScriptRoot
$b=$PostgresBin
$p=Join-Path ([IO.Path]::GetTempPath()) ('sww-academic-e2e-'+[guid]::NewGuid())
if(Get-NetTCPConnection -LocalPort 15432,3000,3443,8080 -State Listen -ErrorAction SilentlyContinue){throw 'One of the isolated browser ports is occupied'}
& "$b/initdb.exe" -D $p -U sww_browser -A trust --no-locale --encoding=UTF8 | Out-Null
if($LASTEXITCODE){throw 'initdb failed'}
& "$b/pg_ctl.exe" -D $p -l "$p/server.log" -o '-p 15432 -h 127.0.0.1' -w start
if($LASTEXITCODE){throw 'start failed'}
try {
 & "$b/createdb.exe" -h 127.0.0.1 -p 15432 -U sww_browser sww_e2e_test
 if($LASTEXITCODE){throw 'create failed'}
 $env:DATABASE_URL='postgres://sww_browser@127.0.0.1:15432/sww_e2e_test?sslmode=disable'
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
 $env:SERVER_PORT='8080'
 Remove-Item Env:SMTP_HOST,Env:SMTP_PORT,Env:SMTP_USERNAME,Env:SMTP_PASSWORD,Env:SMTP_FROM -ErrorAction SilentlyContinue
 @'
INSERT INTO users (email, password_hash, full_name, status, verified_at)
SELECT 'academic-teacher@example.test', password_hash, 'Fictional Teacher', 'ACTIVE', CURRENT_TIMESTAMP
FROM users WHERE email = 'academic-owner@example.test';
INSERT INTO tenants (id, slug, name, status) VALUES
(gen_random_uuid(), 'academic-fixture-a', 'Fictional Academic School A', 'ACTIVE'),
(gen_random_uuid(), 'academic-fixture-b', 'Fictional Academic School B', 'ACTIVE');
INSERT INTO memberships (id, tenant_id, user_id, role)
SELECT gen_random_uuid(), t.id, u.id, CASE WHEN u.email = 'academic-owner@example.test' THEN 'SCHOOL_OWNER' ELSE 'TEACHER' END
FROM tenants t CROSS JOIN users u
WHERE t.slug = 'academic-fixture-a' AND u.email IN ('academic-owner@example.test','academic-teacher@example.test');
'@ | & "$b/psql.exe" -h 127.0.0.1 -p 15432 -U sww_browser -d sww_e2e_test -v ON_ERROR_STOP=1
 if($LASTEXITCODE){throw 'fixture setup failed'}
 Set-Location "$root/apps/web"
 $env:API_INTERNAL_URL='http://127.0.0.1:8080'
 npm.cmd run build
 if($LASTEXITCODE){throw 'build failed'}
 npm.cmd exec -- playwright test --config playwright.academics.config.ts --output "$p/playwright"
 if($LASTEXITCODE){throw 'browser acceptance failed'}
 $counts = & "$b/psql.exe" -h 127.0.0.1 -p 15432 -U sww_browser -d sww_e2e_test -v ON_ERROR_STOP=1 -At -c "SELECT count(*) FILTER (WHERE t.slug = 'academic-fixture-a' AND s.name = '2030/2031 fixture'), count(*) FILTER (WHERE t.slug = 'academic-fixture-b' OR s.name = 'Denied fixture') FROM academic_sessions s JOIN tenants t ON t.id = s.tenant_id;"
 if($LASTEXITCODE -or $counts -ne '1|0'){throw 'Unexpected persisted academic rows'}
 'Verified one authorized session and zero denied or cross-school rows'
} finally {
 & "$b/pg_ctl.exe" -D $p -m fast -w stop
 if($LASTEXITCODE){throw 'Could not stop the disposable PostgreSQL server'}
}
