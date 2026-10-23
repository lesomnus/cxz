# ProjectService 쪼개기 — 서비스 경계

상태: 제안. 구현 전. 남은 결정은 §7.1 하나다 — 파일이 진실인 엔티티의 `id`를
어떻게 만드는가.

`ProjectService`는 생성된 7개 verb(`Add` `Get` `Patch` `Apply` `Erase` `List`
`Watch`) 위에 손으로 쓴 RPC 47개를 얹고 있다. 그중 **26개는 Project를 가리키지
않는다.** 엔진, 진단, 보존 예산, 스킬 라이브러리, MCP 레지스트리, 에이전트 설정,
설치본의 devcontainer 설정이 모두 거기 있다. 이 문서는 무엇이 어디로 가야 하는지와
그 비용을 적는다.

## 1. 측정

`proto/cxz/project_svc.g.proto`의 요청 메시지에 `ProjectRef ref`가 있는지로 센다:

| | 개수 |
| --- | --- |
| 생성된 verb | 7 |
| 손으로 쓴 RPC | 47 |
| 그중 Project를 가리키는 것 | 21 |
| 가리키지 않는 것 | **26** |

비교로, `SessionService`는 손으로 쓴 48개 중 46개가 Session을 가리킨다. 긴 것이
문제가 아니다 — 경계가 없는 것이 문제이고, 그건 `ProjectService`에만 있다.

## 2. 왜 취향 문제가 아닌가

payday가 두 곳에서 같은 말을 한다.

- 스키마 가이드 "An RPC of your own": *"The service keeps the name of the entity
  — `WidgetService` for `Widget`."* 서비스는 엔티티에서 나오고, 그 엔티티의 행에
  대한 호출을 담는다.
- 커맨드 가이드 §5: *"an RPC about a row names it the way every other RPC names
  one."* 이건 문서의 권고로 끝나지 않는다. `pdcmd`가 요청에 `ref` 필드가 있는지
  **보고** CLI 인자를 결정한다. 그리고 `server/pd/pd.g.go`가 생성하는 Gate,
  Audit, Intercept, Batch는 모든 메서드를 그 서비스 이름 밑에 넣는다. 설치본
  전체를 켜고 끄는 호출이 `cxz.ProjectService/StartEngine`으로 감사 기록에
  남으면, 그 기록과 그 권한은 거짓을 말한다.

말해줄 lint는 우리가 꺼 두었다. `buf.yaml`이 `RPC_REQUEST_STANDARD_NAME`,
`RPC_RESPONSE_STANDARD_NAME`, `RPC_REQUEST_RESPONSE_UNIQUE` 세 개를 예외로 둔다.

## 3. 이름 자체의 문제 (`ref`와 별개)

- **생성된 verb와 겹치는 접두사**: `GetEngineInfo`, `GetInstallationVersion`,
  `GetHistoryPolicy`, `SetHistoryPolicy`, `GetSkills`, `GetMcpServers`,
  `PutMcpServer`, `SetSkillDefault`, `SetMcpServerDefault`. payday에서 `Get`은
  생성되는 이름이다.
- **리소스를 요청으로 쓴다**: `SetHistoryPolicy(HistoryPolicy)`. payday에서 그
  모양의 이름은 `Patch`다.
- **서비스가 이미 말한 것을 다시 말한다**: `ProjectService` 안의
  `SetProjectSkill`, `ClearProjectSkill`, `SetProjectMcpServer`,
  `ClearProjectMcpServer`.
- **요청 메시지가 엔티티 이름을 쓰지 않는다**: `EngineRequest`, `SkillsRequest`,
  `McpServersRequest`, `DevcontainerRequest`, `DiagnosticsRequest`,
  `AuxConfigRequest`, `InspectForeignRequest`, `FileMappingsRequest`,
  `HistoryPolicyRequest`.

## 4. `ref`를 가졌지만 Project에 대한 것이 아닌 것

`ref` 검사를 통과한 21개 중 두 개는 더 나쁘다. `AddSkill`과 `RemoveSkill`은
`ProjectRef`를 받지만 프로젝트를 건드리지 않는다 —
`internal/skillconfig/config.go`가 그렇게 적어 두었다: *"Add registers a
directory from the library. Registration is installation-wide; project only
chooses whose view comes back."* ref는 주체가 아니라 응답의 시야를 고르는
필터다. 같은 조작을 MCP 쪽은 `PutMcpServer(id)`로, ref 없이 쓴다. 한 서비스
안에서 같은 일이 두 모양이다.

