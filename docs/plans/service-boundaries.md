# ProjectService 쪼개기 — 서비스 경계

상태: 제안. 구현 전이고, 세 가지 결정을 기다린다(§7).

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

## 5. 엔티티 없는 서비스 — 측정한 것

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

**그리고 반대 방향으로, 파일이 진실인 엔티티는 동작하지 않는 `Watch`를 갖는다.**
payday의 `Watch`는 sink를 통과한 쓰기에 반응해서 발행한다. 쓰기가 layer에서 파일로
가면 sink를 통과하지 않으므로, `Watch`는 선언돼 있고 구독도 성공하는데 아무것도
오지 않는다.

둘을 비교하면 이렇다. 빈 테이블은 스키마를 읽으면 보이고, 아무 호출도 거짓 답을 하지
않는다. 동작하지 않는 `Watch`는 호출이 성공하고 답이 비어 있으므로, 쓰는 쪽은 자기
코드를 의심하게 된다. 그래서 빈 테이블보다 나쁘다.

그래서 파일이 진실인 리소스에 대해 엔티티 없는 서비스는 타협이 아니라 **맞는 답**이다.
지금 26개는 전부 매니저(`s.shared.runtime`)로의 통과 호출이고 DB를 건드리지 않는다.
옮길 저장소가 없다.

## 6. 제안

47개를 여섯 곳으로 나눈다. 새 서비스는 넷이고 **새 테이블은 없다** — 이유는 §5다.

| 가는 곳 | 개수 | 무엇 |
| --- | --- | --- |
| `ProjectService`에 남는다 | 12 | Terminal, SessionLogin, AuxiliaryLogin, Paths, Download, Editor, EditorTunnel, PutSecretFile, DeleteSecretFile, Start, Stop, Remove |
| `AccountService`로 | 2 | AuxModels, AuxLoginInfo |
| 새 `SkillService` | 6 | GetSkills, AddSkill, RemoveSkill, SetSkillDefault, SetProjectSkill, ClearProjectSkill |
| 새 `McpServerService` | 6 | GetMcpServers, PutMcpServer, RemoveMcpServer, SetMcpServerDefault, SetProjectMcpServer, ClearProjectMcpServer |
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

### 6.3 파일이 진실인 둘: `Skill`, `McpServer`

둘 다 같은 모양이다. 등록은 설치본 전체, 활성화는 프로젝트별 override, 진실의 출처는
공유 디렉터리의 파일(`skills.json`, mcp 설정)이고 `cxz share`가 그걸 고친다.

§5의 (3)으로 선언한다 — 엔티티 없는 서비스를 오버레이 안에 쓴다. 그러면 테이블도
죽은 `Watch`도 없고, `ProjectRef`는 받을 수 있다.

```proto
// proto/ext/cxz/project_svc.ext.proto — `ProjectRef`가 있는 파일이라서 여기다.
// 엔티티가 아니므로 `SkillRef`는 없다. 스킬은 라이브러리 안에서 유일한
// 디렉터리 이름으로 가리킨다 — 지금도 그게 식별자다.
service SkillService {
  rpc List(SkillListRequest)       returns (SkillList);  // `in`은 시야
  rpc Add(SkillAddRequest)         returns (SkillList);  // 라이브러리에 등록
  rpc Remove(SkillRemoveRequest)   returns (SkillList);
  rpc SetDefault(SkillDefaultRequest) returns (SkillList);
  rpc Enable(SkillEnableRequest)   returns (SkillList);  // 한 프로젝트에서
  rpc Clear(SkillClearRequest)     returns (SkillList);
}

message SkillListRequest   { ProjectRef in = 1; }  // 없으면 설치본의 기본값
message SkillAddRequest    { string name = 1; }
message SkillRemoveRequest { string name = 1; }
message SkillDefaultRequest{ string name = 1; bool enabled = 2; }
message SkillEnableRequest { string name = 1; ProjectRef in = 2; bool enabled = 3; }
message SkillClearRequest  { string name = 1; ProjectRef in = 2; }
```

`McpServerService`는 `string name` 자리에 `string id`가 오는 같은 모양이다.

