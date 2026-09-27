# cxz 자동 업데이트·재시작 구현 계획

상태: 초기 구현의 설계 기준. 실제 설정·지원 범위·복구 동작은 [운영 문서](../auto-update.md)를 따른다.
기본 채널은 **main의 최신 CI 성공 빌드(edge)**다. 사용자가 선택한 정책이다.
Manager는 호스트 측 관리 서버, Project runtime은 프로젝트 컨테이너 안의 서버를 뜻한다.

## 목표와 보존 조건

실행 중인 cxz가 새 빌드를 발견·검증·준비하고, 각 구성요소를 적절한 시점에 교체한다.
작업 중인 세션은 이전 버전으로 계속 실행하고, Supervisor 업데이트는 안전한 idle까지 기다린다.

- 프로젝트 컨테이너, workspace, 상태 볼륨은 자동 업데이트로 재생성·삭제하지 않는다.
- Manager·Project runtime 교체로 Supervisor·Guard·Agent를 종료하지 않는다.
- Supervisor 교체는 Agent도 재시작한다. Session ID, Account/AuthBinding, provider 대화,
  모델·effort·권한 설정과 journal을 유지하고 Run ID는 새로 발급한다.
- 응답이 유실된 Send/Reply를 자동 재전송하지 않는다. 기존 요청 ID와 저장 결과로 확인한다.
- 안전 여부, 실행 버전, 프로토콜 호환성을 알 수 없으면 대기한다. 시간이 지났다는 이유로 강제 적용하지 않는다.
- 업데이트 준비 실패는 기존 실행에 영향을 주지 않는다. 중지 후 실패는 이전 버전으로 복구한다.
- 프로세스 생존과 API 무중단은 별개다. Manager 재시작 동안 토큰 공급, Runtime 재시작 동안
  입력·승인 전달이 잠시 막힐 수 있다. 초기 구현은 해당 범위의 세션이 모두 안전한 상태일 때 제어 서버를 교체한다.

## 현재 구현에서 재사용할 것

| 위치 | 현재 기능 | 확장할 내용 |
| --- | --- | --- |
| `internal/selfupdate`, `cmd/cxz/self_update.go` | 후보 검증, 실행 파일 교체, 이전 파일 보존. Linux는 소스 빌드, Windows는 릴리스 다운로드 | Linux 릴리스 다운로드, 백그라운드 확인, TUI 재실행 |
| `internal/installer/install.go` | Manager 교체, 공유 tools 바이너리 원자적 교체 | 독립 helper에서 실행 가능한 설치 교체·복구 절차 |
| `internal/workspace/updates.go` | Manager 소유 업데이트 큐, 24시간 확인, 30초 간격 적용, 한 번에 한 Agent 재시작 | cxz 구성요소 작업 및 기존 provider 작업의 충돌 방지 |
| `internal/supervisor/update.go` | 연속 5분 idle, 활동 heartbeat, 도구·자식 프로세스·질문 검사 | Supervisor 버전 보고, cxz 교체에도 동일한 판정 적용 |
| `internal/server/update.go` | 저장된 업데이트 transaction, 중지 직전 재검증, resume·rollback | Supervisor 실행 파일 선택과 복구, provider 업데이트와 통합 |

기존 Claude/Codex/gh 자동 업데이트는 cxz 자체 업데이트와 다른 기능이다.
두 기능은 설정과 상태를 구분하되 동일 세션의 중지·재시작은 공통 조정 절차를 사용한다.

## 1. 빌드 식별과 배포 산출물

- CI 성공 후 클라이언트 바이너리, Linux runtime 바이너리, Manager 이미지와 manifest를 게시한다.
- manifest에는 commit SHA, CI 실행 순번, OS/아키텍처, artifact digest,
  Manager image digest, 프로토콜 지원 범위와 상태 스키마 호환 범위를 포함한다.
- `edge`는 이동하는 이름이다. 확인 시 manifest를 고정하고 해당 transaction은 끝까지 같은
  SHA·digest를 사용한다. 다운로드 중 edge가 바뀌면 검증 실패로 다음 확인까지 보류한다.