`GetSkills`와 `GetMcpServers`도 ref를 시야로 쓰지만, 목록의 필터로서는 정당하다.

`server/lifecycle/history_policy.go`는 아예 이렇게 적혀 있다: *"The budgets
belong to the installation, so they are read and written here rather than per
session."* 설치본의 것이라는 걸 알면서 Project 서비스에 두었다.

## 5. 테이블 없이 서비스를 갖는 두 길 — 측정한 것

처음 이 문서는 "payday에서 서비스는 엔티티에서 나오므로 분리된 service는 곧
테이블이다"라고 적었다. 틀렸다. 실제로 돌려 본 결과는 이렇다.

**(1) 엔티티 없는 서비스는 생성된다.** `proto/cxz/`에 엔티티 선언이 없는
`service`를 하나 쓰고 `pd gen`을 돌리면 `resource/installation.pb.go`,
`resource/installation_grpc.pb.go`, `ts/gen/cxz/installation_pb.ts`가 나오고 ent
스키마는 **생기지 않는다**. `pd gen --check`도 통과한다.

**(2) 다만 그 파일은 행을 가리킬 수 없다.** `ProjectRef`나 `SessionRef`를 쓰려고
하면 생성이 거부한다:

```
proto/cxz/installation.proto:14:3:cannot find `ProjectRef` in this scope
```

`Ref`는 계약이 생성될 때 만들어지는 이름이고, `pd gen`은 `*_svc.g.proto`를 지우고
다시 쓴다. 그래서 그건 입력이 아니라 출력이다 — payday 문서가 오버레이를 오버레이로
만든 이유로 적어 둔 바로 그것이다.

**(3) 오버레이 안에 선언한 두 번째 서비스는 둘 다 된다.**
`proto/ext/cxz/project_svc.ext.proto`에 `service SkillProbeService`를 쓰고
`ProjectRef`를 받게 하면, 병합된 계약(`proto/cxz/project_svc.g.proto`)에 그대로
들어가고 `/cxz.SkillProbeService/Enable`과 클라이언트·서버 stub이 생성된다.
엔티티도 테이블도 없다. `--check`도 통과한다.

**(4) 생성된 층은 안 붙는다.** 어느 경우든 그 서비스는
`resource/store.g.go`의 `Server`·`RegisterServer`와 `server/pd/pd.g.go`의
Gate·Audit·Intercept·Batch에 나타나지 않는다. 직접 등록해야 한다. 이 설치본에서
그게 무엇을 잃는 것인지도 셌다.

| 잃는 것 | 이 설치본에서 실제로 무엇인가 |
| --- | --- |
| Gate | 엔티티 여섯 개가 전부 `global: {}`이고 테넌트도 grant도 없다. 좁힐 것이 없다. |
| Audit | 행은 쓰이지만 읽는 것이 없다 — `Layer.Audit()`이 `Unimplemented`이고 `cmd/`·`internal/`·`ts/src`에 독자가 없다. |
| Batch | 호출자가 없다. |
| Intercept | `pd.InterceptBuild`가 들고 있는 것은 `taskWatchUnary` 하나다. Task 전용이다. |
| 유지되는 것 | otel·로그·recover·deadline(`grpcx.ServerOptions`), `observe` 진단, versionpin, updateGate, 토큰 인증 — 전부 전역 인터셉터다. |

CLI verb도 잃지 않는다. cxz는 `pdcmd`를 쓰지 않고 명령을 직접 쓴다.

**(5) 엔티티를 선언하고 sink가 답하지 않게 하는 길도 있다. 이 저장소가 이미 그렇게
하고 있다.** `Connection`은 선언된 엔티티인데(domain 12, `global`, `list:`) 매니저는
그 서비스를 `Unimplemented`로 답한다. `server/lifecycle/stack.go:122`에 이유가 적혀
있다: *"Left to the store it would serve an empty table instead, which is a worse
answer than a refusal."* 즉 "엔티티는 선언하고 답은 layer가 한다"는 이 저장소의
기존 패턴이다.

그 길을 재 보았다. `Skill`과 `SkillActivation`을 `list:`·`watch:` 없이 선언하고
`pd gen`을 돌린 결과:

- 생성을 통과한다. `list:`와 `watch:`는 선택이다 — 안 쓰면 `List`와 `Watch` RPC가
  아예 **생기지 않는다**(`Add` `Get` `Patch` `Apply` `Erase`만 생긴다). 그래서
  동작하지 않는 `Watch`를 떠안을 필요가 없다. 쓰고 싶으면 `list:`만 선언하고 layer가
  답한다.
