# 구현 진행 상황

## 2026-09-11 — 실사용 마무리 (진행 중)

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
