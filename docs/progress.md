# 구현 진행 상황

## 2026-09-11 — TUI 우선 구현 완료

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

## 남겨 둔 범위

사용자 인증/roster, 새 Claude 로그인 UI, 브라우저 UI, devcontainer 자동 생성,
파일·diff/IDE relay, 외부 암호화 백업, Codex adapter는 이번 TUI 수직 구현에
포함하지 않는다. 장기 아키텍처의 전체 MVP가 완료되었다는 의미는 아니다.
현재 journal은 세션 단위로 메모리에 재생하며 조회 시 파일을 스캔하므로
대용량 장기 운영에는 segment/index 및 보관 정책이 후속으로 필요하다.
