# 보조 AI 작업 API

상태: 설계 제안. 구현 없음. 현재 동작은 [보조 AI 작업](../auxiliary-ai.md),
설계 배경은 [보조 AI 작업: 인증과 제한된 세션 문맥](auxiliary-ai.md)을 참고한다.

보조 AI 기능에는 전용 API가 없다. 클라이언트는 `Project.Docker`에
`action: "auxiliary"`와 JSON `spec`을 실어 보낸다. 이 문서는 그 자리에 들어갈
타입 있는 API의 모양과 사용법을 적는다.

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
`queued/running/completed/failed/stale/canceled`로 **잡의 생애**를 적는다. `kind`로
묶자는 방향은 채택하되, 붙는 자리가 세션이 아니라 잡이다.

## API 모양

### 종류

`Task` 문자열이 지금 사실상 kind지만 축이 섞여 있다. 분리해서 적는다.

| 지금 `Task` | 분류 | API에서 |
| --- | --- | --- |
| `summary` | 산출 kind | `AUXILIARY_KIND_SUMMARY` |
| `suggestion` | 산출 kind | `AUXILIARY_KIND_SUGGESTION` |
| `title` | 산출 kind (cadence가 다름) | `AUXILIARY_KIND_TITLE` |
| `combined` | 한 잡이 kind 둘을 산출하는 최적화 | kind 아님. `kinds` 복수로 표현 |
| `checkpoint` | 컨텍스트 내부 압축 | kind 아님. 노출하지 않음 |
| `models` | 계정 능력 조회 | kind 아님. 별도 RPC |

### `proto/cxz/auxiliary.proto`

리소스 엔티티가 아닌 메세지로 시작한다. 저장 위치는 아래 "저장과 단계"를 따른다.

```proto
edition = "2023";
package cxz;
import "cxz/session_svc.g.proto";
import "google/protobuf/timestamp.proto";
option features.field_presence = IMPLICIT;
option go_package = "github.com/lesomnus/cxz/resource";

enum AuxiliaryKind {
  AUXILIARY_KIND_UNSPECIFIED = 0;
  AUXILIARY_KIND_SUMMARY = 1;
  AUXILIARY_KIND_SUGGESTION = 2;
  AUXILIARY_KIND_TITLE = 3;
}

// 보조 AI 실행에 쓰는 계정과 모델. 세션 자격증명을 복사하지 않는다.
message AuxiliaryProfile {
  bool enabled = 1; string account = 2; string agent = 3;
  string backend = 4; string model = 5; string effort = 6;
}

// 한 턴에 대한 한 번의 실행. kinds가 복수인 것이 핵심이다: 같은 프로필이면
// 요약과 추천을 호출 한 번으로 받고, 그 사실이 잡 하나로 기록된다.
message AuxiliaryJob {
  string id = 1;
  SessionRef parent = 2;
  repeated AuxiliaryKind kinds = 3;
  // 이 잡이 답한 턴. 결과를 그 턴 옆에 붙이는 근거다.
  string run_id = 4; uint64 turn = 5;
  // 설정이 바뀌면 진행 중인 잡은 stale이 된다. 바뀐 설정의 결과가 아니기 때문이다.
  string revision = 6;
  string state = 7; // queued, running, completed, failed, stale, canceled
  repeated AuxiliaryResult results = 8;
  repeated AuxiliaryUsage usage = 9;
  string message = 10; // 실패 이유. 사람이 읽는 한 줄
  google.protobuf.Timestamp date_created = 11;
  google.protobuf.Timestamp date_updated = 12;
}

// 산출물. truncated는 한계에서 잘렸음을 말한다 -- 도착한 텍스트는 버리지 않는다.
message AuxiliaryResult { AuxiliaryKind kind = 1; string text = 2; bool truncated = 3; }

// 과금 근거가 아니라 이 잡이 쓴 양이다. 계정 누적 총액을 뜻하지 않는다.
message AuxiliaryUsage { string account = 1; string model = 2; AuxiliaryKind kind = 3; bytes data = 4; }

// 부모 세션에 붙어 보존되는 산출물. 잡은 최신 것만 남아도 요약은 턴마다 쌓인다.
message AuxiliarySummary { string run_id = 1; uint64 turn = 2; string text = 3; }

// kind별 자동 실행 여부. 세션 설정이 설치 기본값을 덮는다.
message AuxiliaryPreference { AuxiliaryKind kind = 1; bool enabled = 2; }
```

