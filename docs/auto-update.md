# cxz 자동 업데이트

cxz는 **main의 최신 CI 성공 빌드(edge)**를 확인한다. 수정되지 않은 main 계열 빌드에서
기본 활성화되며, 확인 주기는 24시간이다. 서버 적용 큐는 30초마다 안전 여부를 확인한다.
개발/수정 빌드와 게시된 main의 조상이 아닌 빌드는 자동 교체하지 않는다.

## 설정과 상태

```sh
cxz self-update status
cxz self-update check
cxz self-update disable
cxz self-update enable

# 로컬 설치의 Manager 정책/상태 (Linux 호스트에서 실행)
cxz self-update status --server
cxz self-update check --server
cxz self-update disable --server
cxz self-update enable --server
```

이 명령들은 JSON을 출력하고 종료한다. `check`는 최신 manifest를 확인하며 직접 재시작하지
않는다. `disable`은 다음 자동 적용을 보류하며 이미 시작된 복구 transaction을 취소하지 않는다.
기존 `cxz self-update [--ref ...] [--client-only]` 수동 명령도 유지된다.

클라이언트와 서버 설정은 각각의 상태 디렉터리 `cxz-update-policy.json`에 저장한다.
원격 TUI를 연결하는 것만으로 서버 정책은 바뀌지 않는다. TUI의 Settings에서 **frontend**
정책을 토글하고 실행/목표 revision, 대기 이유를 볼 수 있다. 서버 정책은 설치 호스트의
CLI로 관리한다. 기존 `CXZ_AUTO_UPDATE=0`은 Claude/Codex/gh 업데이트 설정이며 cxz 자체
업데이트 설정과 별개다.

`cxz-update.json`에는 확인 시각, 목표 manifest, 적용 상태와 실패 이유를 저장한다.
`running`과 `update.release`는 각각 실행 빌드와 준비된 목표다. 목표 파일이 다운로드됐다는
이유만으로 실행 중인 모든 프로세스가 최신이라고 표시하지 않는다. 실패한 revision은
24시간 동안 재시도를 보류한다.

## 무엇이 재시작되는가

| 구성요소 | 적용 시점 | 보존 대상 |
| --- | --- | --- |
| Manager | 설치의 실행 세션이 모두 안전한 idle일 때, 독립 helper 컨테이너에서 교체 | 프로젝트 컨테이너, 상태 볼륨, Supervisor/Agent |
| Project runtime | 프로젝트 세션이 모두 안전할 때, 같은 컨테이너 안의 독립 helper에서 교체 | 컨테이너 ID, Supervisor/Agent, Session/Run ID |
| Supervisor/Guard/Agent | 해당 세션이 연속 5분 안전한 idle일 때 | Session ID, 계정·인증 binding, provider 대화, 모델·권한, journal (Run ID는 변경) |
| TUI | 5분간 입력이 없고 초안·모달·첨부·진행 중 요청·터미널 등이 없을 때 | 서버의 작업과 대화; 연결·프로젝트·세션·journal 화면 위치 복원 |
| 공유 tools의 cxz | Manager 검증 후 원자적 파일 교체 | 이미 실행 중인 프로세스는 이전 image로 계속 실행 |

Supervisor는 Agent의 소유자이므로 Supervisor 교체 시 Agent도 재시작한다. 작업 중,
도구/백그라운드 작업/자식 프로세스가 남아 있거나 승인·질문·설정 변경·클라이언트 입력이
진행 중이면 기다린다. 활동을 판정할 수 없는 경우도 대기한다. 중지된 세션을 업데이트
때문에 시작하지 않으며 다음 명시적인 resume에서 새 Supervisor를 사용한다.

초기 구현은 역할별 소스 hash 대신 전체 revision을 기준으로 idle 교체를 예약한다.
cxz만 갱신하면 기존 Agent 실행 파일을 유지한다. provider 업데이트도 준비되어 있으면
같은 transaction에서 검증된 Supervisor/Agent 조합으로 한 번만 재시작한다.

TUI는 Agent가 작업 중이어도 재시작할 수 있다. 세션 종료 RPC를 호출하지 않는다.
미전송 입력이나 비밀값은 재시작 파일에 저장하지 않는다. Unix/Windows 모두 터미널을
정리한 뒤 launcher가 새 frontend를 실행하고 시작 확인을 받는다. 시작 전에 실패하면
이전 파일을 복구한다. 시작 확인 뒤 사용자가 입력한 요청은 자동 재전송하지 않는다.
여러 TUI가 같은 실행 파일을 사용하면 파일 교체와 시작 확인을 잠금 아래 진행하고,
이미 설치된 동일 파일의 backup을 덮어쓰지 않는다. Windows에서는 실행 중인 이전 launcher의
파일을 삭제하지 않도록 자동 교체마다 고유한 backup 경로를 사용한다.

## 교체·복구 구조