- `SkillRef{id | alias}`가 생긴다. 요청이 스킬을 alias로 가리킬 수 있다.
- ent 스키마와 `skill`·`skillactivation` 테이블이 생긴다. 그리고 `go build ./...`가
  **코드 수정 없이 통과한다** — `Layer`가 `resource.Overlay`를 품고 있어서 구현하지
  않은 메서드가 자동으로 아래로 흘러간다.
- Gate·Audit·Intercept·Batch가 붙는다(`server/pd/pd.g.go`에 `SkillService_` 항목
  10개). (4)에서 잃는다고 적은 것들을 이 길은 유지한다.

**조심할 것은 그 자동 흐름 하나다.** 구현하지 않은 메서드는 빈 테이블을 읽어
"없음"이나 빈 목록을 답한다. 막는 방법도 `Connection`이 보여 준다 — 답하지 않을
메서드는 명시적으로 `Unimplemented`를 돌려준다.

**드는 비용은 테이블이 아니라 식별자다.** payday 엔티티는 모두 uuid `id`를 키로
가지고, 그 안에 domain 바이트가 들어간다(`pdid`). `Add`는 그 id를 담아 답한다. 진실이
파일이면 layer가 스킬마다 **안정된** id를 만들어 들고 있어야 하는데, `skills.json`에는
id가 없다. 그래서 파일에 하나 적거나, 이름에서 `pdid` 형식에 맞게 결정적으로
파생시켜야 한다. 새로 생기는 유일한 실제 작업이고, §7.1이 그것이다.

지금 26개는 전부 매니저(`s.shared.runtime`)로의 통과 호출이고 DB를 건드리지 않는다.
그래서 (1)과 (5) 중 어느 쪽이든 저장 위치는 그대로다 — 다른 것은 요청이 행을 어떻게
가리키느냐뿐이다.

## 6. 제안

47개를 여섯 곳으로 나눈다. 새 서비스는 넷이고 **새 테이블은 없다** — 이유는 §5다.

| 가는 곳 | 개수 | 무엇 |
| --- | --- | --- |
| `ProjectService`에 남는다 | 12 | Terminal, SessionLogin, AuxiliaryLogin, Paths, Download, Editor, EditorTunnel, PutSecretFile, DeleteSecretFile, Start, Stop, Remove |
| `AccountService`로 | 2 | AuxModels, AuxLoginInfo |
| 새 `SkillService`+`SkillActivationService` | 6 | GetSkills, AddSkill, RemoveSkill, SetSkillDefault, SetProjectSkill, ClearProjectSkill — 전부 생성된 CRUD가 된다(§6.3) |
| 새 `McpServerService`+`McpActivationService` | 6 | GetMcpServers, PutMcpServer, RemoveMcpServer, SetMcpServerDefault, SetProjectMcpServer, ClearProjectMcpServer — 같다 |
| 새 `McpConnectionService` | 1 | McpSessions |
| 새 `InstallationService` | 18 | 엔진 7, 진단 2, 보존 3, InspectForeign, AuxConfig, AuxSetConfig, Devcontainer, RenderDevcontainer, FileMappings |
| 리소스 API에서 제거 | 2 | SyncSkills, SyncMcpServers |

### 6.1 `AccountService`로 가는 둘

`AuxModels`와 `AuxLoginInfo`는 `string account`를 받는다. Account 행에 대한
호출이다. 옮기고 `AccountRef ref = 1`을 받게 하면 CLI가 `@alias`를 공짜로 받는다.
새 엔티티도, 저장 위치 변경도 없다. 반나절.

### 6.2 리소스 API에서 빠지는 둘

`SyncSkills`와 `SyncMcpServers`는 프로젝트 런타임이 **답하는** 호출이다.
`internal/workspace/skills.go`가 런타임 API로 부르고,
`internal/server/skills.go`가 그렇게 적어 두었다: *"A project runtime answers
only SyncSkills: it is the side being told."* 매니저의 `SyncSkills`는 호출되면
에러를 던진다. 클라이언트가 보는 표면에 있을 이유가 없다 — 지운다.

### 6.3 행으로 선언하고 파일이 답하는 둘: `Skill`, `McpServer`

둘 다 같은 모양이다. 등록은 설치본 전체, 활성화는 프로젝트별, 진실의 출처는 공유
디렉터리의 파일(`skills.json`, mcp 설정)이고 `cxz share`가 그걸 고친다.

