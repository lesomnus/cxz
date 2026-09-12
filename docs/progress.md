# 구현 진행 상황

## 2026-09-12 — 프로젝트·대화 TUI 디자인 및 입력 개선

- Codex CLI의 대화 중심 구성과 편집 경험을 참고하여 프로젝트 정보/세션 카드,
  대화 역할/도구 출력, 승인 상태, 하단 rounded 입력창/단축키의 시각적 계층을 정리했다.
  밝은/어두운 터미널 대응 강조색과 Unicode 셀 너비 기반 자르기를 적용했다.
- 여러 줄 편집·붙여넣기, Enter 전송, Alt-Enter/Ctrl-J 줄바꿈, Ctrl-X 지우기,
  세션별 메모리 초안 복구를 추가했다. Esc로 초안을 지우지 않으며 PgUp/PgDn은
  본문만 스크롤한다. 기존 생성·로그인·승인·중단·재접속 API 동작은 유지했다.
- 한글/붙여넣기/명시적 전송/초안/크기별 렌더링 회귀 테스트를 추가했다.
  전체 Go 테스트·vet, CLI/TUI race, 실제 PTY 화면 이동/터미널 복구,
  uncached integration 및 diff 검사를 통과했다.
  실제 OAuth나 유료 모델 호출은 이 UI 변경의 검증에 사용하지 않았다.
  참고: https://learn.chatgpt.com/docs/codex/cli (공식 Codex CLI 문서).

## 2026-09-12 — 워크스페이스 진입 명령과 프로젝트/세션 화면

- install/uninstall을 최상위로 되돌리고 up/down/it을 추가했다. up은 Account 없이
  프로젝트만 준비해 대시보드를 열며, 실행 중인 기존 프로젝트는 재준비 없이 연결한다.
  it은 해당 프로젝트의 최신 생성 세션 화면으로 연결하고 없으면 생성 없이 안내한다.
- 프로젝트 화면에 정보·세션 목록, 방향키/Enter 선택, n 새 세션, s 중지, d 삭제 확인을
  추가했다. 생성 중에는 터미널을 반환해 Account 검색·공식 로그인 후 세션 화면으로
  이어진다. Ctrl-Q는 프로젝트 화면으로 돌아가며 Ctrl-C는 연결만 끊는다.
- payday Project/Session.Erase를 lifecycle 규칙에 맞게 열었다. 프로젝트 삭제는 소유
  컨테이너 정리 후 프로젝트와 세션을 목록에서 제외한다. 세션 삭제는 실행 agent 중지를
  확인한 뒤 제외한다. 소스/볼륨/저널을 보존하고 Listed tombstone은 일반 조회·제어와
  snapshot 재등장을 막는다. 같은 workspace 재등록 시에도 이전 세션은 제외한다.
- 기존 프로젝트당 실행 세션 하나 제약, 저수준 project down의 등록/세션 보존 동작은
  유지한다. DB 재구축은 tombstone을 잃을 수 있으므로 resource DB 백업이 필요하다.
- 준비 반복 시 세션·Account 부작용 없음, 프로젝트 격리·최신 선택·삭제 확인 대상 고정,
  Ctrl-Q·PTY 터미널 복구, 실제 RPC live-session 삭제와 서버 재시작 후 제외를 검증했다.
- 전체 Go 테스트·vet, CLI/TUI/lifecycle/integration race, diff 검사 통과.
  Docker runtime probe 및 실제 OAuth/유료 모델 호출은 이번 변경에서 실행하지 않았다.

## 2026-09-12 — 리소스 중심 CLI와 기본 표 출력

- 공개 작업 명령을 project/session/account/manager/config 및 backend/binding 하위로
  재편했다. projects → project ls, ls → session ls, new → session new,
  up/recreate/down → project 하위, 설치·서버 실행·업데이트 → manager 하위로 옮겼다.
  로그는 manager logs / project logs PROJECT로 분리하며 config show를 명시한다.
  이전 최상위 명령의 호환 별칭은 제공하지 않는다. 내부 프로세스 진입점은 유지한다.
- 데이터 명령의 기본 출력은 표이며 --format table|json을 추가했다. 명령별 옵션이
  루트 옵션보다 우선한다. 목록 주요 열, 상세 FIELD/VALUE, 빈 목록, 한글 표시 폭,
  터미널 너비에 따른 축약과 제어문자 비활성화를 처리한다. JSON/JSONL은 기존
  직렬화 필드·큰 정수·줄바꿈을 보존한다. raw exec/shell/logs/login/TUI는 변환하지 않는다.
- account status도 포맷을 지원하고 파일 존재와 vendor 인증 검증 여부를 구분한다.
  Docker manager의 시작 인자·이미지 CMD, 도움말·완성·안내문, 현재 문서와 probe 호출을
  갱신했다. 자동화 probe는 JSON을 명시하고 과거 릴리스 문서는 원래 버전의 설명을 유지한다.
- 전체 Go 테스트·vet, CLI/installer/integration race, diff 및 probe JS 구문 검사 통과.
  Bake --print 설정 검증 통과. 이번 변경에서 Docker runtime probe나 실제 OAuth는 실행하지 않았다.

## 2026-09-12 — Claude 로그인 코드 입력 표시

- 대화형 Claude auth login을 파이프로 연결하고 cxz 입력 표시기를 추가했다.
  빈 상태는 `[     ]`, 입력된 상태는 길이와 무관하게 고정된
  `[ *** ] inserted; ctrl+x to clear.`를 표시한다. 코드 자체와 글자별 마스킹은 출력하지 않는다.