### `proto/ext/cxz/auxiliary_svc.ext.proto`

```proto
service AuxiliaryService {
  // 한 번만 실행한다. 자동 실행 설정을 바꾸지 않는다.
  rpc Run(AuxiliaryRunRequest) returns (AuxiliaryJob);
  rpc Get(AuxiliaryJobRequest) returns (AuxiliaryJob);
  rpc Cancel(AuxiliaryJobRequest) returns (AuxiliaryJob);
  // 턴에 붙은 산출물을 읽는다. 생성하지 않는다.
  rpc Summaries(AuxiliarySummariesRequest) returns (AuxiliarySummariesReply);
  // 설치 기본 프로필과 세션별 자동 실행 설정.
  rpc GetConfig(AuxiliaryConfigRequest) returns (AuxiliaryConfig);
  rpc SetConfig(AuxiliarySetConfigRequest) returns (AuxiliaryConfig);
  // 계정이 어떤 모델을 쓸 수 있는지. 응답을 생성하지 않으며, 저장된 토큰이
  // 생성에 받아들여진다는 증명도 아니다.
  rpc Models(AuxiliaryModelsRequest) returns (AuxiliaryModelsReply);
  // 로그인에 필요한 정보만 돌려준다. 자격증명은 응답에 담지 않는다.
  rpc LoginInfo(AuxiliaryLoginInfoRequest) returns (AuxiliaryLoginInfo);
}

message AuxiliaryRunRequest { SessionRef parent = 1; repeated AuxiliaryKind kinds = 2; string text = 3; }
message AuxiliaryJobRequest { SessionRef parent = 1; string job_id = 2; }
message AuxiliarySummariesRequest { SessionRef parent = 1; uint64 after_turn = 2; int32 limit = 3; }
message AuxiliarySummariesReply { repeated AuxiliarySummary summaries = 1; AuxiliaryJob job = 2; }
message AuxiliaryConfig {
  string revision = 1;
  repeated AuxiliaryProfile profiles = 2;        // kind별 기본 프로필
  repeated AuxiliaryPreference preferences = 3;  // parent가 지정되면 그 세션 설정
}
// parent가 비어 있으면 설치 기본값, 지정되면 그 세션의 설정을 읽고 쓴다.
message AuxiliaryConfigRequest { SessionRef parent = 1; }
message AuxiliarySetConfigRequest {
  SessionRef parent = 1;
  repeated AuxiliaryProfile profiles = 2;
  repeated AuxiliaryPreference preferences = 3;
}
message AuxiliaryModelsRequest { AuxiliaryProfile profile = 1; }
message AuxiliaryModelsReply { repeated AuxiliaryModel models = 1; bool needs_login = 2; }
message AuxiliaryModel { string name = 1; string label = 2; repeated string efforts = 3; }
message AuxiliaryLoginInfoRequest { AuxiliaryProfile profile = 1; }
// 로그인을 어디서 수행할지 말해줄 뿐이다. 토큰도, grant도 담지 않는다.
message AuxiliaryLoginInfo { string owner = 1; string account = 2; string agent = 3; string backend = 4; }
```

`AuxiliaryRunRequest.text`는 제목을 직접 지정하는 경로(`cxz ai title --text`)에만
쓴다. 비어 있으면 생성한다.

세 가지가 타입으로 해결된다. `parent`가 필드이므로 전송 계층이 payload를 뜯지
않아도 라우팅된다. 제목은 `AuxiliaryResult`의 kind이므로 `Session.Patch` 트리거가
추측이 아니다. 권한 검사를 `Run`/`SetConfig`/`Cancel` 각 메서드에 붙일 수 있다.

