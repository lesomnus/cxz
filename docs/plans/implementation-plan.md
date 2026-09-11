# cxz 구현 계획

작성일: 2026-09-11. 상태: 이번 TUI 수직 구현 및 검증 완료; 장기 제품 계획은 후속.

> 최신 실행 범위: 사용자 지시에 따라 **payday + SQLite + TUI**의 세션 생성·대화·
> 승인·중단·재접속을 먼저 구현한다. 사용자 인증과 웹 UI는 남겨 둔다.
> 아래 P0–P6는 장기 제품 계획이며 이번 작업의 완료 조건과 같지 않다.
> 구체적인 진행/검증 결과는 [진행 문서](../progress.md), 사용법은
> [README](../../README.md)를 따른다. 현재 backend는 기존 개발 컨테이너 안의
> 로컬 프로세스이며 자동 devcontainer provisioning은 아직 포함하지 않는다.

근거 문서:

- [아키텍처](../architecture.md)
- [Codex 검증 기록](codex-verification.md)
- [Claude Code 검증 기록](claude-code-verification.md)

초기 계획 당시 본체는 미구현이었다. 후속 TUI 구현으로 payday gRPC daemon,
SQLite, 독립 supervisor와 세션 클라이언트가 추가되었다. 2026-09-11 검증에서 Claude Code의 승인·거절,
질문 응답·중단·동일 경로에서의 프로세스 재시작 후 resume를 확인했다. 재실행 스크립트는
`scripts/probes/claude-protocol.mjs`, 정제된 fixture는 `testdata/protocol/claude/`에 있다.
로그인은 별도 빈 설정 디렉터리에서 인증 URL 발급까지 확인했으며 실제 OAuth 완료는 남아 있다.
후속 Docker 검증에서는 동일 workspace 경로·UID·named volume을 사용해 대기 중, 승인 대기 중,
도구 실행 중 컨테이너를 제거·재생성한 뒤 문맥과 파일 상태 복구를 확인했다.
당시 probe는 실제 cxz supervisor나 `devcontainer up` 통합 검증은 아니었다.
이후 본체 통합 테스트 및 cxz의 Docker 재생성 결과는 진행 문서에 별도로 기록했다.
아래 순서는 사용자 지시에 따라 Claude 검증·구현 우선으로 갱신했다.

## 1. 구현 전에 정리할 결정

| 항목 | 제안 | 이유 / 확인 조건 |
|---|---|---|
| 첫 에이전트 | Claude Code, 세션별 stream-json 프로세스 | SDK의 `--permission-prompt-tool stdio` 경로로 승인·거절·질문·중단·resume 실측 완료 |
| 두 번째 에이전트 | Codex app-server, 세션별 stdio 프로세스 | 기존 검증 기록은 공통 계약의 비교 자료로 활용하고 두 번째 adapter로 실제 검증 |
| 동시 실행 | 프로젝트당 활성 agent 프로세스 1개, 과거 세션은 여러 개 보존 | 프로젝트별 병렬 실행은 허용. 동일 작업 트리 충돌과 worktree 관리는 연기 |
| 원본 저장소 | 프로젝트 volume의 supervisor journal | daemon의 SQLite는 재생 가능한 인덱스·projection |
| 재시작 의미 | daemon 재시작은 실행 유지; container/host 재시작은 기록 복구 후 명시적 resume | 재부팅 전 프로세스나 진행 중 tool 실행이 그대로 유지되지는 않음 |
| 인증 | 우선 프로젝트별 vendor 로그인 | 중앙 access-token 공급은 문서에서도 실제 동작 확인이 남아 있는 최적화 |
| 구현 구성 | Go daemon/supervisor/CLI + TypeScript SPA, 빌드 결과 go:embed | Go 지향 개발 환경과 단일 배포 바이너리 요구에 맞춤. 프런트엔드 프레임워크는 P1에서 고정 |
| 배포 | daemon도 컨테이너에서 실행, host에는 Docker와 bootstrap 진입점 | devcontainer CLI와 Node는 관리 컨테이너에 포함. host 도구 설치 의존 방지 |

Claude의 필수 경로에 문제가 나오면 SDK 구현과 재현 실험으로 원인을 먼저 확인한다.
실제 제약이 확인되기 전에는 에이전트 순서를 바꾸거나 승인 우회를 기본값으로 삼지 않는다.
브라우저에서 prompt → 승인 → 완료 → 재접속 → resume가 성립하는지가 전체 완료 기준이다.