- Ctrl-X는 미제출 입력을 지우고 Enter는 공식 CLI에 한 줄로 전달한다. 제출/취소/종료 시
  입력 모델을 초기화하며 Esc/Ctrl-C 종료 시 자식 프로세스와 터미널을 정리한다.
  OAuth URL·코드 교환·인증 저장은 기존 공식 CLI와 격리된 staging 경로를 유지한다.
- 고정 Claude 2.1.267의 readline 기반 수동 코드 처리를 로컬 설치 바이너리에서 확인했다.
  입력 모델·합성 자식 프로세스·Linux PTY에서 비노출 표시, 지우기, 제출, 취소,
  종료와 터미널 복구를 검증했다. 실제 OAuth 인증은 실행하지 않았다.
- 전체 Go 테스트·vet, accounts/CLI race, diff 검사 통과.

## 2026-09-12 — 프로젝트 첫 로그인을 up 흐름에 연결

- project-local-oauth의 인증 파일 누락을 구조화된 gRPC ErrorInfo로 전달한다.
  대화형 CLI new/up/recreate는 준비된 프로젝트의 기존 공식 로그인 워크플로를
  실행하고 성공하면 세션 생성/재접속을 한 번 재시도한다.
- 번호/문자열 오류 파싱에 의존하지 않는다. 중앙 backend, 다른 계정, 인증 파일 손상은
  자동 로그인 대상에서 제외한다. 계정 binding·소유권·활성 세션 검사와 OAuth 격리는 유지한다.
- 재시도 시 recreate는 해제하고 request ID와 새 세션 여부는 보존한다. 로그인 출력은
  stderr로 분리한다. 취소 시 준비된 프로젝트를 보존하며 비대화형은 로그인 명령을 안내한다.
- CLI 성공/실패/취소/재시도 제한 테스트와 실제 RPC의 누락 인증 details 전달,
  인증 전 세션 미생성 및 합성 인증 설치 후 동일 요청 성공 통합 검증을 추가했다.
  실제 OAuth 브라우저 로그인이나 유료 모델 호출은 실행하지 않았다.
- 전체 Go 테스트·vet, CLI/accounts/server/integration race, diff 검사 통과.

## 2026-09-12 — 검색 가능한 계정 선택 TUI

- CLI new/up/recreate의 정수 전용 Fscanln 선택기를 Bubble Tea 화면으로 교체했다.
  이름을 입력하면 파싱이 실패하고 나머지 문자가 셸에 남을 수 있던 경로를 제거했다.
- 방향키 선택, 하단 상시 검색 입력, 번호·display name·alias 정확 일치 및
  이름·alias·agent 부분 검색을 제공한다. 중복 후보, 검색 결과 없음, 긴 목록 스크롤,
  한글 입력과 화면 크기 변경을 처리한다. Esc/Ctrl-C는 계정 선택 없이 취소한다.
- 입력/출력은 CLI의 stdin/stderr를 사용해 stdout JSON과 분리한다. 비대화형 계정
  명시 및 기존 세션의 Account 유지 동작은 변경하지 않았다.
- 모델/실제 입력 디코더 회귀 테스트 및 Linux PTY에서 main 입력·Ctrl-C 취소,
  터미널 상태 복구와 입력 잔여 없음 검증을 추가했다.
- 전체 Go 테스트·vet, CLI/TUI race, diff 검사 통과. Docker나 실제 인증 상태는 변경하지 않았다.

## 2026-09-12 — 전체 CLI 입력 계약 점검

- `account add AGENT ACCOUNT`, `update IMAGE`, 내부 `_new-local ACCOUNT WORKSPACE`
  형식으로 필수값을 positional에 드러냈다. agent에는 기본값 없이 enum/completion을 제공한다.
- `project set` 변경값 누락, 계정/프로젝트 alias, 모델/이름, 빈 문자열, config 키,
  reply JSON을 설정 IO·연결 전에 검사한다. 비대화형 new의 Account 누락도 즉시 거부한다.
- 재접속/대화형 선택에 필요한 선택 옵션은 유지하고 도움말에 조건·실제 기본값을 명시했다.
  중앙 인증에 적용되지 않는 `--project`는 계정 조회 후 명시적으로 거부한다.
- 사용되지 않는 루트 agent/claude-config 옵션과 항상 실패하던 login 진입점을 제거했다.
  Account가 agent를 결정하므로 `config set agent`는 거부하고 기존 설정의 unset은 허용한다.
- 모든 하위 명령 도움말, agent completion, Account Add RPC 전달, 잘못된 입력의
  state 미생성·무연결 회귀 테스트를 추가했다. 예제와 Docker probe의 CLI 호출도 갱신했다.
- 전체 Go 테스트·vet, CLI/integration race, diff 검사 통과. 이번 변경에서 Docker probe나
  실제 OAuth/유료 모델 호출은 실행하지 않았다. [전체 명령 점검표](cli.md).

## 2026-09-12 — 세션 조건 fail-fast

- `up`이 컨테이너를 준비한 뒤 빈 AccountRef로 새 세션 binding을 만들던 경로를 수정했다.
- 공통 resource client와 CLI가 등록·provision/recreate 전에 계정·agent/backend,
  활성 세션 충돌·모델 불변 조건을 확인한다. 조회 실패는 즉시 반환한다.
- 기존 세션의 계정을 유지하며, 첫 up/recreate에서는 new와 같은 Account 선택을
  제공한다. 비대화형 계정 누락은 `--account` 안내로 실패하고 준비 메시지도 출력하지 않는다.