§5의 (5)로 선언한다 — 엔티티를 선언하고, 답은 layer가 파일에서 한다. `Connection`이
이미 그렇게 되어 있다. `watch:`는 선언하지 않는다(sink를 통과하는 쓰기가 없으므로
발행할 것이 없다). `list:`는 선언하고 layer가 답한다 — 요청 메시지의 필터 필드가
생성되고, 그 필터를 파일을 읽어 적용한다.

```proto
// proto/cxz/skill.proto
message Skill {
  bytes  id    = 1  [(orm.field) = {type: TYPE_UUID, key: true, default: ""}];
  string alias = 4  [(orm.field) = {unique: true}];   // 스킬 디렉터리 이름
  string desc  = 6;
  bool   enabled = 7;                                  // 프로젝트가 정하지 않았을 때의 기본값
  google.protobuf.Timestamp date_updated = 13 [(orm.field) = {version: {}}];
  google.protobuf.Timestamp date_erased  = 14 [(orm.field) = {erased: {}}];
  google.protobuf.Timestamp date_created = 15 [(orm.field) = {immutable: true, default: ""}];
  option (orm.message) = {rpc: {crud: true}};
  option (payday.entity) = {
    domain: 13
    global: {}
    list: {order: [{field: {name: "alias"}}, {field: {name: "id"}}], by: [{name: "ref"}], size: 100, max: 200}
  };
}

// 스킬이 한 프로젝트에 속하는 것이 아니다. 설치본 전체에 등록되고 프로젝트마다
// 켜지거나 꺼진다. 그래서 `Skill`에 `Project` edge를 두면 거짓이 된다 — 관계는
// 속성(`enabled`)을 가진 다대다이고, 그건 행 하나다.
message SkillActivation {
  bytes   id      = 1 [(orm.field) = {type: TYPE_UUID, key: true, default: ""}];
  Skill   skill   = 8 [(orm.edge) = {immutable: true}];
  Project project = 9 [(orm.edge) = {immutable: true}];
  bool    enabled = 10;
  ...
  option (payday.entity) = {
    domain: 14
    global: {}
    list: {order: [...], by: [{name: "ref"}, {name: "skill"}, {name: "project"}], size: 200, max: 500}
  };
}
```

