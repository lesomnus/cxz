# 프로젝트를 Kubernetes pod으로

상위 구현계획 cxz#371의 설계 기록. pod의 단위는 **프로젝트**다. 세션은 지금처럼 그 pod 안의 프로세스로 남는다.

## 1. 무엇을 풀려는 것인가

프로젝트가 늘면서 여러 프로젝트가 같은 노드에서 컴파일 같은 무거운 작업을 돌려 서로를 밀어낸다. 노리는 것은 두 가지다.

- **여유 있는 곳에 프로비저닝.** 프로젝트를 서로 다른 노드에 배치한다.
- **유휴 프로젝트를 잠깐 내린다.** 클러스터를 다른 앱과 공유하므로 쓰지 않는 프로젝트가 자원을 계속 점유하지 않게 한다. 보통 한두 프로젝트에 집중하므로 프로젝트 단위 scale-down이 실제 사용과 일치한다.

무거운 세션은 프로젝트당 보통 한두 개다. 그래서 **세션=pod은 필요 없다**: 분산 효과는 프로젝트 수에서 나오고, "지금 안 쓴다"의 단위도 프로젝트다.

## 2. 왜 생각보다 작은 변경인가

### 매니저↔프로젝트는 이미 네트워크 경계다

```go
// internal/workspace/manager.go:268
ip := v.NetworkSettings.Networks[p.Network].IPAddress
conn, e := transport.Remote(ip+":7348", p.Token)
```

gRPC, TCP 7348, 프로젝트별 토큰. **프로토콜도 인증도 바뀌지 않는다.** Docker에 묶인 것은 주소를 얻는 방법(`dockerx.Owned` → inspect → network IP)과 살아 있는지 보는 방법(`v.State.Running`)뿐이고, 둘 다 `m.client()` 한 함수에 있다.

### 소유권이 이미 라벨이다

프로젝트 컨테이너는 `cxz.owner`와 `cxz.project` 라벨이 둘 다 맞아야 인정된다(`dockerx.Owned(ctx, id, m.Owner, p.storageID())`, `docs/projects.md`). k8s 라벨 셀렉터와 개념이 같다.

### 컨테이너 상태가 이미 인벤토리에서 유도된다

`ProjectStatus.container_state`는 저장하지 않고 읽을 때마다 계산한다. proto 주석이 "A look that failed is not an absence"까지 못박아 두었다. k8s 인벤토리를 꽂을 자리가 비어 있다.

### 저장소가 역할별로 분리돼 있다

`cxz.db`와 `resources.db`를 여는 것은 `internal/server` 한 패키지지만 **`root` 기준으로 각자** 연다. 매니저는 자기 root에, 각 프로젝트 런타임은 자기 root에(`internal/workspace/memory_resource.go:14` — "the project's resources.db").

따라서 프로젝트 pod 하나가 자기 root를 독점하므로 **RWO 볼륨 하나**로 충분하다. 단일 writer가 유지되어 SQLite의 WAL과 파일 락이 안전하다. RWX도, pod 사이 SQLite 공유도 필요 없다.

### pod 안에서는 아무것도 바뀌지 않는다

unix 소켓(`core.Socket(root, id)`), inotify 기반 변경 감지(`internal/server/observe_linux.go`), size+mtime fence, 프로세스 그룹 kill과 Pdeathsig, 세션 DB 파일 직접 읽기 — 전부 pod 안에 머물러 그대로 유효하다. 세션=pod이라면 이 전제들이 모두 깨지지만, 프로젝트=pod이면 깨지지 않는다.

## 3. 프로젝트 정의: 지금 무엇이고 무엇으로 바꾸는가

### 지금

실행자는 **devcontainer CLI**다. cxz가 `projects/<storageID>/devcontainer.json`을 쓰고(`internal/workspace/provision.go:311`) CLI에 넘긴다. compose는 devcontainer의 세 형태(image / Dockerfile / `dockerComposeFile`) 중 하나로 **네이티브로** 실행된다 — 번역이 아니다. CLI가 compose devcontainer를 생성된 Dockerfile로 감싸는 것도 CLI의 일이다.