- 새 세션 조건 실패 시 Project Add/Up/Recreate가 호출되지 않는 테스트와 CLI 회귀,
  기존 세션의 모든 재접속 상태 및 로그인 prepare-only 예외 테스트를 추가했다.
- 전체 Go 테스트·vet, resourceclient/CLI/integration race, diff 검사 통과.

## 2026-09-12 — devcontainer 신뢰 검사 진단

- `--trust-config` 요구 오류에 설정 파일명과 차단된 항목의 JSON Pointer 경로,
  탐지 이유를 모두 표시한다. devcontainer JSONC와 Compose YAML 양쪽에 적용했다.
- 기존 차단 기준은 변경하지 않았다. 출력 순서를 고정하고 경로의 특수문자를
  이스케이프하며 명령·환경변수·mount 원문 값은 출력하지 않는다.
- 각 탐지 규칙, 여러 원인 동시 표시, 값 비노출, 경로 이스케이프, explicit trust,
  recreate 전 preflight의 파일명·항목 경로를 회귀 테스트에 추가했다.
- 전체 Go 테스트, workspace race·vet, diff 검사 통과.

## 2026-09-12 — Codex 중앙 인증 공급

- 신규 Codex Account의 기본 backend를 `brokered-access-token`으로 변경했다.
  기존 Account의 backend는 유지하고 Claude의 프로젝트별 OAuth도 유지한다.
- 공식 Codex device login과 app-server managed refresh를 중앙 Account별 격리
  프로필에서 실행한다. cxz는 OAuth endpoint/code 교환을 재구현하지 않는다.
- 프로젝트별 capability를 비공개로 전달하고 Unix socket을 통해 access token만
  공급한다. AuthBinding은 프로젝트별 공급 권한을 고정하며 token/capability는
  공개 resource·audit·대화 journal에 넣지 않는다. 프로젝트는 ephemeral 인증을 쓴다.
- 재로그인 시 workspace/account ID와 사용자 subject를 고정하고, 중앙 로그인·갱신은
  계정 lock으로 직렬화한다. 다른 계정, 위조 capability, manager 중단은 명시적으로 실패한다.
- 전체 Go/race·vet·payday 코드 생성 일치·diff 검사 통과. Docker에서 중앙 로그인으로 두 프로젝트 사용, 동시 갱신,
  개인/회사 프로필, 승인·중단, resume·manager restart·project recreate 통과.
  non-root(node 사용자) 프로젝트를 포함한 최종 회귀도 통과했다.
- 네이티브 Codex 0.154.0도 합성 JWT로 external login 수용·account/read·ephemeral
  auth.json 비저장을 확인했다. 실제 OAuth 로그인/갱신과 유료 모델 호출은 하지 않았다.
- [인증 설계와 제약](accounts.md): 공식 external-token API는 실험적이며, 토큰 추출은
  고정 버전의 managed auth.json 형식에 의존한다. 사용자 인증/roster와 권한 철회 UI는 별도다.
- 추가 검증 중 공유 Docker 주소 풀 고갈을 만났지만 이번 시험의 이전 소유 네트워크만
  정리하고 재시도하여 통과했다. 타 프로젝트 자원/prune은 사용하지 않았다.
- [검증 요약](../testdata/recovery/codex-broker/summary.json).
- 시험에 사용한 두 manager와 프로젝트 컨테이너, 합성 인증 state/tools 볼륨 및
  소유 네트워크는 삭제했다. scratch workspace·이미지 빌드 캐시는 보존했다.

## 2026-09-12 — AuthBackend / AuthBinding 분리

- AgentKind별 기본/지원 backend factory registry와 `Info/Binding/Login/Check/Launch`
  계약을 도입했다. 기존 프로젝트 OAuth 로그인·인증 확인·환경/인자 구성을 backend로 옮겼다.
- payday Account.auth_backend, AuthBinding(domain 10), Session.auth_binding을 선언·생성했다.
  binding은 project/account/backend별로 안정적인 ID를 가지며 재시도해도 중복 생성되지 않는다.
- CLI `account backends`, `account bindings`, `account add --auth-backend`를 추가했다.
  실제 login/status/new/resume과 manager→project 경로가 backend/binding을 사용한다.
- backend/binding은 manifest와 조회 JSON에 보존한다. 누락·불명 backend와 다른 범위의
  binding은 실행/복구 시 거부한다. 기본값은 Account 생성 시에만 선택한다.
- Claude/Codex의 프로젝트별 OAuth만 활성화했다. 중앙 토큰 공급·API key·실계정 인증은
  추가하지 않았다. 실제 OAuth 로그인·유료 모델 호출 없이 합성 agent로 검증했다.
- 전체 Go 테스트·race·vet와 payday 코드 생성 일치 검사를 통과했다. Claude/Codex
  로그인 인자·환경 구성, 실패한 로그인 시 기존 인증 보존, 잘못된 저장 binding의
  실행 전 거부를 테스트했다.
- Docker `owned-accounts.mjs`: 지원 backend 조회·미지원 backend 거부, 재로그인 시
  binding 중복 방지, 계정/프로젝트별 binding 분리, Session 전달, manager 재시작 및
  project recreate 후 동일 backend/binding 유지 통과. 기존 Account 격리 회귀도 통과했다.
- `cxz-container-recreate.mjs`: non-root·network-none에서 대화 보존, 명시적 resume과
  새 Run, 과거 승인 거부, 진행 중 컨테이너 소실 후 대화 복구 통과.