- 버전 비교는 `source-<sha>` 문자열의 대소 비교가 아니다. 같은 채널의 게시 순번과
  정확한 revision/digest로 새 빌드를 판별한다. 자동으로 과거 빌드를 선택하지 않는다.
- Linux도 게시된 산출물을 다운로드한다. 주기적인 소스 빌드는 하지 않으며 수동 소스 빌드는 유지한다.
- 후보의 checksum·플랫폼·빌드 정보·실행 가능 여부를 먼저 검사한다. API/DB 초기화가 필요한
  검사는 실제 상태를 변경하지 않는 별도 검사 모드나 격리된 임시 상태에서 수행한다.
- 자동 업데이트는 기존 배포 채널의 신뢰 경계를 따른다. checksum 검증을 독립적인 서명 검증으로 표현하지 않는다.

처음에는 단일 `cxz` 바이너리를 유지한다. 별도 Supervisor 바이너리 분리는 선행 조건이 아니다.
실행 경로는 `/cxz/tools/cxz-builds/releases/<revision>/<platform>/cxz`처럼 불변으로 만들고,
각 프로세스의 **실제 실행 빌드**와 다음 시작에 사용할 **목표 빌드**를 따로 저장한다.
`os.Executable()`만으로 다음 Supervisor를 선택하는 현재 launch 경로를 명시적 선택으로 바꾼다.
Guard도 해당 Supervisor와 같은 빌드에서 실행한다.

Supervisor 코드가 같은 배포에서는 불필요한 재시작을 줄일 수 있도록 CI가 역할별 implementation ID를
생성한다. 대상 소스·전이 의존성·빌드 조건을 포함하고, 변경을 판단할 수 없으면 달라진 것으로 처리한다.
이 ID는 프로토콜 호환 버전과 별개다. 초기 단계에서 정확한 역할별 판별이 준비되지 않으면
전체 revision 변경을 기준으로 안전한 idle 재시작을 예약하고, 이를 상태에 명시한다.

## 2. 업데이트 조정과 영속 상태

- 서버 측 설치마다 Manager 하나가 확인·다운로드·적용 큐를 관리한다. TUI 접속 수만큼 작업을 만들지 않는다.
- 클라이언트 바이너리는 각 클라이언트 머신이 관리한다. 원격 접속만으로 서버의 자동 업데이트
  정책을 켜거나 변경하지 않는다. 서버 정책은 해당 설치에 저장한다.
- 기본 정책 제안: 활성화, edge 채널, 시작 시 마지막 확인 시각 검사 및 이후 24시간 간격 확인,
  30초 간격 적용 검사, 세션 idle 5분, 실패 빌드 재시도 24시간 보류.
- 구성요소별 일시 보류·자동 적용 끄기를 제공한다. 기존 `CXZ_AUTO_UPDATE`의 provider 업데이트
  의미는 그대로 유지하고 cxz 설정과 구분한다.
- 작업 상태는 `discovered → downloading → staged → waiting → applying → healthy`로 관리하고,
  실패 시 `rolling_back → rolled_back` 또는 `failed`로 기록한다.
- 설치·구성요소·세션 ID, 이전/목표 빌드, Run ID, 적용 단계, 대기 이유, 실패 시각,
  복구 담당자의 식별자를 원자적으로 저장한다. 외부 부작용 전에 intent를 기록한다.
- provider 자동 업데이트, 수동 self-update, cxz 자동 업데이트가 같은 대상을 동시에 교체하지
  못하도록 공통 transaction 잠금과 상태 재확인을 둔다. Manager의 메모리 mutex만으로 해결하지 않는다.
- 살아 있는 프로세스와 복구 transaction이 참조하는 이전 artifact는 정리하지 않는다.

## 3. 구성요소별 적용 방식

### Manager

Manager 자신의 자식 프로세스만으로 컨테이너를 교체하면 그 담당자도 같이 종료될 수 있다.
교체·health check·rollback은 **별도 helper 컨테이너**에서 수행한다.

1. 이미지를 digest로 준비하고 상태 스키마 및 구버전 Runtime/Supervisor 호환성을 검사한다.
2. 설치 내 실행 세션이 안전한 상태인지 확인하고 신규 실행·변경 요청 접수를 잠시 막는다.
   장시간 작업 중인 세션이 하나라도 있으면 초기 구현은 Manager 적용을 보류한다.