compose 경로에서 cxz는 세 파일을 병합한다(`internal/workspace/devcontainer_render.go:91`): 프로젝트의 `dockerComposeFile`, 설치 전역 override(`compose.user.json`), cxz 자신의 override(`compose.json`, 마지막에 병합되어 이김). 병합 결과는 **Compose CLI로 해석한다**(`docker compose config`) — cxz는 병합 의미론을 직접 구현하지 않는다.

번역은 지금도 있으나 반대 방향이다. `Override`의 주석이 그것이다(`internal/workspace/compose_override.go:46`):

> a Compose devcontainer gets another Compose file to merge, while an image or Dockerfile devcontainer gets **the same intent translated**, since it never runs Compose.

즉 **하나의 의도를 두 형식으로 표현하는 개념이 이미 있다.** 다만 cxz 자신의 override에만 적용된다.

### 바꾸는 것

진짜 질문은 compose→pod이 아니라 **cxz가 프로젝트에게서 무엇을 필요로 하는가**다. 코드에서 열거된다:

| 필요한 것 | 지금 어디서 오는가 |
|---|---|
| 이미지 | devcontainer CLI의 빌드 또는 `image` |
| env | `provision.go:170-182` — `CXZ_PROJECT_ID`, `CXZ_STATE=/cxz/state/data`, `GH_CONFIG_DIR`, `CXZ_SECRET_STORAGE`, 필요시 `DOCKER_HOST` |
| 마운트 | `provision.go:185-191` — 워크스페이스, `/cxz/state` 볼륨, `/cxz/tools` readonly |
| remote user, 워크스페이스 경로 | devcontainer 설정, `ProjectStatus.remote_user`/`remote_workspace` |
| 런타임 기동 | `postStartCommand`의 `cxz-runtime` 훅 |
| 소유권 | `cxz.owner` / `cxz.project` 라벨 |
| 도달 | 토큰 인증이 붙은 7348 |

이것이 실제 계약이다. devcontainer+compose는 **한 입력 형식**이고 pod spec은 **한 출력 형식**이다. 지금은 devcontainer CLI가 실행자여서 둘이 한 덩어리다.

그래서 **프로젝트 런타임 스펙**을 내부에 하나 두고, devcontainer/compose는 그것을 만드는 프런트엔드로, Docker와 k8s는 그것을 소비하는 두 백엔드로 둔다. **사용자가 쓰는 포맷은 바뀌지 않는다** — 계속 devcontainer.json이다.

**순서가 중요하다.** 스펙을 먼저 혼자 설계하면 소비자가 하나뿐이어서 Docker의 전제를 그대로 인코딩한다. 반대로 지금 상태에 k8s를 붙이면 k8s 경로에만 임시 번역이 생겨 고착된다. 그래서 **스펙을 추출하고 곧바로 Docker 경로를 그 위로 이전한다.** Docker가 동작 변화 없이 지나가면 스펙이 맞다는 증거이고, k8s는 두 번째 소비자로 붙는다.

## 4. 확정한 범위 제한

- **dind 미지원.** `EngineMode{OFF, DIND}`의 DIND는 k8s 백엔드에서 제공하지 않는다. `internal/engine` 전체(dind 전용 구조체: `Name() = "cxz-<owner>-docker"`, `Endpoint() = tcp://…:2375`, compose 렌더링)와 `internal/networkservices`의 상당 부분이 k8s 경로에서 빠진다.
- **미리 빌드된 이미지만.** devcontainer의 `image` 형태만 지원한다. Dockerfile 빌드, features, compose 기반 빌드는 **명시적으로 거부**한다. 이로써 번역의 어려운 절반(빌드)이 사라진다. 거부 방식은 이미 있는 패턴을 따른다 — `internal/engine/config.go:132`가 dind 서비스 옵션에 대해 허용 목록을 두고 `unsupported managed Docker service option: %s`로 거부한다.
- **라이프사이클 훅 미지원.** `postStartCommand` 등을 흉내내지 않는다. 런타임은 컨테이너의 command로 PID 1에서 뜬다 — devcontainer 경로보다 단순하다.

이 세 제한으로 k8s 백엔드의 입력은 "이미지 + env + 마운트 + 사용자/경로 + 자원 요청"으로 좁아진다.