- 합성 인증 시험에 사용한 전용 컨테이너와 state/tools 볼륨은 정리했다.
  scratch workspace·이미지 캐시는 보존했다. [검증 요약](../testdata/recovery/auth-backends/summary.json).

## 2026-09-12 — Account 인증 프로필

- payday Account 리소스(domain 9), 고정 Session.account 연결, 계정 등록/조회,
  프로젝트별 로그인·상태 확인, CLI --account와 TUI 계정 선택을 구현했다.
- 최초 구현의 중앙 vault 복제는 architecture §4.2와 충돌하여 최종 구현에서 제거했다.
  실제 OAuth grant는 프로젝트×Account별로 독립 발급·저장한다. 계정별 HOME/config를
  사용하고 inherited vendor 인증 환경변수를 차단한다. refresh token을 복제하지 않는다.
- 누락된 인증·다른 vendor·다른 Account로의 live attach를 거부한다. 정지 세션의
  resume은 계정을 유지하며, 재로그인은 다음 실행 시 반영한다.
- 전체 Go 테스트 및 `CXZ_TEST_RACE=1 go test -race ./... -count=1` 통과.
  Account 리소스/인증 환경/갱신 보존/DB 복구 테스트를 추가했다.
- Docker `owned-accounts.mjs`: 실제 CLI의 staged login/status(합성 vendor), 두 Account
  별도 세션, 계정별 HOME/config, 환경 인증 차단, 잘못된 live attach/활성 로그인 거부,
  원래 계정 resume, manager 재시작·project recreate 후 동일 계정 유지 통과.
  manager에 인증 저장소가 생기지 않는 것도 확인했다.
- 별도 Project에서 동일 Account를 선택해도 기존 프로젝트 인증을 사용하지 못하고
  독립 로그인을 요구하는 것을 확인했다. 기존 이름/alias up·exec·logs·rename도 재검증했다.
- 최종 `go vet ./...`, `go tool pd gen --check .`, `git diff --check` 통과.
- `cxz-container-recreate.mjs`: non-root·network-none에서 Account/대화 보존,
  새 Run·과거 승인 거부·진행 중 컨테이너 소실 후 복구 통과.
- 실계정 로그인/유료 대화는 수행하지 않았다. `owned-session-live.mjs`는 명시적으로
  인증된 Project/Account를 받도록 바꾸고, host token 복사 경로를 제거했다.
- 합성 인증을 사용한 시험 manager/project 컨테이너와 전용 state/tools 볼륨은 정리했다.
  실제 계정에는 로그인하지 않았으며 scratch workspace와 이미지 빌드 캐시는 보존했다.
- 범위·제약: [Account 설계](accounts.md). 사용자 인증/roster와 프로젝트별 허용 계정
  정책은 별도이며, 동일 OS 사용자 간의 적대적 코드 격리를 주장하지 않는다.

## 2026-09-12 — Project display name / alias

- payday Project의 `name`을 표시 이름으로 사용하고 `alias = 4`에 전역 unique
  인덱스를 선언했다. 생성된 ProjectRef의 alias 조회와 metadata-only Patch를 지원한다.
- cld의 짧은 이름/단어 이니셜/긴 단어 절단 방식을 참고해 자동 alias를 생성한다.
  자동 충돌은 ID 기반 suffix로 해소하고 기존 alias는 바꾸지 않는다. 직접 지정한
  중복/잘못된 alias는 거부한다. Patch는 version 조건을 요구하고 상태 수정은 막는다.
- `project add --name/--alias`, `project set --name/--alias`, new/up/recreate의
  이름 옵션을 추가했다. up/down/attach/exec/shell/login/logs가 공통 project resolver를
  사용한다. ID·경로 → alias → 표시 이름 순서이며 중복 표시 이름은 임의 선택하지 않는다.
- JSON 조회와 TUI에 표시 이름·alias를 노출하고 프로젝트 인자 자동완성을 추가했다.
  표시 이름만 바꿔도 alias는 유지하며, alias를 바꾸면 이전 alias는 더 이상 해석하지 않는다.
- 통과: 자동 생성/충돌, 명시적 중복 거부, generated alias ref, 변경 후 ID 유지,
  재시작 후 유지, 등록 재시도 시 metadata 보존, 상태 Patch 우회 차단, 전체 race.
  Docker alias 흐름도 `scripts/probes/owned-project-names.mjs`로 통과했다:
  alias 기반 up/exec/logs/down, 이름·alias 변경 후 동일 session 재접속.
  테스트 manager/container는 정리하고 작업 파일·볼륨은 보존했다.

## 2026-09-11 — Docker Bake 및 edge 게시

- cld의 build → app 흐름을 참고해 루트 `Dockerfile`, `docker-bake.hcl`,
  source-only `.dockerignore`를 추가했다. build는 amd64/arm64 정적 바이너리를
  `dist/linux-*/cxz`로 내보내고 app은 기존 설치용 Dockerfile로 이미지를 만든다.
- 설치 명령의 tar context도 같은 아키텍처별 경로를 사용한다. release workflow의
  임시 Dockerfile 생성/sed를 제거하고 같은 Bake app target을 사용한다.
- CI는 test 성공 후 이미지를 빌드한다. main push만 GHCR에 `edge`와
  `sha-<commit>`을 게시하고, PR은 로그인/게시하지 않는다. 이전 main 실행이 늦게
  edge를 덮어쓰지 않도록 workflow concurrency를 설정했다.
