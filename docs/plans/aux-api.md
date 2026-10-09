# Aux: 보조 AI가 쓰는 리소스와 API

상태: 1·2단계 구현됨 — 타입 있는 API, 매니저 소유 저장, 그리고 push. 사용 방법은
[보조 AI 작업](../auxiliary-ai.md), 설계 배경은
[보조 AI 작업: 인증과 제한된 세션 문맥](auxiliary-ai.md)을 참고한다. 아래는
구현된 모양이며, 초안과 달라진 곳은 "초안에서 달라진 것"에 모아 적었다.

보조 AI 기능에는 전용 API가 없다. 클라이언트는 `Project.Docker`에
`action: "auxiliary"`와 JSON `spec`을 실어 보낸다. 이 문서는 그 자리에 들어갈
리소스 `Aux`와 `AuxService`의 모양, 그리고 클라이언트별 사용법을 적는다.

## 지금 무엇이 문제인가

`DockerInput`은 `{string action, bytes spec}` 한 줄이다
(`internal/runtimeproto/cxz.proto:126`). 원래 뜻은 프로젝트 컨테이너가 공유하는
dind 엔진 관리(`cxz docker up/down/status/sync`)였고, 지금은 proto 메세지를 따로
만들지 않은 모든 기능의 봉투다. 리소스 API에도 그대로 노출된다
(`proto/ext/cxz/project_svc.ext.proto:28`).

보조 AI는 그 봉투에 실린 것 중 가장 큰 승객이고, 네 가지 증상을 모두 보인다.

- **전송 계층이 payload를 뜯는다.** 어느 연결로 보낼지 알려고 `spec`을 JSON
  파싱해 `.Session`을 꺼내고 다시 직렬화한다 (`internal/multiclient/routes.go:19`).
- **응답을 스니핑해서 부작용을 낸다.** Docker 응답을 `auxiliary.Reply`로 해석해
  `Title`이 있으면 `Session.Patch`로 세션 이름을 바꾼다
  (`server/lifecycle/project.go:266`).
- **권한이 메서드 단위가 아니다.** `ProjectServer.Docker`는 트랜잭션 금지
  검사(`effect()`)만 하고, 실제 권한 제한은 핸들러가 기억했을 때만 들어간다.
- **리소스가 틀렸다.** 세션에 대한 작업이 `ProjectService`에 있다. `SessionService`는
  `Restore/Memory/Logs/Transcript/Search/EventDetails/Models`까지 전부 타입이 있고
  이것만 예외다.

그리고 `auxiliary` 안에는 2차 디스패처가 있다 — `list/put/models/session/title/
status/forget/cancel/login-info` (`internal/workspace/auxiliary.go`). 기능을 하나
더하면 enum 값이 아니라 문자열이 하나 더 늘어난다.

## 무엇을 리소스로 볼 것인가

보조 AI를 "부모 세션에 달린, 컨텍스트 크기가 제한된 임시 대화 세션"으로 모델링하는
방안을 검토했다. **그 임시 컨텍스트는 이미 존재하고, 세션으로 만들 대상은 그것이
아니다.**

### 임시 컨텍스트는 이미 부모당 하나다

`auxiliary.State`가 부모 세션당 롤링 컨텍스트를 들고 있다.

| 필드 | 뜻 |
| --- | --- |
| `Recent []Turn` | 최근 완료 턴 (`RecentLimit = 20KiB`, 32턴) |
| `Checkpoint`, `Through` | 넘치면 `task:"checkpoint"` 호출로 압축하고 오래된 턴을 버린다 (`controller.go:319`) |
| `Gap` | 저장 한계로 턴이 빠졌음을 프롬프트에 명시한다 |

compaction 있는 제한된 대화 상태 그 자체다. 새로 만들 것이 아니라 이름을 줄 대상이다.

### 그 컨텍스트는 kind별이 아니다

요약과 추천은 **같은 컨텍스트를 읽는다**. 프로필이 같으면 호출 한 번으로 둘을 받고
(`task:"combined"`, `controller.go:350`), 다르면 요약을 먼저 받아 **추천 호출의
컨텍스트로 넣어준다** (`controller.go:375`). kind마다 보조 세션을 따로 만들면 이 두
가지를 표현할 수 없고, 호출 수·토큰·usage 기록이 그대로 2배가 된다.

### 공급자 쪽에는 세션이 없다

`runAuxiliary`는 `docker run --rm -i`로 일회용 컨테이너를 띄워 stdin에
`Input{Profile, Task, Text}` 하나를 주고 stdout에서 `Output` 하나를 받는다
(`internal/auxiliary/helper.go:21`). 멀티턴도, 재개도, 공급자 세션도 없다. 세션성은
전부 cxz가 들고 있는 롤링 컨텍스트뿐이다.