## 5. 새로 필요한 개념: 프로젝트 자원 요청

"여유 있는 곳에 프로비저닝"은 **스케줄러가 requests를 보고 하는 일**이다. limits만 걸면 스케줄러는 분산하지 못하고, requests가 없으면 노드가 과밀해진다. 컴파일처럼 버스트가 큰 작업은 "requests는 평상시, limits는 피크"를 따로 정해야 한다.

지금 프로젝트에는 자원 개념이 없다. `mem_limit`/`cpus`는 **dind 엔진 서비스에만** 있는 옵션이다(`internal/engine/config.go:132`). 따라서 이것은 포팅이 아니라 **새 기능**이며, 이 전환에서 사용자에게 보이는 유일한 새 개념이다. 리소스 모델과 설정 표면, 그리고 devcontainer 설정과의 관계(어디에 적는가)를 정해야 한다.

## 6. scale-down과 wake-on-demand

### 선행 조건

supervisor가 메모리에 든 것이 복구되어야 한다: `Log.events`(전체 이력 RAM 사본, 측정상 세션당 86~95MiB·4만8천 이벤트), `s.snap`, 미해결 승인, 대기 입력, receipt, codex thread 상태. **이것이 #337이다.** 대화 자체는 PVC에 남고 잃는 것은 RAM 상태다.

종료 신호도 바뀐다. 지금은 SIGKILL과 프로세스 그룹 수거가 전제다(`kill(-pid)`, Pdeathsig, guard). k8s는 SIGTERM + terminationGracePeriod이므로 supervisor가 내려간다는 신호를 받아 복구 상태를 내구화할 창이 필요하다.

### 이미 있는 자리

**scale-down 판단은 업데이트 idle 게이트와 같은 질문이다.** "지금 내려도 되나"는 `internal/supervisor/history.go:115`의 조건이 묻는 것과 같다 — idle이고, 미해결 승인·대기 입력이 없고, 활성 background 작업이 없는가. 재사용할 수 있고, 따라서 **#311**(멈춘 트랜잭션이 idle 게이트를 우회)과 **#312**(비종단 트랜잭션에 backoff 없음)가 그대로 scale-down의 버그가 된다. 먼저 고친다.

**wake-on-demand에도 자리가 있다.** 멈춘 프로젝트에 접근하면 `m.client()`가 `Precondition{"project is stopped; run cxz project up"}`을 돌려준다(`internal/workspace/manager.go:266`). 그 분기가 "scale up하고 기다린다"가 된다. 매니저가 프로젝트별 락(`m.projectLock`)을 들고 있어 동시 wake가 직렬화되고, 웹·TUI는 이미 정지된 프로젝트 상태를 표시한다.

### 스토리지가 성패를 가른다

RWO PVC는 bind되면 노드에 묶인다. 네트워크 CSI면 detach 후 다른 노드에 붙을 수 있지만 **local PV는 그 노드에 고정**된다. 그러면 "내렸다가 여유 있는 다른 노드에 올린다"가 불가능해지고 두 목표가 충돌한다. **네트워크 볼륨이 전제다.**

cold start 비용(이미지 pull + PVC attach + 런타임 기동 + provider 세션 재개)은 측정해야 한다.

## 7. Postgres

payday가 1급으로 지원한다: `config/dbpgx`, `DialectPostgres`, 생성 코드의 pgx 인지(`server/pd/pd.g.go:5877` — "NULL to pgx and an empty blob to SQLite's driver"). 더 중요하게 **`spin.lease: postgres`**가 있다(`server/pd/pd.g.go:9127`):

> Under `spin.WithLease` — `spin.lease: postgres` — one process of a deployment runs it; without a lease every process does.

즉 payday는 pg 위의 다중 replica 배포를 전제로 설계돼 있다. 이것이 중요한 이유: 지금 `cxz.db`/`resources.db`는 `MaxOpenConns: 1`에 WAL이고 단일 writer를 가정하므로(`internal/server/server.go:88, 245`) **매니저는 replica 1 고정이고 Deployment strategy가 `Recreate`여야 한다** — RollingUpdate는 같은 RWO 볼륨에 writer 둘을 만든다. pg로 가면 이 제약이 사라지고 롤링 업데이트가 가능해진다.