- 이미지 라벨과 바이너리 version 출력에 commit 정보를 포함한다.
- 로컬 확인: Bake 설정 해석, amd64/arm64 정적 바이너리 빌드, workflow actionlint,
  전체 Go 테스트 통과. amd64 Bake 이미지의 `version` 실행에서 edge 및 build hash를
  확인했다. CI는 별도로 두 아키텍처 이미지를 빌드하고 main에서 게시한다.

## 2026-09-11 — proto 버전 표기 제거

- 사용자 요청에 따라 공개 proto 패키지를 `cxz`, 정의 경로를 `proto/cxz`,
  확장 경로를 `proto/ext/cxz`로 정리했다. 버전 접미사나 하위호환 별칭은 없다.
- 내부 view model도 버전 대신 `cxz.runtime` namespace와
  `internal/runtimeproto` 경로를 사용한다. 공개 서비스로 등록하지 않는다.
- payday/ORM 및 내부 protobuf 생성 코드를 새 이름으로 재생성했다.
- 전체 Go 테스트와 proto namespace 회귀 테스트를 통과했다.

## 2026-09-11 — payday resource framework 전환

이전의 config/DB/grpcx만 이용한 구현은 사용자가 요청한 payday 프레임워크
사용을 충족하지 않았다. 아래 전환은 기존 rc.1 태그와 별개의 변경이다.

- `proto/cxz/{project,session}.proto`에 tenant 없는 global 리소스를 선언하고
  `proto/ext/cxz/*_svc.ext.proto`에 Up/Down/Recreate 및
  Resume/Send/Reply/Interrupt/Stop/Events/History를 선언했다.
- 버전 고정 Go tools와 Buf lock으로 실제 `pd gen`을 실행했다.
  `resource/`, `server/bare/`, `server/pd/`, `internal/ent/`가 생성 산출물이다.
- `server/lifecycle/`가 generated Sink·ent SQLite·payday audit/gate·Watch를
  실제 서버 요청 경로에 연결한다. Docker/process 작업은 DB transaction 밖에서
  실행하고, 내부 projection 쓰기도 commit 이후 Watch 알림을 발행한다.
- CLI/TUI 및 manager→project는 generated resource client를 사용한다.
  기존 Sessions 공개 service 등록은 제거했다. runtime DTO만 내부에 남겼다.
- Project.Up은 provisioning만 한다. Session.Add와 Resume는 별도 요청이다.
  runtime/vendor ID·저널·생성 재시도 키를 유지하면서 payday domain UUID에 매핑한다.
- 통과: resource API를 통한 생명주기/승인/중단/복구 및 두 SQLite DB 재구성,
  xli CLI 전달, generated audit 기록·tenant 불필요·stale run 거부,
  상태 Patch 우회 거부, payday Watch snapshot/update, `pd gen --check`.
- 통과: `go test ./...`, 전체 `CXZ_TEST_RACE=1 go test -race ./... -count=1`,
  `go vet ./...`, `pd gen --check`, Docker foreign/recreate/프로젝트 권한/단일 agent/down
  경계, manager 강제 종료 checkpoint 복구와 동시 up 수렴.
- 통과: non-root 컨테이너 재생성 후 대화·저널·vendor ID 유지, 새 run의 명시적 Resume,
  stale 승인 거부, 진행 중 turn 손실 후 재접속. 인증 없는 deterministic agent로 검증했다.
  증거: `testdata/recovery/payday-resource/summary.json`.
- 첫 Docker 테스트는 개발 바이너리를 CGO 기본값으로 빌드해 Alpine loader 실패;
  배포와 동일한 CGO_ENABLED=0 정적 빌드로 재검증했다. 실제 vendor 인증/과금 대화는
  이번 전환에서 다시 실행하지 않았고, 이전 vendor-specific 잔여 검증은 그대로 남는다.
- 사용자 인증/roster, HTTP/Web UI는 이번 범위에 추가하지 않는다.
  Dockerfile은 기존 `internal/installer/image.Dockerfile`; Bake 전환은 하지 않았다.
- 전체 state volume 백업은 필요하며 resource 표시 이름/설명과
  audit는 저널로 재생성되지 않는다.

설계: [payday resource API](plans/payday-resources.md).

## 2026-09-11 — 실사용 보강·prerelease 게시 (남은 검증 별도)

순서: 복구·재시도 → TUI 설정/진단 → 설치·배포. 웹 UI/사용자 인증은 후속 범위.

- 설치 identity를 리소스 생성 전에 원자적으로 보존해 실패한 첫 설치도 같은
  owner/volume을 재사용한다. 동일 이미지의 설치 재호출은 readiness 재확인,
  정지한 manager는 시작한다. endpoint는 기존 manager 제거 전에 검증한다.
- 프로젝트 준비 단계/시도 횟수를 manifest에 기록한다. manager 재시작은 실행 중
  job을 interrupted로 표시하며 SQLite 재구성 후에도 마지막 단계를 유지한다.
- 시작/프로젝트 조회 시 소유 label 기반 inventory를 재확인한다. 생성 후 저장 전에
  끊긴 container ID를 복구하고, 중복 소유 컨테이너는 임의 선택하지 않는다.
- 재시도는 실제 리소스 소유권을 다시 확인하고 수렴한다. devcontainer 사용자 hook은
  다시 실행될 수 있어 멱등하게 작성해야 한다. prompt 자동 재전송은 하지 않는다.
- 통과: 설치 실패 identity 재사용, endpoint 사전 거부, checkpoint/DB 재구성,
  inventory 복구·foreign/중복 거부, 기존 workspace/CLI 테스트.
