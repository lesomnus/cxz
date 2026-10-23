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

## 5. 제약: payday에서 서비스는 엔티티에서 나온다

이게 이 제안의 값이자 비용이다.

- `payday.proto`의 확장은 message·field·file 세 개뿐이다. 서비스만 있는 파일을
  위한 선언이 없다.
- `resource/store.g.go`의 `Server` 인터페이스와 `RegisterServer`,
  `server/pd/pd.g.go`의 Gate·Audit·Intercept·Batch는 모두 **엔티티 서비스별로**
  쓰인다. 엔티티 없이 손으로 쓴 서비스는 직접 등록해야 하고 그 넷을 전부 잃는다.
- 그러므로 "분리된 service"는 곧 "엔티티를 선언한다"다. 반대로, 엔티티를
  선언하면 CRUD·List·Watch·CLI verb·TS 클라이언트가 공짜로 따라온다.

지금 26개는 전부 매니저(`s.shared.runtime`)로의 통과 호출이고 DB를 건드리지
않는다. 그래서 진짜 질문은 "어디에 저장하는가"다.

## 6. 제안

47개를 다섯 곳으로 나눈다.

| 가는 곳 | 개수 | 무엇 |
| --- | --- | --- |
| `ProjectService`에 남는다 | 12 | Terminal, SessionLogin, AuxiliaryLogin, Paths, Download, Editor, EditorTunnel, PutSecretFile, DeleteSecretFile, Start, Stop, Remove |
| `AccountService`로 | 2 | AuxModels, AuxLoginInfo |
| 새 `SkillService` | 6 | GetSkills, AddSkill, RemoveSkill, SetSkillDefault, SetProjectSkill, ClearProjectSkill |
| 새 `McpServerService` | 7 | GetMcpServers, PutMcpServer, RemoveMcpServer, SetMcpServerDefault, SetProjectMcpServer, ClearProjectMcpServer, McpSessions |
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

### 6.3 행이 될 자격이 있는 둘: `Skill`, `McpServer`

둘 다 같은 모양이다. 등록은 설치본 전체, 활성화는 프로젝트별 override, 저장은
공유 디렉터리의 JSON(`skills.json`, mcp 설정).

엔티티로 선언하면 손으로 쓴 13개가 이렇게 접힌다:

| 지금 | 다음 |
| --- | --- |
| `AddSkill`, `PutMcpServer` | 생성된 `Add` / `Patch` |
| `RemoveSkill`, `RemoveMcpServer` | 생성된 `Erase` |
| `GetSkills`, `GetMcpServers` | 생성된 `List` (프로젝트 시야는 필터) |
| `SetSkillDefault`, `SetMcpServerDefault` | 생성된 `Patch` |
| `SetProjectSkill`, `SetProjectMcpServer` | `rpc Enable(SkillEnableRequest)` |
| `ClearProjectSkill`, `ClearProjectMcpServer` | `rpc Clear(SkillClearRequest)` |

`Enable`의 모양은 payday 자신의 테스트 앱이 쓰는 것과 같다 —
`RobotMoveRequest{RobotRef ref = 1; CellRef to = 2}`:

```proto
message SkillEnableRequest {
  SkillRef   ref     = 1;  // 무엇을
  ProjectRef in      = 2;  // 어디서
  bool       enabled = 3;
}
```

서비스의 엔티티가 `ref`고, 다른 리소스는 인자다. 지금의
`SetProjectSkill(ProjectRef, name)`은 이 둘을 뒤집어 놓은 것이다.

**비용**: 지금 진실의 출처는 공유 디렉터리의 파일이고, `cxz share`가 그걸 고친다.
행으로 옮기면 출처가 둘이 된다. 두 길이 있다.

- **(a) 행이 진실이 되고 디렉터리는 payload만 남는다.** Project가 행이고
  워크스페이스가 디스크에 있는 것과 같은 모양이다. 권한다.
- **(b) 계약만 엔티티로 선언하고 `Get`/`List`/`Patch`를 layer에서 파일로
  구현한다.** 테이블은 생기고 비어 있다. 권하지 않는다 — 빈 테이블이 거짓을
  말하고, 그게 우리가 방금 `ProjectStatus.state`에서 걷어낸 종류의 거짓이다.