3. 설치별 교체 잠금을 helper에 인계한다. 기존 provider 업데이트도 중복 실행되지 않게 한다.
4. 소유권을 검증한 Manager 컨테이너만 교체한다. 프로젝트 컨테이너·공유 Docker 엔진은 유지한다.
5. API 준비, 설치 ID, 프로젝트 연결, 중앙 token broker 준비 상태를 확인한다.
6. 실패 시 이전 이미지·설정으로 Manager를 복구하고 기존 프로젝트에 다시 연결한다.

helper는 설치 상태 볼륨에 transaction을 남긴다. helper 자체가 실패한 경우 다음 실행에서
실제 Docker 상태와 프로세스 식별을 대조해 복구한다. 동일 설치의 교체 helper를 중복 실행하지 않는다.
DB migration은 이전 Manager가 다시 읽을 수 있는 범위만 자동 적용한다. 비호환 migration은
자동 적용 대상에서 제외하고 별도 계획이 필요하다고 표시한다.

### Project runtime

1. 해당 프로젝트의 안전 상태와 진행 중인 세션 생성·로그인·업데이트 RPC를 확인한다.
2. 신규 변경 요청을 막고 진행 중인 요청을 마무리한다. 실제 중지 직전 상태를 다시 확인한다.
3. 같은 컨테이너 안에서 교체 helper를 독립 프로세스로 실행한다. Runtime의 종료와 함께
   helper가 종료되지 않도록 실행 세션·stdio·취소 context를 분리한다.
4. PID와 시작 시각을 검증한 기존 Runtime만 종료한다. 이름 패턴으로 `cxz` 프로세스들을 종료하지 않는다.
5. 기존 상태 디렉터리, 소켓, 인증 식별자를 유지한 채 목표 바이너리의 `_project`를 실행한다.
6. 새 Runtime이 기존 Supervisor와 journal을 정상 조회하는지 확인한다. Supervisor·Agent PID와
   Run ID가 그대로인지 검증한 뒤 변경 요청 접수를 재개한다. 실패 시 이전 Runtime으로 복구한다.

`project recreate`나 devcontainer lifecycle hook은 실행하지 않는다.
이전 Runtime의 종료 처리가 새 소켓을 삭제하지 않도록 완전 종료·잠금 해제 후 새 서버를 시작한다.

### Session supervisor·Guard·Agent

- 적용 대상은 실행 빌드가 뒤처졌고 안전 조건을 만족하는 세션이다. 중지된 세션을 업데이트
  때문에 시작하지 않는다. 다음 명시적 시작에 새 빌드를 사용한다.
- idle 판정은 기존 `updateReason`을 사용한다. 작업·도구·백그라운드 프로세스·승인·질문·설정 변경·
  TUI 초안/입력 활동이 있거나 상태가 불명확하면 보류한다. 클라이언트 lease 만료 후에는 새 idle 구간을 기다린다.
- 같은 잠금 안에서 안전 조건과 Run ID를 재검증하고 입력 접수를 막은 후 `update-stop`을 수행한다.
- 기존 Supervisor·Guard·Agent가 완전히 종료하고 세션/profile 잠금을 반환한 뒤 새 빌드를 시작한다.
- cxz만 바뀌는 경우 Agent 실행 파일 버전은 유지한다. provider 업데이트도 대기 중이면
  검증된 Supervisor/Agent 조합으로 한 번만 재시작하도록 목표를 합친다.
- provider 대화를 resume하고 초기화·상태 응답을 확인한다. 성공하기 전 사용자 입력을 받지 않는다.
- 실패하면 이전 Supervisor·Agent 조합으로 복구한다. 이미 새 Run이 입력을 받은 정황이 있으면
  자동으로 죽이지 않고 실제 상태를 조회해 판정한다.
- 설치 전체에서 세션 재시작은 한 번에 하나만 진행한다. Manager/Runtime 교체와도 겹치지 않는다.

### Client·TUI