- Claude 반복 복구 미통과는 아직 해소하지 않았으며 후속 검증 항목으로 유지한다.

추가 구현:

- `config`의 기본 agent/vendor별 model, `new --model`, TUI 새 세션 기본값.
  model은 세션 manifest/API에 저장하고 Claude argv/Codex thread start/resume로 전달한다.
  기존 세션의 모델은 변경하지 않는다. 설정은 secret/권한 변경을 허용하지 않는다.
- TUI 진단/실패 payload와 인증 의심 메시지의 login 안내, `doctor`, `logs`, `version`.
- 버전 고정 `update --image`, 이전 manager 이미지 `rollback`. 이미지 다운로드/플랫폼
  검증을 기존 manager 제거 전에 수행한다. named volumes/project 프로세스는 보존한다.
- Linux amd64/arm64 release build와 checksum, SHA-pinned CI/release workflow.
  원격 게시/arm64 실실행은 로컬 cross-build와 별도로 검증해야 한다.
- 통과: 전체 test/race/vet, 설정 offline 검증·0600·잘못된 값/secret key 거부,
  API model 전달·영속 세션·Codex protocol, TUI 진단/로그인 안내.
- 실제 Docker foreign/명시적 recreate/RO tools/내부 범위/down 보존 경계 6개 통과.
- Claude 재검증에 쓸 기존 access token의 유효기간이 부족하다. refresh token을 사용하거나
  사용자의 계정을 조작하지 않았고 반복 복구의 기존 미통과 판정은 유지한다.
- 운영 절차와 범위는 [operations.md](operations.md). 준비 중 강제 종료/동시 재시도,
  update/rollback, Codex live 및 release 게시 검증은 아래에 결과를 추가할 예정이다.

실제 검증 추가:

- `owned-provision-retry.mjs`: hook 실행 중 manager SIGKILL → interrupted checkpoint,
  저장 직전 container ID 복구 → 동시 up 2회에서 동일 container/session 유지, 로그 조회 통과.
  최초 probe의 short/full Docker ID 비교 오류는 `--no-trunc`로 수정 후 전체 재실행했다.
- `owned-install-update.mjs`: 빈 state 설치, 반복 install, 정지 manager 시작, 존재하지 않는
  update 이미지 거부 시 기존 manager/locator 보존, update/rollback identity·volume 유지 통과.
  별도 검증 manager는 제거했고 named volumes와 locator는 보존했다.
- Codex 0.154.0 실제 인증 대화, manager restart 동일 run, 도구 승인/거절,
  실행 중 interrupt, 컨테이너 재생성 2회 후 동일 vendor ID/대화 기억: 7개 모두 통과.
  이번 검증의 일회성 access token은 제거했고 refresh token은 복사하지 않았다.
- Linux amd64/arm64 artifact cross-build 및 SHA256SUMS 검사 통과.
  Claude 재인증/반복 복구는 미통과로 남겨 prerelease와 안정판 완료를 구분한다.

배포/인계 결과:

- `v0.1.0-rc.1`을 GitHub Release에 게시했다. release commit은 `4a386ad`이며
  xli CLI/설정/진단/복구/업데이트 기능을 포함한다. 이후 main 변경은 검증 도구와 문서다.
- GitHub CI 및 Release workflow 성공. 공개 바이너리 두 아키텍처를 다시 다운로드하여
  SHA256 검증, amd64 version/revision 일치를 확인했다.
- Docker 로그인 설정이 없는 별도 client에서 GHCR digest로 빈 설치 → doctor →
  프로젝트 foreign/recreate/내부 범위/RO tools/down 6개 경계 검증 통과.
- 실행 중 Codex supervisor를 둔 manager update/rollback에서도 같은 run 유지.
  강제 컨테이너 제거 및 manager SQLite 재구성, 실제 TUI 자동 재접속도 재검증 통과.
- Claude 문제를 분리하는 `CXZ_PROBE_MEMORY_ONLY=1` 모드를 추가했다. syntax 검증만 했으며
  Claude 실제 최소 재현은 재로그인 후 실행해야 한다. 안전장치는 변경하지 않았다.
- arm64는 artifact/image 빌드와 checksum만 검증했다. 현재 amd64 Docker engine에서 실행은
  `exec format error`로 불가능했다. 공유 엔진에 전역 QEMU/binfmt 설정을 설치하지 않았다.
- 검증용 manager 3개와 이번 프로젝트 컨테이너를 제거했다. 사용자 작업 폴더/영속 볼륨,
  복구 가능한 SQLite backup, 다운로드 산출물은 보존했으며 prune은 실행하지 않았다.
- 정제된 증거: `testdata/recovery/operations-summary.json`.
  최초 vendor 로그인, Claude 반복 복구 재검증, native arm64 인수 검증은 남은 항목이다.
  웹 UI/roster/IDE/외부 백업은 이번 세 우선순위의 범위가 아니며 여전히 후속 계획이다.

## 2026-09-11 — xli CLI 전환

- `flag.FlagSet`, 수동 최상위 명령 분기, `optionsFirst` 순서 보정 제거.
  `xli.Command` 트리, typed flags/args, 명령별 handler로 교체했다.
- payday가 이미 요구하는 xli revision `bf8cac633057`을 직접 의존성으로 사용하며
  payday 버전과 서버/supervisor/agent API는 변경하지 않았다.
