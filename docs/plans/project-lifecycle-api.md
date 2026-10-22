# Project 생명주기 API 전환

상태: 구현 중. 설정 신뢰의 digest 동의와 매니저의 컨테이너 작업 분리가 들어갔고
(`internal/workspace/trust.go`, `container.go`), 아직 어떤 RPC에도 연결되지 않았으므로
외부 동작은 그대로다. 남은 것은 아래 "구현 순서와 완료 조건"의 2번부터이며, 그 대부분은
리소스 proto 재생성을 필요로 한다 — `tools/genproto`는 런타임 proto만 다루고, 리소스
쪽은 orm-ent → ent → pd 다단계 생성이라 그 환경이 있는 곳에서 해야 한다.

Project는 DB에 등록된 워크스페이스다. 컨테이너가 존재하거나 실행 중이어야 리소스가
존재하는 것은 아니다. 리소스의 등록·삭제와 컨테이너의 생성·시작·정지·제거를 분리한다.
목록에서 무엇을 보여줄지는 클라이언트가 결정한다.

## 합의한 계약

- `Project.listed`를 제거한다. 기본 `List`는 `date_erased`가 없는 모든 Project를
  페이지 단위로 반환한다. 명시적 검색 조건과 접근 범위는 계속 적용한다.
- `Get`과 `Watch`도 같은 삭제 기준을 사용한다. 컨테이너가 없거나 정지됐다는 이유로
  리소스를 숨기지 않는다. `Watch`는 초기 목록과 이후의 리소스 변경을 전달하고,
  Erase 완료 시에는 기존 구독자가 항목을 제거할 수 있는 삭제 알림을 보낸다.
- `Add`는 DB 리소스만 만든다. 런타임 manifest, 컨테이너, 네트워크, 볼륨, 자격 증명,
  세션을 만들지 않는다.
- 컨테이너 작업은 `CreateContainer`, `StartContainer`, `StopContainer`,
  `RemoveContainer`로 나눈다. CRI의 이름을 그대로 쓴다. 파괴적 의미가 뒤집히는 옛 이름을
  재사용하지 않는 것이 두 번째 이유다 — "이름과 울타리"를 보라.
- `CreateContainer`는 Dev Container 생명주기를 끝까지 수행한다. 컨테이너를 만들고
  시작하고 `onCreateCommand`·`updateContentCommand`·`postCreateCommand`를 돌린 뒤
  **실행 중인 컨테이너**를 남긴다. 세션은 만들지 않는다.
- `Erase`는 현재 프로젝트 Purge의 데이터 정리를 수행한다. 사용자에게도 `Erase`라고
  표시한다. 기존 `Erase`의 `listed=false` 보관 의미는 없앤다.
- `Recreate` RPC를 제거한다. CLI와 UI의 재생성 기능은 여러 RPC를 조합한다.
- 설정 신뢰는 불리언이 아니라 **동의한 설정의 digest**로 기록한다 — "설정 신뢰"를 보라.

명명은 [CRI RuntimeService][cri]의 `CreateContainer`, `StartContainer`,
`StopContainer`, `RemoveContainer`를 따른다. containerd도 [Task.Start][containerd]를
제공하지만 정지·제거는 `Kill`·`Delete`로 표현한다. cxz는 이들 런타임의 구현을
채택하는 것이 아니라 작업의 경계와 이름을 맞춘다.

## 현재 구현과의 차이

| 영역 | 현재 | 전환 후 |
| --- | --- | --- |
| 목록 | 기본 `listed=true`, erased 제외 | erased만 기본 제외 |
| `Add` | 런타임 등록 파일과 DB를 함께 만듦 | DB 리소스만 생성 |
| `Up` | 생성·시작·초기화·세션 선택이 한 경로 | `CreateContainer`와 `StartContainer`로 분리, 세션은 분리 |
| `Down` | 컨테이너 제거 | `StopContainer`는 정지, `RemoveContainer`는 제거 |
| `Recreate` | 서버의 제거·재프로비저닝 복합 작업 | 클라이언트 조합 |
| `Erase` | 프로젝트·세션을 `listed=false`로 보관 | 소유 데이터 정리 후 삭제 표식 |
| `Remove` | 프로젝트 소유 데이터와 DB 행 영구 제거 | 사라짐. 데이터 삭제는 `Erase`, 컨테이너 제거는 `RemoveContainer` |
| 상태 | 문자열 `status.state`와 `container_id` | enum `status.container_state`와 관측 정보 |
| 설정 신뢰 | `Up`의 `trust_config` 불리언이 `Project.Trusted`를 영구히 켬 | 동의한 설정 digest를 기록, 내용이 바뀌면 다시 묻는다 |