## 2. 아키텍처에서 보완할 부분

1. **영속성과 실행 지속성을 분리한다.** daemon 장애에는 supervisor가 실행을 유지한다.
   supervisor/container/host 장애 시에는 실행을 `interrupted`로 기록하고 vendor의 resume를
   이용한다. 실패한 resume를 새 대화로 조용히 바꾸거나 이전 prompt를 자동 재전송하지 않는다.
2. **원본 이벤트와 projection의 번호를 분리한다.** 원본 한 건이 UI 이벤트 여러 건으로
   정규화될 수 있으므로 `seq = log offset`을 그대로 쓰지 않는다. 아래 journal 계약을 적용한다.
3. **공유 네트워크는 격리가 아니다.** 프로젝트별 네트워크와 인증된 supervisor 연결을 기본으로
   제안한다. Compose의 여러 서비스는 같은 프로젝트 경계에 포함하되, 다른 프로젝트에 대한
   접근과 host의 Docker API 접근을 별도로 차단·검증한다.
4. **container loopback에는 daemon이 직접 접근할 수 없다.** IDE와 개발 서버는 supervisor의
   인증된 HTTP/WebSocket relay를 통해 접근한다. 임의 host를 받는 일반 프록시 대신
   컨테이너 내부의 등록된 loopback port만 대상으로 삼는다.
5. **cxz가 만든 컨테이너라고 자동으로 격리되는 것은 아니다.** privileged, host network,
   Docker socket, 민감한 host bind mount를 호환성·신뢰 등급으로 표시한다. 기본 격리 프로필에서
   허용할 설정과 명시적으로 신뢰한 프로젝트에서 허용할 설정을 구분한다. 현재 개발용
   `.devcontainer/docker-compose.yaml`의 `privileged: true`는 제품 기본 정책의 근거가 아니다.
6. **로그인과 일반 대화 기록을 분리한다.** 토큰을 담은 인증 응답을 raw 대화 journal에 넣지
   않는다. 원본 보존은 대화 프로토콜에 적용하고 인증 채널은 결과 상태만 기록한다.
   대화 자체에도 민감한 정보가 들어갈 수 있으므로 journal은 접근 제한과 암호화 백업 대상이다.
7. **자격 증명 백업이 로그인 복구를 보장하지는 않는다.** 백업 후 refresh token이 회전하면
   백업의 토큰은 무효일 수 있다. 복원 후 재인증 경로가 필요하고 기존 실행 인스턴스와 복원본을
   동시에 활성화하지 않는다. SQLite 파일 중 무엇이 정말 재생 가능한지도 vendor별로 검증한다.
8. **브라우저 보안은 MVP부터 포함한다.** 원격 노출만 나중이다. HttpOnly는 쿠키 읽기를
   제한하지만 같은 origin의 악성 코드가 인증된 요청을 보내는 것을 막지는 않는다.
   control/IDE/개발 서버 origin 분리, CSRF 방어, WebSocket Origin 검증을 초기 계약에 넣는다.

문서 정합성 수정 대상: §10의 이미 조사된 Codex 저장 형식,
§4.2와 Appendix의 중앙 로그인 일반화, §3.1/§10의 journal 위치, §5.5의 loopback 접근 경로.
검증 기록의 오래된 '계정 필요/미검증' 표시는 뒤에 추가된 결과와 맞춰 정리한다.

## 3. MVP 완료 범위

여러 프로젝트를 등록하고 프로젝트마다 하나의 agent를 실행한다. 브라우저에서 로그인,
대화, 승인·질문 응답, 중단, 세션 목록, 파일·diff 확인, 검색, 업로드·다운로드를 사용할 수 있다.
클라이언트 재접속과 daemon 재시작 시 기록과 실행을 이어 보고, 컨테이너 재생성 후에는
대화를 resume할 수 있다. 승인 요청과 turn 완료 알림, 로컬 VS Code 열기, 암호화된 외부
백업·복원도 MVP 완료 조건이다. 브라우저가 닫힌 동안의 push 알림은 별도 기능으로 둔다.

PTY attach, 동일 프로젝트 동시 실행, 브라우저 파일 편집, 중앙 vendor 로그인,
원격 TLS/SSH 제공, 실제 IDE 호스팅은 후속 단계다. IDE origin/relay 계약은 MVP에서 마련한다.

## 4. 데이터·프로토콜 계약

### 식별자와 상태