- API 연결은 실행 handler에서만 수행한다. 도움말·완성은 설치/네트워크 연결 없이 동작.
- 자동 도움말, zsh completion, vendor/승인 값 및 workspace/config 경로 완성을 추가.
- 명령 IO는 xli의 입출력을 사용한다. JSON stdout과 진행/확인 stderr를 분리하고
  `exec PROJECT -- COMMAND...`의 나머지 인자를 그대로 보존한다.
- 문법 변경: `new --agent codex .`, `recreate --yes .`, `login --agent codex PROJECT`.
  위치 인자 뒤 플래그는 xli 규칙에 따라 거부한다. README와 기존 probe를 갱신했다.
- 내부 `_boot/_project/_supervise/_guard/_bridge/_ready/_login/_new-local`과
  `attach/it`, `tui/watch` alias는 유지한다. 기존 root-level serve flags도 호환 유지.
- 통과: xlitest 도움말/입력 검증/완성/exec 인자, 실제 gRPC 전달 테스트,
  기존 프로세스 통합 테스트, Docker install/경계 probe 6개 검사,
  `CXZ_TEST_RACE=1 go test -race ./... -count=1`, `go vet ./...`.
  vendor adapter를 변경하지 않았으므로 유료 모델 호출/로그인은 재실행하지 않았다.
- 검증 후 이번 실행의 project container와 manager를 제거했다. 기존 workspace,
  named volumes와 private evidence는 보존했고 공유 엔진에 prune을 실행하지 않았다.

## 2026-09-11 — cld 사용 흐름 + Codex 확장 (현재)

이전 로컬 TUI 구현은 기반 단계였다. 현재 완료 기준은
[cld-parity 계획](plans/cld-parity.md)이며, 아래의 과거 제외 범위를 대체한다.

- 구현: Docker manager install/reinstall/uninstall, official devcontainer CLI
  provisioning, image/Dockerfile/Compose 설정, 프로젝트별 state volume와 RO tools.
- 구현: `up/new` 자동 TUI 연결, Claude/Codex 선택, `attach/it`, `login`, `shell/exec`,
  `down/up/recreate`. 외부 컨테이너는 편입하지 않으며 확인한 recreate만 허용.
- 구현: Codex app-server 초기화/thread/turn/approval/question/interrupt/resume.
  공식 schema/docs를 확인해 numeric request ID 보존과 빈 thread 재시작을 처리.
- 실검증 통과: image+기존 hook, Alpine Claude 시작, Compose Codex 시작,
  sidecar DNS, tools readonly, 프로젝트 내부 scoped client.
- 실검증 통과: 실제 Codex 인증 대화, manager restart 시 동일 supervisor run,
  도구 승인/거절, 실행 중 중단, 컨테이너 down/up **2회** 후 동일 vendor thread
  및 대화 기억 복구. `owned-session-live.mjs`의 7개 검사 모두 통과.
  호스트 refresh token은 복사하지 않았고 일회성 access-token 투영은 검증 후 제거.
- 실검증 통과: Claude 대화/승인/거절/중단, manager restart, 첫 컨테이너
  재생성 후 동일 vendor 대화와 기억 복구. 두 번째 재생성의 기억 확인은
  vendor API safeguard refusal(`reasoning_extraction`)로 재현되어 **미통과**로 기록.
  vendor ID와 실패 이벤트는 보존되며, vendor의 안전 설정을 끄지 않았다.
- 실검증 통과: 생성한 기본 non-root 설정, Dockerfile+worker 사용자+hooks,
  공식 git feature, foreign 무변경 거부/명시적 recreate/내부 범위 제한 등 6개 경계 검사.
- 실검증 통과: 실제 TUI Codex attach, Ctrl-N/Tab agent 선택, manager restart
  중 cursor 재접속, Ctrl-C detach. Feature provisioning 중에도 다른 프로젝트를 조회.
- 실검증 통과: Codex 컨테이너 강제 제거 후 같은 세션/thread를 새 run으로 복구;
  manager SQLite 파일을 백업 위치로 이동 후 manifest 재구성, 기존 project run 유지.
  데이터베이스 원본은 named volume에 복구 가능한 이름으로 보존했다.
- 검증 요약: `testdata/recovery/owned-workspaces-summary.json`. 재실행 도구는
  `owned-session-live.mjs`, `owned-boundaries.mjs`, `owned-recovery.mjs`.
- 보강: 준비 확인을 socket 파일 존재가 아닌 실제 RPC로 변경, 프로젝트별
  lifecycle lock으로 다른 프로젝트의 provisioning이 세션 제어를 막지 않도록 함.
- 최종 회귀 통과: `go test ./...`, `CXZ_TEST_RACE=1 go test -race ./internal/... -count=1`,
  `go vet ./...`, protobuf 재생성 후 생성물 diff 없음, `git diff --check`.
- 중간 커밋: `bab4541` (manager/Codex), `e9fc20e` (CLI/Compose/복구 보강).
- 정리: 검증 소유 label을 가진 프로젝트 컨테이너와 manager를 제거했다.
  테스트 workspace, named volumes, DB backup과 private journal은 보존했다.
  공유 Docker 엔진의 다른 프로젝트나 이미지/볼륨을 prune하지 않았다.

현재 인계 범위: cld식 설치→devcontainer 생성→vendor 선택→TUI 접속/제어→복구의
핵심 흐름 구현. 전체 cld 편의 기능의 복제나 웹 MVP 완료를 뜻하지 않는다.
최초 vendor 로그인, 웹/IDE UI, roster 인증, 중앙 로그인 broker, 자동 release update,
dotfile/SSH forwarding, off-host backup은 미구현/사용자 단계로 명시한다.
Claude의 두 번째 반복 복구 검증은 위 vendor 응답 제한 때문에 미통과로 유지한다.