## 어떻게 쓰는가

### 자동 요약 — 클라이언트는 트리거하지 않는다

생성 트리거는 클라이언트에 없다. 매니저가 자기 폴링 루프에서 세션 저널을
관찰하고(`Observe`), `turn_end`를 보면 잡을 띄운다. 클라이언트가 하나도 붙어 있지
않아도 생성되고, 여럿 붙어 있어도 두 번 돌지 않는다 — 세션당 상태 파일 하나,
뮤텍스 하나, 진행 중 작업 한 칸(`c.active[id]`).

따라서 클라이언트가 하는 일은 **읽기**다.

```
AuxiliaryService.Summaries{parent: <session>}
  → summaries: [{run_id, turn, text}, ...]
    job: {state: "running", kinds: [SUMMARY, SUGGESTION]}
```

TUI는 이것을 1초에 한 번 이하로 폴링하고(`internal/tui/auxiliary.go:171`),
`summary.run_id == event.run_id && summary.turn == event.seq`로 해당 턴 옆에
그린다. 웹 클라이언트도 같은 응답을 같은 방식으로 읽는다 — 생성·중복 방지·저장이
모두 매니저에 있으므로 클라이언트가 늘어도 응답은 동일하다.

### 한 번만 생성

```
AuxiliaryService.Run{parent: <session>, kinds: [SUMMARY]}  → AuxiliaryJob{state: "queued"}
AuxiliaryService.Get{parent: <session>, job_id: <id>}      → AuxiliaryJob{state: "completed", results: [...]}
```

`Run`은 자동 실행 설정을 바꾸지 않는다. 자동이 켜져 있는 kind를 `Run`하면 아무
일도 하지 않는다 — 지금 `/summary`의 동작과 같다.

### 자동 실행 켜고 끄기

```
AuxiliaryService.SetConfig{parent: <session>, preferences: [{kind: SUGGESTION, enabled: true}]}
```

`parent`가 없으면 설치 기본값(프로필)을 바꾸고 `revision`이 올라간다. 올라간
revision으로 진행 중인 잡은 `stale`이 되어 결과를 버린다. 바뀐 설정의 결과가 아니기
때문이고, 이 규칙은 현재 구현(`cfg.Revision != c.config.Revision`)과 같다.

### 취소

```
AuxiliaryService.Cancel{parent: <session>}  → AuxiliaryJob{state: "canceled"}
```

`job_id` 없이 부르면 그 세션의 진행 중인 잡을 취소한다. 요약과 추천이 한 호출로
묶인 잡이면 둘 다 멈춘다 — 지금도 그렇다.

### 설정과 로그인

```
AuxiliaryService.Models{profile: {account: "work1"}}   → models: [...] 또는 needs_login
AuxiliaryService.SetConfig{profiles: [{kind: SUMMARY, enabled: true, account: "work1", model: ..., effort: ...}]}
AuxiliaryService.LoginInfo{profile: {account: "work1"}} → {owner, agent, backend}
```

로그인 자체는 지금처럼 `ProjectService.AuxiliaryLogin` 스트림을 쓴다. 이 설계가
바꾸지 않는다.

### CLI 매핑

`cxz ai`의 표면은 그대로 두고 뒤에서 부르는 것만 바뀐다.

| 명령 | 지금 | 제안 |
| --- | --- | --- |
| `cxz ai list` | `Docker{auxiliary, {action:"list"}}` | `GetConfig{}` |
| `cxz ai set TASK --account --model` | `{action:"put"}` | `SetConfig{profiles:[...]}` |
| `cxz ai disable TASK` | `{action:"put"}` (enabled 없음) | `SetConfig{profiles:[{enabled:false}]}` |
| `cxz ai models ACCOUNT` | `{action:"models"}` | `Models{profile:{account}}` |
| `cxz ai status SESSION` | `{action:"status"}` | `Summaries{parent}` |
| `cxz ai cancel SESSION` | `{action:"cancel"}` | `Cancel{parent}` |
| `cxz ai title SESSION [--text]` | `{action:"title"}` | `Run{parent, kinds:[TITLE], text}` |
| `cxz ai login ACCOUNT` | `{action:"login-info"}` + 스트림 | `LoginInfo{}` + 기존 스트림 |
| (TUI `/summary`, `/suggest`) | `{action:"session"}` | `SetConfig{parent, preferences}` / `Run{parent, kinds}` |
| (내부) `{action:"forget"}` | 세션 purge 경로에 남김 | `Controller.Forget` 직접 호출 |