고쳐지는 것은 이름만이 아니다. 지금 `AddSkill(ProjectRef ref, name)`은 설치본
전체에 등록하면서 프로젝트를 주체인 것처럼 받는다(§4). 위에서는 등록이 프로젝트를
받지 않고, 프로젝트를 받는 것은 `Enable`·`Clear`·`List`뿐이다 — 실제로 프로젝트가
관여하는 셋이다.

**받아들여야 하는 것 하나.** payday에서 `proto/ext/cxz/project_svc.ext.proto`는
"`ProjectService`에 더할 것을 쓰는 파일"이다. 거기에 `SkillService`를 쓰면 파일 이름이
내용을 다 말하지 않게 된다 — `SkillService`를 찾는 사람이 그 파일을 열어 볼 이유가
없다. 실행에는 아무 영향이 없고, 찾기 어려워지는 것이 전부다.

그래도 그 파일이어야 하는 이유는 §5의 (2)다: `ProjectRef`는 생성된 계약 파일에만
있고, 오버레이는 이름으로 그 계약에 매칭된다. 대안은 §7의 1번에 적었다. 이 길을
택한다면 파일 머리에 "이 파일은 project 계약에 병합되는 것들이고, 여기 있는 서비스가
모두 Project에 대한 것은 아니다"라고 적는다.

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
// project_svc.ext.proto — `ProjectRef`를 받으므로 여기다.
service McpConnectionService {
  rpc List(McpConnectionListRequest) returns (McpConnectionList);
}
message McpConnectionListRequest { ProjectRef in = 1; }
```

세션을 `SessionRef`로 받고 싶겠지만 못 받는다. 지금 proto에 적힌 이유가 그대로
유효하다: project 계약은 session 계약을 가리킬 수 없고, 의존 방향이 반대다. 나눠도
그건 안 풀린다 — 나누는 이유가 그것이어서는 안 된다.

### 6.6 결과

`ProjectService` = 생성된 7개 + 손으로 쓴 12개. 전부 Project 행을 가리킨다. 새
테이블은 없고, 마이그레이션도 없다.

개수는 많이 줄지 않는다. 47개에서 둘을 지우고 나머지를 여섯 서비스에 나누면 45개이고,
`Installation`에서 읽기 몇 개가 `Get` 하나로 합쳐져 41개쯤이 된다. 그게 이 제안의
값이 아니다 — 값은 **어느 RPC도 자기 것이 아닌 서비스에 있지 않다**는 것이다. 엔티티로
갔다면 스무 개가 생성된 CRUD로 사라졌겠지만, 그 대가는 테이블과 죽은 `Watch`였다.

## 7. 결정해야 하는 것

§5를 측정한 뒤 남은 것은 셋이다. 저장소 결정은 사라졌다 — 옮길 저장소가 없다.

### 7.1 새 서비스 셋을 어느 파일에 선언하는가

`SkillService`, `McpServerService`, `McpConnectionService`는 `ProjectRef`를 받아야
한다. 그러려면 §5의 (2)·(3) 때문에 선택지가 둘뿐이다.

- **A.** `proto/ext/cxz/project_svc.ext.proto`에 선언한다. `ProjectRef`를 받는다.
  대가는 그 파일에 `ProjectService`가 아닌 서비스가 들어가서, 파일 이름만 보고는
  안에 무엇이 있는지 알 수 없다는 것이다. 실행에 영향은 없다.
- **B.** `proto/cxz/skill.proto` 같은 자기 파일에 선언한다. 파일 이름이 내용과
  맞는다. 대가는 프로젝트를 `ProjectRef`가 아니라 `string`(uuid 또는 runtime id)으로
  받아야 한다는 것이다. `ProjectRef`는 `id`/`alias`/`workspace`/`runtime_id` 중
  하나를 담는 oneof이고, 서버는 그 네 가지를 다 받아 준다.

B의 실제 손실은 생각보다 작다. 지금도 alias를 푸는 것은 클라이언트다 —
`internal/resourceclient/skills.go`의 `projectRef()`가 `Projects`를 불러
`projectref.Resolve`로 handle을 id로 바꾼 뒤 보낸다. 그러니 `cxz skill enable pn`은
B에서도 그대로 동작한다. 잃는 것은 "서버가 네 가지 표기를 다 받아 준다"는 성질과,
요청만 보고 그 문자열이 프로젝트를 가리킨다는 것을 알 수 있다는 점이다.

**A를 권한다.** 파일 이름은 주석 한 줄로 설명되고, 타입은 안 된다.

### 7.2 `McpConnectionService`를 지금 만드는가, 이름은 무엇인가

두 가지를 같이 정해야 한다.

1. **이름.** 제안한 `McpSessionService`는 `McpSessionStatus.session`이 에이전트
   세션의 id라서 cxz의 `Session`과 같은 단어를 쓰게 된다. 돌아가는 것은 한 에이전트
   세션과 한 MCP 서버 사이의 연결이므로 `McpConnectionService`를 제안한다.
   `McpSessionService`로 가도 동작에는 차이가 없고, 읽는 사람이 "MCP의 세션"과
   "세션의 MCP"를 구분해야 한다.
2. **지금 만드는가.** 옮길 RPC가 `McpSessions` 하나뿐이다. RPC 하나짜리 서비스를
   지금 만들 것인지, 두 번째 조작("이 세션의 이 MCP를 다시 띄워라", "왜 실패했는지
   보여 달라")이 생길 때까지 `McpSessions`를 `ProjectService`에 두고 §2 위반 하나를
   남길 것인지.

**지금 만드는 쪽을 권한다.** `McpSessions`는 `ProjectRef`를 받으므로 `ProjectService`에
두어도 §2 위반은 아니다. 그러나 등록(`McpServerService`)과 실행 중인 것이 다른 리소스라는
것을 한 번 적어 두면, 두 번째 조작이 생길 때 그게 어디로 갈지 묻지 않아도 된다.

### 7.3 `MarkHistoryTrimmable`을 지우는가

**무엇인지부터.** cxz는 세션 대화를 SQLite 파일(`sessions`/`events` 테이블)에
저장하고, 보존 예산을 넘으면 오래된 것을 잘라낸다(compaction). 잘라내기 **전에**
수퍼바이저가 이 RPC를 부르고, 받은 쪽은 그 파일에 `PRAGMA user_version=2`를 쓴다.
목적은 옛 버전 보호다: 잘라내기를 모르는 옛 cxz 빌드가 그 파일을 열면 version 2를
보고, 구멍 난 대화를 보여 주는 대신 거부한다. 수퍼바이저는 이 호출이 실패하면
잘라내기를 멈춘다(`internal/supervisor/history.go:298`,
`internal/server/history_policy.go:50`).

**그래서 이건 설치본에 대한 것이 아니다.** 대화 파일 하나에 표식을 쓰는 조작이고,
부르는 쪽은 프로젝트 컨테이너 안의 수퍼바이저다. `InstallationService`에 둘 것이
아니다 — 제안의 §6.4에서 빼 둔 이유다.

[where-we-are](where-we-are.md)의 "바로 다음" 2번은 이 울타리가 "이제 쓰이지 않는다"며
지우자고 적어 두었다. 그 문장과 코드가 어긋난다: 호출자는 아직 있다. 어긋남이 어느
쪽인지는 사람이 안다 — 지금 쓰이는 어느 빌드도 잘라내기를 모르지 않는다면 울타리가
할 일이 없고, 그러면 RPC·수퍼바이저의 호출·`PRAGMA` 셋을 함께 지운다. 그게 아니라면
남기고, 이 제안이 건드리지 않는다(클라이언트가 부를 일이 없으므로 리소스 API에서
빼고 런타임 API에만 두는 것이 맞다 — §6.2의 `SyncSkills`와 같은 모양이다).

**지우는 쪽을 권한다.** 다만 "옛 빌드가 더 없다"는 판단은 내 것이 아니다.

## 8. 순서

1. §6.1(2개 이동)과 §6.2(2개 제거). 독립적이고, 결정을 기다릴 것이 없다.
2. §6.4 `Installation`. 자기 파일 하나, 테이블 없음, 마이그레이션 없음. 메시지를
   옮기는 것이 일의 대부분이다. 웹 호출자는 `auxConfig`/`auxSetConfig`
   둘뿐이고(`ts/src`), 나머지는 CLI(`cmd/cxz`)와
   `internal/resourceclient`·`internal/multiclient`다.
3. §6.3 `Skill`·`McpServer`와 §6.5 `McpConnection`. 1번 결정이 선행한다.

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