## 2026-09-11 — 로컬 TUI 기반 구현 (이전 기록)

요청 범위: payday + SQLite, TUI의 세션 생성·대화·승인·중단·재접속.
사용자 인증·브라우저 UI는 이번 구현에서 제외한다. 서버는 다른 OS 사용자가
접근할 수 없는 Unix socket만 제공한다. Claude 인증은 기존 로컬 CLI 설정을 사용한다.

- 완료: Claude stream-json 실측, 승인/질문/interrupt/resume 및 컨테이너 재생성 probe.
  증거: `testdata/protocol/claude/manifest.json`, `docs/plans/claude-code-verification.md`.
- 완료: protobuf API, payday 런타임, SQLite 세션 레지스트리/이벤트 projection,
  독립 supervisor, TUI/CLI 기본 구현.
- 통과: `TestLifecycle` — 실제 daemon/supervisor 프로세스 + deterministic agent.
  생성 멱등성, 작업공간 중복 거부, 대화, 승인/거절/질문, 중단, cursor 재생,
  daemon SIGKILL 중 출력 보존, supervisor SIGKILL 후 새 run으로 resume,
  이전 run 승인 거부, SQLite 재구성.
- 통과: 저널 torn-tail 복구 및 committed corruption 거부 단위 테스트.
- 통과: 실제 Claude Code 2.1.267 live integration 7개 경로 — 대화, 승인,
  거절, 질문, 실행 중 중단, daemon 재접속, 동일 vendor 세션 resume.
  중단된 명령의 예정 종료 시각 이후에도 파일 부작용이 없음을 확인했다.
  정제된 결과: `testdata/recovery/claude-live-summary.json`.
- 통과: 실제 터미널에서 TUI 세션 생성, 대화, F2 승인, F3 거절, 질문 JSON 응답,
  F4 중단, Ctrl+C detach 후 살아 있는 세션 확인, Ctrl+R resume,
  승인 대기 중 daemon SIGKILL 후 TUI 자동 재접속 및 승인 처리.
  키 매핑·중복 이벤트 제거·
  터미널 escape 제거는 TUI 단위 테스트로도 검증한다.
- 통과: cxz 앱 자체의 비루트 Docker 재생성 4개 검사. 승인 대기 및 실행 중
  컨테이너 소실 후 영속 volume으로 registry/journal 복구, 새 run resume,
  stale 승인 거부, 복구 후 대화. fixture agent 사용, 네트워크 없음.
  결과: `testdata/recovery/cxz-container/summary.json`. 공유 엔진의 테스트 소유
  컨테이너/volume만 label 확인 후 제거했다.
- 보강: raw와 정규화 이벤트를 단일 JSONL batch로 원자적으로 기록하고,
  중간에 끊긴 batch는 전부 제외한다. 기존 단건 형식도 읽는다.
- 보강: supervisor SIGKILL 시 shell 자식까지 종료하는 liveness guardian,
  workspace flock, manifest 디렉터리 fsync, 조회 projection 경쟁 직렬화.
- 최종 회귀 통과:
  - `go test ./...`
  - `CXZ_TEST_RACE=1 go test -race ./internal/... -count=1` (자식 바이너리도 race 빌드)
  - `go vet ./...`
  - `go run ./tools/genproto` (추적 중인 생성물과 diff 없음)
  - 실제 Claude live integration 재실행: 7개 경로, 40.17초, 모두 통과.
  - 최종 바이너리 Docker 재실행: 4개 검사 모두 통과.
    `testdata/recovery/cxz-container-final/summary.json`.
  - `git diff --check`
- 설계 결정: raw append-only 저널을 원본으로 유지하고 SQLite를 재구성 가능한
  조회 projection으로 사용한다. daemon/TUI 종료는 agent 종료와 분리한다.
- 설계 결정: 현재 개발 컨테이너 안에서 agent를 실행한다. 별도 devcontainer 자동
  provisioning과 사용자 로그인은 후속 작업이다. 컨테이너 자체가 사라진 경우
  프로세스 생존이 아니라 영속 상태에서 명시적으로 resume한다.
- 설계 결정: payday의 config/DB 및 gRPC 서버 구성을 사용한다. tenant/holder/ent
  CRUD 생성 대신 세션 전용 protobuf를 사용하며 인증 도입 시 별도 경계를 연결한다.

검증 결과와 실행 명령은 구현 단계마다 이 문서와 README에 갱신한다.

Git: 첫 기반 커밋 `2eaeda5`를 `lesomnus/cxz`의 `main`에 push했다.
HTTPS 인증은 없지만 기존 SSH 인증을 사용할 수 있어 push URL만 SSH로 설정했다.
두 번째 기능 커밋 `73b95c3`도 push 완료.
복구 보강 및 실제 검증 커밋 `df146cd`도 push 완료. 최종 검증 기록을 추가 커밋한다.

## 당시 남겨 둔 범위 (위 현재 단계에서 일부 구현됨)

사용자 인증/roster, 새 Claude 로그인 UI, 브라우저 UI, devcontainer 자동 생성,
파일·diff/IDE relay, 외부 암호화 백업, Codex adapter는 이번 TUI 수직 구현에
포함하지 않는다. 장기 아키텍처의 전체 MVP가 완료되었다는 의미는 아니다.
현재 journal은 세션 단위로 메모리에 재생하며 조회 시 파일을 스캔하므로
대용량 장기 운영에는 segment/index 및 보관 정책이 후속으로 필요하다.