현재 경로는 `server/lifecycle/project.go`, `delete.go`, `project_remove.go`와
`internal/workspace/manager.go`, `register.go`, `provision.go`, `remove.go`에 있다.
일반 조회의 erased 제외는 이미 `server/bare/project.g.go`의 `ProjectNarrow`가 수행한다.

## 상태 모델

리소스의 삭제 여부는 `date_erased`로, 컨테이너 상태는 다음 enum으로 표현한다.
별도의 `exists` boolean을 두지 않는다.

| `container_state` | 의미 |
| --- | --- |
| `UNKNOWN` | 아직 확인하지 못했거나 Docker 조회에 실패함. 기본값 |
| `ABSENT` | 소유 컨테이너가 없음을 확인함 |
| `CREATED` | 컨테이너가 생성됐으나 아직 시작되지 않음 |
| `RUNNING` | 컨테이너가 실행 중 |
| `STOPPED` | 시작됐던 컨테이너가 정지됨 |

`CREATED`는 **관측되는 상태이고 어떤 RPC의 보장도 아니다.** cxz의 생성 경로는 Dev
Container 생명주기를 끝까지 수행하므로 `CreateContainer`가 성공하면 컨테이너는 실행
중이다. 밖에서 누가 만들어 두거나 생성과 훅 사이에서 죽은 컨테이너를 그대로 보고하기
위해 값은 남긴다.

`container_id`는 관측한 대상의 식별자다. ID가 남아 있다는 사실만으로 존재를 판정하지
않는다. `ABSENT`를 확인하면 현재 컨테이너 ID를 비우고, 조회 실패를 `ABSENT`로
덮어쓰지 않는다. 확인 시각과 관측 오류를 전달해 마지막 확인 상태임을 알 수 있게 한다.