- `ProjectID`: 경로와 분리한 영속 ID. 실제 host path, container workspace path를 별도 보관한다.
  경로 이동은 명시적인 재연결 작업으로 처리한다.
- `ContainerGeneration`: 재생성마다 변경한다. 예전 컨테이너의 인증 정보와 응답은 무효화한다.
- `SessionID`: cxz 대화 ID. vendor session/thread ID, vendor 버전, workspace를 연결한다.
- `RunID`: agent 프로세스를 시작할 때마다 발급한다. resume도 새 RunID를 사용한다.
- `TurnID`, `ItemID`: 메시지·도구 호출을 연결한다. vendor ID를 별도 필드에 보존한다.
- 실행 상태: `starting / idle / working / waiting_input / interrupted / failed / stopped`.
  연결 상태 `connected / disconnected`는 별도다. daemon과 연결이 끊겼다고 turn을 완료 처리하지 않는다.
- capability는 세션 시작 시 기록한다. 승인, 질문, 이미지, 로그인, usage, resume 지원을 구분하고
  미지원 수치는 0이 아닌 부재로 표현한다.

### Journal과 재생

supervisor가 프로젝트 volume의 `/cxz/state/sessions/<session-id>/`에 기록한다.
vendor config와 journal 경로는 분리하고 vendor config를 workspace 파일 API에 노출하지 않는다.

각 레코드는 `schema_version, session_id, run_id, record_seq, received_at,
direction, record_type, raw_bytes`를 갖는다. vendor timestamp는 별도 보존한다.
raw bytes를 변형 없이 담을 수 있는 framing을 택하고 길이·checksum으로 부분 기록을 판별한다.
프로토콜 stdout과 진단 stderr는 분리한다. 생명주기·입력 intent도 별도 레코드 종류로 기록한다.

- journal은 supervisor가 단독 append한다. tail 구독은 durable commit 이후에만 공개한다.
  commit을 묶는 경우 지연·크기 상한을 두고, 승인 intent와 dispatch 경계는 명시적으로 동기화한다.
- projection cursor는 `(projection_version, record_seq, subindex)`다. adapter 변경으로
  이벤트 개수가 바뀌어도 원본 cursor를 혼동하지 않는다.
- 브라우저 SSE ID는 projection cursor에 대응한다. `Last-Event-ID` 이후 backlog를 전송하고
  동일 cursor 기준으로 live tail에 합류시켜 구간 누락을 막는다.
- projection 버전이 다르면 reset 신호 후 재생한다. UI는 이벤트 중복을 제거하고,
  text delta와 최종 message를 ItemID로 합쳐 같은 내용을 두 번 표시하지 않는다.
- 시작 시 불완전한 마지막 레코드를 격리하고 마지막 유효 commit까지 복구한다.
  디스크 부족이면 새 입력을 거절하고 실행 중 프로세스를 중단하는 정책을 정의한다.
- 메모리 queue, 레코드 크기, stderr 크기, 검색 결과에 상한을 둔다. 느린 클라이언트는
  연결을 끊고 cursor로 재연결하게 하며 supervisor의 기록을 막지 않는다.

### 입력과 승인

입력은 `client_input_id, session_id, run_id, turn_id, request_id, payload`로 식별한다.
supervisor가 프로젝트 활성 실행 잠금과 요청 상태의 최종 권한을 가진다.

1. 대상 generation/run과 capability, payload schema를 검사한다.
2. 승인 요청은 pending 상태인지 확인하고 원자적으로 claimed 처리한다.
3. intent를 durable 기록한 뒤 vendor에 한 번 전달한다. 정상 경로에서 중복 클릭은 동일 결과를 돌려준다.
4. 전달 도중 장애가 나서 결과를 알 수 없으면 `delivery_unknown`으로 남긴다. vendor의 idempotency가
   확인되지 않은 요청을 자동 재전송하지 않는다. vendor 상태 조회나 사용자 확인으로 해결한다.

서로 다른 승인 메서드의 decision enum을 하나로 합치지 않는다. adapter가 요청별 허용 응답을
UI에 제공하고 다시 검증한다. 과거 RunID의 pending 요청은 resume 후 재사용하지 않는다.
질문 응답·거절·취소·interrupt를 구분하고, 지원하지 않는 필수 vendor request는 명시적인
프로토콜 오류로 처리하여 무한 대기를 피한다. 로그인 비밀은 이 일반 journal 경로를 우회한다.

## 5. 코드 경계와 API 초안

예상 구조:

```text
cmd/cxz/                    CLI, daemon/supervisor 진입점
internal/domain/            ID, 상태, capability, 입력 계약
internal/project/           등록, 경로 매핑, 활성 실행 잠금
internal/container/         devcontainer 실행, 소유권, reconcile
internal/supervisor/        프로세스 소유, 입력, journal 서비스
internal/journal/           append, commit, 복구, cursor
internal/agent/             최소 adapter 계약
internal/agent/claude/       stream-json, 승인 control, 정규화, resume
internal/agent/codex/        두 번째 구현 단계에서 JSON-RPC adapter 추가
internal/release/            버전 고정, checksum, 패키지 설치
internal/auth/              사용자 로그인, supervisor 인증
internal/projection/        상태·이벤트 인덱스, 재생
internal/httpapi/           REST, SSE, origin 경계
internal/workspace/         파일, 검색, upload, diff
internal/relay/             인증된 container loopback relay
internal/backup/            allowlist, 암호화, snapshot/restore
web/                        SPA와 embedded build
testdata/protocol/           비밀 제거한 vendor fixture
tests/integration/          Docker 기반 생명주기·복구 시험
```

처음부터 모든 optional interface를 구현하지 않는다. 첫 실제 adapter에서 필요한 launch,
decode/project, request encode, resume, capability 계약만 추출하고 두 번째 adapter로 검증한다.

| 브라우저/CLI API 초안 | 의미 |
|---|---|
| `POST /api/projects`, `GET /api/projects` | 등록·목록 |
| `POST /api/projects/{id}/up` | 멱등적인 provisioning job 시작 |
| `GET /api/operations/{id}` | 긴 작업의 진행·실패 원인 |
| `POST /api/projects/{id}/sessions`, `GET /api/sessions` | 세션 생성·목록 |
| `GET /api/sessions/{id}/events` | cursor 기반 SSE |
| `POST /api/sessions/{id}/inputs` | prompt·요청 응답 |
| `POST /api/sessions/{id}/interrupt` | 진행 중 turn 중단 |
| `POST /api/sessions/{id}/resume` | 중단된 대화의 새 run 시작 |
| `/api/projects/{id}/files`, `/search`, `/diff`, `/uploads` | workspace 전용 API |
| `/api/projects/{id}/agent-login` | 비밀이 journal에 남지 않는 로그인 흐름 |

브라우저 API, CLI 관리 API, supervisor API의 권한을 구분한다. foreign 컨테이너 recreate는
정확한 대상과 손실 범위를 보여주는 별도 작업이며 일반 `up`이 자동으로 삭제하지 않는다.
컨테이너 내부 요청이 제공한 project ID로 권한을 결정하지 않는다.

## 6. 단계별 작업과 완료 조건

### P0 — 구현을 막는 가정 검증

산출물: 버전 manifest, 재실행 가능한 probe, 비밀을 제거한 fixture, 결정 기록.

- Claude 프로토콜 fixture와 실행 스크립트를 확보했다. 테스트는 scratch 작업 경로에서 실행하고,
  기존 credential store를 복제 없이 사용했다. 설정 파일은 바꾸지 않고 실행 옵션으로 설정·hook·MCP를
  제한했다. 로그인 URL 검증만 credential이 없는 별도 config를 사용했다.
- Claude 승인 등록 조건, 3.5초 승인 대기, 허용·거절, 질문 응답, 여러 turn, 실행 중 interrupt,
  프로세스 종료 후 같은 session ID와 문맥 복구를 확인했다. `aborted_tools`와 실제 실패를 구분한다.
- Docker 직접 검증에서 완료된 turn, 승인 대기, 도구 실행 중 컨테이너 재생성 후 같은 세션·문맥을
  복구했다. 부분 파일 변경은 보존됐고 이전 명령은 재전송하지 않았다. 예전 승인 응답은 새 요청을
  허용하지 않았다. `scripts/probes/claude-container-recreate.mjs`와 fixture에 결과를 기록했다.
- 남은 Claude 항목: 실제 OAuth 완료, 실제 cxz/devcontainer를 통한 재생성, 경로·UID 변경 후 resume,
  승인 대기 중 interrupt와 duplicate 응답, 이미지 입력. 미검증과 미지원은 구분해 기록한다.
- Codex는 기존 검증 결과로 공통 계약을 검토하고 두 번째 adapter 단계에서 재검증한다.
- `devcontainer up`에 mount/network/supervisor를 주입하는 방법을 image/Dockerfile/Compose
  세 형태에서 검증한다. 원본 설정은 수정하지 않고 생성된 overlay/지원 CLI 옵션을 사용한다.
