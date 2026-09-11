# 구현 진행 상황

## 2026-09-11 — TUI 우선 구현

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
- 진행: 실제 Claude live integration, TUI 검증, race/vet, 장애 경계 보강.
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