`RUNNING`은 에이전트 준비 완료와 다르다. `provision_state`, `provision_step`, 오류는
별도로 유지한다. 컨테이너는 실행됐지만 초기화에 실패한 경우도 그대로 표현한다.
`CreateContainer` 중의 이미지 빌드와 생성 시점 훅, `StartContainer`의 시작 시점 훅도
이 진행 상태로 구분한다. 기록된 실패는 런타임이 응답하면 은퇴한다([cxz#282][settle]).

현재도 클라이언트는 컨테이너를 직접 조회하지 않는다. Manager가 Docker를 조회하고,
`server/lifecycle/projection.go`가 결과를 DB status에 반영한 뒤 `List/Watch`로 전달한다.
이 경로를 유지하되 다음을 바꾼다.

- 등록된 리소스만 관측 결과를 갱신한다. 런타임 목록에 있다는 이유로 Project를 새로
  만들거나 erased 리소스를 되살리지 않는다. 최초 데이터 이관은 별도 절차로 둔다.
- 일반 `List/Get`의 응답은 DB에서 읽는다. Docker나 프로젝트 런타임 장애가 목록 전체의
  조회 실패나 누락으로 이어지지 않게 한다. 첫 요청의 전체 런타임 동기화도 분리한다.
- 성공한 변이 RPC는 해당 작업의 관측 결과를 반영한 후 응답한다. 응답 직후 이전 상태를
  반환하고 나중의 주기적 동기화에 맡기는 경합을 없앤다.
- 외부 Docker 변경은 백그라운드 재확인으로 반영한다. 관측 쓰기는 변경된 리소스만
  `Watch`에 발행하며, 늦게 끝난 관측이 새 컨테이너의 상태를 덮어쓰지 않게 한다.

## RPC별 전이

아래의 리소스는 erased가 아닌 Project를 뜻한다. erased 또는 존재하지 않는 리소스에
대한 컨테이너 작업은 `NotFound`다. 표의 상태는 RPC 성공 시 보장할 결과다.

| RPC | 호출 전 | 성공 후 | 범위 |
| --- | --- | --- | --- |
| `Add` | 리소스 없음 | 리소스 존재, 관측 전 `UNKNOWN` | 메타데이터 저장만 수행 |
| `CreateContainer` | 리소스 존재, 컨테이너 없음 | `RUNNING`, 생성 시점 훅까지 완료 | 이미지·설정·마운트·소유 자원 준비, 생성, 시작, 생성 시점 훅 |
| `StartContainer` | `CREATED` 또는 `STOPPED` | `RUNNING`, 런타임 준비 완료 | 기존 컨테이너 시작과 시작 시점 훅 |
| `StopContainer` | `RUNNING` | `STOPPED` | 세션과 부속 프로세스 정리, 컨테이너 정지 |
| `RemoveContainer` | `CREATED` / `STOPPED` / `RUNNING` | `ABSENT` | 실행 중이면 정지 절차를 거쳐 컨테이너 제거 |
| `Erase` | 리소스 존재, 모든 컨테이너 상태 | 소유 데이터 정리, `date_erased` 설정 | 일반 `Get/List/Watch`에서 제거 |

`Add` 직후의 `UNKNOWN`은 컨테이너 조회를 하지 않았다는 뜻이다. 이후 관측으로
`ABSENT` 등을 채운다. `CreateContainer`는 자신의 작업 전에 실제 소유 상태를 확인하므로
사용자가 별도로 상태가 갱신되기를 기다릴 필요는 없다.

반복 호출과 잘못된 전이는 다음과 같이 처리한다.

- `Add`: 이미 등록된 워크스페이스는 `AlreadyExists`. erased 프로젝트를 재등록할 때는
  새 리소스 UUID를 만들며 과거 세션이나 데이터를 복원하지 않는다.
- `CreateContainer`: 컨테이너가 이미 있으면 `AlreadyExists`. 만들다 실패한 재시도는
  같은 컨테이너로 수렴한다. 숨겨진 교체는 하지 않는다.
- `StartContainer`: 컨테이너가 없으면 `FailedPrecondition`. 실행 중이면 준비 상태를
  확인하고 수렴하며 기존 컨테이너를 교체하지 않는다. 초기화 실패 후 재시도도 이
  경로다. 설정이 바뀌었으면 `FailedPrecondition`으로 알리고, 적용은 클라이언트가
  `RemoveContainer`+`CreateContainer`로 결정한다.
- `StopContainer`: 이미 정지됐거나 컨테이너가 없으면 성공. 정상 종료를 기다리는 제한
  시간이 지나면 소유 프로세스를 강제 종료한다. 성공은 컨테이너가 실행 중이 아님을
  보장한다.
- `RemoveContainer`: 컨테이너가 이미 없으면 성공. 리소스, 세션, 대화 DB, 프로필, 첨부
  파일, 공유 메모리, 프로젝트 상태 볼륨·디렉터리는 유지한다.
- `Erase`: 이미 삭제된 UUID에 대한 반복 호출은 성공. 도중 실패하면 남은 정리를
  재시도할 수 있어야 하며 성공한 것처럼 응답하지 않는다.
- 소유권이 다른 컨테이너는 어느 작업에서도 가져오거나 지우지 않는다. 여러 소유
  컨테이너가 충돌하면 오류로 보고하며 임의의 하나를 선택하지 않는다.

컨테이너 작업은 어느 것도 세션을 생성하거나 재개하지 않는다. 컨테이너가 떠 있다는 것과
에이전트가 돌고 있다는 것은 다른 사실이고, 다시 실행할지는 클라이언트가
`Session.Resume`으로 결정한다. `StopContainer` 성공 후 소속 세션이 `working`으로 남지
않도록 세션 상태는 갱신한다.

## 생성과 시작의 경계

경계를 Dev Container 생명주기에 맞춘다. "만들었지만 실행되지 않은 컨테이너"를
만들어내려 하지 않는다 — 생성 시점 훅은 컨테이너가 떠 있어야 돌기 때문에, 그 상태를
보장하려면 잠깐 시작했다가 정지하는 눈속임밖에 없다.

Dev Container CLI 0.89.0으로 측정한 결과다(alpine 이미지, 훅마다 마커를 남김):

| 한 일 | 결과 |
| --- | --- |
| `up` | 빌드·생성·시작, `onCreate`→`updateContent`→`postCreate`→`postStart`→`postAttach`, 컨테이너 running |
| `stop` 후 `up --expect-existing-container` | **`postStart`+`postAttach`만** 재실행, 생성 시점 훅은 반복되지 않음 |
| 설정을 바꾼 뒤 `up --expect-existing-container` | 같은 컨테이너 유지, 변경은 **조용히 무시**, outcome success |
| 컨테이너 없이 `--expect-existing-container` | `Error: The expected container does not exist.` |

그래서 구현은 이렇게 나뉜다.

- `CreateContainer` = 지금의 `devcontainer up`(`provision.go`) + cxz의 도구 설치와
  `_boot`. 성공하면 컨테이너는 실행 중이다.
- `StartContainer` = `devcontainer up --expect-existing-container` + 런타임 준비 확인.
  훅의 일회성은 CLI가 스스로 지킨다. 컨테이너가 없으면 CLI가 실패하므로 그것을
  `FailedPrecondition`으로 옮긴다.
- `StopContainer`·`RemoveContainer`는 CLI에 해당 명령이 없으므로 지금처럼 docker
  레벨이다.

세 번째 줄이 cxz의 몫을 정한다. **CLI는 설정이 바뀐 것을 알려주지 않는다.** 이미
`manager.go`가 compose 오버라이드에 대해 digest를 들고 바뀌면 거부하는데
(`devcontainer.compose changed; run cxz project recreate`), 그 검사를 **렌더된 설정
전체**로 넓혀 `StartContainer`의 계약으로 삼는다. 같은 digest가 아래 "설정 신뢰"의
동의 대상이기도 하므로 Project가 들고 있는 digest는 하나다.

남은 회귀 검증은 구성별 차이다: 단일 이미지·Dockerfile·Compose에서 위 표가 같은지,
Features·사용자 UID·Compose 의존 서비스·네트워크·마운트가 두 단계에 걸쳐 보존되는지.
`--prebuild`(onCreate/updateContent까지만), `--skip-post-create`,
`devcontainer run-user-commands`, `devcontainer build`가 있으므로 CLI가 어떤 구성에서
이 경계를 지키지 않더라도 훅을 직접 고르는 길이 남아 있다.

## 설정 신뢰

`devcontainer.json`은 저장소 안의 파일이고, 매니저는 root로 호스트 Docker 소켓을 들고
돈다. 그래서 cxz는 렌더된 설정에서 `privileged`, `initializeCommand`, host
network/PID, `docker.sock`, `SYS_ADMIN` 같은 것을 찾고 발견이 있으면 프로비저닝을
거부한다(`CheckTrust`, `provision.go`). 클라이언트에는 `FailedPrecondition` +
`ErrorInfo{Domain:"cxz.config", Reason:"TRUST_REQUIRED"}`와 발견 목록이 간다.

**검사는 서버의 일이다.** 대상은 머지된 Features와 compose 오버라이드, fallback
템플릿까지 반영된 *유효* 설정이고, 그것을 아는 쪽은 워크스페이스를 읽는 매니저뿐이다.
클라이언트가 보내는 값은 검사 결과가 아니라 **동의의 기록**이다. 권한 경계는 아니다 —
어떤 클라이언트든 동의를 보낼 수 있다. 값은 기본이 거부라는 것, 그리고 결정을 기억한다는
데 있다.

지금은 `Up`의 `trust_config` 불리언이 `Project.Trusted`를 영구히 켠다. 한 번 켜지면
**승인한 내용과 무관하게** 유효하므로, 저장소가 나중에 `privileged`나 `docker.sock`
마운트를 추가해도 다시 묻지 않는다. 전환 후에는 digest로 바꾼다.

- Project는 **동의한 설정의 digest**를 들고 있고 불리언은 없앤다. 컨테이너가 만들어져
  있다는 것은 그때의 내용이 승인됐다는 뜻이고, digest가 그 사실을 정확히 적는다.
- 판정은 `렌더 → CheckTrust`의 결과로 한다. 발견이 없으면 묻지 않는다(지금도 그렇다).
  발견이 있고 digest가 동의한 값과 같으면 진행한다. 다르면 `TRUST_REQUIRED`다.
- 요청 필드는 불리언이 아니라 **사용자가 본 내용의 digest**다. 발견 목록을 보고 승인하는
  사이에 파일이 바뀔 수 있으므로, 서버는 지금 렌더한 digest와 같은지 확인하고 다르면
  다시 묻는다.
- 동의를 받는 지점은 서버가 렌더된 설정을 손에 든 자리 전부다: `CreateContainer`(빌드
  전에 거부), `StartContainer`(digest 비교 — 경계 절의 그 검사), 그리고 읽기 전용
  `RenderDevcontainer`/`Devcontainer`는 **동의 없이 발견 목록을 반환**한다. UI가
  `CreateContainer` 전에 근거를 보여줄 수 있는 유일한 자리다.

이 digest는 설정 변경 감지와 같은 값이다. Project가 들고 있는 설정 digest는 하나고 용도가
둘이다: 바뀌었으면 `StartContainer`가 거부하고, 바뀐 내용에 발견이 있으면 동의를 다시 받는다.

## Project의 정체성

전환이 바꾸지 않는 것이지만, 위의 digest 논의가 성립하는 전제이므로 적어 둔다.

**Project는 워크스페이스 경로다.** `register.go`가 경로의 sha256으로 ID를 만들고, proto에서
`workspace`와 `runtime_id`는 `unique, immutable`이다. 같은 경로면 같은 Project다.
`devcontainer.json`의 `name` 필드는 정체성에 쓰이지 않는다 — 코드에서 읽는 곳이 없다.
`Project.name`은 디렉터리 이름이 기본값인 표시용이고 사용자가 고친다.

설정이 바뀌어도 다른 Project가 되지 않는다. 컨테이너는 교체 가능한 것이고 Project는
등록이다 — 그것이 이 전환의 요지다. 컨테이너를 지우고 다시 만들어도 UUID·alias·세션·
대화·업로드·메모리는 그대로 남는다. 새 Project가 되는 경우는 Erase 후 같은 경로를 다시
`Add`하는 것 하나뿐이고, 그때는 과거 데이터를 복원하지 않는다.

## Erase의 삭제 경계

`Erase`는 되돌리기 불가능한 데이터 정리다. CLI와 UI는 대상 Project를 조회하여
식별 정보를 보여주고 명시적으로 확인받는다. 기존 `Remove(confirmed=false)`의
미리보기 동작은 어디에도 남기지 않는다. 옛 `Remove`는 사라지므로 물려받을 곳도 없다.

정리 대상은 현재 프로젝트 영구 제거 경로를 기준으로 한다: 소유 컨테이너, 프로젝트
전용 볼륨과 매니저의 프로젝트 상태 디렉터리, 세션 대화·프로필·첨부·메모리·파생 데이터,
프로젝트별 자격 부여와 MCP/Skill 설정. 전역 계정과 로그인, 공유 엔진·네트워크·이미지,
다른 프로젝트, 사용자 소유 마운트와 워크스페이스 소스 파일은 유지한다.
세션·Task·AuthBinding 등 종속 리소스도 일반 조회에서 남지 않도록 처리한다.

컨테이너 정지와 외부 데이터 정리를 끝낸 뒤 Project와 종속 리소스의 삭제 표식을
DB 트랜잭션으로 확정한다. Project 메타데이터는 payday의 `date_erased`로 남긴다.
삭제 표식은 실행 데이터의 보존이나 복원 가능성을 뜻하지 않는다. 정리 중에는 같은
프로젝트의 생성·시작·세션 쓰기가 데이터를 다시 만들지 못하게 막는다.

외부 정리는 원자적이지 않다. 실패 시 완료한 단계와 남은 대상을 보존하고 같은 대상에
대한 재시도로 수렴하게 한다. 매니저 재시작 후에도 미완료 Erase를 식별할 수 있어야 한다.
정리가 끝나기 전에 삭제 표식만 먼저 남겨 재시도 대상을 잃지 않는다.

같은 워크스페이스에 다시 `Add`하면 새 UUID와 새 실행 데이터로 시작한다. 기존의
경로 해시 기반 runtime ID 및 `resourceID` 매핑이 tombstone과 충돌하지 않도록
생성 경로를 바꾼다. 기존 프로젝트의 UUID·runtime ID·세션 ID·vendor ID는 보존한다.
runtime ID 자체를 없애는 별도 작업([cxz#237][runtime-id])은 이 전환에 합치지 않는다.

## 클라이언트 조합과 동시성

재생성은 공용 클라이언트 로직에서 다음 순서로 수행한다.

```text
대상과 변경 영향 확인 → StopContainer → RemoveContainer → CreateContainer
```

`CreateContainer`가 실행 중인 컨테이너를 남기므로 조합은 거기서 끝난다. 에이전트를 다시
띄우는 것은 그 뒤 클라이언트의 `Session.Resume`이다. 각 단계의 완료 후 다음을 호출하며
실패한 지점에서 멈춘다. 어느 단계까지 끝났는지와
현재 상태를 표시하고, 재조회 후 재시도한다. 실패를 만회하려고 `Erase`를 호출하지 않는다.
CLI `recreate`와 UI의 재생성 버튼은 이 조합을 사용할 수 있지만 서버 RPC는 두지 않는다.
일괄 재생성의 대상 선택과 부분 실패 보고도 클라이언트에 둔다.

서버는 각 변이 RPC를 프로젝트 단위로 직렬화한다. 단계를 여러 RPC로 나누면 그 사이에
다른 클라이언트가 개입할 수 있으므로 컨테이너 대상 작업에 기대 ID/세대 조건을 둔다.
없는 상태에서의 `CreateContainer`도 세대 조건으로 보호한다. 조건 불일치는 `Aborted`로 반환하고
자동으로 새 대상을 골라 계속하지 않는다. 재시도 키는 같은 작업의 재전송에만 재사용한다.
메타데이터 버전과 컨테이너 세대는 별도로 다뤄 주기적 status 갱신이 모든 작업을
무효화하지 않게 한다.

## 이름과 울타리

이번 변경에서 조용히 데이터를 파괴할 수 있는 지점은 하나뿐이다. **옛 `Erase`는 보관이고
새 `Erase`는 삭제다.** 낡은 클라이언트가 "보관해줘"라는 뜻으로 부르면 새 매니저는 대화와
볼륨을 지운다.

호환 계층은 만들지 않는다. 사용자가 한 명이고 설치본이 하나다. 대신 **낡은 호출이 조용히
성공하지 않게** 두 가지만 한다.

1. **컨테이너 작업에 CRI 이름을 그대로 쓴다.** 그러면 파괴적 의미가 뒤집히는 이름
   재사용이 사라진다. 옛 `Remove`는 제거되어 `Unimplemented`로 떨어지므로, 낡은
   `cxz project remove`가 컨테이너만 지우는 새 동작으로 오해될 여지가 없다.
2. **`Erase`는 낡은 클라이언트가 보내지 않는 명시 필드를 요구한다.** 없으면 실행 전에
   `FailedPrecondition`과 안내로 거부한다. 지금 `TRUST_REQUIRED`가 쓰는 그 패턴이고,
   새로운 개념을 만들지 않는다.

설치본 내부(매니저·프로젝트 런타임·헬퍼)는 이미 있는 울타리로 끝난다:
`cxzupdate.Protocol`과 `Schema`가 릴리스 수락, 헬퍼 빌드 검증, 핀 검증에서 호환되지 않는
빌드가 섞이는 것을 막는다. 새 계약이 들어가는 릴리스에서 `Protocol`을 올린다. "옛 빌드가
기록을 거부한다"의 선례는 대화 저장소 5단계의 `sessionstore.Compat`이다.

**함께 롤아웃되지 않는 것**이 남는다: 호스트의 `cxz` 바이너리, 릴레이, 웹 게이트웨이.
자동 업데이트는 그 둘을 두고 가므로([cxz#283][aux-drift]) 게이트웨이가 매니저보다 수십
커밋 뒤처진 상태가 실제로 존재한다. 그래서 cxz#283을 이 전환보다 **먼저** 고치고, 옛
`Up`/`Down`을 지우는 단계는 그 뒤에 둔다.

## 데이터 이행

호환을 버려도 남는 작업이다. DB는 전환을 넘어 살아남는다.

1. 기존 `listed=false` Project는 **삭제하거나 erased로 변환하지 않는다.** 데이터 보존을
   위해 `date_erased`가 없으면 일반 목록에 포함한다. 이름·보관용 alias도 임의 복원하지
   않는다. 기존 `Session.listed`와 세션 보관/복원 계약은 이번 범위에서 변경하지 않는다.
2. 기존 리소스와 runtime manifest를 한 번 대응시키고 등록 의도를 DB로 확정한다.
   Docker가 없어도 이관할 수 있어야 한다. 이후 런타임 발견으로 리소스를 자동 생성하는
   경로를 제거한다. 기존 프로젝트 상태 볼륨과 새 매니저 내 상태 디렉터리를 모두 다루며
   진행 중인 대화 저장소 이행([현재 위치](where-we-are.md))과 중복 복사하지 않는다.
3. Project proto, Add/Select/Filter와 생성물에서 `listed`를 제거하고 삭제한 필드명·번호를
   예약한다. 문자열 `state` 번호를 enum으로 재사용하지 않고 새 필드로 전환한다.
   저장된 status JSON도 명시적으로 변환하며 확인되지 않은 값은 `UNKNOWN`으로 둔다.
   `listed` DB 컬럼 제거는 데이터 이관 검증 뒤 수행한다. `Trusted` 불리언은 설정 digest
   동의로 전환하며, 내용을 확인할 수 없는 기존 값은 동의 없음으로 둔다.
4. 매니저·프로젝트 런타임·CLI/TUI·웹·WASM sandbox를 같은 계약으로 배포한다.
   컨테이너 존재 여부를 옛 `listed`의 대체 서버 필터로 넣지 않는다. 웹은 현재
   `ResourceInventory`의 명시적 `listed:true` 요청도 제거한다.

**이행을 API 전환과 같은 배포에 싣지 않는다.** 이번 주의 교훈이다 — 대화 저장소 5단계는
코드가 맞았는데도 파일 소유권 하나로 모든 `resume`이 죽었다(cxz#280). 2·3번의 데이터
이행을 먼저 독립 릴리스로 내보내면 전환 자체는 코드만 바뀌는 변경이 된다. 3단계(상태
서브패스) → 5단계가 실제로 그 순서로 성공했다.

## 구현 순서와 완료 조건

각 단계는 구현 PR의 단위다.

1. **구성별 경계 회귀.** 위 "생성과 시작의 경계"를 단일 이미지·Dockerfile·Compose에서
   재확인한다. `StartContainer`가 생성 시점 훅을 다시 돌리지 않는 것, Features·사용자
   UID·Compose 의존 서비스·네트워크·마운트가 두 단계에 걸쳐 보존되는 것을 본다.
   컨테이너 세대와 재시도·오류·진행 상태를 proto에 반영할 수준으로 확정한다.
2. **스키마와 이관.** enum·관측 필드, `listed` 제거, 설정 digest 동의, tombstone·새
   Add 식별자, 일회성 manifest 이관과 `Protocol` 울타리를 구현한다. 실제 데이터 사본으로 이관을
   재실행해도 ID와 기록이 보존되는지 확인한다.
3. **런타임 작업 분리.** `internal/workspace`의 생성·시작·정지·컨테이너 제거를 분리한다.
   기존 대화 저장소와 상태 볼륨/디렉터리의 소유권·보존 경계를 유지한다.
4. **서비스와 관측.** `proto/ext/cxz/project_svc.ext.proto`, `server/lifecycle`과
   `internal/resourceclient`를 새 계약으로 연결한다. Erase 정리, 잠금·세대 조건,
   실패 후 복구, DB 기반 조회와 Watch까지 같은 전이 규칙을 사용한다.
5. **클라이언트와 sandbox.** CLI/TUI·웹의 기존 Up/Down/Recreate/프로젝트 Purge 호출을
   새 동작이나 조합으로 바꾼다. `StopContainer`/`RemoveContainer`/`Erase`의 설명과 확인 UI를
   구별한다 — 하나는 멈추고, 하나는 컨테이너를 버리고, 하나는 대화를 지운다.
   프로젝트 표시 필터는 클라이언트 설정으로 처리한다. WASM도 같은 상태·오류를 재현한다.
6. **통합·출시.** 아래 수용 기준을 통과하고 `docs/projects.md`, CLI/API 문서,
   기능 명세와 클라이언트 지원표를 갱신한다. 옛 RPC와 분기를 제거하고 계약 불일치 시
   안내를 확인한다. 이 계획 문서의 상태도 구현 결과에 맞춘다.

## 수용 기준

- Docker를 사용할 수 없는 환경에서도 `Add`는 리소스를 만들고 `Get/List/Watch`는
  erased가 아닌 리소스를 반환한다. 페이지 전체와 Watch 초기 목록에 빠진 항목이 없다.
- 각 RPC의 정상·반복·잘못된 전이를 테스트한다. `CreateContainer` 후 컨테이너는 실행 중이고
  생성 시점 훅이 한 번 돌았으며, `StopContainer` 후에는 존재하되 정지됐고,
  `RemoveContainer` 후에는 없어야 한다. `StopContainer`→`StartContainer`가 생성 시점 훅을
  다시 돌리지 않는다.
- 컨테이너 작업과 재생성 조합이 세션을 새로 만들거나 재개하거나 프롬프트를 재전송하지
  않는다. `StopContainer → StartContainer`와 `RemoveContainer → CreateContainer` 뒤 기존
  세션을 명시적으로 재개하면 같은 vendor 대화와 UUID·alias·설정·업로드·메모리·기록을 쓴다.
- 이미지 빌드, 생성, 시작 훅, 준비 확인, 정지, 각 삭제 단계에 실패를 주입한다.
  실제 남은 상태·오류가 조회되고 재시도로 수렴한다. `UNKNOWN`이 `ABSENT`로 오인되지 않는다.
- 두 클라이언트의 생성·재생성·Erase 경쟁과 늦은 Docker 관측을 재현한다. 새 컨테이너를
  옛 요청이 지우거나 오래된 관측이 새 상태를 덮어쓰지 않는다.
- Erase는 소유 데이터만 정리하고, 종속 리소스·열린 스트림·늦은 이벤트가 삭제 데이터를
  다시 노출하거나 만들지 않는다. 외부 파일·공유 자원·다른 프로젝트는 유지한다.
- 기존 `listed=false` 데이터는 삭제 없이 목록에 나타난다. erased tombstone은 재시작과
  재확인 후에도 돌아오지 않는다. 같은 워크스페이스의 새 Add는 과거 데이터를 복원하지 않는다.
- 낡은 클라이언트의 `Erase`는 실행 전에 거부되고, 사라진 `Remove`는 `Unimplemented`로
  떨어진다. 설정이 바뀐 뒤의 `StartContainer`는 거부되고, 발견이 있는 새 설정은 동의를
  다시 요구한다. 동의한 digest와 다른 내용은 승인으로 재사용되지 않는다.
- 결정적 테스트와 WASM 전이 테스트에 더해 실제 Docker에서 생성/시작/정지/제거,
  단일 이미지·Dockerfile·Compose 훅, 두 상태 저장 배치의 보존·삭제를 검증한다.

[cri]: https://github.com/kubernetes/cri-api/blob/master/pkg/apis/runtime/v1/api.proto
[containerd]: https://github.com/containerd/containerd/blob/main/docs/getting-started.md
[settle]: https://github.com/lesomnus/cxz/issues/282
[aux-drift]: https://github.com/lesomnus/cxz/issues/283
[runtime-id]: https://github.com/lesomnus/cxz/issues/237