프로젝트 edge를 8·9에 두는 것은 이 저장소의 관행이다 — `Session.project = 8`,
`Task.project = 8`, `AuthBinding.project = 9`. (payday는 필드 3을 "테넌트보다 작은
집합으로 가는 edge" 자리로 비워 두지만, 이 저장소는 쓰지 않았다.)

이렇게 하면 손으로 쓸 RPC가 남지 않는다. 지금 13개가 이렇게 대응된다:

| 지금 | 다음 |
| --- | --- |
| `AddSkill`, `PutMcpServer` | `Skill.Add` / `Skill.Patch` |
| `RemoveSkill`, `RemoveMcpServer` | `Skill.Erase` |
| `GetSkills`, `GetMcpServers` | `Skill.List` + `SkillActivation.List{project}` |
| `SetSkillDefault`, `SetMcpServerDefault` | `Skill.Patch{enabled}` |
| `SetProjectSkill`, `SetProjectMcpServer` | `SkillActivation.Apply{skill, project, enabled}` |
| `ClearProjectSkill`, `ClearProjectMcpServer` | `SkillActivation.Erase` |

`AddSkill(ProjectRef)`이 설치본 전체에 등록하면서 프로젝트를 받던 문제(§4)도
사라진다. `Skill.Add`는 프로젝트를 받지 않고, 프로젝트를 받는 것은 `SkillActivation`
뿐이다.

**구현에서 지켜야 할 것 둘.**

1. `Layer`가 `Skill()`과 `SkillActivation()`을 모두 덮어써야 한다. 덮어쓰지 않은
   메서드는 `resource.Overlay`를 타고 sink로 내려가 빈 테이블을 읽고 "없음"을
   답한다. 답하지 않을 메서드는 `Connection()`처럼 명시적으로 `Unimplemented`를
   돌려준다.
2. `Skill.id`가 안정돼야 한다 — §7.1.

### 6.4 행이 아닌 것: `Installation`

엔진 7개, 진단 2개, 보존 3개, `InspectForeign`, `AuxConfig`/`AuxSetConfig`, 설정
번들 3개 — 18개가 전부 "이 설치본"에 대한 것이다. 그리고 **하나도 행을 가리키지
않는다.** 그래서 §5의 (1)이 그대로 맞는 자리다: 자기 파일, 엔티티 없음, `Ref` 필요
없음.

```proto
// proto/cxz/installation.proto
edition = "2023";

package cxz;

option go_package = "github.com/lesomnus/cxz/resource";

// 이 설치본. 행이 아니다 — 진실의 출처는 설치 루트의 파일이고 이 서비스는 그것을
// 읽고 쓴다. 엔티티가 아니므로 테이블이 없고, 행을 가리킬 수도 없다(§5의 (2)).
// 여기 있는 것 중 행을 가리켜야 하는 것은 하나도 없다.
service InstallationService {
  // 파일에서 읽는다. 싸다.
  rpc Get(InstallationGetRequest) returns (Installation);
  // docker에게 묻는다. 느릴 수 있어서 `Get`과 나눈다 — 지금 `GetEngineInfo`와
  // `EngineStatus`가 따로 있는 이유도 이것이다.
  rpc Engine(EngineRequest) returns (EngineStatus);

  rpc SaveEngine(SaveEngineRequest)   returns (Installation);
  rpc StartEngine(StartEngineRequest) returns (EngineStatus);
  rpc StopEngine(EngineRequest)       returns (EngineStatus);
  rpc PruneEngine(EngineRequest)      returns (EngineStatus);

  rpc SaveRetention(SaveRetentionRequest)       returns (Installation);
  rpc SaveAgents(SaveAgentsRequest)             returns (Installation);
  rpc SaveDevcontainer(SaveDevcontainerRequest) returns (Installation);
  rpc SaveSharedFiles(SaveSharedFilesRequest)   returns (Installation);
  rpc RenderDevcontainer(RenderDevcontainerRequest) returns (RenderDevcontainerReply);

  rpc Diagnostics(DiagnosticsRequest)     returns (DiagnosticsReply);
  rpc Capture(CaptureRequest)             returns (DiagnosticsReply);
  rpc ForeignContainers(ForeignContainersRequest) returns (ForeignContainersReply);
}

// 저장된 것만. 관측은 `Engine`과 `Diagnostics`가 답한다.
message Installation {
  string               version      = 1;  // 지금 `GetInstallationVersion`
  EngineSpec           engine       = 2;
  HistoryPolicy        retention    = 3;
  repeated AgentProfile agents      = 4;  // 지금 `AuxProfile`
  DevcontainerDefaults devcontainer = 5;  // 지금 `Devcontainer`가 저장하는 Spec
}

message InstallationGetRequest  {}
message EngineRequest           {}
message SaveEngineRequest       { EngineSpec spec = 1; }
message StartEngineRequest      { EngineSpec spec = 1; }
message SaveRetentionRequest    { HistoryPolicy retention = 1; }
message SaveAgentsRequest       { repeated AgentProfile agents = 1; }
message SaveDevcontainerRequest { bytes spec = 1; }
message SaveSharedFilesRequest  { bytes bundle = 1; }
message ForeignContainersRequest {}
message ForeignContainersReply  { repeated ForeignContainer items = 1; }
message CaptureRequest          { int32 seconds = 1; }
```

`EngineSpec`, `EngineMode`, `EngineStatus`(지금 `EngineInfo`+`EngineReply`),
`HistoryPolicy`, `AgentProfile`(지금 `AuxProfile`), `DiagnosticsRequest`/`Reply`,
`RpcRecord`, `RpcStat`, `RenderDevcontainer*`, `RenderedFile`, `ForeignContainer`는
`project_svc.ext.proto`에서 이 파일로 **옮긴다**. 복사가 아니다 — 같은 패키지에
둘이면 충돌이다.

`Save*`로 통일한 이유: 이 서비스의 쓰기는 전부 파일을 쓰는 것이고, payday의
`Patch`인 척하지 않는 편이 낫다. 엔티티가 아니므로 `Patch`의 필드 마스크 의미를
공짜로 얻지 못하고, 손으로 흉내 낸 `Patch`는 생성된 것과 이름만 같아서 더 나쁘다.

`MarkHistoryTrimmable`은 여기 없다 — 아래의 결정이 먼저다.

### 6.5 실행 중인 instance: `McpConnectionService`

등록된 MCP와 그것으로 **돌아가고 있는 것**은 다른 리소스다. 데이터가 그렇게 말한다:

- 등록은 매니저의 파일이고, 설치본 전체이며, 프로젝트 override를 갖는다.
- 돌아가는 것은 `McpSessionStatus{session, title, pending, launch_digest, servers}`
  이고, 읽을 때 프로젝트 컨테이너에 물어서 만든다. 컨테이너가 없으면 "Project is
  not running", 못 물으면 "Runtime status unavailable"이다. 아무 데도 저장되지
  않는다. 식별자는 (에이전트 세션, MCP id) 쌍이다.
- `pending`은 그 세션이 지금 등록된 것과 다른 것으로 떴다는 뜻 — 두 리소스의
  **차이**가 값이 되는 자리다. 한 서비스 안에서는 그게 필드로 보이지만, 나뉘면
  관계로 보인다.

그래서 쪼개는 것이 맞다. 두 가지만 붙인다.

1. **이름.** `McpSession`은 cxz의 `Session`과 충돌한다 —
   `McpSessionStatus.session`이 바로 에이전트 세션의 id다. 돌아가는 것은 한 세션과
   한 MCP 서버 사이의 **연결**이므로 `McpConnectionService`로 읽는 편이 낫다.
   (`Connection` 엔티티는 허브의 것이고 별개다. `McpConnection`은 그 이름을 쓰지
   않는다.)
2. **파생이고 저장되지 않는다.** `container_state`에서 배운 그대로다: 응답을 만들 때
   계산하고, 어디에도 적지 않는다. 그러므로 `Watch`를 선언하지 않는다 — 발행할
   쓰기가 없다.

```proto
// proto/ext/cxz/mcp_svc.ext.proto — MCP 계약의 오버레이.
service McpConnectionService {
  rpc List(McpConnectionListRequest) returns (McpConnectionList);
}
message McpConnectionListRequest { ProjectRef project = 1; }
```

여기서 `ProjectRef`를 쓸 수 있는 이유는 §5의 (2)와 어긋나지 않는다. `McpActivation`이
Project edge를 가지므로 MCP 계약이 `cxz/project_svc.g.proto`를 import하고, 오버레이는
그 계약에 병합된다. `task_svc.ext.proto`가 이미 그렇게 하고 있다 — 생성된 메시지에
`ProjectRef project = 1000`을 더한다(`proto/ext/cxz/task_svc.ext.proto:20-22`).

**그리고 나누면 세션을 제대로 가리킬 수 있게 된다.** 지금 `McpSessionStatus.session`은
불투명한 런타임 id이고, proto에 그 이유가 적혀 있다 — project 계약은 session 계약을
가리킬 수 없다. 사실이다: `session_svc.g.proto`가 `project_svc.g.proto`를
import하므로(Session이 Project edge를 가진다) 반대 방향은 순환이다.

그런데 **새 계약은 둘 다 import할 수 있다.** `task_svc.g.proto`가 이미 그렇게 하고
있고, 그 import를 만든 것은 오버레이다 —
`proto/ext/cxz/task_svc.ext.proto:28`의 `message TaskActor { SessionRef session = 1; }`
(Task 엔티티는 세션을 edge로 갖지 않고 `source_session` 문자열만 갖는다). MCP 계약은
누구도 import하지 않는 잎이므로 같은 일을 할 수 있다.

그래서 연결 항목은 세션을 `SessionRef`로 담는다. `SessionRef`의 oneof에 `runtime_id`가
있으므로(`session_svc.g.proto:156`) 지금 들고 있는 값을 그대로 넣어 타입만 얻는다.
나누는 이유가 이것은 아니지만, 나누면 따라온다.

### 6.6 결과

`ProjectService` = 생성된 7개 + 손으로 쓴 12개. 전부 Project 행을 가리킨다.

손으로 쓴 47개는 이렇게 흩어진다:

| | |
| --- | --- |
| `ProjectService`에 남는다 | 12 |
| `AccountService`로 옮긴다 | 2 |
| 지운다(`SyncSkills`, `SyncMcpServers`) | 2 |
| `MarkHistoryTrimmable` — 지운다(§7.3) | 1 |
| 생성된 CRUD가 흡수한다(스킬·MCP 등록과 활성화) | 12 |
| `InstallationService`로 옮긴다(읽기 몇 개가 `Get`으로 합쳐져 14개쯤) | 17 |
| `McpConnectionService`로 옮긴다 | 1 |

손으로 쓸 것은 47 → 약 29다. 줄어든 열두 개는 §6.3이 엔티티로 가서 생성된 CRUD를
받기 때문이다.

테이블은 넷 생긴다(`skill`, `skill_activation`, `mcp_server`, `mcp_activation`). 전부
비어 있고 답은 layer가 파일에서 한다 — `Connection`이 이미 그 모양이다.
`InstallationService`와 `McpConnectionService`는 엔티티가 아니므로 테이블이 없다.

## 7. 결정

셋 중 둘은 정해졌다(§7.2, §7.3). 남은 것은 §7.1이다.

### 7.1 파일이 진실인 엔티티의 `id`를 어떻게 만드는가

이것만 남았다. §5의 (5)로 가면 `Skill`과 `McpServer`는 payday 엔티티이므로 uuid
`id`를 키로 가져야 하고, `Add`는 그 id를 담아 답한다. 그런데 진실은 파일이고
`skills.json`에는 id가 없다. layer가 어디선가 **안정된** id를 만들어야 한다. 두
가지다.

- **파일에 적는다.** `skills.json`의 각 항목에 `"id"`를 하나 더 둔다. 명시적이다.
  대가는 사람이 고치는 파일에 uuid가 들어간다는 것, 그리고 사람이 디렉터리만 만들어
  둔 스킬에는 id가 없어서 등록 때 채워야 한다는 것이다.
- **이름에서 파생시킨다.** `sha256(alias)`의 앞 16바이트를 uuid로 놓고 payday가
  요구하는 두 바이트를 덮어쓴다 — `v[6] = 0x80 | (v[6] & 0x0F)`, `v[9] = domain`
  (`pdid.New`가 하는 것과 같다). 그러면 `pdid.Of`가 받아 주고, 파일에 새로 적을 것이
  없고, 사람이 디렉터리를 만들어 둔 스킬도 id를 갖는다. 대가는 스킬 이름을 바꾸면
  id가 바뀐다는 것이다.

**파생을 권한다.** 이름이 바뀌면 다른 등록이 되는 것이 맞고 — `SkillActivation`도
파일에서 이름으로 키가 잡혀 있으니 같이 움직인다 — 사람이 고치는 파일에 uuid를
넣지 않는 쪽이 낫다. 시간순 정렬을 잃지만 `list:`의 정렬은 `alias`가 먼저이고 `id`는
커서를 고유하게 만드는 마지막 칸이라 영향이 없다.

이 결정이 아니라 §5의 (1)(엔티티 없는 서비스)로 가겠다면 `id`는 필요 없다. 대신
스킬을 `string name`으로 가리키고, 서비스 선언이 `project_svc.ext.proto`에 들어간다.
그 쪽을 택하면 §6.3의 생성된 CRUD도 함께 포기한다 — 손으로 쓸 RPC 여섯 개가 돌아온다.

### 7.2 `McpConnectionService` — 정해졌다

이름은 `McpConnectionService`, 만드는 시점은 지금. `McpSessionStatus.session`이
에이전트 세션의 id라서 `McpSession`은 cxz의 `Session`과 같은 단어를 두 뜻으로 쓰게
된다. 옮기는 RPC는 `McpSessions` 하나이고, `List`로 시작한다.

### 7.3 `MarkHistoryTrimmable` — 지운다

**무엇인지.** 수퍼바이저가 세션 대화를 잘라내기(trim) **전에** 이 RPC를 부르고,
받은 쪽은 `root/cxz.db`에 `PRAGMA user_version=2`를 쓴다
(`internal/server/history_policy.go:50`). 그 설치 루트를 열 때 cxz는
`PRAGMA user_version`을 읽고 자기가 아는 것보다 높으면 시작을 거부한다 — 지금
빌드는 `version > 2`에서 거부한다(`internal/server/server.go:92`). 그래서 1만 아는
옛 빌드는 2를 보고 거부하게 된다. 구멍 난 대화를 보여 주지 않기 위한 일방향
표식이고, 호출이 실패하면 수퍼바이저는 잘라내기를 멈춘다
(`internal/supervisor/history.go:298`).

잘라내기 자체는 이 파일이 아니라 세션별 대화 파일(`internal/sessionstore`)에서
일어난다. 그 파일에는 자기 울타리가 따로 있다 — `meta` 테이블의
`sessionstore.Schema`와 `Compat`(둘 다 2)이고, 옛 세대 빌드가 **기록하는 것**을
막는다.

**지운다.** 울타리가 막으려던 상황이 더는 올 수 없다. 바이너리가 설치되거나 고정되는
모든 경로가 `Schema == 2`를 요구한다:

| 경로 | 검사 |
| --- | --- |
| 릴리스 수락 | `internal/releasechannel/release.go:45`, `internal/cxzupdate/release.go:105` |
| `cxz use` 고정 | `internal/cxzupdate/pin_linux.go:154` |
| 매니저 헬스 | `internal/cxzupdate/manager_linux.go:232` |
| 프로젝트 런타임 | `internal/cxzupdate/runtime_linux.go:152` |

`Schema`는 `releasechannel.StateSchema = 2`이고, 그 상수에 달린 주석이 바로 이
경계를 말한다: *"Schema 2 journals may start with a retention checkpoint. Schema 1
binaries cannot recover these sessions."* 즉 Schema 1 빌드는 이 설치본에 설치될 수도,
고정될 수도, 헬스를 통과할 수도 없다. `user_version` 표식은 그 울타리가 생기기 전의
전환기용이었다.

마이그레이션도 필요 없다. 지금 빌드는 1과 2를 모두 받아 주므로, 이미 2가 찍힌
설치본은 그대로 열리고 새 설치본은 1에 머문다.

**지울 것, 순서대로.**

1. `proto/ext/cxz/project_svc.ext.proto`에서 `rpc MarkHistoryTrimmable`과
   `HistoryTrimmableRequest`/`HistoryTrimmableReply`를 지우고 `pd gen`.
2. `server/lifecycle/history_policy.go`의 핸들러.
3. 런타임 API: `internal/runtimeproto/cxz.proto`의 `MarkHistoryTrimmable`과
   `internal/server/history_policy.go:50`의 구현(`PRAGMA` 포함),
   `internal/resourceclient/session_purge.go:29`,
   `internal/multiclient/session_purge.go:47`.
4. `internal/supervisor/history.go`: `authorizeHistoryCheckpoint`와
   `retainHistory(root, authorize)`의 두 번째 인자.
5. **테스트 하나를 바꿔 써야 한다.** 수퍼바이저 테스트 네 곳이 그 콜백을 "잘라내기가
   일어났는가"의 신호로 쓴다(`history_test.go:41`, `:69`, `:120`,
   `history_shed_test.go:68`). 콜백이 없어지면 그 단정을 **효과**로 바꾼다 — 저장소의
   floor가 움직였는지, 크기가 줄었는지. 통보가 아니라 결과를 보는 쪽이 더 나은
   단정이기도 하다.