CI는 테스트 성공 후 네 플랫폼의 실행 파일, digest로 고정한 Manager 이미지와
`cxz-update.json` manifest를 게시한다. manifest를 **마지막에** 게시한다. 실행 파일은
revision별 이름을 사용하고 SHA-256 및 `_build-info`로 플랫폼/revision/protocol/schema를
검증한다. checksum은 배포 경로의 무결성 검사이며 별도 서명 검증은 아니다.

프로토콜과 상태 스키마가 현재 지원하는 버전과 정확히 같을 때만 자동 적용한다.
게시 순번과 main ancestry를 모두 확인하므로 더 오래된 edge가 로컬의 새 빌드를
덮어쓰지 않는다. main ancestry 확인 범위는 최근 4096개 commit이다.

서버 artifact 경로는 `/cxz/tools/cxz-builds/releases/<revision>/<platform>/cxz`다.
각 Runtime은 실제 `/proc/self/exe`를 자신의 상태 디렉터리 `runtime-binaries`에 보존한다.
실행 중인 바이너리와 rollback 후보는 자동 정리하지 않는다.

Manager helper는 `cxz-<owner prefix>-update` 컨테이너에서 실행하며 설치 상태 볼륨의
`manager-update.json`과 `rollout.lock`을 사용한다. 기존 컨테이너의 설정·mount·환경·network를
보존하고 Manager만 교체한다. 기존 Manager 컨테이너는 중지 상태로 보존한다.
수동 설치도 같은 helper 이름을 예약하므로 자동 교체와 충돌하지 않는다.

Runtime helper는 `runtime-update.json`에 이전/목표 빌드, PID와 시작 시각을 저장한다.
새 프로세스는 Runtime 초기화 **전에** 자신의 PID를 기록한다. PID 재사용 여부를 확인하고
대상 Runtime만 종료한다. 새 서버에서 Session/Run ID를 대조한 뒤 입력 접수를 재개한다.
Supervisor 교체는 기존 `agent-update.json` transaction과 입력 잠금을 재사용한다.

교체 중에는 private maintenance socket의 lease로 신규 변경 RPC를 차단하고 진행 중인
변경 요청을 마무리한다. 조회·watch는 가능한 동안 유지된다. 클라이언트는 거절된 입력이나
응답이 불명확한 전송을 자동 재전송하지 않는다. helper가 중단되면 저장된 단계와 실제
컨테이너/프로세스 신원을 대조한다. lease가 만료되어 새 작업이 들어왔을 가능성이 있거나
신원이 바뀐 경우 자동 종료·rollback을 하지 않고 복구가 필요하다고 표시한다.

## 최초 도입과 지원 범위

이 기능이 없는 구버전은 안전 상태·교체 protocol을 보고할 수 없다. 최초에는 수동으로
클라이언트와 Manager를 갱신하고, 각 프로젝트/세션은 작업이 끝난 후 새 runtime으로
시작해야 한다. `bootstrap required`를 표시한 대상을 자동 강제 종료하지 않는다.
프로젝트를 recreate하는 수동 절차는 writable layer가 없어질 수 있으므로 기존 설치
문서의 주의사항을 따른다. 자동 업데이트 자체는 프로젝트를 recreate하지 않는다.

서버 자동 교체는 Linux의 기본 named-volume 설치와 Manager 내부의 Unix Docker socket을
대상으로 한다. Windows는 remote frontend 자동 갱신을 지원한다. 쓰기 권한이 없는 실행
파일, 임의의 개발용 foreground 서버, 비호환 schema migration, Docker/프로젝트 OS 이미지
교체는 자동 적용하지 않는다. 네트워크 오류는 기존 프로세스를 유지한 채 상태에 남긴다.

Manager 교체 중 token broker, Runtime 교체 중 입력 전달에 짧은 중단이 있다. 초기 버전은
해당 범위의 작업이 모두 안전할 때까지 기다리는 방식으로 보호한다. 구버전 바이너리와
중지된 이전 Manager 컨테이너는 자동 GC하지 않는다.

## 검증

```sh
TMPDIR=/tmp go test ./...
TMPDIR=/tmp go test -race ./internal/cxzupdate ./internal/server ./internal/supervisor ./internal/workspace ./internal/tui ./cmd/cxz
TMPDIR=/tmp CXZ_TEST_AUTO_UPDATE_DOCKER=1 go test ./internal/cxzupdate -run TestCloneManagerDocker -count=1
```

테스트는 다운로드 검증, idle/입력 경쟁, lease의 재시작·만료·손상, Runtime 초기화 전 실패와
이전 빌드 복구, helper 중단 지점 재개, Manager 생성 실패 rollback, 부분적으로 저장된
프로젝트 목록 재탐색, frontend 시작 실패와 중복 파일 교체를 검사한다. Docker 테스트는
별도 이름/볼륨/network를 만들고 기존 프로젝트 컨테이너를 교체하지 않는지 확인한다.