- 관리 컨테이너가 Docker host에 있는 workspace를 같은 경로로 식별·mount할 수 있게
  host/container path 매핑을 검증한다. supervisor 시작 hook은 기존 lifecycle hook과 공존해야 한다.
- non-root UID, volume 소유권, 최초 시작·재시작, 바이너리 아키텍처 선택을 확인한다.
  직접 Docker 실험은 UID/GID 1000과 `volume-nocopy`를 사용했고, 각 재생성 후 소유권·쓰기 가능
  여부를 확인했다. 첫 시도에서 승인된 Bash 쓰기가 파일 권한으로 실패했으므로 이를 preflight로 둔다.

완료 조건: Claude 필수 경로와 주입 방법이 실제 재현 절차로 입증되고, 승인·로그인·resume가
첫 vendor의 release blocker로 관리된다. 미검증 중앙 로그인은 이 단계 통과를 막지 않는다.

### P1 — Supervisor와 영속 대화의 최소 수직 구현

P0에 의존한다. Go 모듈·빌드·fixture runner와 supervisor/journal/첫 adapter를 구현한다.
먼저 가짜 agent로 장애를 재현하고 실제 고정 버전 agent로 같은 흐름을 실행한다.

완료 조건: CLI 테스트 클라이언트로 prompt → 승인 → 완료 → 두 번째 turn이 작동한다.
클라이언트를 종료해도 agent는 살아 있고, 재접속하면 누락 없이 기록을 읽는다.
부분 기록, 중복 응답, 오래된 request ID, 프로세스 사망, dispatch 중 장애를 시험한다.

### P2 — 프로젝트 컨테이너와 daemon 연결

P1에 의존한다. `cxz up`, 프로젝트 등록, 이름 volume, release cache, 컨테이너 소유 label,
프로젝트별 네트워크, supervisor 인증과 SQLite projection을 연결한다.
devcontainer CLI는 관리 컨테이너에 고정 버전으로 설치하고 Docker socket은 관리 계층만 접근한다.
supervisor는 daemon의 `docker exec` 연결 수명과 무관하게 컨테이너 생명주기로 시작한다.

Docker 이벤트는 변화 알림으로만 쓰고, 시작 시 및 연결 복구 시 실제 컨테이너 목록으로 reconcile한다.
provisioning을 단계별 job으로 기록해 실패 후 재시도 시 volume/container가 중복 생성되지 않게 한다.
foreign container는 감지·표시하고, 명시적인 recreate 작업에서 workspace/volume 보존을 확인한다.

완료 조건: 같은 프로젝트에 동시에 `up`해도 하나만 만들어진다. daemon을 강제 종료해도
agent PID와 turn이 유지된다. daemon 재기동 후 projection이 따라잡는다. container recreate 후
기록·인증 상태가 남고 명시적으로 resume할 수 있다. 타 프로젝트 요청과 오래된 인증을 거절한다.

### P3 — 브라우저에서 대화 운영

P2에 의존한다. 사용자 로그인, 세션 목록, 대화·tool·usage 표시, 승인·질문, interrupt,
vendor 로그인, SSE 재연결, turn 완료/승인 알림, VS Code deep link를 구현한다.
브라우저는 opaque vendor 이벤트도 확인할 수 있게 하고 capability로 UI를 제어한다.

인증은 loopback에서도 적용한다. control origin의 상태 변경 요청은 CSRF 검증을 수행하고
cookie를 다른 origin에 전달하지 않는다. Secure cookie 사용과 local HTTPS 제공 여부는
지원 브라우저에서 확인해 개발 HTTP 예외가 원격 설정으로 전파되지 않게 한다.

완료 조건: 두 브라우저가 동시에 같은 승인을 답해도 한 번만 전달된다. 재연결 후 메시지와
알림이 중복되지 않는다. 서버 재생 이벤트가 오래된 알림을 다시 울리지 않는다.
알림 권한 거부, 잘못된 Origin, 만료된 로그인, 연결 단절을 처리한다.

### P4 — 파일·diff와 IDE 연결 기반

P3에 의존한다. supervisor에 tree/read/search/upload/download를 추가하고 Monaco viewer/diff를 붙인다.
파일 접근은 root 아래에서 원자적으로 열어 검사 시점 이후 symlink 교체도 막는다.
검색·git 결과에 포함된 경로도 재검증하고 command argv에 사용자를 shell 문자열로 삽입하지 않는다.