### `Session` 타입 재사용의 비용

`Session`은 `project` 엣지 필수·immutable, `agent`/`model`, unique한
`runtime_id`/`client_id`, `status{state, run_id, last_seq, pending,
permission_mode, queued}`, payday CRUD/list/watch를 뜻한다
(`proto/cxz/session.proto:14`). 보조 작업은 이 중 아무것도 쓰지 않는다.

더 중요한 건 세션을 순회하는 코드 전부에 "kind가 `aux.*`면 건너뛰기"가 붙는다는
것이고, 그중 하나가 보조 AI 자신이다. `StartAuxiliary`는 `client.List`로 세션을 돌며
`turn_end`를 보고 작업을 띄운다. 보조 세션이 `Session`이면 **보조 세션의 턴이 또
보조 작업을 띄우는 재귀**가 기본 동작이 되고, 그걸 조건문으로 막아야 한다. 설계로
불가능해야 하는 것을 검사로 막는 모양이다.

덧붙여 `Send/Interrupt/Permission/UpdateAgent/Attach/Upload`를 거의 전부 거부해야
하고, 지금 부모 purge와 함께 사라지는 생애(`Controller.Purge`/`Forget`)가 독립
최상위 리소스가 되면서 따로 지워질 수 있게 된다.

### 결론

묶는 단위는 임시 세션이 아니라 **부모에 달린 보조 작업(job)**이다. `Job{ID, Session,
Run, Turn, Revision, Status, Usage, Error}`가 이미 그 모양이고 `Status`는
`queued/running/completed/failed/stale/canceled`로 **작업의 생애**를 적는다 — 대화의
생애가 아니다. `kind`로 묶자는 방향은 채택하되, 붙는 자리가 세션이 아니라 이 작업이다.

## 이름

리소스 이름은 **`Aux`**다. 리소스와 서비스 이름을 맞추는 기존
규칙(`Project`/`ProjectService`, `Session`/`SessionService`)을 따르면 서비스는
`AuxService`가 되는데, 그 서비스는 아직 없다 — 아래 "서비스는 엔티티를 전제한다"를
참고한다.