6. `internal/server/server.go:92`의 `version > 2`는 **그대로 둔다.** 이미 2가 찍힌
   파일을 계속 받아 줘야 하고, 그 검사는 미래의 3을 막는 일을 계속 한다.
7. [where-we-are](where-we-are.md)의 "바로 다음" 2번을 지운다.

그 문서는 이 울타리가 "이제 쓰이지 않는다"고 적었는데 호출자는 있었다. 어긋남의
답은 "호출자는 있지만 막을 대상이 없다"이고, 위의 표가 그 근거다.

## 8. 순서

1. §6.1(2개 이동), §6.2(2개 제거), §7.3(`MarkHistoryTrimmable` 삭제). 셋 다
   독립적이고 기다릴 결정이 없다.
2. §6.4 `Installation`. 자기 파일 하나, 테이블 없음, 마이그레이션 없음. 메시지를
   옮기는 것이 일의 대부분이다. 웹 호출자는 `auxConfig`/`auxSetConfig`
   둘뿐이고(`ts/src`), 나머지는 CLI(`cmd/cxz`)와
   `internal/resourceclient`·`internal/multiclient`다.
3. §6.3 `Skill`·`McpServer`와 §6.5 `McpConnection`. §7.1이 선행한다.

세 단계 모두 등록이 한 줄씩 필요하다 — `internal/server/server.go`,
`internal/websandbox/server.go`, 그리고 웹의 `ts/src/shared/api/connection.ts`.
생성된 `resource.RegisterServer`가 이들을 모르기 때문이다(§5의 (4)).

## 측정을 다시 하는 방법

**`ref` 세기.** `proto/cxz/project_svc.g.proto`를 읽고 각 RPC의 요청 메시지에
`ProjectRef ref`가 있는지 센다. 손으로 쓴 RPC는 생성된 일곱 개(`Add` `Get` `Patch`
`Apply` `Erase` `List` `Watch`)를 뺀 나머지다.

**§5를 다시 확인하기.** 엔티티 없는 `service`를 `proto/cxz/`에 한 파일로 쓰고
`pd gen`을 돌린다 — `resource/*.pb.go`와 `ts/gen/cxz/*_pb.ts`가 나오고 ent 스키마는
나오지 않는다. 그 파일에서 `ProjectRef`를 쓰면 `cannot find` 'ProjectRef' `in this
scope`로 거부된다. 같은 `service`를 `proto/ext/cxz/project_svc.ext.proto`에 쓰면
`ProjectRef`째로 병합돼 생성된다. 실패한 `pd gen`은 `*_svc.g.proto`를 지운 상태로
멈추므로, 시험한 뒤에는 `git checkout -- proto/`로 되돌린다.
