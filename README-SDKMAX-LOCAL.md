# SDKMAX Local Source Development

This workspace is prepared for SDKMAX development on top of New API `v1.0.0-rc.10`.

## Repository Layout

- Source path: `D:\sdkmax\new-api`
- Official upstream: `https://github.com/QuantumNous/new-api.git`
- Baseline commit in this local repository: `24cc04a chore: baseline new-api v1.0.0-rc.10`
- Baseline tag: `v1.0.0-rc.10-baseline`
- Branches:
  - `main`: clean New API rc.10 baseline
  - `sdkmax-dev`: local SDKMAX development branch
  - `sdkmax-prod`: production release branch

The GitHub source archive was used because `git clone` from GitHub timed out on this machine. The `origin` remote is still set to the official GitHub repository.

## Local Tooling

Detected or installed locally:

- Git: installed system-wide.
- Node.js: installed system-wide, currently `v24.14.0`.
- npm: installed, but PowerShell blocks `npm.ps1`; use `cmd /c npm ...` or run `npm.cmd`.
- pnpm: available through Corepack: `cmd /c "corepack pnpm --version"`.
- Go: portable Go `1.25.1` installed at `D:\sdkmax\tools\go`.
- MySQL: portable MySQL Community Server `8.4.10` installed at `D:\sdkmax\tools\mysql-8.4.10-winx64`.

Recommended shell setup for each PowerShell session:

```powershell
$env:PATH = "D:\sdkmax\tools\go\bin;D:\sdkmax\tools\mysql-8.4.10-winx64\bin;$env:PATH"
cd D:\sdkmax\new-api
```

## MySQL

This machine uses Windows native portable MySQL, not Docker.

Current local database:

- Data directory: `D:\sdkmax\mysql-data`
- Host: `127.0.0.1`
- Port: `3306`
- Database: `sdkmax_new_api`
- User: `sdkmax`
- Password: stored only in ignored `.env`

Start MySQL if it is not running:

```powershell
Start-Process -FilePath "D:\sdkmax\tools\mysql-8.4.10-winx64\bin\mysqld.exe" `
  -ArgumentList @("--basedir=D:\sdkmax\tools\mysql-8.4.10-winx64","--datadir=D:\sdkmax\mysql-data","--port=3306","--bind-address=127.0.0.1","--console") `
  -WindowStyle Hidden

mysqladmin -uroot ping
```

Verify the SDKMAX database user:

```powershell
mysql --user=sdkmax --password=sdkmax_local_dev_password_change_me --host=127.0.0.1 --execute="SHOW DATABASES LIKE 'sdkmax_new_api';"
```

For a fresh setup, initialize MySQL and create the database:

```powershell
mysqld --initialize-insecure --basedir="D:\sdkmax\tools\mysql-8.4.10-winx64" --datadir="D:\sdkmax\mysql-data"
mysqld --basedir="D:\sdkmax\tools\mysql-8.4.10-winx64" --datadir="D:\sdkmax\mysql-data" --port=3306 --bind-address=127.0.0.1 --console
```

Then in another shell:

```sql
CREATE DATABASE IF NOT EXISTS sdkmax_new_api CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS 'sdkmax'@'127.0.0.1' IDENTIFIED BY 'replace_with_local_password';
GRANT ALL PRIVILEGES ON sdkmax_new_api.* TO 'sdkmax'@'127.0.0.1';
FLUSH PRIVILEGES;
```

## Environment

`.env` exists locally and is ignored by Git. Keep all secrets there.

Committed template:

```powershell
Copy-Item .env.sdkmax.example .env
```

Important variables:

- `SQL_DSN`: MySQL DSN used by the Go backend.
- `SESSION_SECRET` and `CRYPTO_SECRET`: generate unique values for local and production.
- `VITE_REACT_APP_SERVER_URL`: frontend dev proxy target, normally `http://localhost:3000`.
- `DEPLOY_*`: production deployment script settings.

Before committing, check:

```powershell
git status --short
git check-ignore -v .env
```

## Backend

Install or download Go modules:

```powershell
$env:PATH = "D:\sdkmax\tools\go\bin;$env:PATH"
$env:GOPROXY = "https://goproxy.cn,direct"
go mod download
```

Start the backend from source:

```powershell
cd D:\sdkmax\new-api
go run main.go
```

Backend URL:

- `http://localhost:3000`
- Status: `http://localhost:3000/api/status`

The app auto-loads `.env` from the repository root through `godotenv`.

## Frontend

The default frontend is in `web/default`.

Using npm:

```powershell
cd D:\sdkmax\new-api\web\default
cmd /c npm install
cmd /c npm run dev
```

Using pnpm through Corepack:

```powershell
cd D:\sdkmax\new-api\web\default
cmd /c "corepack pnpm install"
cmd /c "corepack pnpm dev"
```

Using Bun is also supported by the upstream lockfile if Bun is installed:

```powershell
cd D:\sdkmax\new-api\web\default
bun install
bun run dev
```

The Rsbuild dev server proxies `/api`, `/mj`, and `/pg` to `VITE_REACT_APP_SERVER_URL`, defaulting to `http://localhost:3000`.

## Full Local Build

The Go backend embeds both frontend builds:

- `web/default/dist`
- `web/classic/dist`

Build the default frontend:

```powershell
cd D:\sdkmax\new-api\web\default
cmd /c npm install --no-package-lock
cmd /c npm run build
```

Build the classic frontend with npm-compatible pins from `bun.lock`:

```powershell
cd D:\sdkmax\new-api\web\classic
cmd /c npm install --no-package-lock --legacy-peer-deps
cmd /c npm install --no-save --legacy-peer-deps @douyinfe/semi-ui@2.72.2 @douyinfe/semi-icons@2.72.2 react-icons@5.5.0 antd@5
cmd /c npm run build
```

Then build the backend binary:

```powershell
cd D:\sdkmax\new-api
$env:PATH = "D:\sdkmax\tools\go\bin;$env:PATH"
go build -o .test\new-api-local.exe .
```

## Debugging

Backend:

```powershell
$env:DEBUG = "true"
$env:GIN_MODE = "debug"
go run main.go
```

Frontend:

```powershell
cd D:\sdkmax\new-api\web\default
cmd /c npm run dev
```

Useful checks:

```powershell
go test ./...
cmd /c npm run typecheck
cmd /c npm run lint
```

For Go debugging in VS Code or GoLand, set the working directory to `D:\sdkmax\new-api`, program to `main.go`, and keep environment loading from `.env`.

## Production Flow

Develop on `sdkmax-dev`, merge or cherry-pick stable changes into `sdkmax-prod`, then deploy:

```powershell
git switch sdkmax-prod
git merge sdkmax-dev
powershell -ExecutionPolicy Bypass -File .\scripts\deploy-prod.ps1
```

Production deployment uses `docker-compose.prod.yml`, not the upstream sample `docker-compose.yml`.

`docker-compose.prod.yml` builds from the server's checked-out SDKMAX source through the repository `Dockerfile`. It does not use `calciumion/new-api:latest`, and it does not start bundled PostgreSQL or MySQL containers. The production container uses host networking so `SQL_DSN` can point at the BT-managed native MySQL instance on `127.0.0.1:3306`.

Before the first deploy, add a private deployment remote. Keep `origin` for the official upstream repository only:

```powershell
git remote -v
git remote add deploy <your-private-sdkmax-repo-or-server-bare-repo-url>
```

Then set these values in ignored `.env`:

```dotenv
DEPLOY_REMOTE=deploy
DEPLOY_REMOTE_NAME_ON_SERVER=origin
DEPLOY_REMOTE_APP_DIR=/www/wwwroot/new-api
DEPLOY_REMOTE_DATA_DIR=/www/wwwroot/new-api/data
DEPLOY_COMPOSE_FILE=docker-compose.prod.yml
DEPLOY_REMOTE_DB_BACKUP_CMD=mysqldump -u<user> -p'<password>' sdkmax_new_api
```

Confirm the real server path in BT panel or with `docker inspect` before the first production run. Do not deploy through `origin`; the script refuses to push to the public `QuantumNous/new-api` upstream.

The deploy script:

- loads deployment settings from `.env`;
- backs up the remote database and `data` directory;
- pushes the selected local branch;
- SSHes to the server;
- pulls the selected branch;
- runs `docker compose -f docker-compose.prod.yml build` and `docker compose -f docker-compose.prod.yml up -d`;
- checks `https://api.sdkmax.com` with retries;
- rolls back to the previous commit if health check fails.

Keep production secrets in the local `.env` and the server environment, never in Git.