리소스 이름에 기능 이름을 쓰지 않는다. 보조 AI는 이 리소스를 **써서** 구현되는
기능이고, 이 리소스 자체가 아니다. 공유 메모리
compact([#36](https://github.com/lesomnus/cxz/issues/36))처럼 같은 물건을 쓰는 다음
기능이 생겼을 때 "그건 보조 AI인가?"라는, 답할 필요가 없는 질문이 생기지 않게 한다.

용어집에 넣을 정의:

| | |
|---|---|
| **Aux** | 세션에 딸려 한 번 수행되고 끝나는 모델 작업. 문맥은 cxz가 구성해서 넘기고, 대화로 쌓이지 않는다. 요약·추천·제목은 Aux의 kind다. 공급자 호출 한 번을 뜻하지는 않는다 — 한 Aux가 문맥 압축을 포함해 여러 번 호출할 수 있다. |

이 정의가 위 질문에 답한다. 메모리 compact도 cxz가 문맥을 구성해 한 번 수행하는
모델 작업이므로 Aux의 kind다. 반대로 사용자가 보낸 입력으로 턴이 쌓이는 것은 Aux가
아니다. 그것은 Session이다.

**기능 이름으로서의 "보조 AI(auxiliary)"는 그대로 남는다**: `cxz ai`, 설정 화면의
AI tasks, `ProjectService.AuxiliaryLogin`(보조 계정 프로필 로그인),
`internal/auxiliary` 패키지. 두 이름은 다른 층을 가리킨다 — `Aux`는 무엇을 실행하는
단위이고, "보조 AI"는 그것으로 만든 사용자 기능이다.

## 서비스는 엔티티를 전제한다

payday에서 **서비스는 엔티티에서 생성된다.** `resource.Server`는 엔티티마다 메서드
하나를 갖고(`resource/store.g.go:28`), `proto/ext/cxz/*.ext.proto`는 그렇게 생성된
계약에 RPC를 얹는 overlay다. 엔티티 없는 서비스를 선언하면 생성기가 그대로 말한다:

```
pd: proto/ext/cxz/aux_svc.ext.proto extends a contract that does not exist,
    so it is never merged: nothing generates cxz/aux_svc.g.proto
```

그래서 `AuxService`는 `Aux`를 payday 엔티티로 올리는 것과 같은 일이고, 그것은
ent 테이블과 보존 정책을 뜻한다 — 아래 "저장과 단계"의 2단계다. 1단계에서 테이블을
만들어 두고 쓰지 않는 것은 아무것도 쓰지 않는 테이블을 남기는 일이라 하지 않았다.

**1단계는 기존 계약에 RPC를 얹는다.** 세션에 속한 것은 `SessionService`에,
설치에 속한 것은 `ProjectService`에 둔다. `Aux`는 그 RPC들이 주고받는 타입으로
존재한다. 2단계에서 엔티티가 되면 이 RPC들은 `AuxService`로 옮겨가며, 타입과
이름은 그대로다.

## API 모양

### 종류

`Task` 문자열이 지금 사실상 kind지만 축이 섞여 있다. 분리해서 적는다.

| 지금 `Task` | 분류 | API에서 |
| --- | --- | --- |
| `summary` | 산출 kind | `AUX_KIND_SUMMARY` |
| `suggestion` | 산출 kind | `AUX_KIND_SUGGESTION` |
| `title` | 산출 kind (cadence가 다름) | `AUX_KIND_TITLE` |
| `combined` | 한 Aux가 kind 둘을 산출하는 최적화 | kind 아님. `kinds` 복수로 표현 |
| `checkpoint` | 컨텍스트 내부 압축 | kind 아님. 노출하지 않음 |
| `models` | 계정 능력 조회 | kind 아님. 별도 RPC |

### `proto/ext/cxz/session_svc.ext.proto` — 세션에 속한 것

네 호출이 모두 같은 것으로 답한다. 작업을 요청하는 것과 지난 작업이 무엇을
만들었는지 읽는 것은 같은 질문을 두 시점에 하는 것이고, 방금 요청한 호출자는 왕복
한 번을 더 하지 않고 답을 원한다.

```proto
service SessionService {
  rpc AuxRun(AuxRunRequest) returns (AuxState);
  rpc AuxStatus(AuxStatusRequest) returns (AuxState);
  rpc AuxPrefer(AuxPreferRequest) returns (AuxState);
  rpc AuxCancel(AuxCancelRequest) returns (AuxState);
}

// kinds는 요청이고 results는 산출이다. 그래서 한 작업이 kind 둘을 답할 수 있고 --
// 요약과 추천이 프로필을 공유할 때 모델 호출 한 번이 둘을 답하는 것이 그것이다 --
// state가 running인 동안에도 results가 채워질 수 있다. 요약은 옆의 추천이 아직
// 생성되는 중에 공개되기 때문이다.
message Aux {
  string id = 1; bytes session_id = 2;
  repeated AuxKind kinds = 3;
  string run_id = 4; uint64 turn = 5;   // 이 작업이 답한 턴
  string revision = 6;                  // 설정이 바뀌면 stale이 된다
  string state = 7; // queued, running, completed, failed, stale, canceled
  repeated AuxResult results = 8;
  repeated AuxUsage usage = 9;
  string message = 10;
}
message AuxResult { AuxKind kind = 1; string text = 2; bool truncated = 3; }
message AuxUsage { string account = 1; string model = 2; AuxKind kind = 3; bytes data = 4; }
message AuxSummary { string run_id = 1; uint64 turn = 2; string text = 3; }
// since는 읽기 전용이다: kind를 켜는 것이 이미 지나간 대화를 소급하지 않는다.
message AuxPreference { AuxKind kind = 1; bool enabled = 2; google.protobuf.Timestamp since = 3; }
message AuxState {
  repeated AuxSummary summaries = 1;
  Aux current = 2;
  repeated AuxPreference preferences = 3;
  string title = 4;   // 세션 이름을 기록하는 호출자를 위해
  string message = 5;
}
message AuxRunRequest { SessionRef ref = 1; repeated AuxKind kinds = 2; string text = 3; }
message AuxStatusRequest { SessionRef ref = 1; uint64 after_turn = 2; int32 limit = 3; }
message AuxPreferRequest { SessionRef ref = 1; repeated AuxPreference preferences = 2; }
message AuxCancelRequest { SessionRef ref = 1; string aux_id = 2; }
```

### `proto/ext/cxz/project_svc.ext.proto` — 설치에 속한 것

어떤 kind를 어떤 계정·모델로 돌리는지는 한 세션의 것이 아니라 설치의 설정이므로,
나머지 설치 설정이 이미 있는 곳에 둔다.

```proto
service ProjectService {
  rpc AuxConfig(AuxConfigRequest) returns (AuxConfigReply);
  rpc AuxSetConfig(AuxSetConfigRequest) returns (AuxConfigReply);
  rpc AuxModels(AuxModelsRequest) returns (AuxModelsReply);
  rpc AuxLoginInfo(AuxLoginInfoRequest) returns (AuxLoginInfoReply);
}

// kind를 하나 더하는 것은 RPC가 아니라 여기 값 하나를 더하는 일이다.
enum AuxKind {
  AUX_KIND_UNSPECIFIED = 0;
  AUX_KIND_SUMMARY = 1;
  AUX_KIND_SUGGESTION = 2;
  AUX_KIND_TITLE = 3;
}
// kind별이다. 두 kind가 프로필을 공유하는지가 한 호출로 둘을 답할 수 있는지를
// 결정한다. agent와 backend는 읽기 전용 -- 등록된 계정에서 읽고, 호출자에게서
// 받지 않는다.
message AuxProfile {
  AuxKind kind = 1; bool enabled = 2; string account = 3; string agent = 4;
  string backend = 5; string model = 6; string effort = 7;
  google.protobuf.Timestamp since = 8;  // 읽기 전용
}
message AuxConfigRequest {}
message AuxSetConfigRequest { repeated AuxProfile profiles = 1; }
message AuxConfigReply {
  string revision = 1; repeated AuxProfile profiles = 2;
  string message = 3; string owner = 4;
}
// 등록된 계정 하나의 목록 조회. 응답을 생성하지 않으며, 저장된 토큰이 생성에
// 받아들여진다는 증명도 아니다.
message AuxModelsRequest { string account = 1; }
message AuxModelsReply { repeated AuxModel models = 1; bool needs_login = 2; }
// 로그인을 어디서 어떤 프로필로 해야 하는지만 말한다. 자격증명은 담지 않는다.
message AuxLoginInfoRequest { string account = 1; }
message AuxLoginInfoReply {
  string owner = 1; string account = 2; string agent = 3; string backend = 4;
}
```

### `internal/runtimeproto/cxz.proto` — 내부 표면

런타임 API(서버↔매니저)에도 같은 호출들이 있다. 두 가지가 다르다.

- **kind가 문자열이다.** 이 API의 다른 모든 kind가 그렇고, 공개 표면이 아니므로
  모르는 kind를 거부하는 일은 앞단의 enum이 이미 한다.
- **`AuxForget`이 여기에만 있다.** 세션을 지울 때 리소스 서버가 매니저에게 보내는
  호출이고(`server/lifecycle/delete.go` `forgetAuxiliary`), 클라이언트가 부를 일이
  없다. 리소스 클라이언트에서 호출하면 조용히 성공하지 않고 거부한다 — 보조 상태를
  지우려던 호출자에게 "됐다"고 답하면 남은 상태가 그대로 남는다.

`internal/auxkind`가 둘 사이를 옮긴다. 모르는 kind에는 이름이 없고, 그래서 위
계층이 추측하지 않고 거부할 수 있다.

## 어떻게 쓰는가

### 자동 요약 — 클라이언트는 트리거하지 않는다

생성 트리거는 클라이언트에 없다. 매니저가 자기 폴링 루프에서 세션 저널을
관찰하고(`Observe`), `turn_end`를 보면 Aux를 띄운다. 클라이언트가 하나도 붙어 있지
않아도 생성되고, 여럿 붙어 있어도 두 번 돌지 않는다 — 세션당 상태 파일 하나,
뮤텍스 하나, 진행 중 작업 한 칸(`c.active[id]`).

따라서 클라이언트가 하는 일은 **읽기**다.

```
Session.AuxStatus{ref: <session>}
  → summaries: [{run_id, turn, text}, ...]
    current: {state: "running", kinds: [SUMMARY, SUGGESTION], results: [{kind: SUMMARY, ...}]}
    preferences: [{kind: SUMMARY, enabled: true, since: ...}, ...]
```

TUI는 이것을 1초에 한 번 이하로 폴링하고(`internal/tui/auxiliary.go:171`),
`summary.run_id == event.run_id && summary.turn == event.seq`로 해당 턴 옆에
그린다. 웹 클라이언트도 같은 응답을 같은 방식으로 읽는다 — 생성·중복 방지·저장이
모두 매니저에 있으므로 클라이언트가 늘어도 응답은 동일하다.

### 한 번만 생성

```
Session.AuxRun{ref: <session>, kinds: [SUMMARY]}  → AuxState{current: {state: "queued"}}
Session.AuxStatus{ref: <session>}                  → AuxState{current: {state: "completed", results: [...]}}
```

`AuxRun`은 자동 실행 설정을 바꾸지 않는다. 자동이 켜져 있는 kind를 `AuxRun`하면
아무 일도 하지 않는다 — 지금 `/summary`의 동작과 같다.

### 자동 실행 켜고 끄기

```
Session.AuxPrefer{ref: <session>, preferences: [{kind: SUGGESTION, enabled: true}]}
Project.AuxSetConfig{profiles: [{kind: SUGGESTION, enabled: true, account: ..., model: ...}]}
```

앞은 이 세션만, 뒤는 설치 기본값이다. 설치 기본값을 바꾸면 `revision`이 올라간다. 올라간
revision으로 진행 중인 Aux는 `stale`이 되어 결과를 버린다. 바뀐 설정의 결과가 아니기
때문이고, 이 규칙은 현재 구현(`cfg.Revision != c.config.Revision`)과 같다.

### 취소

```
Session.AuxCancel{ref: <session>}  → AuxState{current: {state: "canceled"}}
```

`aux_id` 없이 부르면 그 세션의 진행 중인 Aux를 취소한다. 요약과 추천이 한 호출로
묶인 Aux면 둘 다 멈춘다 — 지금도 그렇다.

### 설정과 로그인

```
Project.AuxModels{account: "work1"}     → models: [...] 또는 needs_login
Project.AuxSetConfig{profiles: [{kind: SUMMARY, enabled: true, account: "work1", model: ..., effort: ...}]}
Project.AuxLoginInfo{account: "work1"}  → {owner, account, agent, backend}
```

요청은 계정 이름만 보낸다. 그 계정이 무엇으로 인증하는지(agent, backend)는 서버가
등록된 Account 리소스에서 읽는다 — 그래야 요청이 한 계정을 지목하면서 다른 계정으로
인증하는 일이 없다.

로그인 자체는 지금처럼 `ProjectService.AuxiliaryLogin` 스트림을 쓴다. 이 설계가
바꾸지 않는다.

### CLI 매핑

`cxz ai`의 표면은 그대로 두고 뒤에서 부르는 것만 바뀐다.

| 명령 | 전에 | 지금 |
| --- | --- | --- |
| `cxz ai list` | `Docker{auxiliary, {action:"list"}}` | `Project.AuxConfig{}` |
| `cxz ai set KIND --account --model` | `{action:"put"}` | `Project.AuxSetConfig{profiles:[...]}` |
| `cxz ai disable KIND` | `{action:"put"}` (enabled 없음) | `Project.AuxSetConfig{profiles:[{enabled:false}]}` |
| `cxz ai models ACCOUNT` | `{action:"models"}` | `Project.AuxModels{account}` |
| `cxz ai status SESSION` | `{action:"status"}` | `Session.AuxStatus{ref}` |
| `cxz ai cancel SESSION` | `{action:"cancel"}` | `Session.AuxCancel{ref}` |
| `cxz ai title SESSION [--text]` | `{action:"title"}` | `Session.AuxRun{ref, kinds:[TITLE], text}` |
| `cxz ai login ACCOUNT` | `{action:"login-info"}` + 스트림 | `Project.AuxLoginInfo{account}` + 기존 스트림 |
| TUI `/summary on\|off` | `{action:"session"}` + enabled | `Session.AuxPrefer{ref, preferences}` |
| TUI `/summary` | `{action:"session"}` | `Session.AuxRun{ref, kinds}` |
| TUI 폴링 | `{action:"status"}` | `Session.AuxStatus{ref}` |
| (내부) `{action:"forget"}` | 세션 purge 경로 | 런타임 API `AuxForget` |

`forget`은 클라이언트가 부르지 않는다. 리소스 서버가 세션을 지울 때 매니저에게
보내는 호출이다 (`server/lifecycle/delete.go:116` `forgetAuxiliary`). 그래서 표면이
둘로 갈린다.

- **리소스 API**(`proto/ext/cxz/*`, 클라이언트가 보는 것)에서 `auxiliary` action은
  사라진다. 이 문서가 설계하는 것이 그 자리다.
- **런타임 API**(`internal/runtimeproto`, 서버↔매니저)는 `forget`처럼 클라이언트가
  볼 일 없는 호출을 계속 가진다. 거기서도 타입은 받았지만(`AuxForget`), 공개 표면이
  아니므로 kind는 문자열로 둔다.

## 현재 동작을 그대로 재현하는가

문맥 보존과 combined 호출은 **API에 나타나지 않는다**. 둘 다 매니저가 하는 결정이고,
API는 무엇을 요청했고 무엇이 나왔는지만 말한다. 그래서 판단 기준은 설계가 그 동작을
허용하는지가 아니라 **간섭하지 않는지**다.

| 현재 동작 | 어디서 결정 | API에 보이는 것 |
| --- | --- | --- |
| 제한된 롤링 문맥 (`Recent` 20KiB·32턴) | 매니저 내부 | 없음. API는 문맥을 보내지도 받지도 않는다 |
| 넘치면 `checkpoint`로 압축 | 매니저 내부 | 없음. kind도 아니다 |
| 프로필이 같으면 요약·추천을 한 호출로 | 매니저가 프로필 비교로 결정 | Aux 하나에 `kinds:[SUMMARY,SUGGESTION]`, `results` 둘 |
| 프로필이 다르면 추천이 요약을 문맥으로 받음 | 매니저 내부 | 같은 모양. 공급자 호출 횟수를 노출하지 않는다 |
| 요약을 먼저 공개하고 추천은 계속 실행 | — | `state:"running"` + `results:[SUMMARY]` (부분 결과) |
| 새 입력이 오면 진행 중 결과를 버림 | — | `state:"stale"`, `results` 비움 |
| 설정이 바뀌면 진행 중 결과를 버림 | — | `revision` + `state:"stale"` |
| 켠 시점 이후의 턴만 대상 | — | `AuxPreference.since` (설치 기본값·세션 양쪽) |
| 작업 중 다른 요청이 오면 뒤에 대기 (`PendingTask`) | 매니저 내부 | 끝난 뒤 새 Aux가 이어서 나타난다 |
| 턴별 요약 누적, 히스토리에 보관되는 사본은 4KiB로 제한 | — | `AuxSummary` 목록 |
| 한계에서 잘린 산출물을 버리지 않고 표시 | — | `AuxResult.truncated` |
| 계정별 직렬 실행, 프로필 잠금 | 매니저 내부 | `state:"queued"` |
| 제목 자동 생성 → 세션 이름 | — | `AUX_KIND_TITLE` 결과 + `Session.Patch` |
| 모델 목록에 로그인이 필요함 | — | `AuxModelsReply.needs_login` |
| 설정 저장 후 사람이 읽는 한 줄 | — | `AuxConfig.message` |
| 세션 purge 시 보조 상태 삭제 | 서버→매니저 | 리소스 API에 없음 (위 참고) |

재현되지 않는 동작은 찾지 못했다. 다만 초안에 빠져 있던 것 세 개를 채웠다:
`AuxPreference.since`(없으면 켤 때 지난 대화를 소급 실행한다),
`AuxConfig.message`(지금 `Reply.Message`가 TUI에 보여주는 문장),
그리고 `AuxProfile.kind`(kind별 프로필이 없으면 combined 여부를 결정할 수 없다).
`Aux.results`가 `running` 상태에서도 채워진다는 것도 명시했다 — 그렇지 않으면 추천을
기다리는 동안 요약이 보이지 않는다.

## 저장과 단계

**1단계 — 타입만 (구현됨).** 저장은 지금 그대로다. 보조 상태는 매니저 state의
`<aux root>/<sha256(session)>.json`에 남고, Aux는 세션당 최신 하나
(`State.Job`), 요약은 턴마다 `State.Summaries`에 쌓인다. 바뀌는 것은 전송 표면뿐이고,
`Project.Docker`에서 `auxiliary` action이 사라진다.

**2단계 — 매니저가 소유하는 저장과 push (구현됨).** 이력과 질의, 그리고 폴링 제거가
목표였다.
payday 엔티티가 아니라 **매니저 자체 DB + 스트림 중계**로 한다. 이유는 아래
"왜 payday 엔티티가 아닌가"에 적었다.

- 저장: `<aux root>/aux.db` — `tasks`(+`results`)와 `summaries`. 작업은 턴마다 행으로
  남고, 요약은 턴당 하나로 갱신된다. 롤링 문맥(`Recent`, `Checkpoint`, `Through`,
  `Seen`, `Gap`)과 **진행 중인 작업**은 JSON에 남는다 — 그것은 리소스가 아니라 작업
  상태이고, 매 턴 다시 쓰인다. 파일은 그 작업의 기록이고 DB는 **무엇이 실행됐는가의
  기록**이다: `save`가 둘을 같은 락 안에서 쓴다.
- push: `AuxEvents`가 `AuxState`를 스트림으로 보낸다. 컨트롤러가 쓰는 자리에서
  깨우고(`wake`), 리소스 층이 `Events`와 같은 방식으로 중계한다. TUI의 1초 폴링이
  사라졌고 웹도 같은 것을 받을 수 있다. 깨움은 "읽을 것이 생겼다"는 신호일 뿐이므로,
  이미 대기 중인 깨움이 있으면 버린다 — 읽는 쪽이 세션 상태 전체를 다시 읽기 때문에
  그 읽기가 이번 변경까지 포함한다.
- 보존: 세션당 작업 200개, 요약 200턴, 그리고 세션 256개 — 마지막 것은 옆의 문맥
  파일 상한과 같고, 파일 쪽 정리가 할 수 없는 일이다: 그 파일들은 세션의 해시로
  이름이 붙어 있어 정리된 이름은 어느 세션이었는지 말해주지 않는다.
- 세션이 지워지면 그 세션의 행도 지워진다 — `Forget`/`Purge`가 그 자리다.

### 왜 payday 엔티티가 아닌가

엔티티 선언은 코드 생성을 주지만 **저장을 강제하지 않는다**. `migrate`가 생성된
스키마를 모두 만들기 때문에(`server/lifecycle/stack.go:67`) 테이블은 생기지만, CRUD
RPC를 켜지 않으면 아무도 그것을 통하지 않는다. 그래서 "선언만 하고 저장은 따로"가
가능하다 -- 다만 그러면 **아무도 쓰지 않는 테이블**이 남고, 나중에 누가 CRUD를 켜면
JSON과 어긋날 자리가 된다.

그리고 엔티티화의 상품이던 `watch`는 그 테이블에 묶여 있다. 생성된 `Watch`는
`watchRead`로 **행을 읽고**, 쓰기 때 recorder가 publish한 것을 구독한다
(`server/pd/pd.g.go:538`). 저장이 테이블 밖에 있으면 읽을 행도, publish할 쓰기도
없다. 즉 `watch`는 "엔티티를 선언하면" 따라오는 것이 아니라 "리소스 DB에 쓰면"
따라온다.

**push는 payday watch만의 것이 아니다.** 리소스 층은 이미 런타임 스트림을 그대로
중계한다 -- 저널 이벤트(`SessionService.Events` → `runtime.Watch`,
`server/lifecycle/session.go:336`)와 검색(`Search`, 같은 파일 347)이 그렇고, 둘 다
리소스 DB를 거치지 않는다. 클라이언트가 보는 서버와 보조 컨트롤러가 **같은
프로세스**이기 때문이다: `lifecycle.Runtime`은 `api.SessionsServer`를 임베드한 Go
인터페이스이고(`server/lifecycle/stack.go:28`), 구현이 `internal/server.Server`다.
런타임 API가 실제 gRPC로 쓰이는 구간은 매니저 → 프로젝트 컨테이너 하나뿐이다.

그래서 매니저 자체 DB를 쓰면:

| | payday 엔티티 (리소스 DB) | 매니저 자체 DB + 스트림 중계 |
|---|---|---|
| 이력·질의 | 얻는다 | 얻는다 |
| push | `watch` | `AuxEvents` 중계 |
| 쓰는 주체 | 리소스 층 — 매니저는 리소스를 쓰지 않으므로 층을 뒤집거나 30초 reconcile에 의존해야 한다 | 매니저가 자기 DB를 직접 쓴다 |
| 백업·보존 | `resources.db`에 포함 | 매니저 상태 볼륨, 자체 정책 |
| 추가로 얻는 것 | audit 행, 필터 있는 watch, 생성된 list | — |
| 추가로 드는 것 | 쓰기 경로 결정, 보존 정책, 대화 텍스트가 리소스 DB로 | 마이그레이션·보존을 직접 쓴다 |

레포에 이 모양의 선례가 있다. `conversations.db`(convindex)가 매니저 소유의 파생
SQLite이고, `cxz.db`가 런타임 레지스트리·이벤트 캐시다. 매니저가 소유하는 파생
데이터는 이 집에서 매니저 자체 SQLite로 둔다.

리소스 DB가 추가로 주는 것(audit, 필터 있는 watch)은 지금 필요한 것이 아니다.
필요해지면 그때 옮기면 되고, 그때도 타입과 RPC 이름은 그대로다.

### 생성은 클라이언트에 의존하지 않는다 — 기록 시점만 문제였다

매니저의 5초 루프(`StartAuxiliary`)가 저널을 관찰해 작업을 띄우므로, 클라이언트가
하나도 없어도 요약은 생성된다. 어느 선택지도 이것을 바꾸지 않는다.

문제가 되는 것은 **기록이 언제 쓰이는가**다. 클라이언트가 매니저까지 읽어가는 대신
행을 구독하게 되면, 아무도 접속하지 않은 동안 만들어진 요약은 무언가가 행을 쓴
뒤에야 보인다. 리소스 DB라면 그 "무언가"가 리소스 층이고, 그래서 30초
reconcile(`internal/server/server.go:239`)에 의존하면 진행 중인 턴의 요약이 최대
30초 늦는다. 세션 제목이 지금 정확히 그렇게 동작한다 -- 클라이언트가 물어볼 때나
reconcile이 돌 때 기록된다.

매니저가 자기 DB를 직접 쓰면 이 문제가 없다. 작업이 끝나는 그 자리에서 쓰이고,
같은 자리에서 알린다.

주의: 보조 산출물은 저널 이벤트가 **아니다**. 저널의 이벤트 kind에
`summary`/`suggestion`/`auxiliary`는 없고, 보조 컨트롤러는 저널을 읽기만 한다.
따라서 요약은 저널에서 재구축되지 않는다. 2단계로 그것이 백업 가능한 DB로
옮겨갔지만 — 매니저 상태 볼륨 안이다 — 재구축 불가라는 성질 자체는 그대로다.

## 하위호환

사용자 배포 전이므로 `Project.Docker`의 `auxiliary` action에 호환 경로를 남기지
않았다. 봉투를 비워 두면 비울 이유가 없어진다. `DockerInput` 자체는
`save/up/down/prune/status`에 남는다 — 그 메세지가 실제로 뜻하는 것이다.

## 초안에서 달라진 것

구현하면서 바뀐 것들과, 왜 바꿨는지.

- **`AuxService`가 없다.** payday에서 서비스는 엔티티에서 생성된다. 위 "서비스는
  엔티티를 전제한다"를 참고한다. RPC는 `SessionService`와 `ProjectService`에 얹혀
  있고, 2단계에서 옮겨간다.
- **세션 호출 네 개가 모두 `AuxState`로 답한다.** 초안은 `Run`/`Cancel`이 `Aux`를
  돌려주게 했는데, 그러면 제목을 요청한 호출이 돌려줄 것이 없다 — 제목은 작업의
  생애가 아니라 세션에 기록되는 것이기 때문이다. 그리고 방금 요청한 클라이언트는
  곧바로 상태를 다시 읽는다. 한 응답 타입이 그 왕복을 없앤다.
- **`Get`이 없다.** 세션당 작업은 최신 하나뿐이므로 `AuxStatus`가 그 역할을 한다.
  작업 이력은 2단계의 것이다.
- **`AuxModels`/`AuxLoginInfo`가 프로필이 아니라 계정 이름을 받는다.** 두 호출에
  kind는 의미가 없고, agent·backend는 서버가 등록된 Account에서 읽는다. 프로필을
  받으면 호출자가 채울 수 없는 필드를 가진 메세지를 보내게 된다.
- **`AuxProfile.kind`가 추가됐다.** 없으면 kind별 프로필을 비교할 수 없고, 그 비교가
  한 호출로 두 kind를 답할 수 있는지를 결정한다.
- **`since`는 읽기 전용이다.** 경계를 정하는 것은 서버이고(켠 시각), 클라이언트는
  그것을 읽어 "무엇부터 적용되는지"를 말할 뿐이다.
- **`AuxConfigReply.message`와 `AuxState.message`가 있다.** 지금 설정 화면과
  알림줄에 뜨는 문장을 그대로 유지한다.
- **`AuxState.preferences`는 보고된 것만 기록한다.** 선호를 말하지 않은 응답은
  "선호가 꺼졌다"고 말한 응답이 아니다. 비어 있으면 클라이언트가 가진 것을 유지한다.

## 열어 두는 질문

- **대화형 보조.** "이 대화에 대해 보조 AI에 직접 질문하기"가 생기면 그것은 턴이
  쌓이는 진짜 스레드이고, `AuxThread`(messages를 가진 별도 타입, 역시
  `Session`은 아니다)가 맞다. 그때도 요약·추천은 Aux다. Aux로 설계해도 이 길이
  막히지 않는다.
- **요약을 검색 대상에 넣을지.** 대화 색인은
  `input/assistant/tool_call/tool_output/tool_result`만 넣으므로 요약은 검색되지
  않는다. 요약은 *대화에 대해 모델이 쓴 글*이라 히트에 섞이면 아무도 쓰지 않은
  문장이 결과에 나온다. 넣는다면 출처를 구분해 표시해야 한다.
- **요약 검색은 2단계 다음에 쉬워진다.** 요약이 행이 되면 색인에 넣는 일이 파일을
  여는 일이 아니라 질의가 된다. 넣을지 말지는 여전히 위의 질문이다.
- **`info` 응답 분리.** `Docker{action:"info"}`는 엔진 정보·히스토리 정책·cxz
  버전/채널/핀을 한 응답에 섞는다. 이 설계와 독립적이지만 같은 성질의 문제다.

## 비목표

- 보조 AI의 동작·프롬프트·품질을 바꾸지 않는다. 1단계는 전송 표면만, 2단계는
  저장과 알림만 바꾼다.
- 인증 모델을 바꾸지 않는다. 프로필 분리, grant 범위, 프로필 잠금은 그대로다.
- 보조 실행을 프로젝트 컨테이너 안에서 구동할 수 있게 만들지 않는다.
- 자격증명·grant·토큰을 어떤 응답에도 담지 않는다.