`forget`은 클라이언트가 부를 일이 없다. 세션 purge/삭제 경로에서만 쓰이므로 공개
API에 두지 않는다.

## 저장과 단계

**1단계 — 타입만.** 저장은 지금 그대로다. 보조 상태는 매니저 state의
`<aux root>/<sha256(session)>.json`에 남고, 잡은 세션당 최신 하나
(`State.Job`), 요약은 턴마다 `State.Summaries`에 쌓인다. 바뀌는 것은 전송 표면뿐이고,
`Project.Docker`에서 `auxiliary` action이 사라진다.

**2단계 — 필요해지면 엔티티화.** `AuxiliaryJob`을 payday 리소스로 올리면
(`domain: 11`, 7~10은 사용 중) `watch`를 얻는다. 그러면 TUI/웹의 1초 폴링이
구독으로 바뀌고 잡 이력이 남는다. 다만 새 테이블과 보존 정책이 필요하므로 1단계와
섞지 않는다.

주의: 보조 산출물은 저널 이벤트가 **아니다**. 저널의 이벤트 kind에
`summary`/`suggestion`/`auxiliary`는 없고, 보조 컨트롤러는 저널을 읽기만 한다.
따라서 요약은 저널에서 재구축되지 않으며, 그 JSON이 사라지면 지난 요약도 사라진다.
엔티티화는 이 성질을 바꿀 수 있는 유일한 단계다.

## 하위호환

사용자 배포 전이므로 `Project.Docker`의 `auxiliary` action에 호환 경로를 남기지
않는다. 봉투를 비워 두면 비울 이유가 없어진다. `DockerInput` 자체는
`save/up/down/prune/status`에 남는다 — 그 메세지가 실제로 뜻하는 것이다.

## 열어 두는 질문

- **대화형 보조.** "이 대화에 대해 보조 AI에 직접 질문하기"가 생기면 그것은 턴이
  쌓이는 진짜 스레드이고, `AuxiliaryThread`(messages를 가진 별도 타입, 역시
  `Session`은 아니다)가 맞다. 그때도 요약·추천은 잡이다. 잡으로 설계해도 이 길이
  막히지 않는다.
- **요약을 검색 대상에 넣을지.** 대화 색인은
  `input/assistant/tool_call/tool_output/tool_result`만 넣으므로 요약은 검색되지
  않는다. 요약은 *대화에 대해 모델이 쓴 글*이라 히트에 섞이면 아무도 쓰지 않은
  문장이 결과에 나온다. 넣는다면 출처를 구분해 표시해야 한다.
- **잡 이력 보존.** 2단계에서 잡을 영속화할 때 몇 개를, 얼마나 오래 남길지.
- **`info` 응답 분리.** `Docker{action:"info"}`는 엔진 정보·히스토리 정책·cxz
  버전/채널/핀을 한 응답에 섞는다. 이 설계와 독립적이지만 같은 성질의 문제다.

## 비목표

- 보조 AI의 동작·프롬프트·품질을 바꾸지 않는다. 전송 표면만 바꾼다.
- 인증 모델을 바꾸지 않는다. 프로필 분리, grant 범위, 프로필 잠금은 그대로다.
- 보조 실행을 프로젝트 컨테이너 안에서 구동할 수 있게 만들지 않는다.
- 자격증명·grant·토큰을 어떤 응답에도 담지 않는다.