- 다운로드·후보 검증·실행 파일 교체와 현재 TUI 프로세스의 재시작을 분리한다.
- TUI가 입력 중이거나 초안·붙여넣기/첨부·비밀 입력·로그인·승인 모달·열린 터미널 PTY가 있으면
  자동 재시작을 보류한다. 자동 재시작을 위해 비밀 값이나 미전송 입력을 디스크에 쓰지 않는다.
- 초기 정책은 5분 입력 활동이 없고 보류 항목이 없을 때 재시작한다. 에이전트가 작업 중이어도
  TUI만 교체하는 것은 가능하며, 세션 중지 RPC를 호출하지 않는다.
- 연결 이름, 선택 Project/Session ID, 화면 위치 등 복원 가능한 UI 메타데이터만 저장한다.
  재접속 후 journal cursor를 기준으로 누락 이벤트를 받고, 초안이나 마지막 전송을 재전송하지 않는다.
- Bubble Tea 종료와 터미널 모드 복원 후 launcher가 새 frontend를 실행하고 시작을 확인한다.
  Unix/Windows 모두 같은 수명 관리 절차를 사용하며 실패 시 이전 파일로 복구한다.
- 자동 재실행은 TUI에만 적용한다. 일반 CLI 서브커맨드는 실행을 마친 뒤 종료하며 재실행하지 않는다.
- 여러 TUI가 같은 파일을 사용하면 파일 교체는 잠금 아래 한 번만 수행하고 각 프로세스는
  자신의 상태에 따라 나중에 재시작한다. 자동 재시작 실패 시 반복 루프 대신 이전 버전으로 복구한다.

### Wisp

연결 helper이므로 새 연결부터 새 빌드를 사용한다. 실행 중인 경로 조회나 secret 요청의 연결을
중간에 끊지 않는다. 프로토콜 호환 범위를 확인하고 요청 사이에서 연결을 교체한다.
Wisp 교체 때문에 세션을 재시작하거나 보존 중인 secret 파일을 삭제하지 않는다.

## 4. 적용 순서와 호환성

서버 측 기본 순서는 **artifact 준비 → Manager → Project runtime → idle Supervisor**다.
TUI는 별도 주기로 갱신하되 연결된 서버와 호환되는 버전인지 확인한다.
프로젝트별 적용은 순차 진행하며 바쁜 프로젝트/세션은 대기 상태로 남긴다.

- 모든 통신 경계에서 build ID, protocol version, 지원 capability를 조회할 수 있게 한다.
- 새 Manager가 구버전 Runtime/Supervisor와 공존해야 한다. API 확장은 additive하게 하고,
  실행 중인 구버전을 강제 재시작해야만 동작하는 변경은 자동 롤아웃에 포함하지 않는다.
- 동일 파일명이나 디스크의 최신 버전을 실행 버전으로 간주하지 않는다.
- 최초 도입 시 구버전에 필요한 교체·상태 프로토콜이 없다면 `bootstrap required`로 표시한다.
  첫 수동 갱신 이후부터 자동 경로를 보장한다. 누락된 프로토콜을 프로세스 강제 종료로 우회하지 않는다.
- 기본 설치의 Linux Manager/Runtime과 Linux·Windows TUI를 지원 대상으로 한다.
  개발용 foreground 서버, 쓰기 불가능한 실행 파일, 패키지 관리 설치 등은 자동 적용 가능 여부를
  별도로 판정하고 지원하지 않는 경우 확인 결과와 적용 불가 이유를 출력한다.

## 5. 사용자 인터페이스

- TUI 설정에서 채널, 자동 확인/적용 여부와 구성요소별 실행·목표 버전을 표시한다.
- `대기: 세션 작업 중`, `대기: 미전송 초안`, `업데이트 중`, `완료`, `이전 버전 복구`를 구분한다.
- 최신 파일이 준비됐어도 기존 프로세스가 실행 중이면 전체 업데이트 완료로 표시하지 않는다.
- 확인/상태 조회/일시 보류를 CLI에서도 제공한다. 명령 이름은 기존 `self-update`의 옵션 확장을
  우선 검토하고 `--format json`을 지원한다. 서브커맨드는 TUI를 열지 않는다.