### 6.4 행이 아닌 것: `Installation` 한 행

엔진 7개, 진단 2개, 보존 3개, `InspectForeign`, `AuxConfig`/`AuxSetConfig`,
설치본의 devcontainer 설정 3개 — 18개가 전부 "이 설치본"에 대한 것이다. 설치본은
하나이고, 버전과 엔진 spec과 보존 예산을 가진다. 그건 한 행이다.

| 지금 | 다음 |
| --- | --- |
| `GetHistoryPolicy`, `GetEngineInfo`, `EngineStatus`, `GetInstallationVersion`, `AuxConfig` | 생성된 `Get`의 필드들 |
| `SetHistoryPolicy`, `SaveEngine`, `AuxSetConfig`, `Devcontainer` | 생성된 `Patch` |
| `StartEngine`, `StopEngine`, `PruneEngine` | 그대로, `InstallationRef ref = 1`을 받는다 |
| `Diagnostics`, `CaptureDiagnostics` | `Diagnostics`, `Capture` |
| `InspectForeign` | `ForeignContainers` |
| `MarkHistoryTrimmable` | 옮기기 전에 지울 것인지 정한다(아래) |
| `RenderDevcontainer` | 그대로 — handle은 ref로 바꿀 수 없다(아래) |
| `FileMappings` | `PutFiles` |

`MarkHistoryTrimmable`은 [where-we-are](where-we-are.md)의 "바로 다음" 2번이 이미
지우자고 적어 둔 것이다. 다만 호출자는 아직 있다 —
`internal/supervisor/history.go`가 compaction 전에 묻고, 에러면 멈춘다. 옮기는
것보다 지우는 것이 맞을 수 있으니 그 결정이 먼저다.

`RenderDevcontainer`의 `handle`은 의도된 것이고 `server/lifecycle/project.go`에
이유가 적혀 있다: 워크스페이스 **안의** 디렉터리를 가리킬 수 있고, 어느 등록된
워크스페이스가 그걸 소유하는지는 매니저만 안다. 여기서 ref로 해소하면 묻는
대상이 이미 답이어야 한다. 그대로 둔다.

18개가 `Get`/`Patch` + 손으로 쓴 8개로 줄어든다.

### 6.5 결과

`ProjectService` = 생성된 7개 + 손으로 쓴 12개. 전부 Project 행을 가리킨다.
손으로 쓴 RPC 총합은 47 → 약 27이고, 스무 개가 생성된 CRUD로 흡수된다.

## 7. 결정해야 하는 것

1. **스킬·MCP의 진실의 출처를 DB로 옮기는가**(§6.3의 (a)), 파일로 두는가. (a)가
   아니면 이 둘은 지금 자리에 남는 편이 낫다 — 빈 테이블보다는 잘못된 서비스가
   덜 나쁘다.
2. **`Installation` 단일 행 엔티티를 받아들이는가.** 받아들이지 않으면 18개는
   payday 밖의 손으로 쓴 서비스가 되고, Gate·Audit·Intercept·Batch를 잃는다.
3. **`McpSessions`의 자리.** 한 프로젝트에서 돌아가는 MCP 서버들의 상태다.
   `ProjectService`에 남아도 규칙 위반은 아니고(프로젝트를 가리킨다),
   `McpServerService.Sessions(ProjectRef in)`가 더 맞다. 어느 쪽이든 고른다.

## 8. 순서

1. §6.1(2개 이동)과 §6.2(2개 제거). 독립적이고, 결정을 기다릴 것이 없다.
2. §6.4 `Installation`. 엔티티 하나와 마이그레이션 하나. 웹 호출자는
   `auxConfig`/`auxSetConfig` 둘뿐이고(`ts/src`), 나머지는 CLI(`cmd/cxz`)와
   `internal/resourceclient`·`internal/multiclient`다.
3. §6.3 `Skill`·`McpServer`. 1번 결정이 선행한다.

## 측정을 다시 하는 방법

`proto/cxz/project_svc.g.proto`를 읽고 각 RPC의 요청 메시지에 `ProjectRef ref`가
있는지 센다. 손으로 쓴 RPC는 생성된 일곱 개(`Add` `Get` `Patch` `Apply` `Erase`
`List` `Watch`)를 뺀 나머지다.