working-tree 대 HEAD diff를 기본으로 제공한다. turn별 diff는 vendor가 제공한 변경 기록과
전체 workspace diff를 구분한다. 외부 편집까지 agent 변경으로 귀속시키지 않는다. 새 파일,
binary, 삭제, rename, 큰 파일, Git 저장소가 아닌 workspace를 각각 처리한다.
업로드는 크기 제한과 원자적 완료를 적용하고 기본 저장 위치는 staging으로 둔다.

IDE와 port proxy는 별도 origin 및 프로젝트별 인증을 갖는 relay smoke test까지 만든다.
실제 IDE 설치는 후속 단계이며, HTTP 스트림과 WebSocket upgrade가 컨테이너 loopback까지
도달하는지, control cookie 없이 인증되는지 먼저 검증한다.

완료 조건: `../`, 절대 경로, symlink 교체로 workspace 밖 파일을 읽거나 쓰지 못한다.
파일 검색·업로드·diff가 실제 프로젝트에서 작동한다. 다른 프로젝트의 relay 접근을 거절한다.

### P5 — 외부 백업·복원과 MVP 배포

P4에 의존한다. 대상 외부 저장소와 암호화 도구/키 보관 방식은 구현 착수 시 운영 설정으로 정한다.
백업 키는 백업 대상 host와 별도로 복구 가능해야 한다. P1부터 정의한 BackupPolicy allowlist를
vendor별로 확정하고 설정, 프로젝트 registry, journal, 필요한 vendor 상태와 credentials를 보존한다.

실행 중 journal은 committed high-water mark까지만 복사한다. vendor DB는 재생이 입증된
projection만 제외하고, 나머지는 일관된 vendor/SQLite snapshot 또는 정지 구간으로 보존한다.
작업 트리의 미커밋·미추적 파일은 git만으로 복구되지 않으므로 별도 백업 포함 여부를 설정하고
기본 bootstrap이 복원할 수 있는 범위를 UI와 운영 문서에 명시한다.

완료 조건: 비어 있는 별도 환경에서 bootstrap → 복원 → 프로젝트 재생성 → 대화 조회 → resume를
실행한다. 토큰 만료 시 브라우저 재로그인이 가능하다. checksum 오류·복호화 실패가 기존 상태를
덮어쓰지 않는다. 실제 외부 백업과 복원 리허설이 끝나기 전에는 MVP 완료로 표시하지 않는다.

### P6 이후 — 두 번째 vendor와 확장

1. Codex adapter를 추가해 공통 계약을 검증한다.
   첫 adapter와 동일한 장애·재생 테스트를 실행하고 vendor별 capability 차이를 유지한다.
2. PTY transport와 `cxz attach`를 구현한다. 실행 중인 headless agent를 같은 vendor TUI로
   그대로 attach할 수 있다고 가정하지 않는다. transport 선택은 세션 생성 계약에 명시한다.
3. 원격 TLS와 세션 관리, SSH attach를 추가한다.
4. 개발 서버 port proxy와 터미널 대시보드를 제공한다.
5. hosted VS Code를 on-demand로 시작하고 idle 시 중지한다. 버전 pin, server 사전 다운로드,
   extension/settings 반영, marketplace 동작, 네트워크 없는 시작을 실제 검증한 뒤 제공한다.

## 7. 첫 작업 묶음과 우선순위

첫 구현 작업은 P0 전체와 P1의 작은 수직 경로다. UI부터 만들지 않고 다음 산출물을 먼저 만든다.

1. 첫 vendor와 devcontainer 주입 방식의 검증 fixture 및 버전 manifest.
2. ID·상태·journal·입력 중복 처리 계약과 가짜 agent.
3. supervisor가 실제 agent를 실행하고 승인 요청 하나를 왕복하는 테스트 클라이언트.
4. 클라이언트 재접속과 supervisor crash 복구 시험.

임계 경로는 `승인/로그인/resume 검증 → supervisor/journal → 컨테이너 생명주기 →
브라우저 운영 → 파일/relay → 외부 복원 리허설`이다. 예상 일정은 P0에서 vendor 인증과
devcontainer 주입 난이도를 확인한 뒤 산정한다. 각 단계의 완료 조건을 통과하기 전에는
후속 기능이 앞 단계의 미검증 가정을 사실로 사용하지 않는다.