### 결정을 쪼갠다

| 저장소 | 성질 | 판단 |
|---|---|---|
| `resources.db` | ent/payday 생성 | **pg로 옮긴다.** 위험이 낮고 매니저 replica 제약을 없앤다. |
| `cxz.db` | `internal/server`가 직접 | pg로 옮긴다. 쿼리 확인 필요. |
| `sessions/<id>/conversation.db` | 손으로 쓴 SQLite | **당분간 파일로 유지.** |

대화 저장소를 미루는 이유: SQLite 방언과 파일 모델에 깊이 묶여 있다 — `INSERT OR REPLACE`, `PRAGMA user_version`, `VACUUM INTO`(스냅샷), `length(data)`(보존 한도 계산), `Durability = "FULL"`, 그리고 "파일이 곧 기록"이라는 단일 writer 계약(#278). 프로젝트 pod의 RWO PVC에 있으면 그 계약이 유지된다.

pg로 옮기면 얻는 것도 크다 — 프로젝트 서버가 세션의 파일을 직접 여는 결합이 사라져 #367의 원인 범주와 #339의 IPC 조회 경로가 상당 부분 불필요해진다. 반면 쓰기 경로가 로컬 WAL 커밋에서 네트워크 왕복이 되고, 보존 정책(#335 계열)이 전부 "세션별 파일"을 전제로 설계돼 있다. **#337의 Recorder/Reader 추상화가 끝난 뒤에 결정하면 백엔드 교체가 훨씬 싸다.** 순서가 거꾸로면 두 번 고친다.

## 8. 작업 규모

`dockerx.`를 직접 호출하는 곳이 272군데다(비테스트):

```
80 internal/workspace     25 internal/versionuse     4 internal/containerterm
61 internal/installer     22 cmd/cxz                 3 internal/engine
30 internal/cxzupdate     15 internal/telemetrystack 2 internal/purge
26 internal/networkservices
```

`workspace`(프로젝트 생명주기)와 `networkservices`(네트워크)가 k8s 백엔드의 본체다. `installer`·`cxzupdate`·`versionuse`가 Docker를 쓰는 것은 **자기 자신을 설치/갱신**하기 위해서다 — `runtime-binaries/<commit>/<hash>/cxz`에 바이너리를 써서 교체하는 모델이 불변 이미지와 맞지 않는다. 이것은 추상화가 아니라 재설계이므로 별도로 다룬다.

## 9. 단계

| 단계 | 서브이슈 | 선행 |
|---|---|---|
| 1 | cxz#372 — 프로젝트 런타임 스펙 추출, Docker 경로 이전 (동작 변화 없음) | — |
| 2 | cxz#373 — 프로비저너 인터페이스와 디스커버리 경계 | cxz#372 |
| 3 | cxz#374 — k8s 백엔드: pod + PVC + 도달, 이미지 기반만 | cxz#373 |
| 4 | cxz#375 — 프로젝트 자원 요청 | cxz#372 |
| 5 | cxz#376 — scale-down과 wake-on-demand | cxz#374, cxz#337 |
| 6 | cxz#377 — `resources.db`/`cxz.db` → postgres | — |
| 7 | cxz#378 — self-update 재설계 | cxz#374 |

cxz#372와 cxz#377은 선행이 없어 병렬로 시작할 수 있다. cxz#376은 cxz#311·cxz#312를 먼저 고친다.

## 10. 열린 질문

- 네트워크 CSI 전제가 운영 환경에서 성립하는가. 성립하지 않으면 scale-down의 가치가 크게 줄어든다.
- 프로젝트 자원 요청을 devcontainer 설정에 적는가, cxz 프로젝트 설정에 적는가. 전자는 저장소에 커밋되고 후자는 설치에 남는다.
- compose 기반 프로젝트를 쓰고 있던 사용자에게 k8s 백엔드가 무엇을 답하는가. 거부 메시지가 무엇을 하라고 말해야 하는가.
- cold start가 몇 초인가. "잠깐 내린다"가 감당하는 지연의 상한은 얼마인가.