- 일반 자동 업데이트에는 매번 확인을 묻지 않는다. 정책을 켠 범위에서 수행하고,
  컨테이너 재생성·비호환 migration 등 범위 밖 작업은 자동 실행하지 않는다.

## 6. 구현 단계와 검증

1. **빌드·호환성 기반:** manifest, 불변 artifact 경로, 실행 빌드 조회, explicit launch 선택,
   설정·상태 스키마를 추가한다. 이 단계에서는 실행 중인 프로세스를 교체하지 않는다.
2. **Supervisor 안전 교체:** 기존 provider transaction을 일반화하고 새 cxz 빌드 선택,
   기존 Agent 유지, 조합 rollback을 구현한다. 가짜 에이전트로 입력 경쟁과 복구를 검증한다.
3. **Project runtime 교체:** 독립 helper·drain·상태 재연결을 구현한다. 기존 Supervisor/Agent PID,
   Run ID, journal 연속성이 유지되는지 실제 프로세스 통합 테스트로 확인한다.
4. **Manager 자동 교체:** 외부 helper·설치 잠금·이미지 복구·중단 복구를 구현한다.
   Docker 테스트에서 프로젝트 컨테이너 ID와 세션 프로세스 생존을 확인한다.
5. **TUI 자동 재실행:** Unix/Windows 수명 관리, UI 상태 복원, 재접속, 다중 클라이언트 잠금,
   초안·비밀·PTY 보류 조건을 구현한다.
6. **통합 활성화:** edge 확인·단계별 적용을 연결하고 TUI/CLI 상태 표시와 운영 문서를 완성한다.

필수 실패 시나리오:

- 새 입력과 idle 중지가 동시에 발생해도 입력 유실·중복 실행·작업 중 종료가 없다.
- 백그라운드 셸, 미완료 도구, 승인 대기, 활동 중인 다른 TUI가 있으면 재시작하지 않는다.
- 다운로드 오류, checksum 불일치, 게시 중 edge 변경, 잘못된 플랫폼은 기존 실행을 유지한다.
- 중지 전후, 새 프로세스 시작 전후, health 응답 유실 시점을 각각 끊어 transaction 복구를 검사한다.
- Manager/helper/Runtime 재시작 후 큐 중복 실행, 잘못된 PID 종료, 새 소켓 삭제가 없다.
- 새 버전 초기화 실패 시 이전 조합으로 resume한다. 구버전이 읽을 수 없는 DB 변경은 적용 전에 차단한다.
- 구버전 Supervisor와 새 Manager/Runtime의 혼합 상태, 원격 연결 단절, 여러 TUI를 검증한다.
- provider 자동 업데이트와 cxz 업데이트가 동시에 대기해도 한 세션을 두 번 중지하지 않는다.
- TUI 재시작 동안 Agent가 계속 작업하며 재접속 후 이벤트가 누락되지 않는다.
- 완료 판정은 버전 문자열뿐 아니라 실제 프로세스·컨테이너 식별과 복구 가능한 상태로 검증한다.

## 초기 범위에서 제외

Supervisor를 살린 채 Agent의 stdio·프로세스 소유권을 다른 Supervisor로 이관하는 기능,
프로젝트 컨테이너/OS 이미지의 자동 재생성, 비호환 DB migration 자동화는 포함하지 않는다.
Manager가 작업 중인 모든 세션에 대해 완전 무중단으로 교체되도록 token broker까지 독립 서비스로
분리하는 작업도 별도 단계다. 초기 구현은 해당 교체를 안전한 시점까지 보류한다.

## 초기 구현에서 구체화한 선택

- 호환 범위는 protocol/schema 버전의 정확한 일치로 제한한다.
- 역할별 implementation ID 대신 전체 revision을 사용한다.
- 정책은 frontend와 서버 설치 두 범위다. TUI에서는 frontend 정책과 상태를 표시하며,
  서버 정책·상태는 설치 호스트의 `self-update ... --server`에서 관리한다.
- CLI 상태 명령은 항상 JSON을 출력한다.
- 만료된 maintenance lease, 변경된 프로세스 신원은 강제 복구하지 않는다.
- 이전 바이너리/컨테이너 자동 정리는 후속 작업으로 남긴다.
